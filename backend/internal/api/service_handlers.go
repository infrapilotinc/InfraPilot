package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ServiceConfig is the persistent definition of a service.
// A service = service_name + environment on a given agent.
// Deployments are events triggered against a ServiceConfig.
type ServiceConfig struct {
	ID              uuid.UUID                  `json:"id"`
	OrgID           uuid.UUID                  `json:"org_id"`
	AgentID         uuid.UUID                  `json:"agent_id"`
	ServiceName     string                     `json:"service_name"`
	Environment     string                     `json:"environment"`
	ImageRepository string                     `json:"image_repository"`
	ImageTag        string                     `json:"image_tag"`
	ContainerConfig *DeploymentContainerConfig `json:"container_config,omitempty"`
	GitRepo         *string                    `json:"git_repo,omitempty"`
	GitBranch       *string                    `json:"git_branch,omitempty"`
	WebhookID       *uuid.UUID                 `json:"webhook_id,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`

	// Runtime info joined from latest deployment
	Status        *string    `json:"status,omitempty"`
	CurrentTag    *string    `json:"current_tag,omitempty"`
	DeployedAt    *time.Time `json:"deployed_at,omitempty"`
	ContainerName *string    `json:"container_name,omitempty"`
	ContainerID   *string    `json:"container_id,omitempty"`
}

type UpsertServiceRequest struct {
	AgentID         uuid.UUID                  `json:"agent_id" binding:"required"`
	ServiceName     string                     `json:"service_name" binding:"required"`
	Environment     string                     `json:"environment" binding:"required,oneof=dev staging prod"`
	ImageRepository string                     `json:"image_repository" binding:"required"`
	ImageTag        string                     `json:"image_tag"`
	ContainerConfig *DeploymentContainerConfig `json:"container_config,omitempty"`
	GitRepo         *string                    `json:"git_repo,omitempty"`
	WebhookID       *uuid.UUID                 `json:"webhook_id,omitempty"`
}

type ServiceDeployRequest struct {
	// Optional: if service doesn't exist yet, these auto-create it
	ImageRepository string     `json:"image_repository,omitempty"`
	AgentID         *uuid.UUID `json:"agent_id,omitempty"`
	// Optional environment override (fallback if ?env not in query)
	Environment string `json:"environment,omitempty"`
	// Optional: defaults to service's configured tag if omitted
	ImageTag    string  `json:"image_tag,omitempty"`
	ImageDigest *string `json:"image_digest,omitempty"`
	GitBranch   *string `json:"git_branch,omitempty"`
	GitCommit   *string `json:"git_commit,omitempty"`
	CIProvider  *string `json:"ci_provider,omitempty"`
	CIBuildURL  *string `json:"ci_build_url,omitempty"`
}

// listServices returns all service configs for the org, with latest deployment status joined.
func (h *Handler) listServices(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT
			s.id, s.org_id, s.agent_id, s.service_name, s.environment,
			s.image_repository, s.image_tag, s.container_config,
			s.git_repo, s.git_branch, s.webhook_id, s.created_at, s.updated_at,
			d.status, d.image_tag, d.deployed_at, d.container_name, d.container_id
		FROM service_configs s
		LEFT JOIN LATERAL (
			SELECT status, image_tag, deployed_at, container_name, container_id
			FROM deployments
			WHERE agent_id = s.agent_id
			  AND service_name = s.service_name
			  AND environment = s.environment
			ORDER BY created_at DESC LIMIT 1
		) d ON true
		WHERE s.org_id = $1
		ORDER BY s.service_name, s.environment
	`, orgID)
	if err != nil {
		h.logger.Error("failed to list services", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list services"})
		return
	}
	defer rows.Close()

	services := []ServiceConfig{}
	for rows.Next() {
		var s ServiceConfig
		var cfgJSON []byte
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.AgentID, &s.ServiceName, &s.Environment,
			&s.ImageRepository, &s.ImageTag, &cfgJSON,
			&s.GitRepo, &s.GitBranch, &s.WebhookID, &s.CreatedAt, &s.UpdatedAt,
			&s.Status, &s.CurrentTag, &s.DeployedAt, &s.ContainerName, &s.ContainerID,
		); err != nil {
			h.logger.Error("failed to scan service", zap.Error(err))
			continue
		}
		if len(cfgJSON) > 0 {
			var cfg DeploymentContainerConfig
			if err := json.Unmarshal(cfgJSON, &cfg); err == nil {
				s.ContainerConfig = &cfg
			}
		}
		services = append(services, s)
	}

	c.JSON(http.StatusOK, services)
}

// getService returns a single service config with latest deployment status.
func (h *Handler) getService(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")
	environment := c.DefaultQuery("env", "prod")

	var s ServiceConfig
	var cfgJSON []byte
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT
			s.id, s.org_id, s.agent_id, s.service_name, s.environment,
			s.image_repository, s.image_tag, s.container_config,
			s.git_repo, s.git_branch, s.webhook_id, s.created_at, s.updated_at,
			d.status, d.image_tag, d.deployed_at, d.container_name, d.container_id
		FROM service_configs s
		LEFT JOIN LATERAL (
			SELECT status, image_tag, deployed_at, container_name, container_id
			FROM deployments
			WHERE agent_id = s.agent_id
			  AND service_name = s.service_name
			  AND environment = s.environment
			ORDER BY created_at DESC LIMIT 1
		) d ON true
		WHERE s.org_id = $1 AND s.service_name = $2 AND s.environment = $3
	`, orgID, serviceName, environment).Scan(
		&s.ID, &s.OrgID, &s.AgentID, &s.ServiceName, &s.Environment,
		&s.ImageRepository, &s.ImageTag, &cfgJSON,
		&s.GitRepo, &s.WebhookID, &s.CreatedAt, &s.UpdatedAt,
		&s.Status, &s.CurrentTag, &s.DeployedAt, &s.ContainerName, &s.ContainerID,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}
	if len(cfgJSON) > 0 {
		var cfg DeploymentContainerConfig
		if err := json.Unmarshal(cfgJSON, &cfg); err == nil {
			s.ContainerConfig = &cfg
		}
	}

	c.JSON(http.StatusOK, s)
}

