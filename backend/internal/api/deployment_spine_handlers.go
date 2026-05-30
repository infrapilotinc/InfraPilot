package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ==================== Deployment Spine Types ====================

type DeploymentSpine struct {
	Deployment      DeploymentInfo      `json:"deployment"`
	Source          *SourceInfo         `json:"source,omitempty"`
	Image           DeploymentImageInfo           `json:"image"`
	SecurityScan    *SecurityScanInfo   `json:"security_scan,omitempty"`
	SBOM            *SBOMInfo           `json:"sbom,omitempty"`
	PolicyEval      *PolicyEvalInfo     `json:"policy_eval,omitempty"`
	Runtime         *RuntimeInfo        `json:"runtime,omitempty"`
	Timeline        []TimelineEvent     `json:"timeline"`
	RelatedDeployments []RelatedDeployment `json:"related_deployments,omitempty"`
}

type DeploymentInfo struct {
	ID              string    `json:"id"`
	ServiceName     string    `json:"service_name"`
	Environment     string    `json:"environment"`
	Status          string    `json:"status"`
	StatusMessage   *string   `json:"status_message,omitempty"`
	DeployedBy      *string   `json:"deployed_by,omitempty"`
	DeployedAt      *time.Time `json:"deployed_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	BuildLog        *string   `json:"build_log,omitempty"` // captured output for source builds
}

type SourceInfo struct {
	Type         string    `json:"type"` // webhook, manual, api
	GitRepo      *string   `json:"git_repo,omitempty"`
	GitBranch    *string   `json:"git_branch,omitempty"`
	GitCommit    *string   `json:"git_commit,omitempty"`
	CIProvider   *string   `json:"ci_provider,omitempty"`
	CIPipelineID *string   `json:"ci_pipeline_id,omitempty"`
	CIBuildURL   *string   `json:"ci_build_url,omitempty"`
	WebhookEvent *WebhookEventSummary `json:"webhook_event,omitempty"`
}

type WebhookEventSummary struct {
	ID          string    `json:"id"`
	WebhookID   string    `json:"webhook_id"`
	WebhookName string    `json:"webhook_name"`
	Provider    string    `json:"provider"`
	EventType   string    `json:"event_type"`
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
}

type DeploymentImageInfo struct {
	Registry   *string `json:"registry,omitempty"`
	Repository string  `json:"repository"`
	Tag        *string `json:"tag,omitempty"`
	Digest     *string `json:"digest,omitempty"`
	FullImage  string  `json:"full_image"`
}

type SecurityScanInfo struct {
	ID              string    `json:"id"`
	TotalVulns      int       `json:"total_vulns"`
	CriticalCount   int       `json:"critical_count"`
	HighCount       int       `json:"high_count"`
	MediumCount     int       `json:"medium_count"`
	LowCount        int       `json:"low_count"`
	FixableCount    int       `json:"fixable_count"`
	ScannedAt       time.Time `json:"scanned_at"`
	Scanner         string    `json:"scanner"`
	ScannerVersion  string    `json:"scanner_version"`
	TopVulns        []VulnSummary `json:"top_vulnerabilities,omitempty"`
}

type VulnSummary struct {
	ID           string  `json:"id"`
	CVE          string  `json:"cve"`
	Severity     string  `json:"severity"`
	PackageName  string  `json:"package_name"`
	FixedVersion *string `json:"fixed_version,omitempty"`
	Score        *float64 `json:"score,omitempty"`
}

type SBOMInfo struct {
	ID              string `json:"id"`
	Format          string `json:"format"`
	TotalPackages   int    `json:"total_packages"`
	OSPackages      int    `json:"os_packages"`
	LibraryPackages int    `json:"library_packages"`
	TopPackages     []PackageSummary `json:"top_packages,omitempty"`
}

type PackageSummary struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Type            string `json:"type"`
	Vulnerabilities int    `json:"vulnerabilities"`
}

type PolicyEvalInfo struct {
	Decision      string    `json:"decision"`
	Reason        *string   `json:"reason,omitempty"`
	Errors        []string  `json:"errors,omitempty"`
	Warnings      []string  `json:"warnings,omitempty"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
	PolicyVersion string    `json:"policy_version"`
	RulesFired    []string  `json:"rules_fired,omitempty"`
}

type RuntimeInfo struct {
	ContainerID     *string    `json:"container_id,omitempty"`
	ContainerName   *string    `json:"container_name,omitempty"`
	ContainerStatus string     `json:"container_status"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	ProxyConfigured bool       `json:"proxy_configured"`
	ProxyURL        *string    `json:"proxy_url,omitempty"`
}

type TimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // created, scanned, sbom_generated, policy_evaluated, deployed, failed
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // success, warning, error, info
	Details     map[string]interface{} `json:"details,omitempty"`
}

type RelatedDeployment struct {
	ID           string    `json:"id"`
	ServiceName  string    `json:"service_name"`
	Environment  string    `json:"environment"`
	Status       string    `json:"status"`
	ImageTag     *string   `json:"image_tag,omitempty"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	Relationship string    `json:"relationship"` // previous, next, same_service
}

