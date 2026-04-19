package alerts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// AlertEvaluator runs periodic checks and triggers alerts
type AlertEvaluator struct {
	db       *pgxpool.Pool
	docker   *client.Client
	notifier *Notifier
	logger   *zap.Logger

	// Track last trigger times to enforce cooldowns
	cooldowns   map[string]time.Time
	cooldownsMu sync.RWMutex

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// AlertRule represents a rule from the database
type AlertRule struct {
	ID           uuid.UUID
	OrgID        uuid.UUID
	Name         string
	RuleType     string
	Conditions   map[string]interface{}
	Channels     []uuid.UUID
	CooldownMins int
	Enabled      bool
}

// ContainerMetrics holds metrics for evaluation
type ContainerMetrics struct {
	ContainerID   string
	ContainerName string
	State         string
	Status        string
	RestartCount  int
	OOMKilled     bool
	CPUPercent    float64
	MemoryPercent float64
	MemoryUsage   uint64
	MemoryLimit   uint64
}

// NewAlertEvaluator creates a new alert evaluator
func NewAlertEvaluator(db *pgxpool.Pool, logger *zap.Logger) (*AlertEvaluator, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &AlertEvaluator{
		db:        db,
		docker:    docker,
		notifier:  NewNotifier(logger),
		logger:    logger,
		cooldowns: make(map[string]time.Time),
		stopCh:    make(chan struct{}),
	}, nil
}

// Start begins the evaluation loop
func (e *AlertEvaluator) Start(ctx context.Context, interval time.Duration) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		e.logger.Info("Alert evaluator started", zap.Duration("interval", interval))

		for {
			select {
			case <-ticker.C:
				e.runEvaluation(ctx)
			case <-e.stopCh:
				e.logger.Info("Alert evaluator stopped")
				return
			case <-ctx.Done():
				e.logger.Info("Alert evaluator context cancelled")
				return
			}
		}
	}()
}

// Stop gracefully stops the evaluator
func (e *AlertEvaluator) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

// runEvaluation runs a single evaluation cycle
func (e *AlertEvaluator) runEvaluation(ctx context.Context) {
	// Get all enabled rules
	rules, err := e.getEnabledRules(ctx)
	if err != nil {
		e.logger.Error("Failed to get enabled rules", zap.Error(err))
		return
	}

	if len(rules) == 0 {
		return
	}

	// Get container metrics
	metrics, err := e.getContainerMetrics(ctx)
	if err != nil {
		e.logger.Error("Failed to get container metrics", zap.Error(err))
		return
	}

	// Evaluate each rule
	for _, rule := range rules {
		e.evaluateRule(ctx, rule, metrics)
	}
}

// getEnabledRules fetches all enabled alert rules
func (e *AlertEvaluator) getEnabledRules(ctx context.Context) ([]AlertRule, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, org_id, name, rule_type, conditions, channels, cooldown_mins, enabled
		FROM alert_rules
		WHERE enabled = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		var rule AlertRule
		var conditionsJSON, channelsJSON []byte
		if err := rows.Scan(
			&rule.ID, &rule.OrgID, &rule.Name, &rule.RuleType,
			&conditionsJSON, &channelsJSON, &rule.CooldownMins, &rule.Enabled,
		); err != nil {
			continue
		}

		json.Unmarshal(conditionsJSON, &rule.Conditions)
		json.Unmarshal(channelsJSON, &rule.Channels)
		rules = append(rules, rule)
	}

	return rules, nil
}

