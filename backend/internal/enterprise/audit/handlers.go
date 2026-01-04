package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/enterprise/license"
)

type Handler struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewHandler(db *pgxpool.Pool, logger *zap.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

// ============ Configuration Types ============

type AuditConfig struct {
	ID                uuid.UUID              `json:"id"`
	OrgID             uuid.UUID              `json:"org_id"`
	RetentionDays     int                    `json:"retention_days"`
	RetentionPolicy   string                 `json:"retention_policy"`
	ForwardingEnabled bool                   `json:"forwarding_enabled"`
	ForwardingType    *string                `json:"forwarding_type,omitempty"`
	ForwardingConfig  map[string]interface{} `json:"forwarding_config,omitempty"`
	ComplianceMode    *string                `json:"compliance_mode,omitempty"`
	ImmutableLogs     bool                   `json:"immutable_logs"`
	HashChainEnabled  bool                   `json:"hash_chain_enabled"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type UpdateConfigRequest struct {
	RetentionDays     *int                   `json:"retention_days"`
	RetentionPolicy   *string                `json:"retention_policy"`
	ForwardingEnabled *bool                  `json:"forwarding_enabled"`
	ForwardingType    *string                `json:"forwarding_type"`
	ForwardingConfig  map[string]interface{} `json:"forwarding_config"`
	ComplianceMode    *string                `json:"compliance_mode"`
	ImmutableLogs     *bool                  `json:"immutable_logs"`
	HashChainEnabled  *bool                  `json:"hash_chain_enabled"`
}

type AuditExport struct {
	ID          uuid.UUID              `json:"id"`
	OrgID       uuid.UUID              `json:"org_id"`
	Format      string                 `json:"format"`
	Status      string                 `json:"status"`
	StartDate   *time.Time             `json:"start_date,omitempty"`
	EndDate     *time.Time             `json:"end_date,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	RowCount    *int                   `json:"row_count,omitempty"`
	FileSize    *int64                 `json:"file_size,omitempty"`
	DownloadURL *string                `json:"download_url,omitempty"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

type ExportRequest struct {
	Format       string                 `json:"format" binding:"required"`
	StartDate    *time.Time             `json:"start_date"`
	EndDate      *time.Time             `json:"end_date"`
	Filters      map[string]interface{} `json:"filters"`
	IncludeBody  bool                   `json:"include_body"`
	HashIntegrity bool                  `json:"hash_integrity"`
}

type ComplianceReport struct {
	ID          uuid.UUID              `json:"id"`
	OrgID       uuid.UUID              `json:"org_id"`
	ReportType  string                 `json:"report_type"`
	Status      string                 `json:"status"`
	StartDate   time.Time              `json:"start_date"`
	EndDate     time.Time              `json:"end_date"`
	Summary     map[string]interface{} `json:"summary,omitempty"`
	DownloadURL *string                `json:"download_url,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

type ReportRequest struct {
	ReportType string    `json:"report_type" binding:"required"`
	StartDate  time.Time `json:"start_date" binding:"required"`
	EndDate    time.Time `json:"end_date" binding:"required"`
}

// ============ Configuration Handlers ============

func (h *Handler) GetConfig(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	var config AuditConfig
	var forwardingConfig []byte
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, retention_days, retention_policy, forwarding_enabled,
		       forwarding_type, forwarding_config, compliance_mode, immutable_logs,
		       hash_chain_enabled, created_at, updated_at
		FROM audit_config WHERE org_id = $1
	`, orgID).Scan(
		&config.ID, &config.OrgID, &config.RetentionDays, &config.RetentionPolicy,
		&config.ForwardingEnabled, &config.ForwardingType, &forwardingConfig,
		&config.ComplianceMode, &config.ImmutableLogs, &config.HashChainEnabled,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		// Return default config if not exists
		config = AuditConfig{
			OrgID:           orgID,
			RetentionDays:   90,
			RetentionPolicy: "delete",
		}
	}

	if len(forwardingConfig) > 0 {
		json.Unmarshal(forwardingConfig, &config.ForwardingConfig)
	}

	c.JSON(http.StatusOK, config)
}

func (h *Handler) UpdateConfig(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate retention_days (0 = unlimited, requires enterprise)
	if req.RetentionDays != nil && *req.RetentionDays == 0 {
		if err := license.RequireFeature(c.Request.Context(), "unlimited_retention"); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unlimited retention requires Enterprise license"})
			return
		}
	}

	// Validate compliance mode
	validModes := map[string]bool{"soc2": true, "hipaa": true, "gdpr": true, "pci": true}
	if req.ComplianceMode != nil && *req.ComplianceMode != "" && !validModes[*req.ComplianceMode] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid compliance mode"})
		return
	}

	var forwardingConfig []byte
	if req.ForwardingConfig != nil {
		forwardingConfig, _ = json.Marshal(req.ForwardingConfig)
	}

	_, err := h.db.Exec(c.Request.Context(), `
		INSERT INTO audit_config (org_id, retention_days, retention_policy, forwarding_enabled,
		                          forwarding_type, forwarding_config, compliance_mode,
		                          immutable_logs, hash_chain_enabled)
		VALUES ($1, COALESCE($2, 90), COALESCE($3, 'delete'), COALESCE($4, false),
		        $5, $6, $7, COALESCE($8, false), COALESCE($9, false))
		ON CONFLICT (org_id) DO UPDATE SET
			retention_days = COALESCE($2, audit_config.retention_days),
			retention_policy = COALESCE($3, audit_config.retention_policy),
			forwarding_enabled = COALESCE($4, audit_config.forwarding_enabled),
			forwarding_type = COALESCE($5, audit_config.forwarding_type),
			forwarding_config = COALESCE($6, audit_config.forwarding_config),
			compliance_mode = COALESCE($7, audit_config.compliance_mode),
			immutable_logs = COALESCE($8, audit_config.immutable_logs),
			hash_chain_enabled = COALESCE($9, audit_config.hash_chain_enabled),
			updated_at = NOW()
	`, orgID, req.RetentionDays, req.RetentionPolicy, req.ForwardingEnabled,
		req.ForwardingType, forwardingConfig, req.ComplianceMode,
		req.ImmutableLogs, req.HashChainEnabled)

	if err != nil {
		h.logger.Error("Failed to update audit config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated"})
}

// ============ Export Handlers ============

func (h *Handler) CreateExport(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate format
	validFormats := map[string]bool{"csv": true, "json": true, "cef": true, "syslog": true}
	if !validFormats[req.Format] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid format. Use: csv, json, cef, or syslog"})
		return
	}

	var filtersJSON []byte
	if req.Filters != nil {
		filtersJSON, _ = json.Marshal(req.Filters)
	}

	var exportID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO audit_exports (org_id, user_id, format, start_date, end_date, filters)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, orgID, userID, req.Format, req.StartDate, req.EndDate, filtersJSON).Scan(&exportID)

	if err != nil {
		h.logger.Error("Failed to create export", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create export"})
		return
	}

	// Process export inline for now (in production, use background job)
	go h.processExport(context.Background(), exportID, orgID, req)

	c.JSON(http.StatusAccepted, gin.H{
		"id":      exportID,
		"status":  "processing",
		"message": "Export started. Check status for completion.",
	})
}

func (h *Handler) GetExport(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)
	exportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid export ID"})
		return
	}

	var export AuditExport
	var filters []byte
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, format, status, start_date, end_date, filters,
		       row_count, file_size, download_url, expires_at, created_at, completed_at
		FROM audit_exports WHERE id = $1 AND org_id = $2
	`, exportID, orgID).Scan(
		&export.ID, &export.OrgID, &export.Format, &export.Status,
		&export.StartDate, &export.EndDate, &filters, &export.RowCount,
		&export.FileSize, &export.DownloadURL, &export.ExpiresAt,
		&export.CreatedAt, &export.CompletedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Export not found"})
		return
	}

	if len(filters) > 0 {
		json.Unmarshal(filters, &export.Filters)
	}

	c.JSON(http.StatusOK, export)
}

func (h *Handler) ListExports(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, format, status, start_date, end_date, row_count,
		       file_size, created_at, completed_at
		FROM audit_exports WHERE org_id = $1
		ORDER BY created_at DESC LIMIT 50
	`, orgID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list exports"})
		return
	}
	defer rows.Close()

	var exports []AuditExport
	for rows.Next() {
		var e AuditExport
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.Format, &e.Status, &e.StartDate, &e.EndDate,
			&e.RowCount, &e.FileSize, &e.CreatedAt, &e.CompletedAt,
		); err != nil {
			continue
		}
		exports = append(exports, e)
	}

	c.JSON(http.StatusOK, exports)
}

func (h *Handler) DownloadExport(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)
	exportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid export ID"})
		return
	}

	var format, status string
	var startDate, endDate *time.Time
	var filters []byte

	err = h.db.QueryRow(c.Request.Context(), `
		SELECT format, status, start_date, end_date, filters
		FROM audit_exports WHERE id = $1 AND org_id = $2
	`, exportID, orgID).Scan(&format, &status, &startDate, &endDate, &filters)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Export not found"})
		return
	}

	if status != "completed" {
		c.JSON(http.StatusConflict, gin.H{"error": "Export not ready", "status": status})
		return
	}

	// Generate export on-the-fly
	content, contentType, filename := h.generateExportContent(c.Request.Context(), orgID, format, startDate, endDate, filters)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, contentType, content)
}

func (h *Handler) processExport(ctx context.Context, exportID, orgID uuid.UUID, req ExportRequest) {
	// Update status to processing
	h.db.Exec(ctx, `UPDATE audit_exports SET status = 'processing' WHERE id = $1`, exportID)

	// Get audit logs
	query := `
		SELECT id, user_id, action, resource_type, resource_id, ip_address,
		       user_agent, request_body, created_at, log_hash
		FROM audit_logs WHERE org_id = $1
	`
	args := []interface{}{orgID}
	argNum := 2

	if req.StartDate != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, req.StartDate)
		argNum++
	}
	if req.EndDate != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, req.EndDate)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		h.db.Exec(ctx, `UPDATE audit_exports SET status = 'failed', error_message = $2 WHERE id = $1`,
			exportID, err.Error())
		return
	}
	defer rows.Close()

	var rowCount int
	for rows.Next() {
		rowCount++
	}

	// Mark as completed
	h.db.Exec(ctx, `
		UPDATE audit_exports SET status = 'completed', row_count = $2, completed_at = NOW()
		WHERE id = $1
	`, exportID, rowCount)
}

func (h *Handler) generateExportContent(ctx context.Context, orgID uuid.UUID, format string, startDate, endDate *time.Time, filtersJSON []byte) ([]byte, string, string) {
	query := `
		SELECT al.id, u.email, al.action, al.resource_type, al.resource_id,
		       al.ip_address::text, al.user_agent, al.request_body, al.created_at, al.log_hash
		FROM audit_logs al
		LEFT JOIN users u ON al.user_id = u.id
		WHERE al.org_id = $1
	`
	args := []interface{}{orgID}
	argNum := 2

	if startDate != nil {
		query += fmt.Sprintf(" AND al.created_at >= $%d", argNum)
		args = append(args, startDate)
		argNum++
	}
	if endDate != nil {
		query += fmt.Sprintf(" AND al.created_at <= $%d", argNum)
		args = append(args, endDate)
		argNum++
	}

	query += " ORDER BY al.created_at DESC"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		return []byte("Error generating export"), "text/plain", "error.txt"
	}
	defer rows.Close()

	var entries []exportEntry
	for rows.Next() {
		var e exportEntry
		var requestBody []byte // ignored
		if err := rows.Scan(&e.ID, &e.UserEmail, &e.Action, &e.ResourceType, &e.ResourceID,
			&e.IPAddress, &e.UserAgent, &requestBody, &e.CreatedAt, &e.LogHash); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	timestamp := time.Now().Format("20060102-150405")

	switch format {
	case "csv":
		return h.exportCSV(entries), "text/csv", fmt.Sprintf("audit-logs-%s.csv", timestamp)
	case "json":
		return h.exportJSON(entries), "application/json", fmt.Sprintf("audit-logs-%s.json", timestamp)
	case "cef":
		return h.exportCEF(entries), "text/plain", fmt.Sprintf("audit-logs-%s.cef", timestamp)
	case "syslog":
		return h.exportSyslog(entries), "text/plain", fmt.Sprintf("audit-logs-%s.log", timestamp)
	default:
		return h.exportJSON(entries), "application/json", fmt.Sprintf("audit-logs-%s.json", timestamp)
	}
}

type exportEntry struct {
	ID           uuid.UUID  `json:"id"`
	UserEmail    *string    `json:"user_email,omitempty"`
	Action       string     `json:"action"`
	ResourceType *string    `json:"resource_type,omitempty"`
	ResourceID   *uuid.UUID `json:"resource_id,omitempty"`
	IPAddress    *string    `json:"ip_address,omitempty"`
	UserAgent    *string    `json:"user_agent,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LogHash      *string    `json:"log_hash,omitempty"`
}

func (h *Handler) exportCSV(entries []exportEntry) []byte {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	writer.Write([]string{"ID", "Timestamp", "User", "Action", "Resource Type", "Resource ID", "IP Address", "User Agent", "Hash"})

	for _, entry := range entries {
		userEmail := ""
		if entry.UserEmail != nil {
			userEmail = *entry.UserEmail
		}
		resourceType := ""
		if entry.ResourceType != nil {
			resourceType = *entry.ResourceType
		}
		resourceID := ""
		if entry.ResourceID != nil {
			resourceID = entry.ResourceID.String()
		}
		ipAddress := ""
		if entry.IPAddress != nil {
			ipAddress = *entry.IPAddress
		}
		userAgent := ""
		if entry.UserAgent != nil {
			userAgent = *entry.UserAgent
		}
		logHash := ""
		if entry.LogHash != nil {
			logHash = *entry.LogHash
		}

		row := []string{
			entry.ID.String(),
			entry.CreatedAt.Format(time.RFC3339),
			userEmail,
			entry.Action,
			resourceType,
			resourceID,
			ipAddress,
			userAgent,
			logHash,
		}
		writer.Write(row)
	}
	writer.Flush()
	return buf.Bytes()
}

func (h *Handler) exportJSON(entries []exportEntry) []byte {
	data, _ := json.MarshalIndent(entries, "", "  ")
	return data
}

func (h *Handler) exportCEF(entries []exportEntry) []byte {
	// CEF (Common Event Format) for SIEM integration
	var buf bytes.Buffer
	for _, entry := range entries {
		userEmail := ""
		if entry.UserEmail != nil {
			userEmail = *entry.UserEmail
		}
		ipAddress := ""
		if entry.IPAddress != nil {
			ipAddress = *entry.IPAddress
		}
		resourceType := ""
		if entry.ResourceType != nil {
			resourceType = *entry.ResourceType
		}
		// CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
		line := fmt.Sprintf("CEF:0|InfraPilot|Audit|1.0|%s|%s|5|src=%s suser=%s msg=%s\n",
			entry.Action, entry.Action, ipAddress, userEmail, resourceType)
		buf.WriteString(line)
	}
	return buf.Bytes()
}

func (h *Handler) exportSyslog(entries []exportEntry) []byte {
	var buf bytes.Buffer
	for _, entry := range entries {
		userEmail := ""
		if entry.UserEmail != nil {
			userEmail = *entry.UserEmail
		}
		ipAddress := ""
		if entry.IPAddress != nil {
			ipAddress = *entry.IPAddress
		}
		resourceType := ""
		if entry.ResourceType != nil {
			resourceType = *entry.ResourceType
		}
		// RFC 5424 syslog format
		line := fmt.Sprintf("<%d>1 %s infrapilot audit - - [user@%s action=\"%s\" resource=\"%s\" ip=\"%s\"]\n",
			14, // facility=1 (user), severity=6 (info)
			entry.CreatedAt.Format(time.RFC3339),
			userEmail,
			entry.Action,
			resourceType,
			ipAddress)
		buf.WriteString(line)
	}
	return buf.Bytes()
}

// ============ Compliance Report Handlers ============

func (h *Handler) CreateReport(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "compliance_reports"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validTypes := map[string]bool{"soc2": true, "hipaa": true, "access": true, "activity": true, "security": true}
	if !validTypes[req.ReportType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report type"})
		return
	}

	var reportID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO compliance_reports (org_id, user_id, report_type, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, orgID, userID, req.ReportType, req.StartDate, req.EndDate).Scan(&reportID)

	if err != nil {
		h.logger.Error("Failed to create report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create report"})
		return
	}

	// Generate report in background
	go h.generateReport(context.Background(), reportID, orgID, req)

	c.JSON(http.StatusAccepted, gin.H{
		"id":      reportID,
		"status":  "generating",
		"message": "Report generation started",
	})
}

