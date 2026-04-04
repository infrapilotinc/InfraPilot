package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ============ Types ============

type TrafficResource struct {
	ID            uuid.UUID              `json:"id"`
	OrgID         uuid.UUID              `json:"org_id"`
	AgentID       uuid.UUID              `json:"agent_id"`
	Name          string                 `json:"name"`
	Description   *string                `json:"description,omitempty"`
	Domains       []string               `json:"domains"`
	Paths         []PathConfig           `json:"paths"`
	GatewayType   string                 `json:"gateway_type"`
	Status        string                 `json:"status"`
	DesiredState  map[string]interface{} `json:"desired_state,omitempty"`
	AppliedState  map[string]interface{} `json:"applied_state,omitempty"`
	LastAppliedAt *time.Time             `json:"last_applied_at,omitempty"`
	LastError     *string                `json:"last_error,omitempty"`
	Labels        map[string]string      `json:"labels"`
	Annotations   map[string]string      `json:"annotations"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type PathConfig struct {
	Path  string `json:"path"`
	Match string `json:"match"` // "prefix", "exact", "regex"
}

type TrafficUpstream struct {
	ID                     uuid.UUID        `json:"id"`
	TrafficResourceID      uuid.UUID        `json:"traffic_resource_id"`
	Name                   string           `json:"name"`
	Targets                []UpstreamTarget `json:"targets"`
	LBMethod               string           `json:"lb_method"`
	HealthCheckEnabled     bool             `json:"health_check_enabled"`
	HealthCheckPath        string           `json:"health_check_path"`
	HealthCheckIntervalSec int              `json:"health_check_interval_sec"`
	HealthCheckTimeoutSec  int              `json:"health_check_timeout_sec"`
	HealthyThreshold       int              `json:"healthy_threshold"`
	UnhealthyThreshold     int              `json:"unhealthy_threshold"`
	MaxConnections         *int             `json:"max_connections,omitempty"`
	MaxKeepalive           int              `json:"max_keepalive"`
	ConnectTimeoutSec      int              `json:"connect_timeout_sec"`
	ReadTimeoutSec         int              `json:"read_timeout_sec"`
	SendTimeoutSec         int              `json:"send_timeout_sec"`
	LastHealthCheck        *time.Time       `json:"last_health_check,omitempty"`
	HealthyTargets         int              `json:"healthy_targets"`
	TotalTargets           int              `json:"total_targets"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

type UpstreamTarget struct {
	ID                  uuid.UUID  `json:"id,omitempty"`
	Address             string     `json:"address"`
	Port                int        `json:"port"`
	Weight              int        `json:"weight"`
	MaxFails            int        `json:"max_fails"`
	FailTimeoutSec      int        `json:"fail_timeout_sec"`
	IsHealthy           bool       `json:"is_healthy"`
	LastHealthCheck     *time.Time `json:"last_health_check,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastError           *string    `json:"last_error,omitempty"`
	Enabled             bool       `json:"enabled"`
}

type TrafficPolicy struct {
	ID          uuid.UUID              `json:"id"`
	OrgID       uuid.UUID              `json:"org_id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	PolicyType  string                 `json:"policy_type"`
	Config      map[string]interface{} `json:"config"`
	IsGlobal    bool                   `json:"is_global"`
	Priority    int                    `json:"priority"`
	Enabled     bool                   `json:"enabled"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type TrafficPolicyAssignment struct {
	ID                uuid.UUID              `json:"id"`
	PolicyID          uuid.UUID              `json:"policy_id"`
	TrafficResourceID *uuid.UUID             `json:"traffic_resource_id,omitempty"`
	PathPattern       *string                `json:"path_pattern,omitempty"`
	ConfigOverride    map[string]interface{} `json:"config_override,omitempty"`
	Priority          int                    `json:"priority"`
	Enabled           bool                   `json:"enabled"`
	CreatedAt         time.Time              `json:"created_at"`
}

type TrafficTLSConfig struct {
	ID                    uuid.UUID  `json:"id"`
	TrafficResourceID     uuid.UUID  `json:"traffic_resource_id"`
	CertSource            string     `json:"cert_source"`
	CertID                *uuid.UUID `json:"cert_id,omitempty"`
	MinVersion            string     `json:"min_version"`
	MaxVersion            string     `json:"max_version"`
	CipherSuites          []string   `json:"cipher_suites,omitempty"`
	PreferServerCiphers   bool       `json:"prefer_server_ciphers"`
	HSTSEnabled           bool       `json:"hsts_enabled"`
	HSTSMaxAge            int        `json:"hsts_max_age"`
	HSTSIncludeSubdomains bool       `json:"hsts_include_subdomains"`
	HSTSPreload           bool       `json:"hsts_preload"`
	OCSPStapling          bool       `json:"ocsp_stapling"`
	MTLSEnabled           bool       `json:"mtls_enabled"`
	MTLSCACert            *string    `json:"mtls_ca_cert,omitempty"`
	MTLSVerifyDepth       int        `json:"mtls_verify_depth"`
	MTLSVerifyClient      string     `json:"mtls_verify_client"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type TrafficApplyHistory struct {
	ID                  uuid.UUID              `json:"id"`
	TrafficResourceID   uuid.UUID              `json:"traffic_resource_id"`
	Action              string                 `json:"action"`
	PreviousState       map[string]interface{} `json:"previous_state,omitempty"`
	NewState            map[string]interface{} `json:"new_state,omitempty"`
	DryRunResult        map[string]interface{} `json:"dry_run_result,omitempty"`
	ValidationPassed    *bool                  `json:"validation_passed,omitempty"`
	ValidationErrors    []string               `json:"validation_errors,omitempty"`
	AppliedBy           *uuid.UUID             `json:"applied_by,omitempty"`
	AppliedAt           time.Time              `json:"applied_at"`
	ApplyDurationMs     *int                   `json:"apply_duration_ms,omitempty"`
	Success             *bool                  `json:"success,omitempty"`
	ErrorMessage        *string                `json:"error_message,omitempty"`
	RenderedNginxConfig *string                `json:"rendered_nginx_config,omitempty"`
	ConfigHash          *string                `json:"config_hash,omitempty"`
}

type TrafficResourceSummary struct {
	OrgID               uuid.UUID `json:"org_id"`
	TotalResources      int       `json:"total_resources"`
	ActiveResources     int       `json:"active_resources"`
	DraftResources      int       `json:"draft_resources"`
	ErrorResources      int       `json:"error_resources"`
	SystemGateways      int       `json:"system_gateways"`
	ApplicationGateways int       `json:"application_gateways"`
	AgentsWithTraffic   int       `json:"agents_with_traffic"`
}

// ============ Traffic Resources Handlers ============

// GET /api/v1/traffic/resources
func (h *Handler) listTrafficResources(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	// Query parameters
	agentID := c.Query("agent_id")
	status := c.Query("status")
	gatewayType := c.Query("gateway_type")
	limit := c.DefaultQuery("limit", "100")
	offset := c.DefaultQuery("offset", "0")

	query := `
		SELECT
			id, org_id, agent_id, name, description,
			domains, paths, gateway_type, status,
			desired_state, applied_state, last_applied_at, last_error,
			labels, annotations, created_at, updated_at
		FROM traffic_resources
		WHERE org_id = $1
	`
	args := []interface{}{orgID}
	argCount := 1

	if agentID != "" {
		argCount++
		query += fmt.Sprintf(" AND agent_id = $%d", argCount)
		args = append(args, agentID)
	}
	if status != "" {
		argCount++
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, status)
	}
	if gatewayType != "" {
		argCount++
		query += fmt.Sprintf(" AND gateway_type = $%d", argCount)
		args = append(args, gatewayType)
	}

	query += fmt.Sprintf(" ORDER BY name ASC LIMIT $%d OFFSET $%d", argCount+1, argCount+2)
	args = append(args, limit, offset)

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to fetch traffic resources: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch traffic resources"})
		return
	}
	defer rows.Close()

	var resources []TrafficResource
	for rows.Next() {
		var r TrafficResource
		var pathsJSON, desiredStateJSON, appliedStateJSON, labelsJSON, annotationsJSON []byte
		err := rows.Scan(
			&r.ID, &r.OrgID, &r.AgentID, &r.Name, &r.Description,
			&r.Domains, &pathsJSON, &r.GatewayType, &r.Status,
			&desiredStateJSON, &appliedStateJSON, &r.LastAppliedAt, &r.LastError,
			&labelsJSON, &annotationsJSON, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			h.logger.Error("Failed to scan traffic resource: " + err.Error())
			continue
		}

		json.Unmarshal(pathsJSON, &r.Paths)
		json.Unmarshal(desiredStateJSON, &r.DesiredState)
		json.Unmarshal(appliedStateJSON, &r.AppliedState)
		json.Unmarshal(labelsJSON, &r.Labels)
		json.Unmarshal(annotationsJSON, &r.Annotations)

		if r.Paths == nil {
			r.Paths = []PathConfig{{Path: "/", Match: "prefix"}}
		}
		if r.Labels == nil {
			r.Labels = map[string]string{}
		}
		if r.Annotations == nil {
			r.Annotations = map[string]string{}
		}

		resources = append(resources, r)
	}

	if resources == nil {
		resources = []TrafficResource{}
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM traffic_resources WHERE org_id = $1`
	var total int
	h.db.QueryRow(c.Request.Context(), countQuery, orgID).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     total,
	})
}

// GET /api/v1/traffic/resources/:id
func (h *Handler) getTrafficResource(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	query := `
		SELECT
			id, org_id, agent_id, name, description,
			domains, paths, gateway_type, status,
			desired_state, applied_state, last_applied_at, last_error,
			labels, annotations, created_at, updated_at
		FROM traffic_resources
		WHERE id = $1 AND org_id = $2
	`

	var r TrafficResource
	var pathsJSON, desiredStateJSON, appliedStateJSON, labelsJSON, annotationsJSON []byte
	err = h.db.QueryRow(c.Request.Context(), query, resourceUUID, orgID).Scan(
		&r.ID, &r.OrgID, &r.AgentID, &r.Name, &r.Description,
		&r.Domains, &pathsJSON, &r.GatewayType, &r.Status,
		&desiredStateJSON, &appliedStateJSON, &r.LastAppliedAt, &r.LastError,
		&labelsJSON, &annotationsJSON, &r.CreatedAt, &r.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to fetch traffic resource: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch traffic resource"})
		return
	}

	json.Unmarshal(pathsJSON, &r.Paths)
	json.Unmarshal(desiredStateJSON, &r.DesiredState)
	json.Unmarshal(appliedStateJSON, &r.AppliedState)
	json.Unmarshal(labelsJSON, &r.Labels)
	json.Unmarshal(annotationsJSON, &r.Annotations)

	c.JSON(http.StatusOK, r)
}

// POST /api/v1/traffic/resources
func (h *Handler) createTrafficResource(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req struct {
		AgentID     string            `json:"agent_id" binding:"required"`
		Name        string            `json:"name" binding:"required"`
		Description *string           `json:"description"`
		Domains     []string          `json:"domains" binding:"required"`
		Paths       []PathConfig      `json:"paths"`
		GatewayType string            `json:"gateway_type"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	agentUUID, err := uuid.Parse(req.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Set defaults
	if req.Paths == nil || len(req.Paths) == 0 {
		req.Paths = []PathConfig{{Path: "/", Match: "prefix"}}
	}
	if req.GatewayType == "" {
		req.GatewayType = "application"
	}
	if req.Labels == nil {
		req.Labels = map[string]string{}
	}
	if req.Annotations == nil {
		req.Annotations = map[string]string{}
	}

	pathsJSON, _ := json.Marshal(req.Paths)
	labelsJSON, _ := json.Marshal(req.Labels)
	annotationsJSON, _ := json.Marshal(req.Annotations)

	query := `
		INSERT INTO traffic_resources (
			org_id, agent_id, name, description,
			domains, paths, gateway_type,
			labels, annotations
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, status, created_at, updated_at
	`

	var r TrafficResource
	r.OrgID = orgID
	r.AgentID = agentUUID
	r.Name = req.Name
	r.Description = req.Description
	r.Domains = req.Domains
	r.Paths = req.Paths
	r.GatewayType = req.GatewayType
	r.Labels = req.Labels
	r.Annotations = req.Annotations

	err = h.db.QueryRow(c.Request.Context(), query,
		orgID, agentUUID, req.Name, req.Description,
		req.Domains, pathsJSON, req.GatewayType,
		labelsJSON, annotationsJSON,
	).Scan(&r.ID, &r.Status, &r.CreatedAt, &r.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to create traffic resource: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create traffic resource"})
		return
	}

	c.JSON(http.StatusCreated, r)
}

// PUT /api/v1/traffic/resources/:id
func (h *Handler) updateTrafficResource(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	var req struct {
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
		Domains     []string          `json:"domains"`
		Paths       []PathConfig      `json:"paths"`
		GatewayType *string           `json:"gateway_type"`
		Status      *string           `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argCount := 0

	if req.Name != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("name = $%d", argCount))
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("description = $%d", argCount))
		args = append(args, *req.Description)
	}
	if req.Domains != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("domains = $%d", argCount))
		args = append(args, req.Domains)
	}
	if req.Paths != nil {
		argCount++
		pathsJSON, _ := json.Marshal(req.Paths)
		updates = append(updates, fmt.Sprintf("paths = $%d", argCount))
		args = append(args, pathsJSON)
	}
	if req.GatewayType != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("gateway_type = $%d", argCount))
		args = append(args, *req.GatewayType)
	}
	if req.Status != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("status = $%d", argCount))
		args = append(args, *req.Status)
	}
	if req.Labels != nil {
		argCount++
		labelsJSON, _ := json.Marshal(req.Labels)
		updates = append(updates, fmt.Sprintf("labels = $%d", argCount))
		args = append(args, labelsJSON)
	}
	if req.Annotations != nil {
		argCount++
		annotationsJSON, _ := json.Marshal(req.Annotations)
		updates = append(updates, fmt.Sprintf("annotations = $%d", argCount))
		args = append(args, annotationsJSON)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	query := fmt.Sprintf(`
		UPDATE traffic_resources SET %s, updated_at = NOW()
		WHERE id = $%d AND org_id = $%d
		RETURNING id
	`, strings.Join(updates, ", "), argCount+1, argCount+2)
	args = append(args, resourceUUID, orgID)

	var returnedID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), query, args...).Scan(&returnedID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to update traffic resource: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update traffic resource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Traffic resource updated successfully"})
}

// DELETE /api/v1/traffic/resources/:id
func (h *Handler) deleteTrafficResource(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	query := `DELETE FROM traffic_resources WHERE id = $1 AND org_id = $2`
	result, err := h.db.Exec(c.Request.Context(), query, resourceUUID, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete traffic resource"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Traffic resource deleted successfully"})
}

// GET /api/v1/traffic/summary
func (h *Handler) getTrafficSummary(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	query := `SELECT * FROM v_traffic_resource_summary WHERE org_id = $1`

	var summary TrafficResourceSummary
	err := h.db.QueryRow(c.Request.Context(), query, orgID).Scan(
		&summary.OrgID, &summary.TotalResources, &summary.ActiveResources,
		&summary.DraftResources, &summary.ErrorResources,
		&summary.SystemGateways, &summary.ApplicationGateways,
		&summary.AgentsWithTraffic,
	)

	if err == pgx.ErrNoRows {
		summary = TrafficResourceSummary{OrgID: orgID}
	} else if err != nil {
		h.logger.Error("Failed to fetch traffic summary: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch traffic summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// ============ Traffic Policies Handlers ============

// GET /api/v1/traffic/policies
func (h *Handler) listTrafficPolicies(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	policyType := c.Query("type")
	limit := c.DefaultQuery("limit", "100")
	offset := c.DefaultQuery("offset", "0")

	query := `
		SELECT
			id, org_id, name, description, policy_type,
			config, is_global, priority, enabled,
			created_at, updated_at
		FROM traffic_policies
		WHERE org_id = $1
	`
	args := []interface{}{orgID}
	argCount := 1

	if policyType != "" {
		argCount++
		query += fmt.Sprintf(" AND policy_type = $%d", argCount)
		args = append(args, policyType)
	}

	query += fmt.Sprintf(" ORDER BY priority DESC, name ASC LIMIT $%d OFFSET $%d", argCount+1, argCount+2)
	args = append(args, limit, offset)

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch traffic policies"})
		return
	}
	defer rows.Close()

	var policies []TrafficPolicy
	for rows.Next() {
		var p TrafficPolicy
		var configJSON []byte
		err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &p.PolicyType,
			&configJSON, &p.IsGlobal, &p.Priority, &p.Enabled,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}

		json.Unmarshal(configJSON, &p.Config)
		if p.Config == nil {
			p.Config = map[string]interface{}{}
		}

		policies = append(policies, p)
	}

	if policies == nil {
		policies = []TrafficPolicy{}
	}

	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

// GET /api/v1/traffic/policies/:id
func (h *Handler) getTrafficPolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	policyID := c.Param("id")

	policyUUID, err := uuid.Parse(policyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	query := `
		SELECT
			id, org_id, name, description, policy_type,
			config, is_global, priority, enabled,
			created_at, updated_at
		FROM traffic_policies
		WHERE id = $1 AND org_id = $2
	`

	var p TrafficPolicy
	var configJSON []byte
	err = h.db.QueryRow(c.Request.Context(), query, policyUUID, orgID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.PolicyType,
		&configJSON, &p.IsGlobal, &p.Priority, &p.Enabled,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic policy not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch traffic policy"})
		return
	}

	json.Unmarshal(configJSON, &p.Config)

	c.JSON(http.StatusOK, p)
}

// POST /api/v1/traffic/policies
func (h *Handler) createTrafficPolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req struct {
		Name        string                 `json:"name" binding:"required"`
		Description *string                `json:"description"`
		PolicyType  string                 `json:"policy_type" binding:"required"`
		Config      map[string]interface{} `json:"config"`
		IsGlobal    bool                   `json:"is_global"`
		Priority    int                    `json:"priority"`
		Enabled     bool                   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Config == nil {
		req.Config = map[string]interface{}{}
	}

	configJSON, _ := json.Marshal(req.Config)

	query := `
		INSERT INTO traffic_policies (
			org_id, name, description, policy_type,
			config, is_global, priority, enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	var p TrafficPolicy
	p.OrgID = orgID
	p.Name = req.Name
	p.Description = req.Description
	p.PolicyType = req.PolicyType
	p.Config = req.Config
	p.IsGlobal = req.IsGlobal
	p.Priority = req.Priority
	p.Enabled = req.Enabled

	err := h.db.QueryRow(c.Request.Context(), query,
		orgID, req.Name, req.Description, req.PolicyType,
		configJSON, req.IsGlobal, req.Priority, req.Enabled,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to create traffic policy: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create traffic policy"})
		return
	}

	c.JSON(http.StatusCreated, p)
}

// PUT /api/v1/traffic/policies/:id
func (h *Handler) updateTrafficPolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	policyID := c.Param("id")

	policyUUID, err := uuid.Parse(policyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	var req struct {
		Name        *string                `json:"name"`
		Description *string                `json:"description"`
		Config      map[string]interface{} `json:"config"`
		IsGlobal    *bool                  `json:"is_global"`
		Priority    *int                   `json:"priority"`
		Enabled     *bool                  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Build dynamic update
	updates := []string{}
	args := []interface{}{}
	argCount := 0

	if req.Name != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("name = $%d", argCount))
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("description = $%d", argCount))
		args = append(args, *req.Description)
	}
	if req.Config != nil {
		argCount++
		configJSON, _ := json.Marshal(req.Config)
		updates = append(updates, fmt.Sprintf("config = $%d", argCount))
		args = append(args, configJSON)
	}
	if req.IsGlobal != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("is_global = $%d", argCount))
		args = append(args, *req.IsGlobal)
	}
	if req.Priority != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("priority = $%d", argCount))
		args = append(args, *req.Priority)
	}
	if req.Enabled != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("enabled = $%d", argCount))
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	query := fmt.Sprintf(`
		UPDATE traffic_policies SET %s, updated_at = NOW()
		WHERE id = $%d AND org_id = $%d
		RETURNING id
	`, strings.Join(updates, ", "), argCount+1, argCount+2)
	args = append(args, policyUUID, orgID)

	var returnedID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), query, args...).Scan(&returnedID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic policy not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update traffic policy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Traffic policy updated successfully"})
}

