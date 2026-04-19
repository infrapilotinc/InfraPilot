# Docker Compose Stacks

InfraPilot CE lets you deploy, update, and tear down Docker Compose stacks directly from the dashboard — no terminal required.

## Deploying a Stack

1. Go to **Docker → Stacks**
2. Click **Deploy Stack**
3. Give the stack a **name** (used as a prefix for container names)
4. Paste or upload your `docker-compose.yml` content
5. Add any required **environment variables** (see below)
6. Click **Deploy**

InfraPilot sends the Compose file and variables to the agent, which runs `docker compose up -d` on the host.

## Environment Variables

### Inline Variables

You can define variables directly in the stack deployment form. These are injected into the Compose file during deployment, replacing `${VAR_NAME}` and `$VAR_NAME` placeholders.

Example Compose snippet:
```yaml
services:
  app:
    image: myapp:${VERSION:-latest}
    environment:
      DATABASE_URL: ${DATABASE_URL}
      SECRET_KEY: ${SECRET_KEY}
```

Add `VERSION`, `DATABASE_URL`, and `SECRET_KEY` in the **Variables** section of the deploy form.

### Per-Service `.env` Files

For stacks with multiple services each needing their own variables, you can upload per-service `.env` files. Each file applies to the named service only:

| File name | Applies to |
|-----------|-----------|
| `web.env` | service named `web` |
| `worker.env` | service named `worker` |

Files use standard `KEY=VALUE` format (no quotes needed for simple values).

## Updating a Stack

1. Go to **Docker → Stacks**
2. Click the stack name or the **Edit** button
3. Update the Compose content or variables
4. Click **Update**

InfraPilot runs `docker compose up -d --remove-orphans` to apply the changes. Running containers are only restarted if their config changed.

## Redeploying (Pull Latest Images)

To pull the latest image tags without changing the Compose file:

1. Open the stack
2. Click **Redeploy**
3. InfraPilot pulls fresh images and recreates affected containers

This is equivalent to:
```bash
docker compose pull && docker compose up -d
```

## Viewing Stack Status

The stacks list shows each stack's status:

| Status | Meaning |
|--------|---------|
| **Running** | All services are up |
| **Partial** | Some services stopped or unhealthy |
| **Stopped** | All services stopped |
| **Error** | Last deploy/update failed |

Click a stack to see per-service status, including container state, health, and recent logs.

## Tearing Down a Stack

1. Open the stack
2. Click **Remove**
3. Choose whether to also remove **volumes** and **networks**
4. Confirm

This runs `docker compose down` (with `--volumes` if selected).

> **Warning:** Removing volumes is irreversible. Any data stored in named volumes will be lost.

## Stack Logs

View combined logs for all services in a stack from the stack detail page. You can also filter to a specific service.

## Limitations

- Compose files must be valid YAML with at least one `services:` key
- Build instructions (`build:`) are ignored — stacks must use pre-built images
- Registry authentication (for private images) must be configured on the Docker daemon directly
- Secrets and configs (`secrets:`, `configs:`) are not currently managed by InfraPilot

## Example: Deploying a PostgreSQL + App Stack

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: ${DB_NAME}
    volumes:
      - pgdata:/var/lib/postgresql/data

  app:
    image: myorg/myapp:${APP_VERSION:-latest}
    environment:
      DATABASE_URL: postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/${DB_NAME}
    depends_on:
      - postgres
    ports:
      - "8080:8080"

volumes:
  pgdata:
```

Variables to set in the form:
```
DB_USER=myapp
DB_PASSWORD=<strong-password>
DB_NAME=myapp_production
APP_VERSION=1.2.3
```
