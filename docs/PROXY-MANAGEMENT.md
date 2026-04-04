# Implementation Plan: Professional Proxy Config Form UI

## Summary
Transform the proxy config tab from read-only raw config display to a professional form-based UI that:
1. Allows non-technical users to configure proxies via form fields
2. Shows real-time nginx config preview as fields are edited
3. Tests config before applying and shows errors
4. Supports Basic Auth enable/disable with htpasswd management
5. Scans existing configs to populate form fields

---

## Phase 1: Database Schema Updates

### Migration File (NEW)
**File:** `backend/internal/db/migrations/034_proxy_basic_auth.up.sql`

```sql
-- Basic auth configuration for proxies
ALTER TABLE proxy_hosts ADD COLUMN IF NOT EXISTS basic_auth_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE proxy_hosts ADD COLUMN IF NOT EXISTS basic_auth_realm VARCHAR(255) DEFAULT 'Restricted';

-- Basic auth users table
CREATE TABLE IF NOT EXISTS proxy_auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proxy_id UUID NOT NULL REFERENCES proxy_hosts(id) ON DELETE CASCADE,
    username VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL, -- bcrypt hash for htpasswd
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(proxy_id, username)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_proxy_auth_users_proxy_id ON proxy_auth_users(proxy_id);
```

---

## Phase 2: Backend API Updates

### 2.1 Proxy Handlers Updates
**File:** `backend/internal/api/proxies_handlers.go`

**Add to ProxyHost struct:**
```go
BasicAuthEnabled bool   `json:"basic_auth_enabled"`
BasicAuthRealm   string `json:"basic_auth_realm,omitempty"`
```

**Add to CreateProxyRequest/UpdateProxyRequest:**
```go
BasicAuthEnabled *bool   `json:"basic_auth_enabled,omitempty"`
BasicAuthRealm   *string `json:"basic_auth_realm,omitempty"`
```

**Update SQL queries** to include new columns.

**Update generateNginxConfig():**
- Add basic auth location block when enabled
- Generate htpasswd file path reference

### 2.2 New Basic Auth Handlers (NEW)
**File:** `backend/internal/api/basicauth_handlers.go`

```go
// Routes to add in handler.go:
// GET    /agents/:id/proxies/:pid/auth-users     - List auth users
// POST   /agents/:id/proxies/:pid/auth-users     - Create auth user
// DELETE /agents/:id/proxies/:pid/auth-users/:uid - Delete auth user

type AuthUser struct {
    ID        uuid.UUID `json:"id"`
    ProxyID   uuid.UUID `json:"proxy_id"`
    Username  string    `json:"username"`
    CreatedAt time.Time `json:"created_at"`
}

type CreateAuthUserRequest struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required,min=6"`
}

// Handler functions:
func (h *Handler) listAuthUsers(c *gin.Context)
func (h *Handler) createAuthUser(c *gin.Context)
func (h *Handler) deleteAuthUser(c *gin.Context)
func (h *Handler) generateHtpasswd(proxyID uuid.UUID) (string, error) // bcrypt format
```

### 2.3 Config Test Endpoint Enhancement
**File:** `backend/internal/api/proxies_handlers.go`

**Enhance testProxyConfig():**
- Accept optional config preview in request body
- Test the preview config instead of current saved config
- Return detailed error messages on failure

```go
type TestConfigRequest struct {
    ConfigPreview string `json:"config_preview,omitempty"` // Optional: test this config
}

type TestConfigResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
    Error   string `json:"error,omitempty"`
    Line    int    `json:"error_line,omitempty"` // Line number of error
}
```

### 2.4 Config Preview Endpoint (NEW)
**File:** `backend/internal/api/proxies_handlers.go`

```go
// POST /agents/:id/proxies/:pid/config/preview
// Generates nginx config from form values WITHOUT saving

type ConfigPreviewRequest struct {
    Domain           string `json:"domain"`
    UpstreamTarget   string `json:"upstream_target"`
    SSLEnabled       bool   `json:"ssl_enabled"`
    ForceSSL         bool   `json:"force_ssl"`
    HTTP2Enabled     bool   `json:"http2_enabled"`
    IncludeWWW       bool   `json:"include_www"`
    BasicAuthEnabled bool   `json:"basic_auth_enabled"`
    BasicAuthRealm   string `json:"basic_auth_realm"`
    // Security headers
    HSTSEnabled      bool   `json:"hsts_enabled"`
    HSTSMaxAge       int    `json:"hsts_max_age"`
    XFrameOptions    string `json:"x_frame_options"`
    // ... other fields
}

func (h *Handler) previewProxyConfig(c *gin.Context) {
    // Generate config from request values
    // Return raw nginx config string
}
```

---

## Phase 3: Agent Updates

### 3.1 Htpasswd File Management
**File:** `agent/internal/nginx/controller.go`

```go
// WriteHtpasswdFile writes htpasswd content for a proxy
func (c *Controller) WriteHtpasswdFile(domain string, content string) error {
    htpasswdPath := filepath.Join(c.configPath, ".htpasswd_"+sanitizeFilename(domain))
    return os.WriteFile(htpasswdPath, []byte(content), 0644)
}

