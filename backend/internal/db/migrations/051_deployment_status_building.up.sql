-- Push-to-deploy (source build): add a 'building' deployment status + a build_log column.
-- PostgreSQL has no IF NOT EXISTS for ADD VALUE, so guard on pg_enum (mirrors 023).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumlabel = 'building'
          AND enumtypid = (SELECT oid FROM pg_type WHERE typname = 'deployment_status')
    ) THEN
        ALTER TYPE deployment_status ADD VALUE 'building' BEFORE 'scanning';
    END IF;
END$$;

-- Captured build output (stdout/stderr) for source builds; written on success and failure.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS build_log TEXT;