// DELETE /api/v1/traffic/policies/:id
func (h *Handler) deleteTrafficPolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	policyID := c.Param("id")

	policyUUID, err := uuid.Parse(policyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	query := `DELETE FROM traffic_policies WHERE id = $1 AND org_id = $2`
	result, err := h.db.Exec(c.Request.Context(), query, policyUUID, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete traffic policy"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic policy not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Traffic policy deleted successfully"})
}

// ============ Traffic Upstreams Handlers ============

// GET /api/v1/traffic/resources/:id/upstreams
func (h *Handler) listTrafficUpstreams(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	// Verify resource belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM traffic_resources WHERE id = $1 AND org_id = $2)",
		resourceUUID, orgID,
	).Scan(&exists)

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}

	query := `
		SELECT
			id, traffic_resource_id, name, targets, lb_method,
			health_check_enabled, health_check_path, health_check_interval_sec,
			health_check_timeout_sec, healthy_threshold, unhealthy_threshold,
			max_connections, max_keepalive, connect_timeout_sec, read_timeout_sec, send_timeout_sec,
			last_health_check, healthy_targets, total_targets,
			created_at, updated_at
		FROM traffic_upstreams
		WHERE traffic_resource_id = $1
		ORDER BY name
	`

	rows, err := h.db.Query(c.Request.Context(), query, resourceUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch upstreams"})
		return
	}
	defer rows.Close()

	var upstreams []TrafficUpstream
	for rows.Next() {
		var u TrafficUpstream
		var targetsJSON []byte
		err := rows.Scan(
			&u.ID, &u.TrafficResourceID, &u.Name, &targetsJSON, &u.LBMethod,
			&u.HealthCheckEnabled, &u.HealthCheckPath, &u.HealthCheckIntervalSec,
			&u.HealthCheckTimeoutSec, &u.HealthyThreshold, &u.UnhealthyThreshold,
			&u.MaxConnections, &u.MaxKeepalive, &u.ConnectTimeoutSec, &u.ReadTimeoutSec, &u.SendTimeoutSec,
			&u.LastHealthCheck, &u.HealthyTargets, &u.TotalTargets,
			&u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			continue
		}

		json.Unmarshal(targetsJSON, &u.Targets)
		if u.Targets == nil {
			u.Targets = []UpstreamTarget{}
		}

		upstreams = append(upstreams, u)
	}

	if upstreams == nil {
		upstreams = []TrafficUpstream{}
	}

	c.JSON(http.StatusOK, gin.H{"upstreams": upstreams})
}