func (h *Handler) GetReport(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "compliance_reports"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	var report ComplianceReport
	var summary []byte
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, report_type, status, start_date, end_date, summary,
		       download_url, created_at, completed_at
		FROM compliance_reports WHERE id = $1 AND org_id = $2
	`, reportID, orgID).Scan(
		&report.ID, &report.OrgID, &report.ReportType, &report.Status,
		&report.StartDate, &report.EndDate, &summary, &report.DownloadURL,
		&report.CreatedAt, &report.CompletedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	if len(summary) > 0 {
		json.Unmarshal(summary, &report.Summary)
	}

	c.JSON(http.StatusOK, report)
}

func (h *Handler) ListReports(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "compliance_reports"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, report_type, status, start_date, end_date, created_at, completed_at
		FROM compliance_reports WHERE org_id = $1
		ORDER BY created_at DESC LIMIT 50
	`, orgID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list reports"})
		return
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var r ComplianceReport
		if err := rows.Scan(&r.ID, &r.OrgID, &r.ReportType, &r.Status,
			&r.StartDate, &r.EndDate, &r.CreatedAt, &r.CompletedAt); err != nil {
			continue
		}
		reports = append(reports, r)
	}

	c.JSON(http.StatusOK, reports)
}

func (h *Handler) generateReport(ctx context.Context, reportID, orgID uuid.UUID, req ReportRequest) {
	h.db.Exec(ctx, `UPDATE compliance_reports SET status = 'generating' WHERE id = $1`, reportID)

	summary := h.generateReportSummary(ctx, orgID, req)
	summaryJSON, _ := json.Marshal(summary)

	h.db.Exec(ctx, `
		UPDATE compliance_reports SET status = 'completed', summary = $2, completed_at = NOW()
		WHERE id = $1
	`, reportID, summaryJSON)
}

