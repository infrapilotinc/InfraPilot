-- Migration: 006_system_settings
-- Description: System-wide settings including InfraPilot domain configuration

-- System settings table (key-value store for global config)
CREATE TABLE system_settings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    setting_key     VARCHAR(100) NOT NULL,
    setting_value   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(org_id, setting_key)
);

CREATE INDEX idx_system_settings_org ON system_settings(org_id);
CREATE INDEX idx_system_settings_key ON system_settings(setting_key);

-- Trigger for updated_at
CREATE TRIGGER update_system_settings_updated_at
    BEFORE UPDATE ON system_settings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add is_system_proxy column to proxy_hosts to identify InfraPilot's own proxy
ALTER TABLE proxy_hosts ADD COLUMN IF NOT EXISTS is_system_proxy BOOLEAN DEFAULT FALSE;

-- Index for quick lookup of system proxy
CREATE INDEX idx_proxy_hosts_system ON proxy_hosts(agent_id, is_system_proxy) WHERE is_system_proxy = TRUE;