// DeleteHtpasswdFile removes htpasswd file when auth is disabled
func (c *Controller) DeleteHtpasswdFile(domain string) error
```

### 3.2 gRPC Command Updates
**File:** `backend/internal/grpc/service.go`

Add htpasswd content to NginxCommand:
```go
type NginxCommand struct {
    // ... existing fields
    HtpasswdContent string `json:"htpasswd_content,omitempty"`
}
```

---

## Phase 4: Frontend Updates

### 4.1 API Types
**File:** `frontend/lib/api.ts`

```typescript
// Add to ProxyHost interface
basic_auth_enabled: boolean;
basic_auth_realm?: string;

// New interfaces
export interface AuthUser {
  id: string;
  proxy_id: string;
  username: string;
  created_at: string;
}

export interface ConfigPreviewRequest {
  domain: string;
  upstream_target: string;
  ssl_enabled: boolean;
  force_ssl: boolean;
  http2_enabled: boolean;
  include_www: boolean;
  basic_auth_enabled: boolean;
  basic_auth_realm: string;
  hsts_enabled: boolean;
  hsts_max_age: number;
  x_frame_options: string;
  x_content_type_options: boolean;
  x_xss_protection: boolean;
  content_security_policy?: string;
}

// New API functions
export async function listAuthUsers(agentId: string, proxyId: string): Promise<AuthUser[]>
export async function createAuthUser(agentId: string, proxyId: string, data: {username: string, password: string}): Promise<AuthUser>
export async function deleteAuthUser(agentId: string, proxyId: string, userId: string): Promise<void>
export async function previewProxyConfig(agentId: string, proxyId: string, config: ConfigPreviewRequest): Promise<{config: string}>
export async function testProxyConfig(agentId: string, proxyId: string, configPreview?: string): Promise<{success: boolean, message: string, error?: string}>
```

### 4.2 New ProxyConfigForm Component (NEW)
**File:** `frontend/components/ProxyConfigForm.tsx`

Professional form component with sections:

```tsx
interface ProxyConfigFormProps {
  proxy: ProxyHost;
  agentId: string;
  securityHeaders: SecurityHeaders;
  onSave: (data: UpdateProxyData) => Promise<void>;
}