// upsertService creates or updates a service config.
// PUT /api/v1/services — idempotent, same call for create and update.
func (h *Handler) upsertService(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req UpsertServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}

	cfgJSON, err := json.Marshal(req.ContainerConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container_config"})
		return
	}

	var id uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO service_configs
			(org_id, agent_id, service_name, environment, image_repository, image_tag, container_config, git_repo, webhook_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (org_id, agent_id, service_name, environment)
		DO UPDATE SET
			image_repository = EXCLUDED.image_repository,
			image_tag        = EXCLUDED.image_tag,
			container_config = EXCLUDED.container_config,
			git_repo         = EXCLUDED.git_repo,
			webhook_id       = EXCLUDED.webhook_id,
			updated_at       = NOW()
		RETURNING id
	`, orgID, req.AgentID, req.ServiceName, req.Environment,
		req.ImageRepository, req.ImageTag, cfgJSON, req.GitRepo, req.WebhookID,
	).Scan(&id)
	if err != nil {
		h.logger.Error("failed to upsert service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save service"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "message": "service saved"})
}

// deleteService removes a service config.
func (h *Handler) deleteService(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")
	environment := c.DefaultQuery("env", "prod")

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM service_configs WHERE org_id = $1 AND service_name = $2 AND environment = $3
	`, orgID, serviceName, environment)
	if err != nil {
		h.logger.Error("failed to delete service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete service"})
		return
	}
	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "service deleted"})
}

