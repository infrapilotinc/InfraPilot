package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/infrapilot/backend/internal/webhook"
	"go.uber.org/zap"
)

// ==================== Webhook Config Management ====================

func (h *Handler) createWebhook(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req webhook.CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, secret, err := h.webhookService.CreateWebhook(c.Request.Context(), orgID, agentID, &req)
	if err != nil {
		h.logger.Error("failed to create webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create webhook"})
		return
	}

	// Build webhook URL
	webhookURL := fmt.Sprintf("/api/v1/webhooks/%s/receive", config.ID.String())

	response := &webhook.WebhookResponse{
		ID:          config.ID,
		Name:        config.Name,
		Provider:    config.Provider,
		ServiceName: config.ServiceName,
		Environment: config.Environment,
		Enabled:     config.Enabled,
		Secret:      &secret, // Only returned on creation
		WebhookURL:  webhookURL,
		CreatedAt:   config.CreatedAt,
		LastUsedAt:  config.LastUsedAt,
	}

	c.JSON(http.StatusCreated, response)
}

func (h *Handler) listWebhooks(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	configs, err := h.webhookService.ListWebhooks(c.Request.Context(), orgID, agentID)
	if err != nil {
		h.logger.Error("failed to list webhooks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list webhooks"})
		return
	}

	// Convert to response format
	responses := make([]*webhook.WebhookResponse, len(configs))
	for i, config := range configs {
		webhookURL := fmt.Sprintf("/api/v1/webhooks/%s/receive", config.ID.String())
		responses[i] = &webhook.WebhookResponse{
			ID:          config.ID,
			Name:        config.Name,
			Provider:    config.Provider,
			ServiceName: config.ServiceName,
			Environment: config.Environment,
			Enabled:     config.Enabled,
			Secret:      nil, // Never return secret after creation
			WebhookURL:  webhookURL,
			CreatedAt:   config.CreatedAt,
			LastUsedAt:  config.LastUsedAt,
		}
	}

	c.JSON(http.StatusOK, responses)
}

