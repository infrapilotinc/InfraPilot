-- v3/redeploy-stack: persist companion files + the default redeploy service selection on
-- the stack itself, so a stack can be redeployed without re-supplying everything each time.
ALTER TABLE stacks ADD COLUMN IF NOT EXISTS files jsonb;
ALTER TABLE stacks ADD COLUMN IF NOT EXISTS redeploy_services jsonb;

COMMENT ON COLUMN stacks.files IS 'Companion files (v3/45) supplied at create/redeploy time, stored so "keep same config" redeploys do not require re-upload. Same StackFile[] shape as the create request.';
COMMENT ON COLUMN stacks.redeploy_services IS 'Persisted default service-name selection for stack-level redeploy (json array of strings). NULL = all services.';
