package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/alerts"
)

// AlertChannel represents an alert notification channel
type AlertChannel struct {
	ID          uuid.UUID       `json:"id"`
	OrgID       uuid.UUID       `json:"org_id"`
	Name        string          `json:"name"`
	ChannelType string          `json:"channel_type"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// AlertRule represents an alert rule configuration
type AlertRule struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	Name         string          `json:"name"`
	RuleType     string          `json:"rule_type"`
	Conditions   json.RawMessage `json:"conditions"`
	Channels     []uuid.UUID     `json:"channels"`
	CooldownMins int             `json:"cooldown_mins"`
	Enabled      bool            `json:"enabled"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// AlertHistoryEntry represents a triggered alert
type AlertHistoryEntry struct {
	ID          uuid.UUID       `json:"id"`
	RuleID      *uuid.UUID      `json:"rule_id,omitempty"`
	RuleName    *string         `json:"rule_name,omitempty"`
	AgentID     *uuid.UUID      `json:"agent_id,omitempty"`
	AgentName   *string         `json:"agent_name,omitempty"`
	TriggeredAt time.Time       `json:"triggered_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
	Severity    string          `json:"severity"`
	Message     string          `json:"message"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// Channel config types
type SMTPConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password,omitempty"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	UseTLS   bool     `json:"use_tls"`
}

type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Request types
type CreateAlertChannelRequest struct {
	Name        string          `json:"name" binding:"required"`
	ChannelType string          `json:"channel_type" binding:"required"`
	Config      json.RawMessage `json:"config" binding:"required"`
	Enabled     *bool           `json:"enabled"`
}

type UpdateAlertChannelRequest struct {
	Name    *string          `json:"name,omitempty"`
	Config  *json.RawMessage `json:"config,omitempty"`
	Enabled *bool            `json:"enabled,omitempty"`
}

type CreateAlertRuleRequest struct {
	Name         string          `json:"name" binding:"required"`
	RuleType     string          `json:"rule_type" binding:"required"`
	Conditions   json.RawMessage `json:"conditions" binding:"required"`
	Channels     []uuid.UUID     `json:"channels" binding:"required"`
	CooldownMins *int            `json:"cooldown_mins,omitempty"`
	Enabled      *bool           `json:"enabled,omitempty"`
}

type UpdateAlertRuleRequest struct {
	Name         *string          `json:"name,omitempty"`
	Conditions   *json.RawMessage `json:"conditions,omitempty"`
	Channels     *[]uuid.UUID     `json:"channels,omitempty"`
	CooldownMins *int             `json:"cooldown_mins,omitempty"`
	Enabled      *bool            `json:"enabled,omitempty"`
}

// ============ Alert Channels ============

// listAlertChannelsReal returns all alert channels for the org
func (h *Handler) listAlertChannelsReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, name, channel_type, config, enabled, created_at, updated_at
		FROM alert_channels
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list alert channels", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list alert channels"})
		return
	}
	defer rows.Close()

	channels := make([]AlertChannel, 0)
	for rows.Next() {
		var ch AlertChannel
		if err := rows.Scan(&ch.ID, &ch.OrgID, &ch.Name, &ch.ChannelType, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			continue
		}
		channels = append(channels, ch)
	}

	c.JSON(http.StatusOK, channels)
}