func (h *Handler) getWebhook(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	config, err := h.webhookService.GetWebhook(c.Request.Context(), webhookID)
	if err != nil {
		h.logger.Error("failed to get webhook", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	webhookURL := fmt.Sprintf("/api/v1/webhooks/%s/receive", config.ID.String())

	response := &webhook.WebhookResponse{
		ID:          config.ID,
		Name:        config.Name,
		Provider:    config.Provider,
		ServiceName: config.ServiceName,
		Environment: config.Environment,
		Enabled:     config.Enabled,
		Secret:      nil, // Never return secret
		WebhookURL:  webhookURL,
		CreatedAt:   config.CreatedAt,
		LastUsedAt:  config.LastUsedAt,
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) updateWebhook(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	var req webhook.UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.webhookService.UpdateWebhook(c.Request.Context(), webhookID, &req); err != nil {
		h.logger.Error("failed to update webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update webhook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "webhook updated"})
}

func (h *Handler) deleteWebhook(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	if err := h.webhookService.DeleteWebhook(c.Request.Context(), webhookID); err != nil {
		h.logger.Error("failed to delete webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete webhook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "webhook deleted"})
}

// ==================== Webhook Event Handlers ====================

func (h *Handler) receiveWebhook(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	// Read payload
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read payload"})
		return
	}

	// Extract headers
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Get webhook config
	config, err := h.webhookService.GetWebhook(c.Request.Context(), webhookID)
	if err != nil {
		h.logger.Error("webhook not found", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	// Verify and parse webhook
	metadata, err := h.webhookService.VerifyAndParse(c.Request.Context(), webhookID, headers, payload)
	if err != nil {
		h.logger.Error("failed to verify/parse webhook", zap.Error(err))
		// Record failed event
		eventType := headers["X-GitHub-Event"]
		if eventType == "" {
			eventType = headers["X-Gitlab-Event"]
		}
		h.webhookService.RecordWebhookEvent(c.Request.Context(), webhookID, config.Provider, eventType, headers, payload, false, nil, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to verify webhook"})
		return
	}

	// Create deployment from webhook
	deploymentID, err := h.createDeploymentFromWebhook(c.Request.Context(), config, metadata)
	if err != nil {
		h.logger.Error("failed to create deployment from webhook", zap.Error(err))
		eventType := headers["X-GitHub-Event"]
		if eventType == "" {
			eventType = headers["X-Gitlab-Event"]
		}
		h.webhookService.RecordWebhookEvent(c.Request.Context(), webhookID, config.Provider, eventType, headers, payload, true, nil, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create deployment"})
		return
	}

	// Record successful event
	eventType := headers["X-GitHub-Event"]
	if eventType == "" {
		eventType = headers["X-Gitlab-Event"]
	}
	eventID, err := h.webhookService.RecordWebhookEvent(c.Request.Context(), webhookID, config.Provider, eventType, headers, payload, true, &deploymentID, nil)
	if err != nil {
		h.logger.Warn("failed to record webhook event", zap.Error(err))
	}

	h.logger.Info("webhook processed successfully",
		zap.String("webhook_id", webhookID.String()),
		zap.String("deployment_id", deploymentID.String()),
		zap.String("event_id", eventID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":       "webhook processed",
		"deployment_id": deploymentID,
		"event_id":      eventID,
	})
}

func (h *Handler) listWebhookEvents(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	limit := 50
	offset := 0

	events, err := h.webhookService.ListWebhookEvents(c.Request.Context(), webhookID, limit, offset)
	if err != nil {
		h.logger.Error("failed to list webhook events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list events"})
		return
	}

	c.JSON(http.StatusOK, events)
}

// ==================== Helper Functions ====================

func (h *Handler) createDeploymentFromWebhook(ctx context.Context, config *webhook.WebhookConfig, metadata *webhook.BuildMetadata) (uuid.UUID, error) {
	// Build CreateDeploymentRequest from metadata
	req := CreateDeploymentRequest{
		ServiceName:     config.ServiceName,
		Environment:     config.Environment,
		ImageRepository: metadata.ImageRepo,
		ImageTag:        &metadata.ImageTag,
		ImageDigest:     metadata.ImageDigest,
		GitRepo:         &metadata.GitRepo,
		GitBranch:       &metadata.GitBranch,
		GitCommit:       &metadata.GitCommit,
		CIProvider:      &metadata.CIProvider,
		CIPipelineID:    &metadata.CIPipelineID,
		CIBuildURL:      &metadata.CIBuildURL,
	}

	// Create deployment using existing logic
	orgID := config.OrgID
	agentID := config.AgentID

	deployment := &Deployment{
		ID:              uuid.New(),
		OrgID:           orgID,
		AgentID:         agentID,
		ServiceName:     req.ServiceName,
		Environment:     req.Environment,
		ImageRepository: req.ImageRepository,
		ImageTag:        req.ImageTag,
		ImageDigest:     req.ImageDigest,
		GitRepo:         req.GitRepo,
		GitBranch:       req.GitBranch,
		GitCommit:       req.GitCommit,
		CIProvider:      req.CIProvider,
		CIPipelineID:    req.CIPipelineID,
		CIBuildURL:      req.CIBuildURL,
		Status:          StatusPending,
		PolicyDecision:  DecisionAllow, // Will be updated by pipeline
	}

	// Insert deployment into database
	query := `
		INSERT INTO deployments (
			id, org_id, agent_id, service_name, environment,
			image_registry, image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit,
			ci_provider, ci_pipeline_id, ci_build_url,
			status, policy_decision,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW())
	`

	_, err := h.db.Exec(ctx, query,
		deployment.ID, deployment.OrgID, deployment.AgentID,
		deployment.ServiceName, deployment.Environment,
		deployment.ImageRegistry, deployment.ImageRepository, deployment.ImageTag, deployment.ImageDigest,
		deployment.GitRepo, deployment.GitBranch, deployment.GitCommit,
		deployment.CIProvider, deployment.CIPipelineID, deployment.CIBuildURL,
		deployment.Status, deployment.PolicyDecision,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Extract imageDigest string (may be nil)
	imageDigest := ""
	if metadata.ImageDigest != nil {
		imageDigest = *metadata.ImageDigest
	}

	// Load container config from service_config if one exists for this service+env
	var containerConfig *DeploymentContainerConfig
	var cfgJSON []byte
	if err := h.db.QueryRow(ctx, `
		SELECT container_config FROM service_configs
		WHERE org_id = $1 AND agent_id = $2 AND service_name = $3 AND environment = $4
	`, orgID, agentID, config.ServiceName, config.Environment).Scan(&cfgJSON); err == nil {
		if len(cfgJSON) > 0 {
			var cfg DeploymentContainerConfig
			if json.Unmarshal(cfgJSON, &cfg) == nil {
				containerConfig = &cfg
			}
		}
	}

	// Start deployment pipeline in background
	go h.runDeploymentPipeline(context.Background(), orgID, deployment.ID, metadata.ImageRepo, metadata.ImageTag, imageDigest, containerConfig, false, false)

	return deployment.ID, nil
}

func (h *Handler) regenerateWebhookSecret(c *gin.Context) {
	_, err := uuid.Parse(c.Param("wid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook ID"})
		return
	}

	// TODO: Implement secret regeneration in webhook service
	// This would involve:
	// 1. Generate new secret
	// 2. Hash and store it
	// 3. Return the new secret to the user
	c.JSON(http.StatusNotImplemented, gin.H{"error": "secret regeneration not yet implemented"})
}