// ==================== Deployment Spine Handler ====================

func (h *Handler) getDeploymentSpine(c *gin.Context) {
	deploymentID, err := uuid.Parse(c.Param("did"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	// Get deployment details
	query := `
		SELECT d.id, d.service_name, d.environment, d.status, d.status_message,
		       d.image_registry, d.image_repository, d.image_tag, d.image_digest,
		       d.git_repo, d.git_branch, d.git_commit,
		       d.ci_provider, d.ci_pipeline_id, d.ci_build_url,
		       d.scan_result_id, d.sbom_id, d.policy_decision, d.policy_reason,
		       d.container_id, d.container_name, d.proxy_host_id,
		       d.webhook_event_id, d.deployed_by, d.deployed_at,
		       d.created_at, d.updated_at, d.build_log,
		       u.email as deployed_by_email
		FROM deployments d
		LEFT JOIN users u ON d.deployed_by = u.id
		WHERE d.id = $1
	`

	var (
		id, serviceName, environment, status                                     string
		statusMessage, imageRegistry, imageTag, imageDigest                     *string
		imageRepository                                                          string
		gitRepo, gitBranch, gitCommit                                           *string
		ciProvider, ciPipelineID, ciBuildURL                                    *string
		scanResultID, sbomID, proxyHostID, webhookEventID, deployedBy           *uuid.UUID
		policyDecision                                                           string
		policyReason                                                             *string
		containerID, containerName                                               *string
		deployedByEmail, buildLog                                                *string
		deployedAt                                                               *time.Time
		createdAt, updatedAt                                                     time.Time
	)

	err = h.db.QueryRow(c, query, deploymentID).Scan(
		&id, &serviceName, &environment, &status, &statusMessage,
		&imageRegistry, &imageRepository, &imageTag, &imageDigest,
		&gitRepo, &gitBranch, &gitCommit,
		&ciProvider, &ciPipelineID, &ciBuildURL,
		&scanResultID, &sbomID, &policyDecision, &policyReason,
		&containerID, &containerName, &proxyHostID,
		&webhookEventID, &deployedBy, &deployedAt,
		&createdAt, &updatedAt, &buildLog,
		&deployedByEmail,
	)
	if err != nil {
		h.logger.Error("failed to get deployment", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Build spine
	spine := DeploymentSpine{
		Deployment: DeploymentInfo{
			ID:            id,
			ServiceName:   serviceName,
			Environment:   environment,
			Status:        status,
			StatusMessage: statusMessage,
			DeployedBy:    deployedByEmail,
			DeployedAt:    deployedAt,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			BuildLog:      buildLog,
		},
		Image: DeploymentImageInfo{
			Registry:   imageRegistry,
			Repository: imageRepository,
			Tag:        imageTag,
			Digest:     imageDigest,
			FullImage:  buildFullImageName(imageRegistry, imageRepository, imageTag, imageDigest),
		},
		Timeline: []TimelineEvent{},
	}

	// Add deployment created event
	spine.Timeline = append(spine.Timeline, TimelineEvent{
		Timestamp:   createdAt,
		Type:        "created",
		Title:       "Deployment Created",
		Description: fmt.Sprintf("Deployment for %s in %s environment", serviceName, environment),
		Status:      "info",
	})

	// Get source info (webhook or manual)
	if webhookEventID != nil {
		webhookInfo, err := h.getWebhookEventInfo(c, *webhookEventID)
		if err == nil {
			spine.Source = &SourceInfo{
				Type:         "webhook",
				GitRepo:      gitRepo,
				GitBranch:    gitBranch,
				GitCommit:    gitCommit,
				CIProvider:   ciProvider,
				CIPipelineID: ciPipelineID,
				CIBuildURL:   ciBuildURL,
				WebhookEvent: webhookInfo,
			}

			// Add webhook event to timeline
			spine.Timeline = append(spine.Timeline, TimelineEvent{
				Timestamp:   webhookInfo.CreatedAt,
				Type:        "webhook",
				Title:       "CI/CD Trigger",
				Description: fmt.Sprintf("%s webhook from %s", webhookInfo.Provider, webhookInfo.WebhookName),
				Status:      "success",
				Details: map[string]interface{}{
					"provider":  webhookInfo.Provider,
					"commit":    gitCommit,
					"branch":    gitBranch,
					"verified":  webhookInfo.Verified,
				},
			})
		}
	} else if gitRepo != nil {
		spine.Source = &SourceInfo{
			Type:         "manual",
			GitRepo:      gitRepo,
			GitBranch:    gitBranch,
			GitCommit:    gitCommit,
			CIProvider:   ciProvider,
			CIPipelineID: ciPipelineID,
			CIBuildURL:   ciBuildURL,
		}
	}

	// Get security scan info
	if scanResultID != nil {
		scanInfo, err := h.getScanInfo(c, *scanResultID, imageDigest)
		if err == nil {
			spine.SecurityScan = scanInfo

			// Add scan event to timeline
			spine.Timeline = append(spine.Timeline, TimelineEvent{
				Timestamp:   scanInfo.ScannedAt,
				Type:        "scanned",
				Title:       "Security Scan Completed",
				Description: fmt.Sprintf("Found %d vulnerabilities (%d critical, %d high)",
					scanInfo.TotalVulns, scanInfo.CriticalCount, scanInfo.HighCount),
				Status:      getScanStatus(scanInfo.CriticalCount, scanInfo.HighCount),
				Details: map[string]interface{}{
					"scanner": scanInfo.Scanner,
					"total":   scanInfo.TotalVulns,
					"critical": scanInfo.CriticalCount,
					"high":     scanInfo.HighCount,
				},
			})
		}
	}

	// Get SBOM info
	if sbomID != nil {
		sbomInfo, err := h.getSBOMInfo(c, *sbomID)
		if err == nil {
			spine.SBOM = sbomInfo

			// Add SBOM event to timeline
			spine.Timeline = append(spine.Timeline, TimelineEvent{
				Timestamp:   createdAt.Add(time.Second * 5), // Approximate
				Type:        "sbom_generated",
				Title:       "SBOM Generated",
				Description: fmt.Sprintf("%d packages cataloged (%d OS, %d libraries)",
					sbomInfo.TotalPackages, sbomInfo.OSPackages, sbomInfo.LibraryPackages),
				Status:      "success",
				Details: map[string]interface{}{
					"format":        sbomInfo.Format,
					"total_packages": sbomInfo.TotalPackages,
				},
			})
		}
	}

	// Get policy evaluation info
	policyInfo := &PolicyEvalInfo{
		Decision:      policyDecision,
		Reason:        policyReason,
		EvaluatedAt:   createdAt.Add(time.Second * 10), // Approximate
		PolicyVersion: "default",
	}
	spine.PolicyEval = policyInfo

	// Add policy event to timeline
	policyStatus := "success"
	policyDesc := "Deployment allowed by policy"
	if policyDecision == "deny" {
		policyStatus = "error"
		policyDesc = "Deployment blocked by policy"
		if policyReason != nil {
			policyDesc += ": " + *policyReason
		}
	} else if policyDecision == "warn" {
		policyStatus = "warning"
		policyDesc = "Deployment allowed with warnings"
	}

	spine.Timeline = append(spine.Timeline, TimelineEvent{
		Timestamp:   policyInfo.EvaluatedAt,
		Type:        "policy_evaluated",
		Title:       "Policy Evaluation",
		Description: policyDesc,
		Status:      policyStatus,
		Details: map[string]interface{}{
			"decision": policyDecision,
			"reason":   policyReason,
		},
	})

	// Get runtime info
	if containerID != nil || proxyHostID != nil {
		runtimeInfo := &RuntimeInfo{
			ContainerID:     containerID,
			ContainerName:   containerName,
			ContainerStatus: status,
			ProxyConfigured: proxyHostID != nil,
		}

		if status == "running" && deployedAt != nil {
			runtimeInfo.StartedAt = deployedAt
		}

		spine.Runtime = runtimeInfo

		// Add deployment event to timeline
		if deployedAt != nil {
			spine.Timeline = append(spine.Timeline, TimelineEvent{
				Timestamp:   *deployedAt,
				Type:        "deployed",
				Title:       "Deployment Successful",
				Description: fmt.Sprintf("Container %s is running", *containerName),
				Status:      "success",
			})
		}
	}

	// Add failure event if failed
	if status == "failed" && statusMessage != nil {
		spine.Timeline = append(spine.Timeline, TimelineEvent{
			Timestamp:   updatedAt,
			Type:        "failed",
			Title:       "Deployment Failed",
			Description: *statusMessage,
			Status:      "error",
		})
	}

	// Get related deployments (previous and next for same service)
	relatedDeployments, err := h.getRelatedDeployments(c, deploymentID, serviceName, environment)
	if err == nil {
		spine.RelatedDeployments = relatedDeployments
	}

	c.JSON(http.StatusOK, spine)
}

// ==================== Helper Functions ====================

func (h *Handler) getWebhookEventInfo(c *gin.Context, eventID uuid.UUID) (*WebhookEventSummary, error) {
	query := `
		SELECT we.id, we.webhook_id, we.provider, we.event_type, we.verified, we.created_at,
		       wc.name as webhook_name
		FROM webhook_events we
		JOIN webhook_configs wc ON we.webhook_id = wc.id
		WHERE we.id = $1
	`

	var summary WebhookEventSummary
	var webhookID uuid.UUID
	err := h.db.QueryRow(c, query, eventID).Scan(
		&summary.ID, &webhookID, &summary.Provider, &summary.EventType,
		&summary.Verified, &summary.CreatedAt, &summary.WebhookName,
	)
	if err != nil {
		return nil, err
	}
	summary.WebhookID = webhookID.String()
	return &summary, nil
}

func (h *Handler) getScanInfo(c *gin.Context, scanID uuid.UUID, imageDigest *string) (*SecurityScanInfo, error) {
	query := `
		SELECT id, total_count, critical_count, high_count, medium_count, low_count,
		       scanner_name, scanner_version, scanned_at
		FROM scan_results
		WHERE id = $1
	`

	var info SecurityScanInfo
	var id uuid.UUID
	err := h.db.QueryRow(c, query, scanID).Scan(
		&id, &info.TotalVulns, &info.CriticalCount, &info.HighCount,
		&info.MediumCount, &info.LowCount, &info.Scanner, &info.ScannerVersion,
		&info.ScannedAt,
	)
	if err != nil {
		return nil, err
	}
	info.ID = id.String()

	// Get fixable count
	fixableQuery := `
		SELECT COUNT(*) FROM vulnerabilities
		WHERE scan_result_id = $1 AND fixed_version IS NOT NULL AND fixed_version != ''
	`
	h.db.QueryRow(c, fixableQuery, scanID).Scan(&info.FixableCount)

	// Get top 5 critical/high vulnerabilities
	vulnQuery := `
		SELECT id, vulnerability_id, severity, package_name, fixed_version, cvss_score
		FROM vulnerabilities
		WHERE scan_result_id = $1
		ORDER BY
			CASE severity
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				ELSE 4
			END,
			cvss_score DESC NULLS LAST
		LIMIT 5
	`

	rows, err := h.db.Query(c, vulnQuery, scanID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var v VulnSummary
			var id uuid.UUID
			rows.Scan(&id, &v.CVE, &v.Severity, &v.PackageName, &v.FixedVersion, &v.Score)
			v.ID = id.String()
			info.TopVulns = append(info.TopVulns, v)
		}
	}

	return &info, nil
}

func (h *Handler) getSBOMInfo(c *gin.Context, sbomID uuid.UUID) (*SBOMInfo, error) {
	query := `
		SELECT id, format, total_packages, os_packages, library_packages
		FROM sboms
		WHERE id = $1
	`

	var info SBOMInfo
	var id uuid.UUID
	err := h.db.QueryRow(c, query, sbomID).Scan(
		&id, &info.Format, &info.TotalPackages, &info.OSPackages, &info.LibraryPackages,
	)
	if err != nil {
		return nil, err
	}
	info.ID = id.String()

	// Could add top packages here if needed

	return &info, nil
}

func (h *Handler) getRelatedDeployments(c *gin.Context, currentID uuid.UUID, serviceName, environment string) ([]RelatedDeployment, error) {
	query := `
		SELECT id, service_name, environment, status, image_tag, deployed_at, created_at
		FROM deployments
		WHERE service_name = $1 AND environment = $2 AND id != $3
		ORDER BY created_at DESC
		LIMIT 5
	`

	rows, err := h.db.Query(c, query, serviceName, environment, currentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var related []RelatedDeployment
	for rows.Next() {
		var r RelatedDeployment
		var id uuid.UUID
		var createdAt time.Time
		err := rows.Scan(&r.ID, &r.ServiceName, &r.Environment, &r.Status, &r.ImageTag, &r.DeployedAt, &createdAt)
		if err != nil {
			continue
		}
		r.ID = id.String()
		r.Relationship = "previous"
		related = append(related, r)
	}

	return related, nil
}

func buildFullImageName(registry *string, repository string, tag, digest *string) string {
	full := ""
	if registry != nil {
		full = *registry + "/"
	}
	full += repository
	if tag != nil {
		full += ":" + *tag
	}
	if digest != nil {
		full += "@" + *digest
	}
	return full
}

func getScanStatus(critical, high int) string {
	if critical > 0 {
		return "error"
	}
	if high > 0 {
		return "warning"
	}
	return "success"
}
