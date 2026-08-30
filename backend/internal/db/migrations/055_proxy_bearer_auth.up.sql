-- Bearer token auth for proxies, parallel to the existing Basic Auth feature
-- (proxy_auth_users / basic_auth_enabled). Unlike Basic Auth, tokens are embedded as
-- literal values into the generated nginx config (chained `if` comparisons against
-- $http_authorization, since nginx's map directive -- the usual way to check a header
-- against a list of values -- is only legal at http{} scope, not inside the per-domain
-- server{} blocks this app generates), so they're stored encrypted (reversible) rather
-- than hashed (one-way) like Basic Auth passwords.
ALTER TABLE proxy_hosts ADD COLUMN IF NOT EXISTS bearer_auth_enabled BOOLEAN DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS proxy_bearer_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proxy_id        UUID NOT NULL REFERENCES proxy_hosts(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    token_encrypted TEXT NOT NULL,
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proxy_bearer_tokens_proxy_id ON proxy_bearer_tokens(proxy_id);
