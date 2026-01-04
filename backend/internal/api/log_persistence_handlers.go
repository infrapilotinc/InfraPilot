package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================
// Log Entry Types (for persistence)
// ============================================================

// PersistentLogEntry represents a single log entry for ingestion
type PersistentLogEntry struct {
	Source       string            `json:"source"`                  // Container name or "nginx", "system"
	SourceType   string            `json:"source_type"`             // "container", "nginx", "system", "agent"
	Stream       string            `json:"stream,omitempty"`        // "stdout" or "stderr"
	Level        string            `json:"level,omitempty"`         // "debug", "info", "warn", "error"
	Message      string            `json:"message"`                 // Log message
	Timestamp    time.Time         `json:"timestamp"`               // Original timestamp
	Labels       map[string]string `json:"labels,omitempty"`        // Container labels
	Metadata     map[string]any    `json:"metadata,omitempty"`      // Additional context
}

// PersistentLogBatch represents a batch of log entries for bulk ingestion
type PersistentLogBatch struct {
	AgentID string               `json:"agent_id" binding:"required"`
	Entries []PersistentLogEntry `json:"entries" binding:"required"`
}

// StoredLog represents a log entry from the database
type StoredLog struct {
	ID           string            `json:"id"`
	OrgID        string            `json:"org_id"`
	AgentID      string            `json:"agent_id"`
	Source       string            `json:"source"`
	SourceType   string            `json:"source_type"`
	Stream       string            `json:"stream"`
	Level        string            `json:"level"`
	Message      string            `json:"message"`
	LogTimestamp time.Time         `json:"log_timestamp"`
	IngestedAt   time.Time         `json:"ingested_at"`
	Labels       map[string]string `json:"labels,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

// LogRetentionConfig represents retention settings for an org
type LogRetentionConfig struct {
	ID                string    `json:"id"`
	OrgID             string    `json:"org_id"`
	RetentionDays     int       `json:"retention_days"`
	MaxStorageMB      int       `json:"max_storage_mb"`
	Enabled           bool      `json:"enabled"`
	CompressAfterDays int       `json:"compress_after_days"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// LogIngestionStats represents daily log stats
type LogIngestionStats struct {
	Date          string `json:"date"`
	LogCount      int64  `json:"log_count"`
	BytesIngested int64  `json:"bytes_ingested"`
	ErrorCount    int64  `json:"error_count"`
	WarnCount     int64  `json:"warn_count"`
	InfoCount     int64  `json:"info_count"`
	DebugCount    int64  `json:"debug_count"`
}

// ============================================================
// Log Ingestion Handlers
// ============================================================

// IngestLogs handles bulk log ingestion from agents
// POST /api/v1/logs/ingest
func (h *Handler) IngestLogs(c *gin.Context) {
	var batch PersistentLogBatch
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(batch.Entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no log entries provided"})
		return
	}

	// Parse agent ID
	agentID, err := uuid.Parse(batch.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}

	// Get org ID from agent
	var orgID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT org_id FROM agents WHERE id = $1
	`, agentID).Scan(&orgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Check if log persistence is enabled for this org
	var enabled bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT enabled FROM log_retention_config WHERE org_id = $1
	`, orgID).Scan(&enabled)
	if err != nil || !enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "log persistence not enabled for this organization"})
		return
	}

	// Insert logs in batches
	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to start transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	// Track stats
	var errorCount, warnCount, infoCount, debugCount int64
	var totalBytes int64

	for _, entry := range batch.Entries {
		// Default values
		if entry.Stream == "" {
			entry.Stream = "stdout"
		}
		if entry.Level == "" {
			entry.Level = "info"
		}
		if entry.SourceType == "" {
			entry.SourceType = "container"
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now()
		}

		_, err = tx.Exec(c.Request.Context(), `
			INSERT INTO centralized_logs (org_id, agent_id, source, source_type, stream, level, message, log_timestamp, labels, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, orgID, agentID, entry.Source, entry.SourceType, entry.Stream, entry.Level, entry.Message, entry.Timestamp, entry.Labels, entry.Metadata)

		if err != nil {
			h.logger.Error("Failed to insert log entry", zap.Error(err))
			continue
		}

		// Count by level
		switch entry.Level {
		case "error", "fatal":
			errorCount++
		case "warn", "warning":
			warnCount++
		case "info":
			infoCount++
		case "debug", "trace":
			debugCount++
		}

		totalBytes += int64(len(entry.Message))
	}

	// Update daily stats
	today := time.Now().Format("2006-01-02")
	_, err = tx.Exec(c.Request.Context(), `
		INSERT INTO log_ingestion_stats (org_id, agent_id, date, log_count, bytes_ingested, error_count, warn_count, info_count, debug_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (org_id, agent_id, date) DO UPDATE SET
			log_count = log_ingestion_stats.log_count + EXCLUDED.log_count,
			bytes_ingested = log_ingestion_stats.bytes_ingested + EXCLUDED.bytes_ingested,
			error_count = log_ingestion_stats.error_count + EXCLUDED.error_count,
			warn_count = log_ingestion_stats.warn_count + EXCLUDED.warn_count,
			info_count = log_ingestion_stats.info_count + EXCLUDED.info_count,
			debug_count = log_ingestion_stats.debug_count + EXCLUDED.debug_count,
			updated_at = NOW()
	`, orgID, agentID, today, len(batch.Entries), totalBytes, errorCount, warnCount, infoCount, debugCount)

	if err != nil {
		h.logger.Error("Failed to update ingestion stats", zap.Error(err))
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		h.logger.Error("Failed to commit transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	h.logger.Debug("Ingested logs",
		zap.String("agent_id", batch.AgentID),
		zap.Int("count", len(batch.Entries)),
	)

	c.JSON(http.StatusOK, gin.H{
		"ingested": len(batch.Entries),
		"status":   "ok",
	})
}

// ============================================================
// Log Query Handlers
// ============================================================

// GetPersistedLogs retrieves stored logs with filtering
// GET /api/v1/logs/persisted
func (h *Handler) GetPersistedLogs(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	// Parse query params
	agentID := c.Query("agent_id")
	source := c.Query("source")
	level := c.Query("level")
	search := c.Query("search")
	startTime := c.Query("start")
	endTime := c.Query("end")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	if limit > 1000 {
		limit = 1000
	}

	// Build query
	query := `
		SELECT id, org_id, agent_id, source, source_type, stream, level, message, log_timestamp, ingested_at, labels, metadata
		FROM centralized_logs
		WHERE org_id = $1
	`
	args := []any{orgID}
	argNum := 2

	if agentID != "" {
		query += ` AND agent_id = $` + strconv.Itoa(argNum)
		args = append(args, agentID)
		argNum++
	}

	if source != "" {
		query += ` AND source = $` + strconv.Itoa(argNum)
		args = append(args, source)
		argNum++
	}

	if level != "" {
		query += ` AND level = $` + strconv.Itoa(argNum)
		args = append(args, level)
		argNum++
	}

	if search != "" {
		query += ` AND message ILIKE $` + strconv.Itoa(argNum)
		args = append(args, "%"+search+"%")
		argNum++
	}

	if startTime != "" {
		t, err := time.Parse(time.RFC3339, startTime)
		if err == nil {
			query += ` AND log_timestamp >= $` + strconv.Itoa(argNum)
			args = append(args, t)
			argNum++
		}
	}

	if endTime != "" {
		t, err := time.Parse(time.RFC3339, endTime)
		if err == nil {
			query += ` AND log_timestamp <= $` + strconv.Itoa(argNum)
			args = append(args, t)
			argNum++
		}
	}

	query += ` ORDER BY log_timestamp DESC LIMIT $` + strconv.Itoa(argNum)
	args = append(args, limit)

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to query logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer rows.Close()

	var logs []StoredLog
	for rows.Next() {
		var log StoredLog
		err := rows.Scan(
			&log.ID, &log.OrgID, &log.AgentID, &log.Source, &log.SourceType,
			&log.Stream, &log.Level, &log.Message, &log.LogTimestamp,
			&log.IngestedAt, &log.Labels, &log.Metadata,
		)
		if err != nil {
			h.logger.Error("Failed to scan log row", zap.Error(err))
			continue
		}
		logs = append(logs, log)
	}

	c.JSON(http.StatusOK, logs)
}

// GetLogSources returns unique log sources for an org
// GET /api/v1/logs/sources
func (h *Handler) GetLogSources(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	agentID := c.Query("agent_id")

	query := `
		SELECT DISTINCT source, source_type
		FROM centralized_logs
		WHERE org_id = $1
	`
	args := []any{orgID}

	if agentID != "" {
		query += ` AND agent_id = $2`
		args = append(args, agentID)
	}

	query += ` ORDER BY source`

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to query log sources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer rows.Close()

	var sources []struct {
		Source     string `json:"source"`
		SourceType string `json:"source_type"`
	}

	for rows.Next() {
		var s struct {
			Source     string `json:"source"`
			SourceType string `json:"source_type"`
		}
		if err := rows.Scan(&s.Source, &s.SourceType); err == nil {
			sources = append(sources, s)
		}
	}

	c.JSON(http.StatusOK, sources)
}

// ============================================================
// Log Retention Handlers
// ============================================================

// GetLogRetentionConfig retrieves retention settings for the org
// GET /api/v1/logs/retention
func (h *Handler) GetLogRetentionConfig(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	var config LogRetentionConfig
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, retention_days, max_storage_mb, enabled, compress_after_days, created_at, updated_at
		FROM log_retention_config
		WHERE org_id = $1
	`, orgID).Scan(
		&config.ID, &config.OrgID, &config.RetentionDays, &config.MaxStorageMB,
		&config.Enabled, &config.CompressAfterDays, &config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		// Return default config if none exists
		config = LogRetentionConfig{
			OrgID:             orgID.String(),
			RetentionDays:     30,
			MaxStorageMB:      1000,
			Enabled:           false,
			CompressAfterDays: 7,
		}
	}

	c.JSON(http.StatusOK, config)
}

// UpdateLogRetentionConfig updates retention settings
// PUT /api/v1/logs/retention
func (h *Handler) UpdateLogRetentionConfig(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	var req struct {
		RetentionDays     int  `json:"retention_days"`
		MaxStorageMB      int  `json:"max_storage_mb"`
		Enabled           bool `json:"enabled"`
		CompressAfterDays int  `json:"compress_after_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate
	if req.RetentionDays < 1 {
		req.RetentionDays = 1
	}
	if req.RetentionDays > 365 {
		req.RetentionDays = 365
	}
	if req.MaxStorageMB < 100 {
		req.MaxStorageMB = 100
	}

	_, err := h.db.Exec(c.Request.Context(), `
		INSERT INTO log_retention_config (org_id, retention_days, max_storage_mb, enabled, compress_after_days)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id) DO UPDATE SET
			retention_days = EXCLUDED.retention_days,
			max_storage_mb = EXCLUDED.max_storage_mb,
			enabled = EXCLUDED.enabled,
			compress_after_days = EXCLUDED.compress_after_days,
			updated_at = NOW()
	`, orgID, req.RetentionDays, req.MaxStorageMB, req.Enabled, req.CompressAfterDays)

	if err != nil {
		h.logger.Error("Failed to update retention config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// ============================================================
// Log Stats Handlers
// ============================================================

// GetLogStats retrieves log ingestion statistics
// GET /api/v1/logs/stats
func (h *Handler) GetLogStats(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days > 30 {
		days = 30
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT date, SUM(log_count), SUM(bytes_ingested), SUM(error_count), SUM(warn_count), SUM(info_count), SUM(debug_count)
		FROM log_ingestion_stats
		WHERE org_id = $1 AND date >= CURRENT_DATE - $2::INT
		GROUP BY date
		ORDER BY date DESC
	`, orgID, days)

	if err != nil {
		h.logger.Error("Failed to query log stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer rows.Close()

	var stats []LogIngestionStats
	for rows.Next() {
		var s LogIngestionStats
		var date time.Time
		if err := rows.Scan(&date, &s.LogCount, &s.BytesIngested, &s.ErrorCount, &s.WarnCount, &s.InfoCount, &s.DebugCount); err == nil {
			s.Date = date.Format("2006-01-02")
			stats = append(stats, s)
		}
	}

	// Get storage usage
	var storageBytes int64
	h.db.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(pg_column_size(message)), 0) FROM centralized_logs WHERE org_id = $1
	`, orgID).Scan(&storageBytes)

	c.JSON(http.StatusOK, gin.H{
		"daily_stats":   stats,
		"storage_bytes": storageBytes,
		"storage_mb":    float64(storageBytes) / 1024 / 1024,
	})
}

// ============================================================
// Log Cleanup Handler
// ============================================================

// RunLogCleanup triggers cleanup of old logs based on retention
// POST /api/v1/logs/cleanup
func (h *Handler) RunLogCleanup(c *gin.Context) {
	orgID, ok := GetOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "organization context required"})
		return
	}

	// Get retention config
	var retentionDays int
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT retention_days FROM log_retention_config WHERE org_id = $1
	`, orgID).Scan(&retentionDays)
	if err != nil {
		retentionDays = 30 // default
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM centralized_logs
		WHERE org_id = $1 AND log_timestamp < NOW() - ($2 || ' days')::INTERVAL
	`, orgID, retentionDays)

	if err != nil {
		h.logger.Error("Failed to cleanup logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	deleted := result.RowsAffected()
	h.logger.Info("Log cleanup completed",
		zap.String("org_id", orgID.String()),
		zap.Int64("deleted", deleted),
	)

	c.JSON(http.StatusOK, gin.H{
		"deleted":        deleted,
		"retention_days": retentionDays,
	})
}