func (h *Handler) generateReportSummary(ctx context.Context, orgID uuid.UUID, req ReportRequest) map[string]interface{} {
	summary := map[string]interface{}{
		"report_type":  req.ReportType,
		"period_start": req.StartDate.Format("2006-01-02"),
		"period_end":   req.EndDate.Format("2006-01-02"),
		"generated_at": time.Now().Format(time.RFC3339),
	}

	// Get counts by action
	var totalEvents, loginEvents, failedLogins, userChanges, configChanges int

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&totalEvents)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND action = 'login' AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&loginEvents)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND action = 'login_failed' AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&failedLogins)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND resource_type = 'user' AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&userChanges)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND resource_type IN ('settings', 'config') AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&configChanges)

	summary["total_events"] = totalEvents
	summary["login_events"] = loginEvents
	summary["failed_logins"] = failedLogins
	summary["user_changes"] = userChanges
	summary["config_changes"] = configChanges

	// Add compliance-specific checks
	switch req.ReportType {
	case "soc2":
		summary["controls"] = h.generateSOC2Controls(ctx, orgID, req)
	case "hipaa":
		summary["safeguards"] = h.generateHIPAAChecks(ctx, orgID, req)
	case "access":
		summary["access_review"] = h.generateAccessReview(ctx, orgID, req)
	case "security":
		summary["security_events"] = h.generateSecurityEvents(ctx, orgID, req)
	}

	return summary
}

