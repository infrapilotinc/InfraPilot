-- Backfill for a bug where domain/upstream_target/redirect_url could be saved with
-- leading/trailing whitespace (e.g. from a copy-paste), which then landed verbatim in
-- generated nginx config. Since upstream_target is interpolated into a `set $upstream
-- "..."` variable rather than a literal, nginx can't validate the resulting URI at
-- config-load time -- a trailing space silently produced a generic 500 at request time
-- instead of a normal 502, with no clue in the UI about why. The application layer now
-- trims these fields on every create/update; this just fixes rows already in the DB.
UPDATE proxy_hosts SET domain = TRIM(domain) WHERE domain != TRIM(domain);
UPDATE proxy_hosts SET upstream_target = TRIM(upstream_target) WHERE upstream_target IS NOT NULL AND upstream_target != TRIM(upstream_target);
UPDATE proxy_hosts SET redirect_url = TRIM(redirect_url) WHERE redirect_url IS NOT NULL AND redirect_url != TRIM(redirect_url);
