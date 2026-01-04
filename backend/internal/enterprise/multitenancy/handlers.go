package multitenancy

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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

type Organization struct {
	ID                 uuid.UUID       `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Plan               string          `json:"plan"`
	StripeCustomerID   *string         `json:"stripe_customer_id,omitempty"`
	SubscriptionStatus *string         `json:"subscription_status,omitempty"`
	MaxUsers           int             `json:"max_users"`
	MaxAgents          int             `json:"max_agents"`
	Settings           map[string]any  `json:"settings"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type OrganizationMember struct {
	ID        uuid.UUID  `json:"id"`
	OrgID     uuid.UUID  `json:"org_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Role      string     `json:"role"`
	InvitedBy *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt  time.Time  `json:"joined_at"`
	Email     string     `json:"email,omitempty"`
	UserName  string     `json:"user_name,omitempty"`
}

type OrganizationInvitation struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Token      string     `json:"token,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedBy  *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type EnrollmentToken struct {
	ID         uuid.UUID       `json:"id"`
	OrgID      uuid.UUID       `json:"org_id"`
	Token      string          `json:"token,omitempty"`
	Name       *string         `json:"name,omitempty"`
	CreatedBy  *uuid.UUID      `json:"created_by,omitempty"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	MaxUses    *int            `json:"max_uses,omitempty"`
	UseCount   int             `json:"use_count"`
	Labels     map[string]any  `json:"labels"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
	LastUsedAt *time.Time      `json:"last_used_at,omitempty"`
}

// ============ Request/Response Types ============

type CreateOrgRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

type UpdateOrgRequest struct {
	Name      *string         `json:"name"`
	MaxUsers  *int            `json:"max_users"`
	MaxAgents *int            `json:"max_agents"`
	Settings  *map[string]any `json:"settings"`
}

type AddMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role" binding:"required"`
}

type UpdateMemberRequest struct {
	Role string `json:"role" binding:"required"`
}

type CreateInvitationRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required"`
}

type CreateEnrollmentTokenRequest struct {
	Name      string          `json:"name"`
	ExpiresAt *time.Time      `json:"expires_at"`
	MaxUses   *int            `json:"max_uses"`
	Labels    map[string]any  `json:"labels"`
}

type OrgUsage struct {
	Users       int `json:"users"`
	MaxUsers    int `json:"max_users"`
	Agents      int `json:"agents"`
	MaxAgents   int `json:"max_agents"`
}

// ============ Organization Handlers ============

func (h *Handler) ListOrganizations(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	// Use a transaction to bypass RLS for this query
	// This is safe because we're filtering by user_id
	tx, err := h.db.Begin(c)
	if err != nil {
		h.logger.Error("Failed to begin transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list organizations"})
		return
	}
	defer tx.Rollback(c)

	// Temporarily disable RLS for this session
	_, err = tx.Exec(c, "SET LOCAL row_security = off")
	if err != nil {
		h.logger.Error("Failed to disable RLS", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list organizations"})
		return
	}

	rows, err := tx.Query(c, `
		SELECT o.id, o.name, o.slug, o.plan, o.max_users, o.max_agents,
		       COALESCE(o.settings, '{}'), o.created_at, o.updated_at,
		       om.role
		FROM organizations o
		JOIN organization_members om ON o.id = om.org_id
		WHERE om.user_id = $1
		ORDER BY o.name
	`, userID)
	if err != nil {
		h.logger.Error("Failed to list organizations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list organizations"})
		return
	}
	defer rows.Close()

	type OrgWithRole struct {
		Organization
		MemberRole string `json:"member_role"`
	}

	var orgs []OrgWithRole
	for rows.Next() {
		var org OrgWithRole
		if err := rows.Scan(
			&org.ID, &org.Name, &org.Slug, &org.Plan,
			&org.MaxUsers, &org.MaxAgents, &org.Settings,
			&org.CreatedAt, &org.UpdatedAt, &org.MemberRole,
		); err != nil {
			continue
		}
		orgs = append(orgs, org)
	}

	c.JSON(http.StatusOK, orgs)
}

func (h *Handler) GetOrganization(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var org Organization
	err = h.db.QueryRow(c, `
		SELECT id, name, slug, plan, stripe_customer_id, subscription_status,
		       max_users, max_agents, COALESCE(settings, '{}'), created_at, updated_at
		FROM organizations WHERE id = $1
	`, orgID).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Plan,
		&org.StripeCustomerID, &org.SubscriptionStatus,
		&org.MaxUsers, &org.MaxAgents, &org.Settings,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to get organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get organization"})
		return
	}

	c.JSON(http.StatusOK, org)
}