// createAlertChannelReal creates a new alert channel
func (h *Handler) createAlertChannelReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req CreateAlertChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate channel type
	validTypes := map[string]bool{"smtp": true, "slack": true, "webhook": true, "discord": true, "teams": true}
	if !validTypes[req.ChannelType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_type, must be smtp, slack, or webhook"})
		return
	}
	if !h.requireConnectedFeature(c, connectedFeatureForChannel(req.ChannelType)) {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var channel AlertChannel
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO alert_channels (org_id, name, channel_type, config, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, name, channel_type, config, enabled, created_at, updated_at
	`, orgID, req.Name, req.ChannelType, req.Config, enabled).Scan(
		&channel.ID, &channel.OrgID, &channel.Name, &channel.ChannelType,
		&channel.Config, &channel.Enabled, &channel.CreatedAt, &channel.UpdatedAt,
	)

	if err != nil {
		h.logger.Error("Failed to create alert channel", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create alert channel"})
		return
	}

	c.JSON(http.StatusCreated, channel)
}

// updateAlertChannelReal updates an alert channel
func (h *Handler) updateAlertChannelReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	channelIDStr := c.Param("id")

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel ID"})
		return
	}

	var req UpdateAlertChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get existing channel
	var channel AlertChannel
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, channel_type, config, enabled, created_at, updated_at
		FROM alert_channels
		WHERE id = $1 AND org_id = $2
	`, channelID, orgID).Scan(
		&channel.ID, &channel.OrgID, &channel.Name, &channel.ChannelType,
		&channel.Config, &channel.Enabled, &channel.CreatedAt, &channel.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert channel not found"})
		return
	}

	// Apply updates
	if req.Name != nil {
		channel.Name = *req.Name
	}
	if req.Config != nil {
		channel.Config = *req.Config
	}
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}

	// Update in database
	err = h.db.QueryRow(c.Request.Context(), `
		UPDATE alert_channels
		SET name = $1, config = $2, enabled = $3, updated_at = NOW()
		WHERE id = $4 AND org_id = $5
		RETURNING updated_at
	`, channel.Name, channel.Config, channel.Enabled, channelID, orgID).Scan(&channel.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to update alert channel", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update alert channel"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

// deleteAlertChannelReal deletes an alert channel
func (h *Handler) deleteAlertChannelReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	channelIDStr := c.Param("id")

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel ID"})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM alert_channels
		WHERE id = $1 AND org_id = $2
	`, channelID, orgID)

	if err != nil {
		h.logger.Error("Failed to delete alert channel", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete alert channel"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert channel not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert channel deleted"})
}

// testAlertChannelReal sends a test notification
func (h *Handler) testAlertChannelReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	channelIDStr := c.Param("id")

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel ID"})
		return
	}

	// Get channel config
	var channel AlertChannel
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, channel_type, config, enabled, created_at, updated_at
		FROM alert_channels
		WHERE id = $1 AND org_id = $2
	`, channelID, orgID).Scan(
		&channel.ID, &channel.OrgID, &channel.Name, &channel.ChannelType,
		&channel.Config, &channel.Enabled, &channel.CreatedAt, &channel.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert channel not found"})
		return
	}

	// Parse config into map
	var configMap map[string]interface{}
	if err := json.Unmarshal(channel.Config, &configMap); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid channel configuration"})
		return
	}

	// Create notifier and send test
	notifier := alerts.NewNotifier(h.logger)
	channelConfig := alerts.ChannelConfig{
		Type:   channel.ChannelType,
		Config: configMap,
	}

	err = notifier.TestChannel(c.Request.Context(), channelConfig)
	if err != nil {
		h.logger.Error("Failed to send test notification",
			zap.String("channel", channel.Name),
			zap.String("type", channel.ChannelType),
			zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
			"channel": channel.Name,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Test notification sent successfully",
		"channel": channel.Name,
	})
}

// ============ Alert Rules ============

