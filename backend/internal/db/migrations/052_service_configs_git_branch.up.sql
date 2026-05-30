-- A git-backed service remembers its branch so "redeploy" can rebuild from source.
ALTER TABLE service_configs ADD COLUMN IF NOT EXISTS git_branch TEXT;