// POST /api/v1/traffic/resources/:id/upstreams
func (h *Handler) createTrafficUpstream(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	// Verify resource belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM traffic_resources WHERE id = $1 AND org_id = $2)",
		resourceUUID, orgID,
	).Scan(&exists)

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}

	var req struct {
		Name                   string           `json:"name" binding:"required"`
		Targets                []UpstreamTarget `json:"targets"`
		LBMethod               string           `json:"lb_method"`
		HealthCheckEnabled     bool             `json:"health_check_enabled"`
		HealthCheckPath        string           `json:"health_check_path"`
		HealthCheckIntervalSec int              `json:"health_check_interval_sec"`
		HealthCheckTimeoutSec  int              `json:"health_check_timeout_sec"`
		HealthyThreshold       int              `json:"healthy_threshold"`
		UnhealthyThreshold     int              `json:"unhealthy_threshold"`
		MaxConnections         *int             `json:"max_connections"`
		MaxKeepalive           int              `json:"max_keepalive"`
		ConnectTimeoutSec      int              `json:"connect_timeout_sec"`
		ReadTimeoutSec         int              `json:"read_timeout_sec"`
		SendTimeoutSec         int              `json:"send_timeout_sec"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Set defaults
	if req.LBMethod == "" {
		req.LBMethod = "round_robin"
	}
	if req.HealthCheckPath == "" {
		req.HealthCheckPath = "/health"
	}
	if req.HealthCheckIntervalSec == 0 {
		req.HealthCheckIntervalSec = 30
	}
	if req.HealthCheckTimeoutSec == 0 {
		req.HealthCheckTimeoutSec = 5
	}
	if req.HealthyThreshold == 0 {
		req.HealthyThreshold = 2
	}
	if req.UnhealthyThreshold == 0 {
		req.UnhealthyThreshold = 3
	}
	if req.MaxKeepalive == 0 {
		req.MaxKeepalive = 64
	}
	if req.ConnectTimeoutSec == 0 {
		req.ConnectTimeoutSec = 5
	}
	if req.ReadTimeoutSec == 0 {
		req.ReadTimeoutSec = 60
	}
	if req.SendTimeoutSec == 0 {
		req.SendTimeoutSec = 60
	}
	if req.Targets == nil {
		req.Targets = []UpstreamTarget{}
	}

	targetsJSON, _ := json.Marshal(req.Targets)

	query := `
		INSERT INTO traffic_upstreams (
			traffic_resource_id, name, targets, lb_method,
			health_check_enabled, health_check_path, health_check_interval_sec,
			health_check_timeout_sec, healthy_threshold, unhealthy_threshold,
			max_connections, max_keepalive, connect_timeout_sec, read_timeout_sec, send_timeout_sec,
			total_targets
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, created_at, updated_at
	`

	var u TrafficUpstream
	u.TrafficResourceID = resourceUUID
	u.Name = req.Name
	u.Targets = req.Targets
	u.LBMethod = req.LBMethod
	u.HealthCheckEnabled = req.HealthCheckEnabled
	u.HealthCheckPath = req.HealthCheckPath
	u.HealthCheckIntervalSec = req.HealthCheckIntervalSec
	u.HealthCheckTimeoutSec = req.HealthCheckTimeoutSec
	u.HealthyThreshold = req.HealthyThreshold
	u.UnhealthyThreshold = req.UnhealthyThreshold
	u.MaxConnections = req.MaxConnections
	u.MaxKeepalive = req.MaxKeepalive
	u.ConnectTimeoutSec = req.ConnectTimeoutSec
	u.ReadTimeoutSec = req.ReadTimeoutSec
	u.SendTimeoutSec = req.SendTimeoutSec
	u.TotalTargets = len(req.Targets)

	err = h.db.QueryRow(c.Request.Context(), query,
		resourceUUID, req.Name, targetsJSON, req.LBMethod,
		req.HealthCheckEnabled, req.HealthCheckPath, req.HealthCheckIntervalSec,
		req.HealthCheckTimeoutSec, req.HealthyThreshold, req.UnhealthyThreshold,
		req.MaxConnections, req.MaxKeepalive, req.ConnectTimeoutSec, req.ReadTimeoutSec, req.SendTimeoutSec,
		len(req.Targets),
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to create upstream: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upstream"})
		return
	}

	c.JSON(http.StatusCreated, u)
}

// GET /api/v1/traffic/resources/:id/history
func (h *Handler) getTrafficApplyHistory(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	resourceID := c.Param("id")

	resourceUUID, err := uuid.Parse(resourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	// Verify resource belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM traffic_resources WHERE id = $1 AND org_id = $2)",
		resourceUUID, orgID,
	).Scan(&exists)

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}

	query := `
		SELECT
			id, traffic_resource_id, action,
			previous_state, new_state, dry_run_result,
			validation_passed, validation_errors,
			applied_by, applied_at, apply_duration_ms,
			success, error_message, rendered_nginx_config, config_hash
		FROM traffic_apply_history
		WHERE traffic_resource_id = $1
		ORDER BY applied_at DESC
		LIMIT 50
	`

	rows, err := h.db.Query(c.Request.Context(), query, resourceUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch apply history"})
		return
	}
	defer rows.Close()

	var history []TrafficApplyHistory
	for rows.Next() {
		var h TrafficApplyHistory
		var prevStateJSON, newStateJSON, dryRunJSON []byte
		err := rows.Scan(
			&h.ID, &h.TrafficResourceID, &h.Action,
			&prevStateJSON, &newStateJSON, &dryRunJSON,
			&h.ValidationPassed, &h.ValidationErrors,
			&h.AppliedBy, &h.AppliedAt, &h.ApplyDurationMs,
			&h.Success, &h.ErrorMessage, &h.RenderedNginxConfig, &h.ConfigHash,
		)
		if err != nil {
			continue
		}

		json.Unmarshal(prevStateJSON, &h.PreviousState)
		json.Unmarshal(newStateJSON, &h.NewState)
		json.Unmarshal(dryRunJSON, &h.DryRunResult)

		history = append(history, h)
	}

	if history == nil {
		history = []TrafficApplyHistory{}
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}

// POST /api/v1/traffic/policies/:id/assign
func (h *Handler) assignTrafficPolicy(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	policyID := c.Param("id")

	policyUUID, err := uuid.Parse(policyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy ID"})
		return
	}

	// Verify policy belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM traffic_policies WHERE id = $1 AND org_id = $2)",
		policyUUID, orgID,
	).Scan(&exists)

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic policy not found"})
		return
	}

	var req struct {
		TrafficResourceID string                 `json:"traffic_resource_id" binding:"required"`
		PathPattern       *string                `json:"path_pattern"`
		ConfigOverride    map[string]interface{} `json:"config_override"`
		Priority          int                    `json:"priority"`
		Enabled           bool                   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	resourceUUID, err := uuid.Parse(req.TrafficResourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid traffic resource ID"})
		return
	}

	// Verify resource belongs to org
	h.db.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM traffic_resources WHERE id = $1 AND org_id = $2)",
		resourceUUID, orgID,
	).Scan(&exists)

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Traffic resource not found"})
		return
	}

	var configOverrideJSON []byte
	if req.ConfigOverride != nil {
		configOverrideJSON, _ = json.Marshal(req.ConfigOverride)
	}

	query := `
		INSERT INTO traffic_policy_assignments (
			policy_id, traffic_resource_id, path_pattern,
			config_override, priority, enabled
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	var assignment TrafficPolicyAssignment
	assignment.PolicyID = policyUUID
	assignment.TrafficResourceID = &resourceUUID
	assignment.PathPattern = req.PathPattern
	assignment.ConfigOverride = req.ConfigOverride
	assignment.Priority = req.Priority
	assignment.Enabled = req.Enabled

	err = h.db.QueryRow(c.Request.Context(), query,
		policyUUID, resourceUUID, req.PathPattern,
		configOverrideJSON, req.Priority, req.Enabled,
	).Scan(&assignment.ID, &assignment.CreatedAt)

	if err != nil {
		h.logger.Error("Failed to assign policy: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign policy"})
		return
	}

	c.JSON(http.StatusCreated, assignment)
}

// joinStrings helper is defined in dependencies_handlers.go