export function ProxyConfigForm({ proxy, agentId, securityHeaders, onSave }: ProxyConfigFormProps) {
  // Form state
  const [formData, setFormData] = useState({...});
  const [configPreview, setConfigPreview] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [authUsers, setAuthUsers] = useState<AuthUser[]>([]);

  // Debounced config preview generation
  useEffect(() => {
    const timer = setTimeout(() => generatePreview(), 500);
    return () => clearTimeout(timer);
  }, [formData]);

  return (
    <div className="space-y-6">
      {/* Section 1: Basic Settings */}
      <FormSection title="Basic Settings" icon={Globe}>
        <Input label="Domain" value={formData.domain} ... />
        <Input label="Upstream Target" value={formData.upstream_target} ... />
        <div className="grid grid-cols-3 gap-4">
          <Toggle label="SSL/HTTPS" checked={formData.ssl_enabled} ... />
          <Toggle label="Force HTTPS" checked={formData.force_ssl} ... />
          <Toggle label="HTTP/2" checked={formData.http2_enabled} ... />
        </div>
        <Toggle label="Include www subdomain" checked={formData.include_www} ... />
      </FormSection>

      {/* Section 2: Basic Authentication */}
      <FormSection title="Basic Authentication" icon={Lock}>
        <Toggle
          label="Enable Basic Auth"
          checked={formData.basic_auth_enabled}
          description="Require username/password to access this proxy"
        />
        {formData.basic_auth_enabled && (
          <>
            <Input label="Auth Realm" value={formData.basic_auth_realm} ... />
            <AuthUsersManager
              users={authUsers}
              onAdd={handleAddUser}
              onDelete={handleDeleteUser}
            />
          </>
        )}
      </FormSection>

      {/* Section 3: Security Headers */}
      <FormSection title="Security Headers" icon={Shield}>
        <Toggle label="HSTS" checked={formData.hsts_enabled} ... />
        {formData.hsts_enabled && (
          <Input type="number" label="Max Age (seconds)" value={formData.hsts_max_age} ... />
        )}
        <Select label="X-Frame-Options" value={formData.x_frame_options} options={...} />
        <Toggle label="X-Content-Type-Options" checked={formData.x_content_type_options} ... />
        <Toggle label="X-XSS-Protection" checked={formData.x_xss_protection} ... />
        <Textarea label="Content-Security-Policy" value={formData.csp} ... />
      </FormSection>

      {/* Section 4: Config Preview */}
      <FormSection title="Nginx Configuration Preview" icon={Code} collapsible defaultOpen={false}>
        <div className="relative">
          <pre className="bg-gray-900 text-green-400 p-4 rounded-lg text-sm overflow-auto max-h-96">
            {configPreview}
          </pre>
          <CopyButton content={configPreview} />
        </div>
      </FormSection>

      {/* Test & Save Actions */}
      <div className="flex items-center justify-between border-t pt-4">
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={handleTestConfig} loading={testing}>
            <FlaskConical className="w-4 h-4 mr-2" />
            Test Configuration
          </Button>
          {testResult && (
            <TestResultBadge result={testResult} />
          )}
        </div>
        <Button onClick={handleSave} loading={saving} disabled={testResult?.success === false}>
          Save Changes
        </Button>
      </div>

      {/* Error Display */}
      {testResult?.error && (
        <Alert variant="error">
          <AlertTitle>Configuration Error</AlertTitle>
          <AlertDescription>
            {testResult.error}
            {testResult.error_line && ` (Line ${testResult.error_line})`}
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
```

### 4.3 Update Proxies Page
**File:** `frontend/app/(dashboard)/proxies/page.tsx`

Replace the existing config tab content with the new form:

```tsx
// In renderPanelContent() switch statement, case "config":
case "config":
  return (
    <ProxyConfigForm
      proxy={selectedProxy}
      agentId={selectedAgent}
      securityHeaders={securityHeaders}
      onSave={handleUpdateProxy}
    />
  );
```

---

## Phase 5: Nginx Config Generation Updates

### Update generateNginxConfig()
**File:** `backend/internal/api/proxies_handlers.go`

Add basic auth block:
```go
// Inside location block, when basic_auth_enabled:
if proxy.BasicAuthEnabled {
    config += fmt.Sprintf("        auth_basic \"%s\";\n", proxy.BasicAuthRealm)
    config += fmt.Sprintf("        auth_basic_user_file /etc/nginx/conf.d/.htpasswd_%s;\n",
        sanitizeFilename(proxy.Domain))
}
```

### Update dispatchProxyConfig()
Send htpasswd content along with config:
```go
func (h *Handler) dispatchProxyConfig(..., htpasswdContent string) {
    // Include htpasswd in command
    cmd.HtpasswdContent = htpasswdContent
}
```

---

## Implementation Order

1. **Database migration** - Add basic auth columns and users table
2. **Backend API** - Basic auth handlers, config preview, enhanced test
3. **Agent** - Htpasswd file management
4. **Frontend API types** - New interfaces and functions
5. **Frontend component** - ProxyConfigForm with all sections
6. **Integration** - Replace config tab with new form
7. **Testing** - Full flow testing

---

## UI/UX Design

### Form Section Component
```
┌─────────────────────────────────────────────────┐
│ 🌐 Basic Settings                            [-]│
├─────────────────────────────────────────────────┤
│ Domain          [example.com               ]    │
│ Upstream        [http://backend:3000       ]    │
│                                                 │
│ ☑ SSL/HTTPS   ☑ Force HTTPS   ☑ HTTP/2        │
│ ☐ Include www subdomain                        │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ 🔒 Basic Authentication                      [-]│
├─────────────────────────────────────────────────┤
│ ☑ Enable Basic Auth                            │
│                                                 │
│ Auth Realm      [Restricted Area           ]    │
│                                                 │
│ Authorized Users:                               │
│ ┌─────────────────────────────────────────────┐│
│ │ 👤 admin                          [Delete] ││
│ │ 👤 viewer                         [Delete] ││
│ └─────────────────────────────────────────────┘│
│ [+ Add User]                                   │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ 🛡️ Security Headers                          [-]│
├─────────────────────────────────────────────────┤
│ ☑ HSTS             Max Age: [31536000    ] sec │
│ X-Frame-Options:   [SAMEORIGIN        ▼]       │
│ ☑ X-Content-Type-Options (nosniff)             │
│ ☑ X-XSS-Protection                             │
│ Content-Security-Policy:                        │
│ [default-src 'self'...                     ]   │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ </> Nginx Configuration Preview              [+]│
├─────────────────────────────────────────────────┤
│ server {                                        │
│     listen 443 ssl http2;                       │
│     server_name example.com;                    │
│     ...                                        │
│ }                                              │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ [🧪 Test Configuration]     ✅ Config Valid    │
│                                    [💾 Save]   │
└─────────────────────────────────────────────────┘
```

---

## Verification Checklist

1. [ ] Form fields properly populate from existing proxy data
2. [ ] Config preview updates in real-time as fields change
3. [ ] Test config shows success/failure with error details
4. [ ] Cannot save if config test fails
5. [ ] Basic auth users can be added/removed
6. [ ] Htpasswd file is created/updated on agent
7. [ ] Existing basic auth configs are detected and loaded
8. [ ] All security headers work correctly
9. [ ] Config preview matches actual saved config
10. [ ] Nginx reloads successfully after save
