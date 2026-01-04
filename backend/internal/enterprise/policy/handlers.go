package policy

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Handler struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewHandler(db *pgxpool.Pool, logger *zap.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

// ============ Models ============

type Policy struct {
	ID          uuid.UUID              `json:"id"`
	OrgID       uuid.UUID              `json:"org_id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	PolicyType  string                 `json:"policy_type"`
	Conditions  map[string]interface{} `json:"conditions"`
	Action      string                 `json:"action"`
	AppliesTo   map[string]interface{} `json:"applies_to,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	CreatedBy   *uuid.UUID             `json:"created_by,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type PolicyTemplate struct {
	ID                uuid.UUID              `json:"id"`
	Name              string                 `json:"name"`
	Description       *string                `json:"description,omitempty"`
	PolicyType        string                 `json:"policy_type"`
	Conditions        map[string]interface{} `json:"conditions"`
	RecommendedAction string                 `json:"recommended_action"`
	Category          *string                `json:"category,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

type PolicyViolation struct {
	ID             uuid.UUID              `json:"id"`
	PolicyID       uuid.UUID              `json:"policy_id"`
	PolicyName     string                 `json:"policy_name,omitempty"`
	OrgID          uuid.UUID              `json:"org_id"`
	AgentID        *uuid.UUID             `json:"agent_id,omitempty"`
	AgentName      string                 `json:"agent_name,omitempty"`
	ResourceType   string                 `json:"resource_type"`
	ResourceID     *string                `json:"resource_id,omitempty"`
	ResourceName   *string                `json:"resource_name,omitempty"`
	Message        string                 `json:"message"`
	Details        map[string]interface{} `json:"details,omitempty"`
	ActionTaken    string                 `json:"action_taken"`
	Resolved       bool                   `json:"resolved"`
	ResolvedBy     *uuid.UUID             `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	ResolutionNote *string                `json:"resolution_note,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ============ Request Types ============

type CreatePolicyRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description *string                `json:"description"`
	PolicyType  string                 `json:"policy_type" binding:"required"`
	Conditions  map[string]interface{} `json:"conditions" binding:"required"`
	Action      string                 `json:"action" binding:"required"`
	AppliesTo   map[string]interface{} `json:"applies_to"`
	Enabled     *bool                  `json:"enabled"`
	Priority    *int                   `json:"priority"`
}

type UpdatePolicyRequest struct {
	Name        *string                 `json:"name"`
	Description *string                 `json:"description"`
	Conditions  *map[string]interface{} `json:"conditions"`
	Action      *string                 `json:"action"`
	AppliesTo   *map[string]interface{} `json:"applies_to"`
	Enabled     *bool                   `json:"enabled"`
	Priority    *int                    `json:"priority"`
}

type ResolveViolationRequest struct {
	ResolutionNote string `json:"resolution_note"`
}

// ============ Policy Handlers ============

func (h *Handler) ListPolicies(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	policyType := c.Query("type")
	enabledOnly := c.Query("enabled") == "true"

	query := `
		SELECT id, org_id, name, description, policy_type, conditions, action,
		       applies_to, enabled, priority, created_by, created_at, updated_at
		FROM policies
		WHERE org_id = $1
	`
	args := []interface{}{orgID}
	argPos := 2

	if policyType != "" {
		query += " AND policy_type = $" + string(rune('0'+argPos))
		args = append(args, policyType)
		argPos++
	}

	if enabledOnly {
		query += " AND enabled = true"
	}

	query += " ORDER BY priority DESC, name"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list policies", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list policies"})
		return
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &p.PolicyType, &p.Conditions,
			&p.Action, &p.AppliesTo, &p.Enabled, &p.Priority, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			continue
		}
		policies = append(policies, p)
	}

	c.JSON(http.StatusOK, policies)
}

func (h *Handler) GetPolicy(c *gin.Context) {
	policyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	var p Policy
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, description, policy_type, conditions, action,
		       applies_to, enabled, priority, created_by, created_at, updated_at
		FROM policies WHERE id = $1
	`, policyID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.PolicyType, &p.Conditions,
		&p.Action, &p.AppliesTo, &p.Enabled, &p.Priority, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Policy not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to get policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get policy"})
		return
	}

	c.JSON(http.StatusOK, p)
}

func (h *Handler) CreatePolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req CreatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate policy type
	validTypes := map[string]bool{"container": true, "proxy": true, "access": true, "security": true}
	if !validTypes[req.PolicyType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy type"})
		return
	}

	// Validate action
	validActions := map[string]bool{"block": true, "warn": true, "audit": true}
	if !validActions[req.Action] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	appliesTo := req.AppliesTo
	if appliesTo == nil {
		appliesTo = map[string]interface{}{}
	}

	var policyID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO policies (org_id, name, description, policy_type, conditions, action, applies_to, enabled, priority, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, orgID, req.Name, req.Description, req.PolicyType, req.Conditions, req.Action, appliesTo, enabled, priority, userID).Scan(&policyID)

	if err != nil {
		h.logger.Error("Failed to create policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create policy"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": policyID})
}

func (h *Handler) UpdatePolicy(c *gin.Context) {
	policyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate action if provided
	if req.Action != nil {
		validActions := map[string]bool{"block": true, "warn": true, "audit": true}
		if !validActions[*req.Action] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
			return
		}
	}

	// Build dynamic update query
	updates := []string{"updated_at = NOW()"}
	args := []interface{}{policyID}
	argPos := 2

	if req.Name != nil {
		updates = append(updates, "name = $"+string(rune('0'+argPos)))
		args = append(args, *req.Name)
		argPos++
	}
	if req.Description != nil {
		updates = append(updates, "description = $"+string(rune('0'+argPos)))
		args = append(args, *req.Description)
		argPos++
	}
	if req.Conditions != nil {
		updates = append(updates, "conditions = $"+string(rune('0'+argPos)))
		args = append(args, *req.Conditions)
		argPos++
	}
	if req.Action != nil {
		updates = append(updates, "action = $"+string(rune('0'+argPos)))
		args = append(args, *req.Action)
		argPos++
	}
	if req.AppliesTo != nil {
		updates = append(updates, "applies_to = $"+string(rune('0'+argPos)))
		args = append(args, *req.AppliesTo)
		argPos++
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = $"+string(rune('0'+argPos)))
		args = append(args, *req.Enabled)
		argPos++
	}
	if req.Priority != nil {
		updates = append(updates, "priority = $"+string(rune('0'+argPos)))
		args = append(args, *req.Priority)
		argPos++
	}

	query := "UPDATE policies SET "
	for i, u := range updates {
		if i > 0 {
			query += ", "
		}
		query += u
	}
	query += " WHERE id = $1"

	_, err = h.db.Exec(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to update policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update policy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *Handler) DeletePolicy(c *gin.Context) {
	policyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	_, err = h.db.Exec(c.Request.Context(), "DELETE FROM policies WHERE id = $1", policyID)
	if err != nil {
		h.logger.Error("Failed to delete policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete policy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ============ Template Handlers ============

func (h *Handler) ListTemplates(c *gin.Context) {
	category := c.Query("category")

	query := `
		SELECT id, name, description, policy_type, conditions, recommended_action, category, created_at
		FROM policy_templates
	`
	args := []interface{}{}

	if category != "" {
		query += " WHERE category = $1"
		args = append(args, category)
	}

	query += " ORDER BY category, name"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list templates"})
		return
	}
	defer rows.Close()

	var templates []PolicyTemplate
	for rows.Next() {
		var t PolicyTemplate
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.PolicyType, &t.Conditions,
			&t.RecommendedAction, &t.Category, &t.CreatedAt,
		); err != nil {
			continue
		}
		templates = append(templates, t)
	}

	c.JSON(http.StatusOK, templates)
}

func (h *Handler) CreateFromTemplate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var req struct {
		Name      string                 `json:"name" binding:"required"`
		Action    *string                `json:"action"`
		AppliesTo map[string]interface{} `json:"applies_to"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get template
	var t PolicyTemplate
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, name, description, policy_type, conditions, recommended_action
		FROM policy_templates WHERE id = $1
	`, templateID).Scan(&t.ID, &t.Name, &t.Description, &t.PolicyType, &t.Conditions, &t.RecommendedAction)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to get template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get template"})
		return
	}

	action := t.RecommendedAction
	if req.Action != nil {
		action = *req.Action
	}

	appliesTo := req.AppliesTo
	if appliesTo == nil {
		appliesTo = map[string]interface{}{}
	}

	// Create policy from template
	var policyID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO policies (org_id, name, description, policy_type, conditions, action, applies_to, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8)
		RETURNING id
	`, orgID, req.Name, t.Description, t.PolicyType, t.Conditions, action, appliesTo, userID).Scan(&policyID)

	if err != nil {
		h.logger.Error("Failed to create policy from template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create policy"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       policyID,
		"template": t.Name,
	})
}

// ============ Violation Handlers ============

func (h *Handler) ListViolations(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	resolved := c.Query("resolved")
	policyID := c.Query("policy_id")
	agentID := c.Query("agent_id")

	query := `
		SELECT v.id, v.policy_id, p.name, v.org_id, v.agent_id, COALESCE(a.name, ''),
		       v.resource_type, v.resource_id, v.resource_name, v.message, v.details,
		       v.action_taken, v.resolved, v.resolved_by, v.resolved_at, v.resolution_note, v.created_at
		FROM policy_violations v
		LEFT JOIN policies p ON v.policy_id = p.id
		LEFT JOIN agents a ON v.agent_id = a.id
		WHERE v.org_id = $1
	`
	args := []interface{}{orgID}
	argPos := 2

	if resolved != "" {
		query += " AND v.resolved = $" + string(rune('0'+argPos))
		args = append(args, resolved == "true")
		argPos++
	}

	if policyID != "" {
		query += " AND v.policy_id = $" + string(rune('0'+argPos))
		args = append(args, policyID)
		argPos++
	}

	if agentID != "" {
		query += " AND v.agent_id = $" + string(rune('0'+argPos))
		args = append(args, agentID)
		argPos++
	}

	query += " ORDER BY v.created_at DESC LIMIT 100"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list violations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list violations"})
		return
	}
	defer rows.Close()

	var violations []PolicyViolation
	for rows.Next() {
		var v PolicyViolation
		if err := rows.Scan(
			&v.ID, &v.PolicyID, &v.PolicyName, &v.OrgID, &v.AgentID, &v.AgentName,
			&v.ResourceType, &v.ResourceID, &v.ResourceName, &v.Message, &v.Details,
			&v.ActionTaken, &v.Resolved, &v.ResolvedBy, &v.ResolvedAt, &v.ResolutionNote, &v.CreatedAt,
		); err != nil {
			continue
		}
		violations = append(violations, v)
	}

	c.JSON(http.StatusOK, violations)
}

func (h *Handler) GetViolation(c *gin.Context) {
	violationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid violation ID"})
		return
	}

	var v PolicyViolation
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT v.id, v.policy_id, p.name, v.org_id, v.agent_id, COALESCE(a.name, ''),
		       v.resource_type, v.resource_id, v.resource_name, v.message, v.details,
		       v.action_taken, v.resolved, v.resolved_by, v.resolved_at, v.resolution_note, v.created_at
		FROM policy_violations v
		LEFT JOIN policies p ON v.policy_id = p.id
		LEFT JOIN agents a ON v.agent_id = a.id
		WHERE v.id = $1
	`, violationID).Scan(
		&v.ID, &v.PolicyID, &v.PolicyName, &v.OrgID, &v.AgentID, &v.AgentName,
		&v.ResourceType, &v.ResourceID, &v.ResourceName, &v.Message, &v.Details,
		&v.ActionTaken, &v.Resolved, &v.ResolvedBy, &v.ResolvedAt, &v.ResolutionNote, &v.CreatedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Violation not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to get violation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get violation"})
		return
	}

	c.JSON(http.StatusOK, v)
}

