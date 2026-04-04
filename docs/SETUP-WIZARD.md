# Setup Wizard Plan

## Context

InfraPilot currently requires `LICENSE_KEY` as an env var before the server starts — if it's missing the server fatally exits. This is a bad experience for new installs. Instead, the app should start in "setup mode" and walk the user through a first-run wizard that collects the license key and creates the admin account.

A setup flow already partially exists (`/setup` page, `setup_handlers.go`, `/api/v1/setup/status` and `/api/v1/setup` endpoints) but only covers admin creation — no license step.

The `system_settings` table (migration 006) already exists and can store the license key persistently. On next restart, the server reads from DB if no env var is set.

---

## Startup Behavior (New)

```
main.go startup:
  if LICENSE_OFFLINE=true AND env != production → offline client (dev)
  else if LICENSE_KEY env is set → validate with infrapilot.org (as today)
  else → query system_settings for saved license key
    if found → validate with infrapilot.org
    if not found → use offline client (setup mode, all features enabled temporarily)
                   log warning: "No license key configured. Complete setup at the web UI."
```

After setup, the key is in `system_settings`. The in-memory offline client stays active for the current session (all features work). On next container restart, the server reads the DB key and validates properly with infrapilot.org.

---

## Wizard Flow (Frontend)

```
First run → /login checks setup status → setup_required: true → redirect to /setup

/setup page — Step 1: License Key
  [ IP-CE-XXXX-XXXX-XXXX        ]  [Validate →]
  ✓ Community Edition · 1 agent
  skip: if license_configured: true already (e.g. LICENSE_KEY env set)

/setup page — Step 2: Admin Account
  [ email ]  [ password ]  [ confirm ]  [Create Account →]

→ Redirect to dashboard
```

---

## Step 1: Backend — `setup_handlers.go`

### Updated `SetupStatusResponse`
```go
type SetupStatusResponse struct {
    SetupRequired     bool `json:"setup_required"`
    LicenseConfigured bool `json:"license_configured"`
    AdminCreated      bool `json:"admin_created"`
    UserCount         int  `json:"user_count"`
}
```

### Updated `getSetupStatus`
- Count users (existing)
- Check `system_settings` WHERE `setting_key = 'license_key'` AND `org_id = default_org`
- OR check if `cfg.LicenseKey != ""` (env var set) OR `LICENSE_OFFLINE=true`
- Set `LicenseConfigured` and `AdminCreated` accordingly
- `SetupRequired = !AdminCreated`

The handler needs access to `cfg` — pass it via the Handler struct (already has `h.db`, add `h.cfg`).

### New `setupLicense` handler
```
POST /api/v1/setup/license
Body: { "key": "IP-CE-XXXX-XXXX-XXXX" }
```
- Guard: if users exist → 400 "setup already completed"
- Call infrapilot.org `POST /api/license/validate` with the key + placeholder instance_id
- If valid → upsert into `system_settings`:
  - Ensure default org exists first (same pattern as `createInitialAdmin`)
  - `org_id = 00000000-0000-0000-0000-000000000001`
  - `setting_key = 'license_key'`
  - `setting_value = {"key": "IP-CE-...", "tier": "community", "max_agents": 1}`
- Return `{ valid: true, tier: "community", max_agents: 1, features: [...] }`
- If invalid → 400 with error message from infrapilot.org

### New `SetupLicenseRequest` struct
```go
type SetupLicenseRequest struct {
    Key string `json:"key" binding:"required"`
}
```

---

## Step 2: Backend — `handler.go`

Add route alongside existing setup routes:
```go
r.POST("/api/v1/setup/license", h.setupLicense)
```

Also add `cfg *config.Config` field to `Handler` struct and pass it in `NewHandler(...)`.

---

## Step 3: Backend — `main.go`