func (h *Handler) CreateOrganization(c *gin.Context) {
	var req CreateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	tx, err := h.db.Begin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(c)

	// Create organization
	var orgID uuid.UUID
	err = tx.QueryRow(c, `
		INSERT INTO organizations (name, slug, plan, max_users, max_agents)
		VALUES ($1, $2, 'free', 5, 3)
		RETURNING id
	`, req.Name, req.Slug).Scan(&orgID)
	if err != nil {
		h.logger.Error("Failed to create organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create organization"})
		return
	}

	// Add creator as owner
	_, err = tx.Exec(c, `
		INSERT INTO organization_members (org_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, orgID, userID)
	if err != nil {
		h.logger.Error("Failed to add owner", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add owner"})
		return
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": orgID})
}

func (h *Handler) UpdateOrganization(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var req UpdateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	query := "UPDATE organizations SET updated_at = NOW()"
	args := []any{orgID}
	argPos := 2

	if req.Name != nil {
		query += ", name = $" + string(rune('0'+argPos))
		args = append(args, *req.Name)
		argPos++
	}
	if req.MaxUsers != nil {
		query += ", max_users = $" + string(rune('0'+argPos))
		args = append(args, *req.MaxUsers)
		argPos++
	}
	if req.MaxAgents != nil {
		query += ", max_agents = $" + string(rune('0'+argPos))
		args = append(args, *req.MaxAgents)
		argPos++
	}
	if req.Settings != nil {
		query += ", settings = $" + string(rune('0'+argPos))
		args = append(args, *req.Settings)
		argPos++
	}

	query += " WHERE id = $1"

	_, err = h.db.Exec(c, query, args...)
	if err != nil {
		h.logger.Error("Failed to update organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update organization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *Handler) DeleteOrganization(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	_, err = h.db.Exec(c, "DELETE FROM organizations WHERE id = $1", orgID)
	if err != nil {
		h.logger.Error("Failed to delete organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete organization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handler) GetOrganizationUsage(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var usage OrgUsage
	err = h.db.QueryRow(c, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE org_id = $1) as users,
			(SELECT max_users FROM organizations WHERE id = $1) as max_users,
			(SELECT COUNT(*) FROM agents WHERE org_id = $1) as agents,
			(SELECT max_agents FROM organizations WHERE id = $1) as max_agents
	`, orgID).Scan(&usage.Users, &usage.MaxUsers, &usage.Agents, &usage.MaxAgents)
	if err != nil {
		h.logger.Error("Failed to get usage", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage"})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// ============ Member Handlers ============

func (h *Handler) ListMembers(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	rows, err := h.db.Query(c, `
		SELECT om.id, om.org_id, om.user_id, om.role, om.invited_by, om.joined_at,
		       u.email, COALESCE(u.email, '') as user_name
		FROM organization_members om
		JOIN users u ON om.user_id = u.id
		WHERE om.org_id = $1
		ORDER BY om.joined_at
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list members", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list members"})
		return
	}
	defer rows.Close()

	var members []OrganizationMember
	for rows.Next() {
		var m OrganizationMember
		if err := rows.Scan(
			&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.InvitedBy, &m.JoinedAt,
			&m.Email, &m.UserName,
		); err != nil {
			continue
		}
		members = append(members, m)
	}

	c.JSON(http.StatusOK, members)
}

func (h *Handler) AddMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{"admin": true, "member": true, "viewer": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	currentUserID, _ := c.Get("user_id")

	var memberID uuid.UUID
	err = h.db.QueryRow(c, `
		INSERT INTO organization_members (org_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, orgID, req.UserID, req.Role, currentUserID).Scan(&memberID)
	if err != nil {
		h.logger.Error("Failed to add member", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": memberID})
}

func (h *Handler) UpdateMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	userID, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req UpdateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{"owner": true, "admin": true, "member": true, "viewer": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	_, err = h.db.Exec(c, `
		UPDATE organization_members SET role = $1
		WHERE org_id = $2 AND user_id = $3
	`, req.Role, orgID, userID)
	if err != nil {
		h.logger.Error("Failed to update member", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *Handler) RemoveMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	userID, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Don't allow removing the last owner
	var ownerCount int
	h.db.QueryRow(c, `
		SELECT COUNT(*) FROM organization_members
		WHERE org_id = $1 AND role = 'owner'
	`, orgID).Scan(&ownerCount)

	var memberRole string
	h.db.QueryRow(c, `
		SELECT role FROM organization_members
		WHERE org_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&memberRole)

	if memberRole == "owner" && ownerCount <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot remove the last owner"})
		return
	}

	_, err = h.db.Exec(c, `
		DELETE FROM organization_members
		WHERE org_id = $1 AND user_id = $2
	`, orgID, userID)
	if err != nil {
		h.logger.Error("Failed to remove member", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ============ Invitation Handlers ============

func (h *Handler) ListInvitations(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	rows, err := h.db.Query(c, `
		SELECT id, org_id, email, role, expires_at, accepted_at, created_by, created_at
		FROM organization_invitations
		WHERE org_id = $1 AND accepted_at IS NULL
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list invitations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list invitations"})
		return
	}
	defer rows.Close()

	var invitations []OrganizationInvitation
	for rows.Next() {
		var inv OrganizationInvitation
		if err := rows.Scan(
			&inv.ID, &inv.OrgID, &inv.Email, &inv.Role,
			&inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedBy, &inv.CreatedAt,
		); err != nil {
			continue
		}
		invitations = append(invitations, inv)
	}

	c.JSON(http.StatusOK, invitations)
}

func (h *Handler) CreateInvitation(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var req CreateInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{"admin": true, "member": true, "viewer": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	// Generate token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	currentUserID, _ := c.Get("user_id")
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	var invID uuid.UUID
	err = h.db.QueryRow(c, `
		INSERT INTO organization_invitations (org_id, email, role, token, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (org_id, email) WHERE accepted_at IS NULL
		DO UPDATE SET role = $3, token = $4, expires_at = $5, created_by = $6, created_at = NOW()
		RETURNING id
	`, orgID, req.Email, req.Role, token, expiresAt, currentUserID).Scan(&invID)
	if err != nil {
		h.logger.Error("Failed to create invitation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         invID,
		"token":      token,
		"expires_at": expiresAt,
	})
}

func (h *Handler) RevokeInvitation(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	invID, err := uuid.Parse(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	_, err = h.db.Exec(c, `
		DELETE FROM organization_invitations
		WHERE id = $1 AND org_id = $2
	`, invID, orgID)
	if err != nil {
		h.logger.Error("Failed to revoke invitation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func (h *Handler) AcceptInvitation(c *gin.Context) {
	token := c.Param("token")

	tx, err := h.db.Begin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(c)

	// Find invitation
	var inv OrganizationInvitation
	err = tx.QueryRow(c, `
		SELECT id, org_id, email, role, expires_at
		FROM organization_invitations
		WHERE token = $1 AND accepted_at IS NULL
	`, token).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.ExpiresAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to find invitation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find invitation"})
		return
	}

	// Check expiry
	if time.Now().After(inv.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation has expired"})
		return
	}

	// Find user by email
	var userID uuid.UUID
	err = tx.QueryRow(c, "SELECT id FROM users WHERE email = $1", inv.Email).Scan(&userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No account found for this email"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to find user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Add member
	_, err = tx.Exec(c, `
		INSERT INTO organization_members (org_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, (SELECT created_by FROM organization_invitations WHERE id = $4))
		ON CONFLICT (org_id, user_id) DO NOTHING
	`, inv.OrgID, userID, inv.Role, inv.ID)
	if err != nil {
		h.logger.Error("Failed to add member", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	// Mark invitation as accepted
	_, err = tx.Exec(c, `
		UPDATE organization_invitations SET accepted_at = NOW()
		WHERE id = $1
	`, inv.ID)
	if err != nil {
		h.logger.Error("Failed to update invitation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invitation"})
		return
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "accepted",
		"org_id": inv.OrgID,
	})
}

// ============ Enrollment Token Handlers ============

func (h *Handler) ListEnrollmentTokens(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	rows, err := h.db.Query(c, `
		SELECT id, org_id, token, name, created_by, expires_at, max_uses, use_count,
		       COALESCE(labels, '{}'), enabled, created_at, last_used_at
		FROM enrollment_tokens
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list enrollment tokens", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list enrollment tokens"})
		return
	}
	defer rows.Close()

	var tokens []EnrollmentToken
	for rows.Next() {
		var t EnrollmentToken
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.Token, &t.Name, &t.CreatedBy, &t.ExpiresAt,
			&t.MaxUses, &t.UseCount, &t.Labels, &t.Enabled, &t.CreatedAt, &t.LastUsedAt,
		); err != nil {
			continue
		}
		// Mask token for security (show only first 8 chars)
		if len(t.Token) > 8 {
			t.Token = t.Token[:8] + "..." + t.Token[len(t.Token)-4:]
		}
		tokens = append(tokens, t)
	}

	c.JSON(http.StatusOK, tokens)
}

func (h *Handler) CreateEnrollmentToken(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	var req CreateEnrollmentTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate token with prefix
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := "ip_enroll_" + hex.EncodeToString(tokenBytes)

	currentUserID, _ := c.Get("user_id")

	labels := req.Labels
	if labels == nil {
		labels = map[string]any{}
	}

	var tokenID uuid.UUID
	err = h.db.QueryRow(c, `
		INSERT INTO enrollment_tokens (org_id, token, name, created_by, expires_at, max_uses, labels)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, orgID, token, req.Name, currentUserID, req.ExpiresAt, req.MaxUses, labels).Scan(&tokenID)
	if err != nil {
		h.logger.Error("Failed to create enrollment token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create enrollment token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":    tokenID,
		"token": token,
	})
}

func (h *Handler) RevokeEnrollmentToken(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	tokenID, err := uuid.Parse(c.Param("tid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token ID"})
		return
	}

	_, err = h.db.Exec(c, `
		UPDATE enrollment_tokens SET enabled = false
		WHERE id = $1 AND org_id = $2
	`, tokenID, orgID)
	if err != nil {
		h.logger.Error("Failed to revoke token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func (h *Handler) DeleteEnrollmentToken(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}

	tokenID, err := uuid.Parse(c.Param("tid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token ID"})
		return
	}

	_, err = h.db.Exec(c, `
		DELETE FROM enrollment_tokens
		WHERE id = $1 AND org_id = $2
	`, tokenID, orgID)
	if err != nil {
		h.logger.Error("Failed to delete token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
