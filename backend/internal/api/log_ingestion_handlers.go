package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PersistentLogEntry represents a single log entry for ingestion
type PersistentLogEntry struct {
	Source     string            `json:"source"`
	SourceType string            `json:"source_type"`
	Stream     string            `json:"stream,omitempty"`
	Level      string            `json:"level,omitempty"`
	Message    string            `json:"message"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// PersistentLogBatch represents a batch of log entries for bulk ingestion
type PersistentLogBatch struct {
	AgentID string               `json:"agent_id" binding:"required"`
	Entries []PersistentLogEntry `json:"entries" binding:"required"`
}

// IngestLogs handles bulk log ingestion from agents
// POST /api/v1/logs/ingest
func (h *Handler) IngestLogs(c *gin.Context) {
	var batch PersistentLogBatch
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(batch.Entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no log entries provided"})
		return
	}

	agentID, err := uuid.Parse(batch.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}

	var orgID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT org_id FROM agents WHERE id = $1
	`, agentID).Scan(&orgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Check if log persistence is enabled for this org
	var enabled bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT enabled FROM log_retention_config WHERE org_id = $1
	`, orgID).Scan(&enabled)
	if err != nil || !enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "log persistence not enabled for this organization"})
		return
	}

	var errorCount, warnCount, infoCount, debugCount int64
	var totalBytes int64

	dbCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	valueStrings := make([]string, 0, len(batch.Entries))
	valueArgs := make([]any, 0, len(batch.Entries)*10)
	argNum := 1

	for _, entry := range batch.Entries {
		if entry.Stream == "" {
			entry.Stream = "stdout"
		}
		if entry.Level == "" {
			entry.Level = "info"
		}
		if entry.SourceType == "" {
			entry.SourceType = "container"
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now()
		}

		message := strings.ReplaceAll(entry.Message, "\x00", "")

		valueStrings = append(valueStrings, "($"+strconv.Itoa(argNum)+", $"+strconv.Itoa(argNum+1)+", $"+strconv.Itoa(argNum+2)+", $"+strconv.Itoa(argNum+3)+", $"+strconv.Itoa(argNum+4)+", $"+strconv.Itoa(argNum+5)+", $"+strconv.Itoa(argNum+6)+", $"+strconv.Itoa(argNum+7)+", $"+strconv.Itoa(argNum+8)+", $"+strconv.Itoa(argNum+9)+")")
		valueArgs = append(valueArgs, orgID, agentID, entry.Source, entry.SourceType, entry.Stream, entry.Level, message, entry.Timestamp, entry.Labels, entry.Metadata)
		argNum += 10

		switch entry.Level {
		case "error", "fatal":
			errorCount++
		case "warn", "warning":
			warnCount++
		case "info":
			infoCount++
		case "debug", "trace":
			debugCount++
		}
		totalBytes += int64(len(entry.Message))
	}

	insertedCount := 0
	if len(valueStrings) > 0 {
		query := `INSERT INTO centralized_logs (org_id, agent_id, source, source_type, stream, level, message, log_timestamp, labels, metadata) VALUES ` + strings.Join(valueStrings, ", ")
		result, err := h.db.Exec(dbCtx, query, valueArgs...)
		if err != nil {
			h.logger.Error("Failed to insert log entries", zap.Error(err), zap.Int("batch_size", len(batch.Entries)))
		} else {
			insertedCount = int(result.RowsAffected())
		}
	}

	if insertedCount > 0 {
		today := time.Now().Format("2006-01-02")
		_, err = h.db.Exec(c.Request.Context(), `
			INSERT INTO log_ingestion_stats (org_id, agent_id, date, log_count, bytes_ingested, error_count, warn_count, info_count, debug_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (org_id, agent_id, date) DO UPDATE SET
				log_count = log_ingestion_stats.log_count + EXCLUDED.log_count,
				bytes_ingested = log_ingestion_stats.bytes_ingested + EXCLUDED.bytes_ingested,
				error_count = log_ingestion_stats.error_count + EXCLUDED.error_count,
				warn_count = log_ingestion_stats.warn_count + EXCLUDED.warn_count,
				info_count = log_ingestion_stats.info_count + EXCLUDED.info_count,
				debug_count = log_ingestion_stats.debug_count + EXCLUDED.debug_count,
				updated_at = NOW()
		`, orgID, agentID, today, insertedCount, totalBytes, errorCount, warnCount, infoCount, debugCount)
		if err != nil {
			h.logger.Error("Failed to update ingestion stats", zap.Error(err))
		}
	}

	h.logger.Debug("Ingested logs",
		zap.String("agent_id", batch.AgentID),
		zap.Int("count", insertedCount),
		zap.Int("total", len(batch.Entries)),
	)

	c.JSON(http.StatusOK, gin.H{
		"ingested": insertedCount,
		"total":    len(batch.Entries),
		"status":   "ok",
	})
}

// nullString converts an empty string to nil for nullable DB columns
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