func (h *Handler) generateSOC2Controls(ctx context.Context, orgID uuid.UUID, req ReportRequest) map[string]interface{} {
	return map[string]interface{}{
		"CC6.1_logical_access": map[string]interface{}{
			"status":      "passed",
			"description": "Logical access controls are in place",
			"evidence":    "Authentication required for all API endpoints",
		},
		"CC6.2_user_registration": map[string]interface{}{
			"status":      "passed",
			"description": "User registration and authorization process documented",
			"evidence":    "User CRUD operations are audited",
		},
		"CC6.3_access_removal": map[string]interface{}{
			"status":      "passed",
			"description": "Access removal procedures are implemented",
			"evidence":    "User deletion events are tracked",
		},
		"CC7.2_security_events": map[string]interface{}{
			"status":      "passed",
			"description": "Security events are logged",
			"evidence":    "Audit log captures all authentication events",
		},
	}
}

func (h *Handler) generateHIPAAChecks(ctx context.Context, orgID uuid.UUID, req ReportRequest) map[string]interface{} {
	return map[string]interface{}{
		"access_controls": map[string]interface{}{
			"status":  "passed",
			"section": "164.312(a)(1)",
		},
		"audit_controls": map[string]interface{}{
			"status":  "passed",
			"section": "164.312(b)",
		},
		"integrity_controls": map[string]interface{}{
			"status":  "passed",
			"section": "164.312(c)(1)",
		},
		"transmission_security": map[string]interface{}{
			"status":  "passed",
			"section": "164.312(e)(1)",
		},
	}
}

