package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	agentgrpc "github.com/infrapilot/backend/internal/grpc"
)

// ==================== Types ====================

type DeploymentStatus string
type PolicyDecision string

const (
	StatusPending     DeploymentStatus = "pending"
	StatusBuilding    DeploymentStatus = "building"
	StatusScanning    DeploymentStatus = "scanning"
	StatusPolicyCheck DeploymentStatus = "policy_check"
	StatusDeploying   DeploymentStatus = "deploying"
	StatusRunning     DeploymentStatus = "running"
	StatusFailed      DeploymentStatus = "failed"
	StatusRolledBack  DeploymentStatus = "rolled_back"
	StatusStopped     DeploymentStatus = "stopped"

	DecisionAllow PolicyDecision = "allow"
	DecisionWarn  PolicyDecision = "warn"
	DecisionDeny  PolicyDecision = "deny"
)

type Deployment struct {
	ID               uuid.UUID        `json:"id"`
	OrgID            uuid.UUID        `json:"org_id"`
	AgentID          uuid.UUID        `json:"agent_id"`
	ServiceName      string           `json:"service_name"`
	Environment      string           `json:"environment"`
	ImageRegistry    *string          `json:"image_registry,omitempty"`
	ImageRepository  string           `json:"image_repository"`
	ImageTag         *string          `json:"image_tag,omitempty"`
	ImageDigest      *string          `json:"image_digest,omitempty"`
	GitRepo          *string          `json:"git_repo,omitempty"`
	GitBranch        *string          `json:"git_branch,omitempty"`
	GitCommit        *string          `json:"git_commit,omitempty"`
	CIProvider       *string          `json:"ci_provider,omitempty"`
	CIPipelineID     *string          `json:"ci_pipeline_id,omitempty"`
	CIBuildURL       *string          `json:"ci_build_url,omitempty"`
	ScanResultID     *uuid.UUID       `json:"scan_result_id,omitempty"`
	SBOMID           *uuid.UUID       `json:"sbom_id,omitempty"`
	PolicyDecision   PolicyDecision   `json:"policy_decision"`
	PolicyReason     *string          `json:"policy_reason,omitempty"`
	Status           DeploymentStatus `json:"status"`
	StatusMessage    *string          `json:"status_message,omitempty"`
	ContainerID      *string          `json:"container_id,omitempty"`
	ContainerName    *string          `json:"container_name,omitempty"`
	ProxyHostID      *uuid.UUID       `json:"proxy_host_id,omitempty"`
	StackID          *uuid.UUID       `json:"stack_id,omitempty"`
	ServiceOrder     int              `json:"service_order,omitempty"`
	DeployedBy       *uuid.UUID       `json:"deployed_by,omitempty"`
	DeployedAt       *time.Time       `json:"deployed_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// ContainerConfigSecret represents a secret to be mounted in the container
type ContainerConfigSecret struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	MountPath string `json:"mount_path,omitempty"`
}

// ContainerConfigNetwork represents a network configuration
type ContainerConfigNetwork struct {
	NetworkID       *string  `json:"network_id,omitempty"`
	NetworkName     *string  `json:"network_name,omitempty"`
	CreateIfMissing bool     `json:"create_if_missing,omitempty"`
	Driver          string   `json:"driver,omitempty"`
	Aliases         []string `json:"aliases,omitempty"`
}

// ContainerConfigVolume represents a volume configuration
type ContainerConfigVolume struct {
	VolumeName      *string `json:"volume_name,omitempty"`
	MountPath       string  `json:"mount_path"`
	CreateIfMissing bool    `json:"create_if_missing,omitempty"`
	HostPath        *string `json:"host_path,omitempty"`
	ReadOnly        bool    `json:"read_only,omitempty"`
	// Stack companion files (v3/45): when Content is non-nil, HostPath is a stack-root path the
	// agent materializes from Content (with FileMode) before mounting — lets a pasted-compose stack
	// ship the files it bind-mounts. Never persisted with the compose.
	Content  *string `json:"content,omitempty"`
	FileMode string  `json:"file_mode,omitempty"`
}

// DeploymentContainerConfig contains the extended container configuration for deployments
type DeploymentContainerConfig struct {
	ContainerName string                   `json:"container_name,omitempty"`
	EnvVars       map[string]string        `json:"env_vars,omitempty"`
	Secrets       []ContainerConfigSecret  `json:"secrets,omitempty"`
	SecretMethod  string                   `json:"secret_method,omitempty"` // "env_vars" or "docker_secrets"
	Networks      []ContainerConfigNetwork `json:"networks,omitempty"`
	Volumes       []ContainerConfigVolume  `json:"volumes,omitempty"`
	Labels        map[string]string        `json:"labels,omitempty"`
	Command       []string                 `json:"command,omitempty"`
	PullLatest    bool                     `json:"pull_latest,omitempty"` // Whether to pull latest image before deployment
}

type CreateDeploymentRequest struct {
	ServiceName     string           `json:"service_name" binding:"required"`
	Environment     string           `json:"environment" binding:"required,oneof=dev staging prod"`
	ImageRepository string           `json:"image_repository"` // optional: empty + git_repo set ⇒ source build (push-to-deploy)
	ImageTag        *string          `json:"image_tag"`
	ImageDigest     *string          `json:"image_digest"`
	GitRepo         *string          `json:"git_repo"`
	GitBranch       *string          `json:"git_branch"`
	GitCommit       *string          `json:"git_commit"`
	GitPRNumber     *int             `json:"git_pr_number"`
	CIProvider      *string          `json:"ci_provider"`
	CIPipelineID    *string          `json:"ci_pipeline_id"`
	CIBuildURL      *string          `json:"ci_build_url"`
	ContainerConfig *DeploymentContainerConfig `json:"container_config,omitempty"`
}

// ==================== List Deployments ====================

func (h *Handler) listDeployments(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")

	// Parse query parameters for filtering
	service := c.Query("service")
	environment := c.Query("environment")
	status := c.Query("status")

	query := `
		SELECT id, org_id, agent_id, service_name, environment,
		       image_registry, image_repository, image_tag, image_digest,
		       git_repo, git_branch, git_commit,
		       ci_provider, ci_pipeline_id, ci_build_url,
		       scan_result_id, sbom_id,
		       policy_decision, policy_reason,
		       status, status_message,
		       container_id, container_name, proxy_host_id,
		       stack_id, service_order,
		       deployed_by, deployed_at, created_at, updated_at
		FROM deployments
		WHERE org_id = $1 AND agent_id = $2
	`
	args := []interface{}{orgID, agentID}
	argIdx := 3

	if service != "" {
		query += fmt.Sprintf(" AND service_name = $%d", argIdx)
		args = append(args, service)
		argIdx++
	}
	if environment != "" {
		query += fmt.Sprintf(" AND environment = $%d", argIdx)
		args = append(args, environment)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list deployments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
		return
	}
	defer rows.Close()

	deployments := []Deployment{}
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.AgentID, &d.ServiceName, &d.Environment,
			&d.ImageRegistry, &d.ImageRepository, &d.ImageTag, &d.ImageDigest,
			&d.GitRepo, &d.GitBranch, &d.GitCommit,
			&d.CIProvider, &d.CIPipelineID, &d.CIBuildURL,
			&d.ScanResultID, &d.SBOMID,
			&d.PolicyDecision, &d.PolicyReason,
			&d.Status, &d.StatusMessage,
			&d.ContainerID, &d.ContainerName, &d.ProxyHostID,
			&d.StackID, &d.ServiceOrder,
			&d.DeployedBy, &d.DeployedAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			h.logger.Warn("Failed to scan deployment", zap.Error(err))
			continue
		}
		deployments = append(deployments, d)
	}

	c.JSON(http.StatusOK, deployments)
}

// ==================== Get Deployment ====================

func (h *Handler) getDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")
	deploymentID := c.Param("did")

	var d Deployment
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, agent_id, service_name, environment,
		       image_registry, image_repository, image_tag, image_digest,
		       git_repo, git_branch, git_commit,
		       ci_provider, ci_pipeline_id, ci_build_url,
		       scan_result_id, sbom_id,
		       policy_decision, policy_reason,
		       status, status_message,
		       container_id, container_name, proxy_host_id,
		       stack_id, service_order,
		       deployed_by, deployed_at, created_at, updated_at
		FROM deployments
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, deploymentID, orgID, agentID).Scan(
		&d.ID, &d.OrgID, &d.AgentID, &d.ServiceName, &d.Environment,
		&d.ImageRegistry, &d.ImageRepository, &d.ImageTag, &d.ImageDigest,
		&d.GitRepo, &d.GitBranch, &d.GitCommit,
		&d.CIProvider, &d.CIPipelineID, &d.CIBuildURL,
		&d.ScanResultID, &d.SBOMID,
		&d.PolicyDecision, &d.PolicyReason,
		&d.Status, &d.StatusMessage,
		&d.ContainerID, &d.ContainerName, &d.ProxyHostID,
		&d.StackID, &d.ServiceOrder,
		&d.DeployedBy, &d.DeployedAt, &d.CreatedAt, &d.UpdatedAt,
	)

	if err != nil {
		h.logger.Error("Failed to get deployment", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	c.JSON(http.StatusOK, d)
}

// ==================== Create Deployment ====================

func (h *Handler) createDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentID := c.Param("id")

	var req CreateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify agent exists and belongs to org
	var exists bool
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Source build (push-to-deploy): no prebuilt image, build from git on the agent.
	sourceBuild := req.ImageRepository == "" && req.GitRepo != nil && *req.GitRepo != ""
	if req.ImageRepository == "" && !sourceBuild {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_repository or git_repo is required"})
		return
	}
	imageRepoForInsert := req.ImageRepository
	if sourceBuild {
		// Placeholder satisfies the NOT NULL column; the build stage overwrites it.
		imageRepoForInsert = "infrapilot-build/" + sanitizeImageName(req.ServiceName+"-"+req.Environment)
	}

	// Serialize container config to JSON if provided
	var containerConfigJSON []byte
	if req.ContainerConfig != nil {
		var err error
		containerConfigJSON, err = h.marshalContainerConfig(req.ContainerConfig)
		if err != nil {
			h.logger.Error("Failed to marshal container config", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container config"})
			return
		}
	}

	var deploymentID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO deployments (
			org_id, agent_id, service_name, environment,
			image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit, git_pr_number,
			ci_provider, ci_pipeline_id, ci_build_url,
			deployed_by, status, container_config
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, 'pending', $16)
		RETURNING id
	`, orgID, agentID, req.ServiceName, req.Environment,
		imageRepoForInsert, req.ImageTag, req.ImageDigest,
		req.GitRepo, req.GitBranch, req.GitCommit, req.GitPRNumber,
		req.CIProvider, req.CIPipelineID, req.CIBuildURL,
		userID, containerConfigJSON,
	).Scan(&deploymentID)

	if err != nil {
		h.logger.Error("Failed to create deployment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create deployment"})
		return
	}

	h.logger.Info("Deployment created",
		zap.String("deployment_id", deploymentID.String()),
		zap.String("service", req.ServiceName),
		zap.String("environment", req.Environment),
	)

	// Funnel telemetry (v3/40): the activation milestone. EmitOnce fires only on the
	// instance's first-ever deploy; carries no app/repo details (privacy contract).
	h.telemetry.EmitOnce("first_deploy", nil)

	// Register/refresh the service config so this deployment shows up as an app
	// (the Apps list reads service_configs) and can be redeployed later. For source
	// builds we persist git_repo/git_branch so "redeploy" rebuilds from source.
	svcConfigJSON := containerConfigJSON
	if len(svcConfigJSON) == 0 {
		svcConfigJSON = []byte("{}")
	}
	svcImageTag := "latest"
	if req.ImageTag != nil && *req.ImageTag != "" {
		svcImageTag = *req.ImageTag
	}
	if _, sErr := h.db.Exec(c.Request.Context(), `
		INSERT INTO service_configs
			(org_id, agent_id, service_name, environment, image_repository, image_tag, container_config, git_repo, git_branch, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (org_id, agent_id, service_name, environment) DO UPDATE SET
			image_repository = EXCLUDED.image_repository,
			image_tag        = EXCLUDED.image_tag,
			git_repo         = EXCLUDED.git_repo,
			git_branch       = EXCLUDED.git_branch,
			updated_at       = NOW()
	`, orgID, agentID, req.ServiceName, req.Environment,
		imageRepoForInsert, svcImageTag, svcConfigJSON, req.GitRepo, req.GitBranch,
	); sErr != nil {
		h.logger.Warn("failed to upsert service config for deployment", zap.Error(sErr))
	}

	// Trigger deployment pipeline asynchronously
	imageTag := ""
	if req.ImageTag != nil {
		imageTag = *req.ImageTag
	}
	imageDigest := ""
	if req.ImageDigest != nil {
		imageDigest = *req.ImageDigest
	}
	// Use background context for async pipeline (request context will be canceled).
	// CE has no scanning stage, so image deploys skip straight to deploy; source
	// builds populate their own details during the build phase.
	go h.runDeploymentPipeline(context.Background(), orgID, deploymentID, imageRepoForInsert, imageTag, imageDigest, req.ContainerConfig, true, sourceBuild)

	c.JSON(http.StatusCreated, gin.H{
		"id":      deploymentID,
		"status":  "pending",
		"message": "Deployment created and scanning initiated",
	})
}