func (h *Handler) ResolveViolation(c *gin.Context) {
	violationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid violation ID"})
		return
	}

	userID := c.MustGet("user_id").(uuid.UUID)

	var req ResolveViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = h.db.Exec(c.Request.Context(), `
		UPDATE policy_violations
		SET resolved = true, resolved_by = $2, resolved_at = NOW(), resolution_note = $3
		WHERE id = $1
	`, violationID, userID, req.ResolutionNote)

	if err != nil {
		h.logger.Error("Failed to resolve violation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve violation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}

// ============ Stats ============

func (h *Handler) GetPolicyStats(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var stats struct {
		TotalPolicies      int `json:"total_policies"`
		EnabledPolicies    int `json:"enabled_policies"`
		TotalViolations    int `json:"total_violations"`
		UnresolvedViolations int `json:"unresolved_violations"`
		BlockedActions     int `json:"blocked_actions"`
		WarnedActions      int `json:"warned_actions"`
	}

	// Get policy counts
	h.db.QueryRow(c.Request.Context(), `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE enabled = true)
		FROM policies WHERE org_id = $1
	`, orgID).Scan(&stats.TotalPolicies, &stats.EnabledPolicies)

	// Get violation counts
	h.db.QueryRow(c.Request.Context(), `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE resolved = false),
			COUNT(*) FILTER (WHERE action_taken = 'blocked'),
			COUNT(*) FILTER (WHERE action_taken = 'warned')
		FROM policy_violations WHERE org_id = $1
	`, orgID).Scan(&stats.TotalViolations, &stats.UnresolvedViolations, &stats.BlockedActions, &stats.WarnedActions)

	c.JSON(http.StatusOK, stats)
}