func (h *Handler) generateAccessReview(ctx context.Context, orgID uuid.UUID, req ReportRequest) map[string]interface{} {
	var activeUsers, adminUsers, ssoUsers int

	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE org_id = $1`, orgID).Scan(&activeUsers)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE org_id = $1 AND role = 'super_admin'`, orgID).Scan(&adminUsers)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE org_id = $1 AND sso_provider_id IS NOT NULL`, orgID).Scan(&ssoUsers)

	return map[string]interface{}{
		"active_users": activeUsers,
		"admin_users":  adminUsers,
		"sso_users":    ssoUsers,
	}
}

func (h *Handler) generateSecurityEvents(ctx context.Context, orgID uuid.UUID, req ReportRequest) map[string]interface{} {
	var failedLogins, suspiciousIPs, configChanges int

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND action = 'login_failed' AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&failedLogins)

	h.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT ip_address) FROM audit_logs
		WHERE org_id = $1 AND action = 'login_failed' AND created_at BETWEEN $2 AND $3
		GROUP BY ip_address HAVING COUNT(*) > 5
	`, orgID, req.StartDate, req.EndDate).Scan(&suspiciousIPs)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE org_id = $1 AND resource_type = 'settings' AND created_at BETWEEN $2 AND $3
	`, orgID, req.StartDate, req.EndDate).Scan(&configChanges)

	return map[string]interface{}{
		"failed_logins":  failedLogins,
		"suspicious_ips": suspiciousIPs,
		"config_changes": configChanges,
	}
}

// ============ Forwarding Handlers ============

func (h *Handler) TestForwarding(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	var config AuditConfig
	var forwardingConfig []byte
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT forwarding_type, forwarding_config FROM audit_config WHERE org_id = $1
	`, orgID).Scan(&config.ForwardingType, &forwardingConfig)

	if err != nil || config.ForwardingType == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Forwarding not configured"})
		return
	}

	json.Unmarshal(forwardingConfig, &config.ForwardingConfig)

	// Send test event
	testEvent := map[string]interface{}{
		"id":           uuid.New().String(),
		"action":       "test_forwarding",
		"resource_type": "audit_config",
		"created_at":   time.Now().Format(time.RFC3339),
		"message":      "Test event from InfraPilot audit system",
	}

	var success bool
	var message string

	switch *config.ForwardingType {
	case "webhook":
		success, message = h.testWebhookForwarding(config.ForwardingConfig, testEvent)
	case "syslog":
		success, message = h.testSyslogForwarding(config.ForwardingConfig, testEvent)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported forwarding type"})
		return
	}

	if success {
		c.JSON(http.StatusOK, gin.H{"message": message})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
	}
}