// ==================== Deployment Pipeline ====================

func (h *Handler) runDeploymentPipeline(ctx context.Context, orgID, deploymentID uuid.UUID, imageRepo, imageTag, imageDigest string, containerConfig *DeploymentContainerConfig, skipScanning bool, sourceBuild bool) {
	logger := h.logger.With(
		zap.String("deployment_id", deploymentID.String()),
		zap.String("org_id", orgID.String()),
		zap.Bool("skip_scanning", skipScanning),
		zap.Bool("source_build", sourceBuild),
	)

	// Build full image reference
	imageRef := imageRepo
	if imageTag != "" {
		imageRef = imageRepo + ":" + imageTag
	} else if imageDigest != "" {
		imageRef = imageRepo + "@" + imageDigest
	}

	logger.Info("Starting deployment pipeline", zap.String("image", imageRef))

	// Declare variables that will be used in both branches
	var policyDecision string
	var policyReason string
	var deployment struct {
		ServiceName  string
		Environment  string
		ImageRepo    string
		ImageTag     string
		ImageDigest  string
		GitBranch    string
		GitCommit    string
	}

	// Phase 0: Source build (push-to-deploy). Clone + build an image on the agent
	// from git, then run it locally (no registry). In CE this is ungated; the
	// SAST / code-quality gates that can block a build are Enterprise-only.
	if sourceBuild {
		var repoURL, ref, commit string
		var agentID uuid.UUID
		err := h.db.QueryRow(ctx, `
			SELECT service_name, environment, COALESCE(git_repo, ''),
			       COALESCE(git_branch, ''), COALESCE(git_commit, ''), agent_id
			FROM deployments WHERE id = $1
		`, deploymentID).Scan(
			&deployment.ServiceName, &deployment.Environment,
			&repoURL, &ref, &commit, &agentID,
		)
		if err != nil || repoURL == "" {
			h.updateDeploymentStatus(ctx, deploymentID.String(), "failed", "Source build requires a git repository")
			return
		}

		if err := h.updateDeploymentStatus(ctx, deploymentID.String(), "building", "Building image from source"); err != nil {
			logger.Error("Failed to set building status", zap.Error(err))
		}

		commit8 := commit
		if len(commit8) > 8 {
			commit8 = commit8[:8]
		}
		if commit8 == "" {
			commit8 = "latest"
		}
		repoName := "infrapilot-build/" + sanitizeImageName(deployment.ServiceName+"-"+deployment.Environment)
		builtTag := repoName + ":" + commit8

		gitToken := h.resolveGitToken(ctx, orgID, repoURL)
		br, berr := h.buildOnAgent(ctx, agentID, repoURL, ref, commit, builtTag, "", gitToken, nil)
		if br != nil {
			h.db.Exec(ctx, `UPDATE deployments SET build_log = $1 WHERE id = $2`, br.Log, deploymentID)
		}
		if berr != nil || br == nil || !br.Success {
			msg := "build failed"
			if br != nil && br.Message != "" {
				msg = br.Message
			} else if berr != nil {
				msg = berr.Error()
			}
			logger.Error("Source build failed", zap.String("error", msg))
			h.updateDeploymentStatus(ctx, deploymentID.String(), "failed", "Build failed: "+msg)
			return
		}

		// Point the deployment at the locally-built image and run it without pulling.
		h.db.Exec(ctx, `UPDATE deployments SET image_repository = $1, image_tag = $2 WHERE id = $3`,
			repoName, commit8, deploymentID)
		imageRef = builtTag
		if containerConfig == nil {
			containerConfig = &DeploymentContainerConfig{}
		}
		containerConfig.PullLatest = false

		if err := h.updateDeploymentStatus(ctx, deploymentID.String(), "deploying", "Starting container"); err != nil {
			logger.Error("Failed to set deploying status", zap.Error(err))
		}
		logger.Info("Source build complete", zap.String("image", builtTag))
		// Fall through to the deploy step below (policyDecision stays "" → not denied).
	}

	if skipScanning {
		// Skip scanning - go directly to deployment
		logger.Info("Skipping security scanning (skip_scanning=true)")

		// Fetch deployment details (needed for container deployment)
		err := h.db.QueryRow(ctx, `
			SELECT service_name, environment, image_repository,
			       COALESCE(image_tag, ''), COALESCE(image_digest, ''),
			       COALESCE(git_branch, ''), COALESCE(git_commit, '')
			FROM deployments WHERE id = $1
		`, deploymentID).Scan(
			&deployment.ServiceName, &deployment.Environment, &deployment.ImageRepo,
			&deployment.ImageTag, &deployment.ImageDigest,
			&deployment.GitBranch, &deployment.GitCommit,
		)
		if err != nil {
			logger.Error("Failed to fetch deployment details", zap.Error(err))
			h.updateDeploymentStatus(ctx, deploymentID.String(), "failed", "Failed to fetch deployment details")
			return
		}

		if err := h.updateDeploymentStatus(ctx, deploymentID.String(), "deploying", "Deploying container (scanning skipped)"); err != nil {
			logger.Error("Failed to update deployment status", zap.Error(err))
			return
		}
		policyDecision = "allow"
	} // CE: scanning not available in community edition

	// Phase 4: Deploy container to agent
	if policyDecision != "deny" {
		// Get the agent ID for this deployment
		var agentID uuid.UUID
		err := h.db.QueryRow(ctx, `SELECT agent_id FROM deployments WHERE id = $1`, deploymentID).Scan(&agentID)
		if err != nil {
			logger.Error("Failed to get agent ID for deployment", zap.Error(err))
			h.updateDeploymentStatus(ctx, deploymentID.String(), "failed", "Failed to get agent ID")
			return
		}

		containerResult, err := h.deployContainerToAgent(ctx, orgID, deploymentID, agentID, imageRef, deployment.ServiceName, deployment.Environment, containerConfig)
		if err != nil {
			h.updateDeploymentStatus(ctx, deploymentID.String(), "failed",
				fmt.Sprintf("Container deployment failed: %v", err))
			return
		}

		// Update deployment with container info
		_, err = h.db.Exec(ctx, `
			UPDATE deployments
			SET status = 'running',
			    container_id = $1,
			    container_name = $2,
			    deployed_at = NOW(),
			    status_message = 'Container running successfully'
			WHERE id = $3`,
			containerResult.ContainerID,
			containerResult.Name,
			deploymentID,
		)
		if err != nil {
			logger.Error("Failed to update deployment with container info", zap.Error(err))
		}

		logger.Info("Container deployed successfully",
			zap.String("container_id", containerResult.ContainerID),
			zap.String("container_name", containerResult.Name),
		)
	} else {
		h.updateDeploymentStatus(ctx, deploymentID.String(), "failed",
			fmt.Sprintf("Blocked by policy: %s", policyReason))
	}
}

