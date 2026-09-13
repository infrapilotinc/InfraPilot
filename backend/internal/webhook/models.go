package webhook

import (
	"time"

	"github.com/google/uuid"
)

// AllServicesTarget is the ServiceName sentinel meaning "every service in the stack"
// instead of one specific service. Only valid when StackID is set -- a standalone webhook
// (no stack) always names one real service_configs entry.
const AllServicesTarget = "*"

// WebhookConfig represents a configured webhook endpoint
type WebhookConfig struct {
	ID              uuid.UUID  `json:"id"`
	OrgID           uuid.UUID  `json:"org_id"`
	AgentID         uuid.UUID  `json:"agent_id"`
	Name            string     `json:"name"`
	Provider        string     `json:"provider"` // github, gitlab, jenkins, generic
	Secret          string     `json:"-"`        // Never expose in JSON
	SecretHash      string     `json:"-"`        // bcrypt hash of secret (legacy)
	SecretEncrypted []byte     `json:"-"`        // AES-256-GCM encrypted secret for HMAC verification
	Enabled         bool       `json:"enabled"`
	ServiceName     string     `json:"service_name"`
	Environment     string     `json:"environment"`
	StackID         *uuid.UUID `json:"stack_id,omitempty"` // set when this webhook targets a service that belongs to a Stack
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

// WebhookEvent represents a received webhook event
type WebhookEvent struct {
	ID          uuid.UUID  `json:"id"`
	WebhookID   uuid.UUID  `json:"webhook_id"`
	Provider    string     `json:"provider"`
	EventType   string     `json:"event_type"`
	Payload     []byte     `json:"payload"`
	Headers     string     `json:"headers"` // JSON-encoded headers
	Verified    bool       `json:"verified"`
	Processed   bool       `json:"processed"`
	DeploymentID *uuid.UUID `json:"deployment_id,omitempty"`
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// BuildMetadata contains extracted CI/CD build information
type BuildMetadata struct {
	GitRepo      string  `json:"git_repo"`
	GitBranch    string  `json:"git_branch"`
	GitCommit    string  `json:"git_commit"`
	CommitMsg    *string `json:"commit_message,omitempty"`
	Author       *string `json:"author,omitempty"`
	CIProvider   string  `json:"ci_provider"`
	CIPipelineID string  `json:"ci_pipeline_id"`
	CIBuildURL   string  `json:"ci_build_url"`
	BuildNumber  *string `json:"build_number,omitempty"`
	ImageRepo    string  `json:"image_repository"`
	ImageTag     string  `json:"image_tag"`
	ImageDigest  *string `json:"image_digest,omitempty"`
}

// CreateWebhookRequest is the request to create a new webhook. StackID is optional -- set
// it when this webhook should target a service that belongs to a Stack (the recommended
// path, gets real container config and correct stack status tracking); omit it for a
// genuinely standalone service managed outside of Stacks (the older service_configs path).
type CreateWebhookRequest struct {
	Name        string     `json:"name" binding:"required"`
	Provider    string     `json:"provider" binding:"required,oneof=github gitlab jenkins generic"`
	ServiceName string     `json:"service_name" binding:"required"`
	Environment string     `json:"environment" binding:"required,oneof=dev staging prod"`
	StackID     *uuid.UUID `json:"stack_id,omitempty"`
}

// UpdateWebhookRequest is the request to update a webhook
type UpdateWebhookRequest struct {
	Name        *string    `json:"name"`
	Enabled     *bool      `json:"enabled"`
	ServiceName *string    `json:"service_name"`
	Environment *string    `json:"environment"`
	StackID     *uuid.UUID `json:"stack_id"`
}

// WebhookResponse includes the secret only on creation
type WebhookResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Provider    string     `json:"provider"`
	ServiceName string     `json:"service_name"`
	Environment string     `json:"environment"`
	StackID     *uuid.UUID `json:"stack_id,omitempty"`
	Enabled     bool       `json:"enabled"`
	Secret      *string    `json:"secret,omitempty"` // Only on creation
	WebhookURL  string     `json:"webhook_url"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}
