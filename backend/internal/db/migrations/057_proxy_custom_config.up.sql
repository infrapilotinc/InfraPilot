-- Free-form nginx directives per proxy host (e.g. client_max_body_size 12m;), written into
-- each server block that serves traffic. Stored here so they survive config regeneration,
-- unlike hand edits to the generated file.
ALTER TABLE proxy_hosts ADD COLUMN IF NOT EXISTS custom_config TEXT NOT NULL DEFAULT '';