// deployContainerToAgent sends a command to the agent to run a container
func (h *Handler) deployContainerToAgent(ctx context.Context, orgID, deploymentID, agentID uuid.UUID, imageRef, serviceName, environment string, containerConfig *DeploymentContainerConfig) (*agentgrpc.ContainerRunResult, error) {
	// Use container_name from compose if provided, otherwise generate one
	containerName := ""
	if containerConfig != nil && containerConfig.ContainerName != "" {
		containerName = containerConfig.ContainerName
	} else {
		containerName = fmt.Sprintf("%s-%s-%s", serviceName, environment, deploymentID.String()[:8])
	}

	// Build the run command options
	labels := map[string]interface{}{
		"infrapilot.deployment_id": deploymentID.String(),
		"infrapilot.service":       serviceName,
		"infrapilot.environment":   environment,
	}

	// Merge custom labels from container config (e.g., compose project labels)
	if containerConfig != nil && len(containerConfig.Labels) > 0 {
		for k, v := range containerConfig.Labels {
			labels[k] = v
		}
	}

	options := map[string]interface{}{
		"image_ref":      imageRef,
		"name":           containerName,
		"restart_policy": "unless-stopped",
		"labels":         labels,
	}

	// Add container configuration if provided
	if containerConfig != nil {
		// Decrypt any at-rest-encrypted secret values before they're injected. This is the
		// single dispatch funnel for secrets, and decrypt is a no-op on plaintext (fresh
		// requests) and legacy unencrypted rows — so it safely covers every path. See
		// secrets_crypto.go.
		h.decryptSecretValues(containerConfig.Secrets)
		h.decryptEnvVarValues(containerConfig.EnvVars)

		// Add environment variables
		if len(containerConfig.EnvVars) > 0 && containerConfig.SecretMethod != "docker_secrets" {
			options["env"] = containerConfig.EnvVars
		}

		// Add secrets configuration
		if len(containerConfig.Secrets) > 0 {
			secretsOpts := make([]map[string]interface{}, len(containerConfig.Secrets))
			for i, s := range containerConfig.Secrets {
				secretsOpts[i] = map[string]interface{}{
					"name":       s.Name,
					"value":      s.Value,
					"mount_path": s.MountPath,
				}
			}
			options["secrets"] = secretsOpts
			options["secret_method"] = containerConfig.SecretMethod
		}

		// Add networks configuration
		if len(containerConfig.Networks) > 0 {
			networksOpts := make([]map[string]interface{}, len(containerConfig.Networks))
			for i, n := range containerConfig.Networks {
				netOpt := map[string]interface{}{
					"create_if_missing": n.CreateIfMissing,
				}
				if n.NetworkID != nil {
					netOpt["network_id"] = *n.NetworkID
				}
				if n.NetworkName != nil {
					netOpt["network_name"] = *n.NetworkName
				}
				if n.Driver != "" {
					netOpt["driver"] = n.Driver
				}
				if len(n.Aliases) > 0 {
					netOpt["aliases"] = n.Aliases
				}
				networksOpts[i] = netOpt
			}
			options["networks"] = networksOpts
		}

		// Add volumes configuration
		if len(containerConfig.Volumes) > 0 {
			volumesOpts := make([]map[string]interface{}, len(containerConfig.Volumes))
			for i, v := range containerConfig.Volumes {
				volOpt := map[string]interface{}{
					"mount_path":        v.MountPath,
					"create_if_missing": v.CreateIfMissing,
					"read_only":         v.ReadOnly,
				}
				if v.VolumeName != nil {
					volOpt["volume_name"] = *v.VolumeName
				}
				if v.HostPath != nil {
					volOpt["host_path"] = *v.HostPath
				}
				// Stack companion file (v3/45): ship content + mode so the agent materializes the
				// bind source on the host before mounting.
				if v.Content != nil {
					volOpt["content"] = *v.Content
					if v.FileMode != "" {
						volOpt["file_mode"] = v.FileMode
					}
				}
				volumesOpts[i] = volOpt
			}
			options["volumes"] = volumesOpts
		}

		// Add command
		if len(containerConfig.Command) > 0 {
			options["command"] = containerConfig.Command
		}

		// Add pull_latest flag
		options["pull_latest"] = containerConfig.PullLatest
	}

	// Inject registry auth credentials if a matching registry is configured
	if h.registryService != nil {
		if reg, err := h.registryService.GetRegistryByImageRef(ctx, orgID, imageRef); err == nil {
			if creds, err := h.registryService.GetDecryptedCredentials(ctx, orgID, reg.ID); err == nil {
				authMap := map[string]string{}
				namespace := ""
				if reg.Namespace != nil {
					namespace = *reg.Namespace
				}
				switch reg.Provider {
				case "ghcr":
					authMap["username"] = namespace
					authMap["password"] = creds.Token
					authMap["server_address"] = "ghcr.io"
				case "dockerhub":
					authMap["username"] = creds.Username
					authMap["password"] = creds.Password
					authMap["server_address"] = "https://index.docker.io/v1/"
				case "gcr":
					authMap["username"] = "_json_key"
					authMap["password"] = creds.Token
					authMap["server_address"] = "gcr.io"
				case "ecr", "acr":
					authMap["username"] = creds.Username
					authMap["password"] = creds.Password
					if namespace != "" {
						authMap["server_address"] = namespace
					}
				}
				if len(authMap) > 0 {
					options["auth"] = authMap
				}
			}
		}
	}

	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal options: %w", err)
	}

	// Create the command
	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "docker",
		Command:   json.RawMessage(fmt.Sprintf(`{"action":"run_container","options":%s}`, optionsJSON)),
	}

	// Send command to agent
	resp, err := agentgrpc.SendCommand(agentID.String(), cmd, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to send command to agent: %w", err)
	}

	// Parse response
	result, err := resp.GetCommandResult()
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("agent returned error: %s", result.Message)
	}

	// Parse container result
	var containerResult agentgrpc.ContainerRunResult
	if err := json.Unmarshal(result.Data, &containerResult); err != nil {
		return nil, fmt.Errorf("failed to parse container result: %w", err)
	}

	return &containerResult, nil
}