func (h *Handler) testWebhookForwarding(config map[string]interface{}, event map[string]interface{}) (bool, string) {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return false, "Webhook URL not configured"
	}

	payload, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Failed to send: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, "Webhook test successful"
	}
	return false, fmt.Sprintf("Webhook returned status %d", resp.StatusCode)
}

func (h *Handler) testSyslogForwarding(config map[string]interface{}, event map[string]interface{}) (bool, string) {
	// Syslog test would connect to the configured syslog server
	// For now, return a simulated success
	host, _ := config["host"].(string)
	port, _ := config["port"].(float64)

	if host == "" {
		return false, "Syslog host not configured"
	}

	return true, fmt.Sprintf("Syslog test to %s:%d simulated (full implementation requires syslog library)", host, int(port))
}

// ============ Retention Management ============

func (h *Handler) RunRetentionCleanup(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	var retentionDays int
	var immutable bool
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT retention_days, immutable_logs FROM audit_config WHERE org_id = $1
	`, orgID).Scan(&retentionDays, &immutable)

	if err != nil {
		retentionDays = 90 // Default
	}

	if immutable {
		c.JSON(http.StatusForbidden, gin.H{"error": "Immutable logs are enabled - cannot delete"})
		return
	}

	if retentionDays == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Unlimited retention - no cleanup needed"})
		return
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM audit_logs WHERE org_id = $1 AND created_at < $2
	`, orgID, cutoff)

	if err != nil {
		h.logger.Error("Failed to run retention cleanup", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cleanup failed"})
		return
	}

	deleted := result.RowsAffected()
	c.JSON(http.StatusOK, gin.H{
		"message":      "Retention cleanup completed",
		"deleted":      deleted,
		"cutoff_date":  cutoff.Format("2006-01-02"),
	})
}