Replace the fatal-on-missing-key block with:
```go
var licenseClient *license.Client
if os.Getenv("LICENSE_OFFLINE") == "true" && cfg.Env != "production" {
    licenseClient = license.NewOfflineClient(logger)
} else if cfg.LicenseKey != "" {
    // ENV var set — validate immediately (existing behaviour)
    licenseClient, err = license.NewClient(cfg.LicenseKey, cfg.DataDir, version, logger)
    // ... validate, fatal on invalid
} else {
    // Try system_settings (DB)
    var savedKey string
    pool.QueryRow(ctx, `
        SELECT setting_value->>'key' FROM system_settings
        WHERE org_id = '00000000-0000-0000-0000-000000000001'
        AND setting_key = 'license_key'
    `).Scan(&savedKey)

    if savedKey != "" {
        licenseClient, err = license.NewClient(savedKey, cfg.DataDir, version, logger)
        if err == nil {
            resp, _ := licenseClient.Validate()
            if resp != nil && resp.Valid {
                logger.Info("License loaded from database", zap.String("tier", resp.Tier))
            }
        }
    }

    if licenseClient == nil {
        logger.Warn("No license key configured — starting in setup mode")
        licenseClient = license.NewOfflineClient(logger)
    }
}
```

---

## Step 4: Frontend — `lib/api.ts`

Update types:
```typescript
export interface SetupStatusResponse {
    setup_required: boolean;
    license_configured: boolean;
    admin_created: boolean;
    user_count: number;
}

export interface SetupLicenseResponse {
    valid: boolean;
    tier: string;
    max_agents: number;
    features: string[];
}
```

Add API method:
```typescript
setupLicense: (key: string) =>
    fetchAPI<SetupLicenseResponse>("/setup/license", {
        method: "POST",
        body: JSON.stringify({ key }),
    }),
```

---

## Step 5: Frontend — `app/(auth)/setup/page.tsx`

Full rewrite as two-step wizard. Key logic:

```typescript
// On mount: fetch setup status to determine starting step
useEffect(() => {
    api.getSetupStatus().then(status => {
        if (status.license_configured) setStep(2);
        else setStep(1);
    });
}, []);
```

**Step 1 UI:**
- Input for license key (`IP-CE-...`)
- Validate button → calls `api.setupLicense(key)`
- On success: show green badge "Community Edition · 1 agent ✓", then auto-advance to step 2
- On error: show red error from API response
- Link: "Don't have a key? Get one free at infrapilot.org/signup"

**Step 2 UI:**
- Existing admin creation form (email, password, confirm password)
- On success: store tokens → redirect to `/`

**Progress indicator at top:**
```
● License Key  ──  ○ Admin Account    (step 1)
✓ License Key  ──  ● Admin Account    (step 2)
```

---

## Critical Files

| File | Change |
|------|--------|
| `backend/internal/api/setup_handlers.go` | Add `setupLicense`, update `getSetupStatus`, add `cfg` field to Handler |
| `backend/internal/api/handler.go` | Add `POST /api/v1/setup/license` route; add `cfg` to Handler struct + NewHandler |
| `backend/cmd/server/main.go` | DB fallback for license key; no fatal on missing key |
| `frontend/app/(auth)/setup/page.tsx` | Full rewrite as multi-step wizard |
| `frontend/lib/api.ts` | Update types + add `setupLicense()` |

---

## Verification

1. Fresh install (no env vars): server starts → open browser → redirected to `/setup` → step 1 shows
2. Enter license key `IP-CE-DTSE-QPNW-WG7U` → "Community Edition ✓" shown → step 2 auto-opens
3. Create admin account → redirected to dashboard → logged in
4. `docker restart infrapilot` → server reads key from `system_settings` → validates with infrapilot.org → starts with real license
5. Existing install with `LICENSE_KEY` env set → `license_configured: true` → setup wizard skips to step 2 (admin creation only)
6. `LICENSE_OFFLINE=true` → `license_configured: true` → wizard skips to step 2