// ==================== Rollback Deployment ====================

func (h *Handler) rollbackDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentID := c.Param("id")
	deploymentID := c.Param("did")

	// Get deployment to rollback
	var serviceName, environment string
	var replacesDeploymentID *uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT service_name, environment, replaces_deployment_id
		FROM deployments
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, deploymentID, orgID, agentID).Scan(&serviceName, &environment, &replacesDeploymentID)

	if err != nil {
		h.logger.Error("Deployment not found for rollback", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	if replacesDeploymentID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no previous deployment to rollback to"})
		return
	}

	// Create rollback deployment (copy from previous deployment)
	var newDeploymentID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO deployments (
			org_id, agent_id, service_name, environment,
			image_registry, image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit,
			rollback_of_deployment_id, deployed_by, status
		)
		SELECT
			org_id, agent_id, service_name, environment,
			image_registry, image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit,
			$1, $2, 'pending'
		FROM deployments
		WHERE id = $3
		RETURNING id
	`, deploymentID, userID, replacesDeploymentID).Scan(&newDeploymentID)

	if err != nil {
		h.logger.Error("Failed to create rollback deployment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create rollback"})
		return
	}

	// Mark current deployment as rolled back
	_, err = h.db.Exec(c.Request.Context(), `
		UPDATE deployments
		SET status = 'rolled_back', status_message = 'Rolled back to previous deployment'
		WHERE id = $1
	`, deploymentID)

	if err != nil {
		h.logger.Warn("Failed to update original deployment status", zap.Error(err))
	}

	h.logger.Info("Rollback initiated",
		zap.String("original_deployment", deploymentID),
		zap.String("new_deployment", newDeploymentID.String()),
	)

	// TODO: In Phase 2, trigger deployment pipeline
	// go h.runDeploymentPipeline(orgID.String(), newDeploymentID.String())

	c.JSON(http.StatusOK, gin.H{
		"id":      newDeploymentID,
		"message": "Rollback initiated successfully",
	})
}

// ==================== Redeploy Deployment ====================

func (h *Handler) redeployDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}
	deploymentID, err := uuid.Parse(c.Param("did"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	// Parse request body for redeploy options
	var req struct {
		PullLatest   bool `json:"pull_latest"`
		SkipScanning bool `json:"skip_scanning"`
	}
	// Default to true if not specified
	req.PullLatest = true
	req.SkipScanning = false
	if err := c.ShouldBindJSON(&req); err != nil {
		// If there's no body, that's OK - use defaults
		h.logger.Debug("No request body for redeploy, using defaults", zap.Error(err))
	}

	// Fetch original deployment with all fields needed for redeployment
	var original struct {
		ServiceName     string
		Environment     string
		ImageRegistry   *string
		ImageRepository string
		ImageTag        *string
		ImageDigest     *string
		GitRepo         *string
		GitBranch       *string
		GitCommit       *string
		CIProvider      *string
		CIPipelineID    *string
		CIBuildURL      *string
		ContainerConfig []byte
	}

	err = h.db.QueryRow(c.Request.Context(), `
		SELECT service_name, environment, image_registry, image_repository,
		       image_tag, image_digest, git_repo, git_branch, git_commit,
		       ci_provider, ci_pipeline_id, ci_build_url, container_config
		FROM deployments
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, deploymentID, orgID, agentID).Scan(
		&original.ServiceName, &original.Environment, &original.ImageRegistry, &original.ImageRepository,
		&original.ImageTag, &original.ImageDigest, &original.GitRepo, &original.GitBranch, &original.GitCommit,
		&original.CIProvider, &original.CIPipelineID, &original.CIBuildURL, &original.ContainerConfig,
	)

	if err != nil {
		h.logger.Error("Deployment not found for redeploy", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Create new deployment copying all configuration from the original
	newID := uuid.New()
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO deployments (
			id, org_id, agent_id, service_name, environment,
			image_registry, image_repository, image_tag, image_digest,
			git_repo, git_branch, git_commit,
			ci_provider, ci_pipeline_id, ci_build_url,
			container_config, replaces_deployment_id,
			status, deployed_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, 'pending', $18, NOW(), NOW())
	`, newID, orgID, agentID, original.ServiceName, original.Environment,
		original.ImageRegistry, original.ImageRepository, original.ImageTag, original.ImageDigest,
		original.GitRepo, original.GitBranch, original.GitCommit,
		original.CIProvider, original.CIPipelineID, original.CIBuildURL,
		original.ContainerConfig, deploymentID, userID,
	)

	if err != nil {
		h.logger.Error("Failed to create redeploy deployment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create redeployment"})
		return
	}

	h.logger.Info("Redeployment created",
		zap.String("original_deployment", deploymentID.String()),
		zap.String("new_deployment", newID.String()),
		zap.String("service", original.ServiceName),
	)

	// Parse container config for pipeline
	var containerConfig *DeploymentContainerConfig
	if len(original.ContainerConfig) > 0 {
		if err := json.Unmarshal(original.ContainerConfig, &containerConfig); err != nil {
			h.logger.Warn("Failed to parse container config for redeploy", zap.Error(err))
		}
	}

	// Set pull_latest flag from request (create config if nil)
	if containerConfig == nil {
		containerConfig = &DeploymentContainerConfig{}
	}
	containerConfig.PullLatest = req.PullLatest

	h.logger.Info("Redeploy options",
		zap.Bool("pull_latest", req.PullLatest),
		zap.Bool("skip_scanning", req.SkipScanning),
		zap.String("deployment_id", newID.String()),
	)

	// Prepare image tag and digest for pipeline
	imageTag := ""
	if original.ImageTag != nil {
		imageTag = *original.ImageTag
	}
	imageDigest := ""
	if original.ImageDigest != nil {
		imageDigest = *original.ImageDigest
	}

	// Trigger deployment pipeline asynchronously
	go h.runDeploymentPipeline(context.Background(), orgID, newID,
		original.ImageRepository, imageTag, imageDigest, containerConfig, req.SkipScanning, false)

	c.JSON(http.StatusOK, gin.H{
		"id":      newID.String(),
		"status":  "pending",
		"message": "Redeployment created and pipeline initiated",
	})
}

// ==================== Update Deployment Status ====================

func (h *Handler) updateDeploymentStatus(ctx context.Context, id, status, message string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE deployments
		SET status = $1, status_message = $2, updated_at = NOW()
		WHERE id = $3
	`, status, message, id)

	if err != nil {
		h.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", id),
			zap.String("status", status),
			zap.Error(err),
		)
	}

	return err
}

