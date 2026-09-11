-- Lets a webhook target a service that belongs to a Stack, instead of only a bare
-- free-typed service_name/environment string with no relationship to Stacks at all.
-- Nullable: a webhook can still target a genuinely standalone service (the older
-- service_configs/upsertService path), so this isn't a required migration of every
-- existing row.
ALTER TABLE webhook_configs ADD COLUMN IF NOT EXISTS stack_id UUID REFERENCES stacks(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_configs_stack_id ON webhook_configs(stack_id) WHERE stack_id IS NOT NULL;