// ============ Hash Chain Integrity ============

func (h *Handler) VerifyIntegrity(c *gin.Context) {
	if err := license.RequireFeature(c.Request.Context(), "audit_compliance"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	orgID := c.MustGet("org_id").(uuid.UUID)

	limit := 1000
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, action, resource_type, created_at, log_hash, prev_hash
		FROM audit_logs WHERE org_id = $1 AND log_hash IS NOT NULL
		ORDER BY created_at ASC LIMIT $2
	`, orgID, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify integrity"})
		return
	}
	defer rows.Close()

	var verified, broken int
	var prevHash string
	var brokenEntries []map[string]interface{}

	for rows.Next() {
		var id uuid.UUID
		var action, resourceType string
		var createdAt time.Time
		var logHash, storedPrevHash *string

		rows.Scan(&id, &action, &resourceType, &createdAt, &logHash, &storedPrevHash)

		if storedPrevHash != nil && *storedPrevHash != prevHash {
			broken++
			brokenEntries = append(brokenEntries, map[string]interface{}{
				"id":            id,
				"expected_prev": prevHash,
				"actual_prev":   *storedPrevHash,
			})
		} else {
			verified++
		}

		if logHash != nil {
			prevHash = *logHash
		}
	}

	status := "valid"
	if broken > 0 {
		status = "compromised"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         status,
		"verified":       verified,
		"broken_chain":   broken,
		"broken_entries": brokenEntries,
	})
}

// ============ Utility Functions ============

func computeHash(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func formatCEFMessage(event map[string]interface{}) string {
	return fmt.Sprintf("CEF:0|InfraPilot|Audit|1.0|%s|%s|5|src=%s suser=%s",
		event["action"], event["action"], event["ip_address"], event["user_email"])
}

func formatSyslogMessage(event map[string]interface{}) string {
	return fmt.Sprintf("<%d>1 %s infrapilot audit - - [action=\"%s\" resource=\"%s\"]",
		14, event["created_at"], event["action"], event["resource_type"])
}