// ==================== Sync Deployment Status ====================

// syncDeploymentStatus checks actual container state and updates deployment records
func (h *Handler) syncDeploymentStatus(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")

	// Get all "running" or "deploying" deployments with a container_id
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, container_id, container_name, service_name
		FROM deployments
		WHERE org_id = $1 AND agent_id = $2
		  AND status IN ('running', 'deploying')
		  AND container_id IS NOT NULL
	`, orgID, agentID)
	if err != nil {
		h.logger.Error("Failed to query deployments for sync", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query deployments"})
		return
	}
	defer rows.Close()

	type deploymentInfo struct {
		ID            uuid.UUID
		ContainerID   string
		ContainerName *string
		ServiceName   string
	}
	var deployments []deploymentInfo
	for rows.Next() {
		var d deploymentInfo
		if err := rows.Scan(&d.ID, &d.ContainerID, &d.ContainerName, &d.ServiceName); err != nil {
			continue
		}
		deployments = append(deployments, d)
	}

	if len(deployments) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "no running deployments to sync",
			"synced":  0,
		})
		return
	}

	// Connect to Docker to get actual container state
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		h.logger.Error("Failed to connect to Docker for sync", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	// List all containers (including stopped)
	containers, err := cli.ContainerList(c.Request.Context(), container.ListOptions{All: true})
	if err != nil {
		h.logger.Error("Failed to list containers for sync", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list containers"})
		return
	}

	// Build a map of existing container IDs
	containerMap := make(map[string]string) // containerID -> state
	for _, ctr := range containers {
		containerMap[ctr.ID] = ctr.State
		// Also check short ID (first 12 chars)
		if len(ctr.ID) >= 12 {
			containerMap[ctr.ID[:12]] = ctr.State
		}
	}

	// Check each deployment and update if container is missing or stopped
	syncedCount := 0
	for _, d := range deployments {
		state, exists := containerMap[d.ContainerID]
		// Also try short ID match
		if !exists && len(d.ContainerID) >= 12 {
			state, exists = containerMap[d.ContainerID[:12]]
		}

		var newStatus, statusMessage string
		if !exists {
			newStatus = "stopped"
			statusMessage = "Container no longer exists"
		} else if state == "exited" || state == "dead" {
			newStatus = "stopped"
			statusMessage = fmt.Sprintf("Container %s", state)
		}

		if newStatus != "" {
			_, err := h.db.Exec(c.Request.Context(), `
				UPDATE deployments
				SET status = $1, status_message = $2, updated_at = NOW()
				WHERE id = $3
			`, newStatus, statusMessage, d.ID)
			if err != nil {
				h.logger.Warn("Failed to update deployment status during sync",
					zap.String("deployment_id", d.ID.String()),
					zap.Error(err),
				)
				continue
			}
			h.logger.Info("Synced deployment status",
				zap.String("deployment_id", d.ID.String()),
				zap.String("service", d.ServiceName),
				zap.String("new_status", newStatus),
			)
			syncedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("synced %d deployments", syncedCount),
		"synced":  syncedCount,
		"checked": len(deployments),
	})
}

// ==================== Delete Deployment ====================

// deleteDeployment removes a deployment record from the database
func (h *Handler) deleteDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}
	deploymentID, err := uuid.Parse(c.Param("did"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	// Check if deployment exists and get its status
	var status string
	var containerID *string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT status, container_id
		FROM deployments
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, deploymentID, orgID, agentID).Scan(&status, &containerID)

	if err != nil {
		h.logger.Error("Deployment not found for delete", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// If the container is still running, warn the user
	if status == "running" && containerID != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "deployment has a running container",
			"message": "Stop the container first or use force=true to delete anyway",
		})
		return
	}

	// Delete the deployment record
	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM deployments
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, deploymentID, orgID, agentID)

	if err != nil {
		h.logger.Error("Failed to delete deployment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete deployment"})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	h.logger.Info("Deployment deleted",
		zap.String("deployment_id", deploymentID.String()),
		zap.String("agent_id", agentID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "deployment deleted successfully",
	})
}


// ==================== List Service Deployments ====================

func (h *Handler) listServiceDeployments(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")
	environment := c.Query("environment")

	query := `
		SELECT id, agent_id, environment, image_repository, image_tag,
		       git_commit, status, deployed_at, created_at
		FROM deployments
		WHERE org_id = $1 AND service_name = $2
	`
	args := []interface{}{orgID, serviceName}

	if environment != "" {
		query += " AND environment = $3"
		args = append(args, environment)
	}

	query += " ORDER BY created_at DESC LIMIT 50"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("Failed to list service deployments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
		return
	}
	defer rows.Close()

	type ServiceDeployment struct {
		ID              uuid.UUID  `json:"id"`
		AgentID         uuid.UUID  `json:"agent_id"`
		Environment     string     `json:"environment"`
		ImageRepository string     `json:"image_repository"`
		ImageTag        *string    `json:"image_tag"`
		GitCommit       *string    `json:"git_commit"`
		Status          string     `json:"status"`
		DeployedAt      *time.Time `json:"deployed_at"`
		CreatedAt       time.Time  `json:"created_at"`
	}

	deployments := []ServiceDeployment{}
	for rows.Next() {
		var d ServiceDeployment
		if err := rows.Scan(&d.ID, &d.AgentID, &d.Environment,
			&d.ImageRepository, &d.ImageTag, &d.GitCommit,
			&d.Status, &d.DeployedAt, &d.CreatedAt); err != nil {
			continue
		}
		deployments = append(deployments, d)
	}

	c.JSON(http.StatusOK, deployments)
}

// ==================== Get Current Deployment ====================

func (h *Handler) getCurrentDeployment(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	serviceName := c.Param("name")
	environment := c.Query("environment")

	var d Deployment
	var err error

	if environment != "" {
		err = h.db.QueryRow(c.Request.Context(), `
			SELECT id, org_id, agent_id, service_name, environment,
			       image_repository, image_tag, image_digest,
			       git_commit, status, container_id, container_name, deployed_at, created_at
			FROM deployments
			WHERE org_id = $1 AND service_name = $2 AND environment = $3
			  AND status = 'running'
			ORDER BY deployed_at DESC
			LIMIT 1
		`, orgID, serviceName, environment).Scan(
			&d.ID, &d.OrgID, &d.AgentID, &d.ServiceName, &d.Environment,
			&d.ImageRepository, &d.ImageTag, &d.ImageDigest,
			&d.GitCommit, &d.Status, &d.ContainerID, &d.ContainerName, &d.DeployedAt, &d.CreatedAt,
		)
	} else {
		// No env specified — return the most recently running deployment across all envs
		err = h.db.QueryRow(c.Request.Context(), `
			SELECT id, org_id, agent_id, service_name, environment,
			       image_repository, image_tag, image_digest,
			       git_commit, status, container_id, container_name, deployed_at, created_at
			FROM deployments
			WHERE org_id = $1 AND service_name = $2 AND status = 'running'
			ORDER BY deployed_at DESC
			LIMIT 1
		`, orgID, serviceName).Scan(
			&d.ID, &d.OrgID, &d.AgentID, &d.ServiceName, &d.Environment,
			&d.ImageRepository, &d.ImageTag, &d.ImageDigest,
			&d.GitCommit, &d.Status, &d.ContainerID, &d.ContainerName, &d.DeployedAt, &d.CreatedAt,
		)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no running deployment found"})
		return
	}

	c.JSON(http.StatusOK, d)
}

// ==================== Source Build (push-to-deploy) ====================

// buildResult mirrors the agent's docker.BuildImageResult.
type buildResult struct {
	ImageRef string `json:"image_ref"`
	ImageID  string `json:"image_id"`
	Log      string `json:"log"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// buildOnAgent dispatches a "build" docker command to the agent: clone the repo
// and build a local image (Dockerfile or Nixpacks), tagged imageTag. The agent
// returns a buildResult (carried on failure too, for the log).
func (h *Handler) buildOnAgent(ctx context.Context, agentID uuid.UUID, repoURL, ref, commit, imageTag, dockerfilePath, gitToken string, buildArgs map[string]string) (*buildResult, error) {
	options := map[string]interface{}{
		"repo_url":  repoURL,
		"ref":       ref,
		"commit":    commit,
		"image_tag": imageTag,
	}
	if dockerfilePath != "" {
		options["dockerfile_path"] = dockerfilePath
	}
	if gitToken != "" {
		options["git_token"] = gitToken
	}
	if len(buildArgs) > 0 {
		options["build_args"] = buildArgs
	}

	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal build options: %w", err)
	}

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "docker",
		Command:   json.RawMessage(fmt.Sprintf(`{"action":"build","options":%s}`, optionsJSON)),
	}

	// Builds can take a while — generous timeout vs the 120s used for run.
	resp, err := agentgrpc.SendCommand(agentID.String(), cmd, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("failed to send build command to agent: %w", err)
	}

	result, err := resp.GetCommandResult()
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}

	// The agent returns a build result on both success and failure (for logs).
	var br buildResult
	if len(result.Data) > 0 {
		_ = json.Unmarshal(result.Data, &br)
	}
	if !result.Success {
		if br.Message == "" {
			br.Message = result.Message
		}
		return &br, fmt.Errorf("agent build failed: %s", result.Message)
	}
	br.Success = true
	return &br, nil
}

// sanitizeImageName lowercases and replaces anything not valid in a docker image
// name component with '-', so a service+env can become a local image repo.
func sanitizeImageName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		out = "app"
	}
	return out
}

// resolveGitToken returns a token for private-repo clones. CE clones public repos
// only; private-repo credential resolution is an Enterprise capability.
func (h *Handler) resolveGitToken(_ context.Context, _ uuid.UUID, _ string) string {
	return ""
}
