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

	if req.ServiceName == webhook.AllServicesTarget && req.StackID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "targeting all services requires stack_id"})
		return
	}
	if req.StackID != nil {
		var exists bool
		if err := h.db.QueryRow(c.Request.Context(),
			`SELECT EXISTS(SELECT 1 FROM stacks WHERE id = $1 AND org_id = $2 AND agent_id = $3)`,
			*req.StackID, orgID, agentID,
		).Scan(&exists); err != nil || !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "stack not found"})
			return
		}
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
		StackID:     config.StackID,
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
			StackID:     config.StackID,
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
		StackID:     config.StackID,
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
	orgID := config.OrgID
	agentID := config.AgentID

	// Extract imageDigest string (may be nil)
	imageDigest := ""
	if metadata.ImageDigest != nil {
		imageDigest = *metadata.ImageDigest
	}

	if config.StackID != nil {
		return h.createStackDeploymentFromWebhook(ctx, config, metadata, imageDigest)
	}

	// Standalone service (not part of a Stack) -- unchanged from before this feature.
	deployment := &Deployment{
		ID:              uuid.New(),
		OrgID:           orgID,
		AgentID:         agentID,
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

// createStackDeploymentFromWebhook is the config.StackID != nil branch of
// createDeploymentFromWebhook. It mirrors redeployManagedStack's own pattern (same
// head-deployment lookup, same INSERT columns) so a webhook-triggered redeploy behaves
// identically to a manual one: the new row correctly chains onto the service's current
// head deployment (stack_id, service_order, replaces_deployment_id) and inherits its real
// container_config, instead of the service_configs lookup above -- which stack-managed
// services never populate, so a stack service redeployed via webhook used to silently get
// zero env vars/volumes/networks. If this service has never actually been deployed under
// this stack, this fails the webhook outright rather than silently falling back to the
// standalone path with the wrong config.
func (h *Handler) createStackDeploymentFromWebhook(ctx context.Context, config *webhook.WebhookConfig, metadata *webhook.BuildMetadata, imageDigest string) (uuid.UUID, error) {
	stackID := *config.StackID

	if config.ServiceName == webhook.AllServicesTarget {
		return h.redeployWholeStackFromWebhook(ctx, config)
	}

	var head struct {
		DeploymentID    uuid.UUID
		Order           int
		ContainerConfig []byte
	}
	err := h.db.QueryRow(ctx, `
		SELECT DISTINCT ON (service_name) id, service_order, container_config
		FROM deployments
		WHERE stack_id = $1 AND service_name = $2
		ORDER BY service_name, created_at DESC
	`, stackID, config.ServiceName).Scan(&head.DeploymentID, &head.Order, &head.ContainerConfig)
	if err != nil {
		return uuid.Nil, fmt.Errorf("service %q has never been deployed under this webhook's stack: %w", config.ServiceName, err)
	}

	var containerConfig *DeploymentContainerConfig
	if len(head.ContainerConfig) > 0 {
		var cfg DeploymentContainerConfig
		if json.Unmarshal(head.ContainerConfig, &cfg) == nil {
			containerConfig = &cfg
		}
	}

	deploymentID := uuid.New()
	_, err = h.db.Exec(ctx, `
		INSERT INTO deployments (
			id, org_id, agent_id, service_name, environment,
			image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit,
			ci_provider, ci_pipeline_id, ci_build_url,
			status, policy_decision,
			stack_id, service_order, container_config, replaces_deployment_id,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NOW(),NOW())
	`,
		deploymentID, config.OrgID, config.AgentID, config.ServiceName, config.Environment,
		metadata.ImageRepo, metadata.ImageTag, metadata.ImageDigest,
		metadata.GitRepo, metadata.GitBranch, metadata.GitCommit,
		metadata.CIProvider, metadata.CIPipelineID, metadata.CIBuildURL,
		StatusPending, DecisionAllow,
		stackID, head.Order, head.ContainerConfig, head.DeploymentID,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create stack deployment: %w", err)
	}

	go func() {
		bgCtx := context.Background()
		h.runDeploymentPipeline(bgCtx, config.OrgID, deploymentID, metadata.ImageRepo, metadata.ImageTag, imageDigest, containerConfig, false, false)
		h.recomputeStackStatus(bgCtx, stackID)
	}()

	return deploymentID, nil
}

// redeployWholeStackFromWebhook is the config.ServiceName == webhook.AllServicesTarget branch
// of createStackDeploymentFromWebhook: the webhook doesn't carry a build for one specific
// service, so there's no single new image to apply across every service in the stack. Instead
// this mirrors redeployManagedStack's own "all services, pull_latest" default -- each service
// is redeployed on its own already-configured image (a fresh pull picks up a mutable tag),
// exactly what clicking "Redeploy Stack" with no service selection does today, just triggered
// by CI instead of a click.
func (h *Handler) redeployWholeStackFromWebhook(ctx context.Context, config *webhook.WebhookConfig) (uuid.UUID, error) {
	stackID := *config.StackID

	var skipScanning bool
	if err := h.db.QueryRow(ctx, `SELECT skip_scanning FROM stacks WHERE id = $1`, stackID).Scan(&skipScanning); err != nil {
		return uuid.Nil, fmt.Errorf("stack not found: %w", err)
	}

	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT ON (service_name) service_name, id, service_order, image_repository, image_tag, container_config
		FROM deployments
		WHERE stack_id = $1
		ORDER BY service_name, created_at DESC
	`, stackID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to load stack services: %w", err)
	}

	type svcHead struct {
		DeploymentID    uuid.UUID
		Order           int
		ImageRepository string
		ImageTag        *string
		ContainerConfig []byte
	}
	heads := map[string]svcHead{}
	for rows.Next() {
		var name string
		var head svcHead
		if err := rows.Scan(&name, &head.DeploymentID, &head.Order, &head.ImageRepository, &head.ImageTag, &head.ContainerConfig); err != nil {
			h.logger.Warn("Failed to scan stack service for webhook redeploy", zap.Error(err))
			continue
		}
		heads[name] = head
	}
	rows.Close()
	if len(heads) == 0 {
		return uuid.Nil, fmt.Errorf("this stack has no deployed services")
	}

	var targets []redeployTarget
	for name, head := range heads {
		var containerConfig *DeploymentContainerConfig
		if len(head.ContainerConfig) > 0 {
			_ = json.Unmarshal(head.ContainerConfig, &containerConfig)
		}
		if containerConfig == nil {
			containerConfig = &DeploymentContainerConfig{}
		}
		containerConfig.PullLatest = true
		containerConfigJSON, _ := json.Marshal(containerConfig)

		imageTag := ""
		if head.ImageTag != nil {
			imageTag = *head.ImageTag
		}

		newID := uuid.New()
		if _, err := h.db.Exec(ctx, `
			INSERT INTO deployments (
				id, org_id, agent_id, service_name, environment,
				image_repository, image_tag,
				stack_id, service_order,
				container_config, replaces_deployment_id,
				status, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'pending',NOW(),NOW())
		`, newID, config.OrgID, config.AgentID, name, config.Environment,
			head.ImageRepository, head.ImageTag, stackID, head.Order,
			containerConfigJSON, head.DeploymentID,
		); err != nil {
			h.logger.Error("Failed to create webhook whole-stack redeploy row", zap.String("service", name), zap.Error(err))
			continue
		}
		targets = append(targets, redeployTarget{
			DeploymentID:    newID,
			ServiceName:     name,
			ImageRepository: head.ImageRepository,
			ImageTag:        imageTag,
			ContainerConfig: containerConfig,
		})
	}
	if len(targets) == 0 {
		return uuid.Nil, fmt.Errorf("failed to initiate redeploy for any service in this stack")
	}

	h.updateStackStatus(ctx, stackID, StackStatusDeploying, fmt.Sprintf("Redeploying %d service(s) via webhook", len(targets)))
	go h.runStackRedeployPipeline(context.Background(), config.OrgID, stackID, targets, skipScanning)

	// webhook_events.deployment_id is one FK slot; link it to a representative row from this
	// batch -- the full set is still queryable by stack_id, this is just a convenience pointer.
	return targets[0].DeploymentID, nil
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