// deployService triggers a deployment for a named service.
// This is the single canonical deploy path used by CLI, webhook, and UI.
// POST /api/v1/services/:name/deploy?env=dev
// If the service doesn't exist yet, it is auto-created when image_repository + agent_id are provided.
func (h *Handler) deployService(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")

	var req ServiceDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Environment: query param takes precedence, fall back to body field
	environment := c.Query("env")
	if environment == "" {
		environment = req.Environment
	}
	if environment == "" {
		environment = "prod"
	}

	// Load service config
	var svc ServiceConfig
	var cfgJSON []byte
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, image_repository, image_tag, container_config, git_repo, git_branch
		FROM service_configs
		WHERE org_id = $1 AND service_name = $2 AND environment = $3
	`, orgID, serviceName, environment).Scan(&svc.ID, &svc.AgentID, &svc.ImageRepository, &svc.ImageTag, &cfgJSON, &svc.GitRepo, &svc.GitBranch)
	if err != nil {
		// Auto-create if caller supplied enough info
		if req.ImageRepository == "" || req.AgentID == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf(
				"service %q (%s) not found — create it first or pass image_repository + agent_id to auto-create",
				serviceName, environment,
			)})
			return
		}

		tag := req.ImageTag
		if tag == "" {
			tag = "latest"
		}
		var newID uuid.UUID
		createErr := h.db.QueryRow(c.Request.Context(), `
			INSERT INTO service_configs
				(org_id, agent_id, service_name, environment, image_repository, image_tag, container_config, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, '{}', NOW())
			RETURNING id
		`, orgID, *req.AgentID, serviceName, environment, req.ImageRepository, tag).Scan(&newID)
		if createErr != nil {
			h.logger.Error("failed to auto-create service config", zap.Error(createErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create service"})
			return
		}
		svc.ID = newID
		svc.AgentID = *req.AgentID
		svc.ImageRepository = req.ImageRepository
		svc.ImageTag = tag
		cfgJSON = []byte("{}")
	}

	var containerConfig *DeploymentContainerConfig
	if len(cfgJSON) > 0 {
		var cfg DeploymentContainerConfig
		if err := json.Unmarshal(cfgJSON, &cfg); err == nil {
			containerConfig = &cfg
		}
	}

	// Use request tag if provided, else fall back to service's configured tag
	imageTag := req.ImageTag
	if imageTag == "" {
		imageTag = svc.ImageTag
	}
	if imageTag == "" {
		imageTag = "latest"
	}
	imageDigest := ""
	if req.ImageDigest != nil {
		imageDigest = *req.ImageDigest
	}

	// A git-backed service redeploys by rebuilding from source (push-to-deploy),
	// not by re-pulling an image. The build stage overwrites image_repository.
	isGitService := svc.GitRepo != nil && *svc.GitRepo != ""

	// Branch for the rebuild: request override, else the service's stored branch.
	gitBranch := req.GitBranch
	if isGitService && (gitBranch == nil || *gitBranch == "") {
		gitBranch = svc.GitBranch
	}
	var gitRepo *string
	if isGitService {
		gitRepo = svc.GitRepo
	}

	// Insert deployment record
	deploymentID := uuid.New()
	cfgBytes, _ := json.Marshal(containerConfig)
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO deployments (
			id, org_id, agent_id, service_name, environment,
			image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit, ci_provider, ci_build_url,
			container_config, service_config_id,
			status, policy_decision, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'pending','allow',NOW(),NOW())
	`,
		deploymentID, orgID, svc.AgentID, serviceName, environment,
		svc.ImageRepository, imageTag, imageDigest,
		gitRepo, gitBranch, req.GitCommit, req.CIProvider, req.CIBuildURL,
		cfgBytes, svc.ID,
	)
	if err != nil {
		h.logger.Error("failed to create deployment for service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create deployment"})
		return
	}

	// Image deploys advance the service's current tag; git rebuilds keep the
	// source as the source of truth (the build stamps its own tag).
	if !isGitService {
		_, _ = h.db.Exec(c.Request.Context(), `
			UPDATE service_configs SET image_tag = $1, updated_at = NOW()
			WHERE id = $2
		`, imageTag, svc.ID)
	}

	// Run the deployment pipeline (must use Background context — request context
	// is cancelled as soon as the HTTP response is sent). Git services rebuild
	// from source; image services skip straight to deploy (CE has no scan stage).
	go h.runDeploymentPipeline(
		context.Background(), orgID, deploymentID,
		svc.ImageRepository, imageTag, imageDigest,
		containerConfig, true, isGitService, // skipScanning=true for CE, sourceBuild=isGitService
	)

	h.logger.Info("service deploy triggered",
		zap.String("service", serviceName),
		zap.String("env", environment),
		zap.String("tag", imageTag),
		zap.String("deployment_id", deploymentID.String()),
	)

	c.JSON(http.StatusAccepted, gin.H{
		"deployment_id": deploymentID,
		"service":       serviceName,
		"environment":   environment,
		"image":         fmt.Sprintf("%s:%s", svc.ImageRepository, imageTag),
		"message":       "deployment started",
	})
}

// rollbackService rolls back a service to its previous deployment.
func (h *Handler) rollbackService(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")
	environment := c.Query("env")
	if environment == "" {
		// also accept env from JSON body
		var body struct {
			Environment string `json:"environment"`
		}
		if err := c.ShouldBindJSON(&body); err == nil && body.Environment != "" {
			environment = body.Environment
		}
	}
	if environment == "" {
		environment = "prod"
	}

	// Find the previous successful deployment
	var prevID uuid.UUID
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id FROM deployments
		WHERE org_id = $1 AND service_name = $2 AND environment = $3 AND status = 'running'
		ORDER BY deployed_at DESC
		LIMIT 1 OFFSET 1
	`, orgID, serviceName, environment).Scan(&prevID, &agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no previous deployment to roll back to"})
		return
	}

	// Delegate to existing rollback handler logic
	c.Params = append(c.Params, gin.Param{Key: "id", Value: agentID.String()})
	c.Params = append(c.Params, gin.Param{Key: "did", Value: prevID.String()})
	h.rollbackDeployment(c)
}
