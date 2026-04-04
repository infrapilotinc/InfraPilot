# Stack Deployments

Deploy multiple services from a `docker-compose.yml` file as a unified stack, with per-service tracking, dependency ordering, and variable/env-file support.

---

## Overview

- Upload or paste a `docker-compose.yml` in the UI wizard
- Backend parses services, networks, and volumes
- Each service gets an individual deployment record (tracked separately)
- Services are started in `depends_on` order
- Stack status rolls up from individual service statuses

---

## Database Schema

```sql
-- backend/internal/db/migrations/041_stacks.up.sql

CREATE TYPE stack_status AS ENUM (
    'pending', 'deploying', 'running', 'partial', 'failed', 'stopped'
);

CREATE TABLE stacks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id),
    agent_id UUID NOT NULL REFERENCES agents(id),
    name VARCHAR(255) NOT NULL,
    environment VARCHAR(50) NOT NULL CHECK (environment IN ('dev', 'staging', 'prod')),
    compose_yaml TEXT NOT NULL,
    variables JSONB DEFAULT '{}',
    service_count INT DEFAULT 0,
    running_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,
    status stack_status DEFAULT 'pending',
    status_message TEXT,
    deployed_by UUID REFERENCES users(id),
    deployed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Link deployments to stacks
ALTER TABLE deployments ADD COLUMN stack_id UUID REFERENCES stacks(id);
ALTER TABLE deployments ADD COLUMN service_order INT DEFAULT 0;
```

---

## API Endpoints

**File:** `backend/internal/api/stack_handlers.go`

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/agents/{id}/stacks/parse` | Parse & validate compose YAML, return preview |
| POST | `/agents/{id}/stacks` | Create stack and trigger deployment |
| GET | `/agents/{id}/stacks` | List stacks |
| GET | `/agents/{id}/stacks/{sid}` | Get stack with service deployments |
| GET | `/agents/{id}/stacks/{sid}/progress` | Deployment progress (polling) |
| DELETE | `/agents/{id}/stacks/{sid}` | Delete stack and stop containers |

### Request Types

```go
type CreateStackRequest struct {
    Name        string            `json:"name" binding:"required"`
    Environment string            `json:"environment" binding:"required"`
    ComposeYAML string            `json:"compose_yaml" binding:"required"`
    Variables   map[string]string `json:"variables,omitempty"`
    Overrides   []ServiceOverride `json:"overrides,omitempty"`
}

type ServiceOverride struct {
    ServiceName  string            `json:"service_name"`
    TagOverride  *string           `json:"tag_override,omitempty"`
    EnvOverrides map[string]string `json:"env_overrides,omitempty"`
    Enabled      bool              `json:"enabled"`
}

type ParsedCompose struct {
    Services []ComposeService `json:"services"`
    Networks []ComposeNetwork `json:"networks"`
    Volumes  []ComposeVolume  `json:"volumes"`
}

type ComposeService struct {
    Name        string            `json:"name"`
    Image       string            `json:"image"`
    DependsOn   []string          `json:"depends_on,omitempty"`
    EnvFiles    []string          `json:"env_files,omitempty"`
    // ... ports, volumes, networks, etc.
}
```

---

## Deployment Pipeline

```
1. Parse YAML with variable substitution
2. Topological sort by depends_on
3. Create stack record + all deployment records (status: pending)
4. Async — for each service in order:
   - Wait for dependencies to reach "running"
   - Deploy container via agent
   - Update stack counts
5. Final stack status: running / partial / failed
```

No agent changes required. The existing `RunContainerExtended()` handles networks, volumes, environment variables, and labels.

---

## Variable Substitution

Supported patterns:

| Pattern | Meaning |
|---------|---------|
| `${VAR}` | Required variable (error if unset) |
| `${VAR:-default}` | Use `default` if unset or empty |
| `${VAR-default}` | Use `default` only if undefined |

Common use:
```yaml
services:
  app:
    image: ${REGISTRY:-ghcr.io/myorg}/myapp:${TAG:-latest}
```

---

## Per-Service Environment Files (`env_file`)

When a service declares `env_file` in compose, the wizard shows a dedicated upload/paste section for that service in the Services step.

```yaml
services:
  app:
    image: myapp:latest
    env_file: .env.production
```

**How it works:**

1. `parseCompose` returns `env_files: []string` per `ComposeService`
2. The wizard Services step shows an upload/textarea for services with `env_files`
3. User pastes `.env` content → parsed into `KEY=VALUE` pairs
4. Stored in `ServiceOverride.EnvOverrides` for that service only
5. On deploy, per-service env vars are merged into `DeploymentContainerConfig.EnvVars`

Services without `env_file` in compose do not show the env upload section.

---

## Frontend Wizard

**Component:** `frontend/components/StackDeployWizard.tsx`

5-step wizard:

| Step | Description |
|------|-------------|
| 1. YAML | Upload or paste `docker-compose.yml`, syntax validation |
| 2. Variables | Set global substitution variables (`${REGISTRY}`, `${TAG}`, etc.) |
| 3. Services | Toggle services, per-service tag override, per-service env files |
| 4. Resources | Review networks and volumes to be created |
| 5. Review | Summary, deploy button, live progress tracking |

**Frontend types** (`frontend/lib/api.ts`):

```typescript
interface Stack {
  id: string;
  name: string;
  environment: string;
  status: "pending" | "deploying" | "running" | "partial" | "failed" | "stopped";
  service_count: number;
  running_count: number;
  failed_count: number;
  deployments?: Deployment[];
}
```

---

## Key Files

| File | Purpose |
|------|---------|
| `backend/internal/api/stack_handlers.go` | Stack API handlers |
| `backend/internal/db/migrations/041_stacks.up.sql` | Database schema |
| `frontend/components/StackDeployWizard.tsx` | Deploy wizard component |
| `frontend/app/(dashboard)/docker/stacks/page.tsx` | Stacks list page |
| `frontend/lib/api.ts` | Stack types and API functions |
