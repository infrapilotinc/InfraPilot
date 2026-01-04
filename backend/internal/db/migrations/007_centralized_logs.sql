-- Migration: 012_centralized_logs.sql
-- Description: Centralized log storage for SaaS disaster recovery
-- Created: 2026-01-03

-- ============================================================
-- Centralized Logs Table
-- Stores logs from all agents for persistence and querying
-- ============================================================

CREATE TABLE IF NOT EXISTS centralized_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,

    -- Log metadata
    source VARCHAR(255) NOT NULL,          -- Container name/ID or "nginx", "system"
    source_type VARCHAR(50) NOT NULL,      -- "container", "nginx", "system", "agent"
    stream VARCHAR(10) DEFAULT 'stdout',   -- "stdout" or "stderr"

    -- Log content
    level VARCHAR(20) DEFAULT 'info',      -- "debug", "info", "warn", "error", "fatal"
    message TEXT NOT NULL,

    -- Timestamps
    log_timestamp TIMESTAMPTZ NOT NULL,    -- Original log timestamp
    ingested_at TIMESTAMPTZ DEFAULT NOW(), -- When we received it

    -- Optional structured data
    labels JSONB DEFAULT '{}',             -- Container labels, etc.
    metadata JSONB DEFAULT '{}'            -- Additional context
);

-- ============================================================
-- Indexes for efficient querying
-- ============================================================

-- Primary query pattern: logs by org, agent, time range
CREATE INDEX IF NOT EXISTS idx_centralized_logs_org_time
    ON centralized_logs(org_id, log_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_centralized_logs_agent_time
    ON centralized_logs(agent_id, log_timestamp DESC);

-- Filter by source (container name)
CREATE INDEX IF NOT EXISTS idx_centralized_logs_source
    ON centralized_logs(org_id, source, log_timestamp DESC);

-- Filter by level (errors, warnings)
CREATE INDEX IF NOT EXISTS idx_centralized_logs_level
    ON centralized_logs(org_id, level, log_timestamp DESC);

-- Full-text search on message (optional, can be expensive)
CREATE INDEX IF NOT EXISTS idx_centralized_logs_message_search
    ON centralized_logs USING gin(to_tsvector('english', message));

-- Partition-ready: index on ingested_at for retention cleanup
CREATE INDEX IF NOT EXISTS idx_centralized_logs_ingested
    ON centralized_logs(ingested_at);

-- ============================================================
-- Log Retention Configuration (per org)
-- ============================================================

CREATE TABLE IF NOT EXISTS log_retention_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,

    -- Retention settings
    retention_days INT DEFAULT 30,         -- How long to keep logs
    max_storage_mb INT DEFAULT 1000,       -- Max storage per org (soft limit)

    -- Feature flags
    enabled BOOLEAN DEFAULT true,          -- Log persistence enabled
    compress_after_days INT DEFAULT 7,     -- Compress logs older than N days

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- Log Ingestion Stats (for monitoring/billing)
-- ============================================================

CREATE TABLE IF NOT EXISTS log_ingestion_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,

    -- Daily stats
    date DATE NOT NULL,
    log_count BIGINT DEFAULT 0,
    bytes_ingested BIGINT DEFAULT 0,

    -- By level breakdown
    error_count BIGINT DEFAULT 0,
    warn_count BIGINT DEFAULT 0,
    info_count BIGINT DEFAULT 0,
    debug_count BIGINT DEFAULT 0,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(org_id, agent_id, date)
);

-- Index for querying stats
CREATE INDEX IF NOT EXISTS idx_log_ingestion_stats_org_date
    ON log_ingestion_stats(org_id, date DESC);

-- ============================================================
-- Row-Level Security
-- ============================================================

-- Enable RLS on logs table
ALTER TABLE centralized_logs ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only see logs from their organization
DROP POLICY IF EXISTS centralized_logs_org_isolation ON centralized_logs;
CREATE POLICY centralized_logs_org_isolation ON centralized_logs
    FOR ALL
    USING (org_id = current_setting('app.current_org_id', true)::uuid);

-- Enable RLS on retention config
ALTER TABLE log_retention_config ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS log_retention_config_org_isolation ON log_retention_config;
CREATE POLICY log_retention_config_org_isolation ON log_retention_config
    FOR ALL
    USING (org_id = current_setting('app.current_org_id', true)::uuid);

-- Enable RLS on stats
ALTER TABLE log_ingestion_stats ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS log_ingestion_stats_org_isolation ON log_ingestion_stats;
CREATE POLICY log_ingestion_stats_org_isolation ON log_ingestion_stats
    FOR ALL
    USING (org_id = current_setting('app.current_org_id', true)::uuid);

-- ============================================================
-- Helper Functions
-- ============================================================

-- Function to get storage usage for an org
DROP FUNCTION IF EXISTS get_org_log_storage(UUID);
CREATE FUNCTION get_org_log_storage(p_org_id UUID)
RETURNS BIGINT AS $$
    SELECT COALESCE(SUM(pg_column_size(message) + pg_column_size(metadata)), 0)
    FROM centralized_logs
    WHERE org_id = p_org_id;
$$ LANGUAGE SQL STABLE;

-- Function to cleanup old logs based on retention
DROP FUNCTION IF EXISTS cleanup_old_logs(UUID);
CREATE FUNCTION cleanup_old_logs(p_org_id UUID DEFAULT NULL)
RETURNS INT AS $$
DECLARE
    deleted_count INT := 0;
    config RECORD;
BEGIN
    FOR config IN
        SELECT org_id, retention_days
        FROM log_retention_config
        WHERE enabled = true
        AND (p_org_id IS NULL OR org_id = p_org_id)
    LOOP
        DELETE FROM centralized_logs
        WHERE org_id = config.org_id
        AND log_timestamp < NOW() - (config.retention_days || ' days')::INTERVAL;

        deleted_count := deleted_count + (SELECT COUNT(*) FROM centralized_logs WHERE false);
    END LOOP;

    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Default retention config for existing orgs
-- ============================================================

INSERT INTO log_retention_config (org_id, retention_days, max_storage_mb)
SELECT id, 30, 1000
FROM organizations
WHERE id NOT IN (SELECT org_id FROM log_retention_config)
ON CONFLICT (org_id) DO NOTHING;
