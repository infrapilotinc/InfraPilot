# Alerting

InfraPilot CE monitors your containers and infrastructure and sends notifications when defined thresholds are breached.

## Overview

The alerting system has two parts:

- **Channels** — where notifications are delivered (email, Slack, webhook)
- **Rules** — what conditions trigger a notification and which channels to use

## Alert Channels

Go to **Alerts → Channels** to configure notification destinations.

### SMTP (Email)

| Field | Description |
|-------|-------------|
| SMTP Host | Mail server hostname (e.g. `smtp.gmail.com`) |
| Port | Usually `587` (STARTTLS) or `465` (TLS) |
| Username | SMTP login |
| Password | SMTP password or app password |
| From Address | Sender email address |
| To Address | Recipient email address(es), comma-separated |

### Slack

| Field | Description |
|-------|-------------|
| Webhook URL | Slack Incoming Webhook URL from your Slack app settings |
| Channel | Override the channel (optional — defaults to webhook's configured channel) |

Get a webhook URL at: **Slack → Apps → Incoming Webhooks → Add New Webhook**

### Webhook (Generic HTTP)

Sends a JSON `POST` request to any URL when an alert fires.

| Field | Description |
|-------|-------------|
| URL | Endpoint that receives the alert payload |
| Secret | Optional HMAC-SHA256 signing secret — sent as `X-InfraPilot-Signature` header |

**Payload format:**
```json
{
  "rule_name": "High Memory Usage",
  "rule_type": "high_memory",
  "severity": "warning",
  "message": "Container memory usage is high",
  "triggered_at": "2026-04-19T14:23:00Z",
  "metadata": {
    "container_name": "my-app",
    "memory_percent": 87.3,
    "memory_usage": 912261120,
    "memory_limit": 1073741824,
    "threshold": 80
  }
}
```

## Alert Rules

Go to **Alerts → Rules** to create and manage rules.

### Available Rule Types

#### Container Crash (`container_crash`)

Fires when a container enters `exited` or `dead` state.

| Condition | Default |
|-----------|---------|
| *(no conditions — triggers on any crashed container)* | — |

Severity: **critical**

#### OOM Kill (`oom_kill`)

Fires when the Linux OOM killer terminates a container due to memory exhaustion.

| Condition | Default |
|-----------|---------|
| *(no conditions — triggers on any OOM-killed container)* | — |

Severity: **critical**

> The OOM killer is triggered by the kernel when the system or container runs out of memory. This usually means the container's memory limit is too low or the application has a memory leak.

#### Container Stopped (`container_stopped`)

Fires when a specific container (or any container) is not in the `running` state.

| Condition | Description |
|-----------|-------------|
| `container_name` | Target container name (leave blank for all containers) |

Severity: **warning**

#### High Restart Count (`high_restart_count`)

Fires when a container has restarted too many times (indicates a crash loop).

| Condition | Default | Description |
|-----------|---------|-------------|
| `threshold` | `3` | Number of restarts to trigger |

Severity: **warning**

#### High CPU Usage (`high_cpu`)

Fires when a container's CPU usage exceeds a percentage threshold.

| Condition | Default | Description |
|-----------|---------|-------------|
| `threshold` | `80.0` | CPU percentage (0–100) |

Severity: **warning**

#### High Memory Usage (`high_memory`)

Fires when a container's memory usage exceeds a percentage of its limit.

| Condition | Default | Description |
|-----------|---------|-------------|
| `threshold` | `80.0` | Memory percentage (0–100) |

> Requires the container to have a memory limit set (`--memory` flag or `mem_limit` in Compose).

Severity: **warning**

#### SSL Certificate Expiry (`ssl_expiry`)

Checks SSL certificates for all proxy hosts with SSL enabled and fires when a certificate is about to expire.

| Condition | Default | Description |
|-----------|---------|-------------|
| `warning_days` | `14` | Days before expiry to send a warning |
| `critical_days` | `7` | Days before expiry to send a critical alert |

Severity: **warning** or **critical** depending on proximity to expiry.

#### High Error Rate (`high_error_rate`)

Fires when a container produces too many log-level errors in a rolling window.

| Condition | Default | Description |
|-----------|---------|-------------|
| `threshold` | `10.0` | Errors per minute |
| `window_mins` | `5` | Rolling window in minutes |
| `container_pattern` | *(all)* | Filter by container name substring |

Severity: **warning**

### Cooldown Period

Every rule has a **cooldown** (in minutes). After an alert fires for a given target, it won't fire again for that target until the cooldown elapses. This prevents notification floods.

Recommended minimums:
- Container crash: 5 minutes
- SSL expiry: 60 minutes (it only gets worse, not better)
- High CPU/memory: 15 minutes

### Evaluation Interval

The alert evaluator runs every **60 seconds** by default. There may be up to a 1-minute delay between a condition occurring and the alert firing.

## Alert History

Go to **Alerts → History** to see all past alert events with their severity, message, timestamp, and metadata.

## Example: Notify on Container Crash via Slack

1. **Create a channel:**
   - Type: Slack
   - Webhook URL: `https://hooks.slack.com/services/...`

2. **Create a rule:**
   - Name: "Container Crash Alert"
   - Type: `container_crash`
   - Channels: *(select the Slack channel you just created)*
   - Cooldown: 5 minutes
   - Enable

Any container that crashes will now send a Slack message within 60 seconds.

## Troubleshooting

**Alert fired but no notification received**
- Check that the channel config is correct (send a test from the channel settings)
- Check alert history — if the alert is recorded there, the notification delivery failed
- Check backend logs for `"Failed to send notification"` errors

**Alert not firing at all**
- Confirm the rule is enabled
- Confirm the condition is being met (e.g. check container memory usage in the Docker tab)
- Ensure the cooldown period has elapsed since the last firing

**SMTP: "connection refused" or "authentication failed"**
- Many providers require an app-specific password (Gmail, Outlook)
- Try port 587 with STARTTLS, or 465 with implicit TLS
- Check firewall rules if the backend container can't reach the SMTP host