// getContainerMetrics collects metrics from all containers
func (e *AlertEvaluator) getContainerMetrics(ctx context.Context) ([]ContainerMetrics, error) {
	containers, err := e.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	var metrics []ContainerMetrics
	for _, c := range containers {
		name := "unknown"
		if len(c.Names) > 0 {
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		m := ContainerMetrics{
			ContainerID:   c.ID[:12],
			ContainerName: name,
			State:         c.State,
			Status:        c.Status,
		}

		// Get detailed stats for running containers
		if c.State == "running" {
			stats, err := e.docker.ContainerStatsOneShot(ctx, c.ID)
			if err == nil {
				var statsJSON container.StatsResponse
				if json.NewDecoder(stats.Body).Decode(&statsJSON) == nil {
					// Calculate CPU percent
					cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
					systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
					if systemDelta > 0 && cpuDelta > 0 {
						m.CPUPercent = (cpuDelta / systemDelta) * float64(statsJSON.CPUStats.OnlineCPUs) * 100
					}

					// Memory usage
					m.MemoryUsage = statsJSON.MemoryStats.Usage
					m.MemoryLimit = statsJSON.MemoryStats.Limit
					if m.MemoryLimit > 0 {
						m.MemoryPercent = float64(m.MemoryUsage) / float64(m.MemoryLimit) * 100
					}
				}
				stats.Body.Close()
			}
		}

		// Get restart count and OOM status from inspect
		inspect, err := e.docker.ContainerInspect(ctx, c.ID)
		if err == nil {
			m.RestartCount = inspect.RestartCount
			if inspect.State != nil {
				m.OOMKilled = inspect.State.OOMKilled
			}
		}

		metrics = append(metrics, m)
	}

	return metrics, nil
}

// evaluateRule evaluates a single rule against metrics
func (e *AlertEvaluator) evaluateRule(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	switch rule.RuleType {
	case "container_crash":
		e.evaluateContainerCrash(ctx, rule, metrics)
	case "container_stopped":
		e.evaluateContainerStopped(ctx, rule, metrics)
	case "high_restart_count":
		e.evaluateHighRestartCount(ctx, rule, metrics)
	case "high_cpu":
		e.evaluateHighCPU(ctx, rule, metrics)
	case "high_memory":
		e.evaluateHighMemory(ctx, rule, metrics)
	case "oom_kill":
		e.evaluateOOMKill(ctx, rule, metrics)
	case "ssl_expiry":
		e.evaluateSSLExpiry(ctx, rule)
	case "high_error_rate":
		e.evaluateHighErrorRate(ctx, rule)
	}
}

func (e *AlertEvaluator) evaluateContainerCrash(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	for _, m := range metrics {
		if m.State == "exited" || m.State == "dead" {
			e.triggerAlert(ctx, rule, m.ContainerName, "critical",
				"Container crashed or exited unexpectedly",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"state":          m.State,
					"status":         m.Status,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateOOMKill(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	for _, m := range metrics {
		if m.OOMKilled {
			e.triggerAlert(ctx, rule, m.ContainerName, "critical",
				"Container was killed by the OOM killer (out of memory)",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"state":          m.State,
					"status":         m.Status,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateContainerStopped(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	containerName, _ := rule.Conditions["container_name"].(string)
	for _, m := range metrics {
		if containerName != "" && m.ContainerName != containerName {
			continue
		}
		if m.State != "running" {
			e.triggerAlert(ctx, rule, m.ContainerName, "warning",
				"Container is not running",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"state":          m.State,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateHighRestartCount(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	threshold := 3
	if t, ok := rule.Conditions["threshold"].(float64); ok {
		threshold = int(t)
	}

	for _, m := range metrics {
		if m.RestartCount >= threshold {
			e.triggerAlert(ctx, rule, m.ContainerName, "warning",
				"Container has high restart count",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"restart_count":  m.RestartCount,
					"threshold":      threshold,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateHighCPU(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	threshold := 80.0
	if t, ok := rule.Conditions["threshold"].(float64); ok {
		threshold = t
	}

	for _, m := range metrics {
		if m.CPUPercent >= threshold {
			e.triggerAlert(ctx, rule, m.ContainerName, "warning",
				"Container CPU usage is high",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"cpu_percent":    m.CPUPercent,
					"threshold":      threshold,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateHighMemory(ctx context.Context, rule AlertRule, metrics []ContainerMetrics) {
	threshold := 80.0
	if t, ok := rule.Conditions["threshold"].(float64); ok {
		threshold = t
	}

	for _, m := range metrics {
		if m.MemoryPercent >= threshold {
			e.triggerAlert(ctx, rule, m.ContainerName, "warning",
				"Container memory usage is high",
				map[string]interface{}{
					"container_id":   m.ContainerID,
					"container_name": m.ContainerName,
					"memory_percent": m.MemoryPercent,
					"memory_usage":   m.MemoryUsage,
					"memory_limit":   m.MemoryLimit,
					"threshold":      threshold,
				})
		}
	}
}

func (e *AlertEvaluator) evaluateSSLExpiry(ctx context.Context, rule AlertRule) {
	// Get warning threshold (days before expiry)
	warningDays := 14
	if d, ok := rule.Conditions["warning_days"].(float64); ok {
		warningDays = int(d)
	}

	// Get critical threshold (days before expiry)
	criticalDays := 7
	if d, ok := rule.Conditions["critical_days"].(float64); ok {
		criticalDays = int(d)
	}

	// Get all proxy hosts with SSL enabled for this org
	rows, err := e.db.Query(ctx, `
		SELECT id, domain, ssl_enabled
		FROM proxy_hosts
		WHERE org_id = $1 AND ssl_enabled = true
	`, rule.OrgID)
	if err != nil {
		e.logger.Error("Failed to query proxy hosts for SSL check", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var proxyID uuid.UUID
		var domain string
		var sslEnabled bool
		if err := rows.Scan(&proxyID, &domain, &sslEnabled); err != nil {
			continue
		}

		// Check certificate expiry by connecting to the domain
		expiry, err := e.checkCertificateExpiry(domain)
		if err != nil {
			e.logger.Warn("Failed to check certificate expiry",
				zap.String("domain", domain),
				zap.Error(err))
			continue
		}

		daysUntilExpiry := int(time.Until(expiry).Hours() / 24)

		if daysUntilExpiry <= criticalDays {
			e.triggerAlert(ctx, rule, domain, "critical",
				fmt.Sprintf("SSL certificate expires in %d days", daysUntilExpiry),
				map[string]interface{}{
					"domain":            domain,
					"expires_at":        expiry.Format(time.RFC3339),
					"days_until_expiry": daysUntilExpiry,
				})
		} else if daysUntilExpiry <= warningDays {
			e.triggerAlert(ctx, rule, domain, "warning",
				fmt.Sprintf("SSL certificate expires in %d days", daysUntilExpiry),
				map[string]interface{}{
					"domain":            domain,
					"expires_at":        expiry.Format(time.RFC3339),
					"days_until_expiry": daysUntilExpiry,
				})
		}
	}
}

func (e *AlertEvaluator) checkCertificateExpiry(domain string) (time.Time, error) {
	// Connect to the domain on port 443 with a timeout
	conn, err := net.DialTimeout("tcp", domain+":443", 10*time.Second)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Establish TLS connection
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true, // We just want expiry, not full validation
	})
	defer tlsConn.Close()

	if err := tlsConn.Handshake(); err != nil {
		return time.Time{}, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Get certificate expiry
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("no certificates found")
	}

	return certs[0].NotAfter, nil
}

func (e *AlertEvaluator) evaluateHighErrorRate(ctx context.Context, rule AlertRule) {
	// Get threshold (errors per minute)
	threshold := 10.0
	if t, ok := rule.Conditions["threshold"].(float64); ok {
		threshold = t
	}

	// Get time window in minutes (default 5)
	windowMins := 5
	if w, ok := rule.Conditions["window_mins"].(float64); ok {
		windowMins = int(w)
	}

	// Optional: filter by container name pattern
	containerPattern := ""
	if p, ok := rule.Conditions["container_pattern"].(string); ok {
		containerPattern = p
	}

	// Query nginx error logs from the last N minutes
	// This assumes we have a log aggregation table or nginx logs stored
	query := `
		SELECT container_name, COUNT(*) as error_count
		FROM container_logs
		WHERE org_id = $1
		  AND log_level IN ('error', 'ERROR', 'fatal', 'FATAL')
		  AND created_at > NOW() - INTERVAL '%d minutes'
	`
	if containerPattern != "" {
		query += ` AND container_name LIKE '%' || $2 || '%'`
	}
	query += ` GROUP BY container_name`
	query = fmt.Sprintf(query, windowMins)

	var rows interface{ Close() }
	var err error
	if containerPattern != "" {
		rows, err = e.db.Query(ctx, query, rule.OrgID, containerPattern)
	} else {
		// Adjust query for no pattern
		query = fmt.Sprintf(`
			SELECT container_name, COUNT(*) as error_count
			FROM container_logs
			WHERE org_id = $1
			  AND log_level IN ('error', 'ERROR', 'fatal', 'FATAL')
			  AND created_at > NOW() - INTERVAL '%d minutes'
			GROUP BY container_name
		`, windowMins)
		rows, err = e.db.Query(ctx, query, rule.OrgID)
	}
	if err != nil {
		// Table might not exist yet, just log and return
		if !strings.Contains(err.Error(), "does not exist") {
			e.logger.Error("Failed to query error logs", zap.Error(err))
		}
		return
	}
	defer rows.Close()

	// Type assert to get actual rows
	pgRows, ok := rows.(interface {
		Next() bool
		Scan(...interface{}) error
		Close()
	})
	if !ok {
		return
	}

	for pgRows.Next() {
		var containerName string
		var errorCount int
		if err := pgRows.Scan(&containerName, &errorCount); err != nil {
			continue
		}

		// Calculate errors per minute
		errorsPerMin := float64(errorCount) / float64(windowMins)

		if errorsPerMin >= threshold {
			e.triggerAlert(ctx, rule, containerName, "warning",
				fmt.Sprintf("High error rate: %.1f errors/min", errorsPerMin),
				map[string]interface{}{
					"container_name":   containerName,
					"error_count":      errorCount,
					"errors_per_min":   errorsPerMin,
					"window_mins":      windowMins,
					"threshold":        threshold,
				})
		}
	}
}

// triggerAlert sends an alert if not in cooldown
func (e *AlertEvaluator) triggerAlert(ctx context.Context, rule AlertRule, target, severity, message string, metadata map[string]interface{}) {
	// Build unique key for cooldown tracking
	cooldownKey := rule.ID.String() + ":" + target

	// Check cooldown
	e.cooldownsMu.RLock()
	lastTrigger, exists := e.cooldowns[cooldownKey]
	e.cooldownsMu.RUnlock()

	if exists && time.Since(lastTrigger) < time.Duration(rule.CooldownMins)*time.Minute {
		return // Still in cooldown
	}

	// Update cooldown
	e.cooldownsMu.Lock()
	e.cooldowns[cooldownKey] = time.Now()
	e.cooldownsMu.Unlock()

	e.logger.Info("Alert triggered",
		zap.String("rule", rule.Name),
		zap.String("target", target),
		zap.String("severity", severity))

	// Record in alert history
	metadataJSON, _ := json.Marshal(metadata)
	_, err := e.db.Exec(ctx, `
		INSERT INTO alert_history (rule_id, triggered_at, severity, message, metadata)
		VALUES ($1, NOW(), $2, $3, $4)
	`, rule.ID, severity, message, metadataJSON)
	if err != nil {
		e.logger.Error("Failed to record alert history", zap.Error(err))
	}

	// Get channels and send notifications
	for _, channelID := range rule.Channels {
		var channelType string
		var configJSON []byte
		err := e.db.QueryRow(ctx, `
			SELECT channel_type, config FROM alert_channels WHERE id = $1
		`, channelID).Scan(&channelType, &configJSON)
		if err != nil {
			continue
		}

		var config map[string]interface{}
		json.Unmarshal(configJSON, &config)

		payload := AlertPayload{
			RuleName:    rule.Name,
			RuleType:    rule.RuleType,
			Severity:    severity,
			Message:     message,
			TriggeredAt: time.Now(),
			Metadata:    metadata,
		}

		channelConfig := ChannelConfig{
			Type:   channelType,
			Config: config,
		}

		if err := e.notifier.SendNotification(ctx, channelConfig, payload); err != nil {
			e.logger.Error("Failed to send notification",
				zap.String("channel_type", channelType),
				zap.Error(err))
		}
	}
}