// listAlertRulesReal returns all alert rules for the org
func (h *Handler) listAlertRulesReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, name, rule_type, conditions, channels, cooldown_mins, enabled, created_at, updated_at
		FROM alert_rules
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list alert rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list alert rules"})
		return
	}
	defer rows.Close()

	rules := make([]AlertRule, 0)
	for rows.Next() {
		var rule AlertRule
		if err := rows.Scan(&rule.ID, &rule.OrgID, &rule.Name, &rule.RuleType, &rule.Conditions, &rule.Channels, &rule.CooldownMins, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	c.JSON(http.StatusOK, rules)
}

// createAlertRuleReal creates a new alert rule
func (h *Handler) createAlertRuleReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req CreateAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate rule type
	validTypes := map[string]bool{
		"container_crash":    true,
		"high_restart_count": true,
		"container_stopped":  true,
		"high_cpu":           true,
		"high_memory":        true,
		"oom_kill":           true,
		"ssl_expiry":         true,
		"high_error_rate":    true,
		"agent_offline":      true,
	}
	if !validTypes[req.RuleType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_type"})
		return
	}

	cooldownMins := 15
	if req.CooldownMins != nil {
		cooldownMins = *req.CooldownMins
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var rule AlertRule
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO alert_rules (org_id, name, rule_type, conditions, channels, cooldown_mins, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, org_id, name, rule_type, conditions, channels, cooldown_mins, enabled, created_at, updated_at
	`, orgID, req.Name, req.RuleType, req.Conditions, req.Channels, cooldownMins, enabled).Scan(
		&rule.ID, &rule.OrgID, &rule.Name, &rule.RuleType, &rule.Conditions,
		&rule.Channels, &rule.CooldownMins, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err != nil {
		h.logger.Error("Failed to create alert rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create alert rule"})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// updateAlertRuleReal updates an alert rule
func (h *Handler) updateAlertRuleReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	ruleIDStr := c.Param("id")

	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
		return
	}

	var req UpdateAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get existing rule
	var rule AlertRule
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, rule_type, conditions, channels, cooldown_mins, enabled, created_at, updated_at
		FROM alert_rules
		WHERE id = $1 AND org_id = $2
	`, ruleID, orgID).Scan(
		&rule.ID, &rule.OrgID, &rule.Name, &rule.RuleType, &rule.Conditions,
		&rule.Channels, &rule.CooldownMins, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert rule not found"})
		return
	}

	// Apply updates
	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.Conditions != nil {
		rule.Conditions = *req.Conditions
	}
	if req.Channels != nil {
		rule.Channels = *req.Channels
	}
	if req.CooldownMins != nil {
		rule.CooldownMins = *req.CooldownMins
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}

	// Update in database
	err = h.db.QueryRow(c.Request.Context(), `
		UPDATE alert_rules
		SET name = $1, conditions = $2, channels = $3, cooldown_mins = $4, enabled = $5, updated_at = NOW()
		WHERE id = $6 AND org_id = $7
		RETURNING updated_at
	`, rule.Name, rule.Conditions, rule.Channels, rule.CooldownMins, rule.Enabled, ruleID, orgID).Scan(&rule.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to update alert rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update alert rule"})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// deleteAlertRuleReal deletes an alert rule
func (h *Handler) deleteAlertRuleReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	ruleIDStr := c.Param("id")

	ruleID, err := uuid.Parse(ruleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM alert_rules
		WHERE id = $1 AND org_id = $2
	`, ruleID, orgID)

	if err != nil {
		h.logger.Error("Failed to delete alert rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete alert rule"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert rule deleted"})
}

// ============ Alert History ============

// getAlertHistoryReal returns alert history
func (h *Handler) getAlertHistoryReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	// Get limit from query params (default 100)
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT ah.id, ah.rule_id, ar.name, ah.agent_id, a.name,
		       ah.triggered_at, ah.resolved_at, ah.severity, ah.message, ah.metadata
		FROM alert_history ah
		LEFT JOIN alert_rules ar ON ah.rule_id = ar.id
		LEFT JOIN agents a ON ah.agent_id = a.id
		WHERE ar.org_id = $1 OR ah.agent_id IN (SELECT id FROM agents WHERE org_id = $1)
		ORDER BY ah.triggered_at DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		h.logger.Error("Failed to get alert history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get alert history"})
		return
	}
	defer rows.Close()

	history := make([]AlertHistoryEntry, 0)
	for rows.Next() {
		var entry AlertHistoryEntry
		if err := rows.Scan(
			&entry.ID, &entry.RuleID, &entry.RuleName, &entry.AgentID, &entry.AgentName,
			&entry.TriggeredAt, &entry.ResolvedAt, &entry.Severity, &entry.Message, &entry.Metadata,
		); err != nil {
			continue
		}
		history = append(history, entry)
	}

	c.JSON(http.StatusOK, history)
}
