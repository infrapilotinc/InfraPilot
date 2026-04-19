# Proxy Management

InfraPilot CE manages Nginx reverse proxy hosts from the dashboard. No terminal access or manual config file editing required.

## Creating a Proxy Host

1. Go to **Traffic → Proxy Hosts**
2. Click **Add Proxy Host**
3. Fill in:
   - **Domain** — the hostname this proxy serves (e.g. `api.example.com`)
   - **Upstream URL** — target container or service (e.g. `http://my-app:3000`)
   - **SSL** — enable to request a Let's Encrypt certificate
4. Click **Save** — InfraPilot writes the Nginx config and reloads Nginx

The proxy host is active immediately. If SSL is enabled and your domain's DNS points to this server, a certificate is issued automatically.

## SSL Certificates

InfraPilot uses Let's Encrypt via the ACME HTTP-01 challenge.

**Requirements:**
- `LETSENCRYPT_EMAIL` must be set in your environment
- Your domain must resolve to the server's public IP
- Port 80 must be reachable from the internet (for the ACME challenge)

**Certificate renewal** happens automatically 30 days before expiry. You can also trigger a manual renewal from the proxy host edit screen.

**Staging certificates:** By default (`LETSENCRYPT_STAGING=true`) staging certificates are issued — they are functionally valid but browser-untrusted. Set `LETSENCRYPT_STAGING=false` for production.

## Security Controls

Each proxy host has its own security settings, configurable from the dashboard:

### IP Allowlist / Denylist

Restrict which IP addresses or CIDR ranges can access the proxy host:

- **Allowlist** — only listed IPs/ranges can access the endpoint; all others get 403
- **Denylist** — listed IPs/ranges are blocked; all others pass through

Examples: `192.168.1.0/24`, `10.0.0.5`, `2001:db8::/32`

Both lists can be used simultaneously. The denylist is evaluated first.

### Basic Authentication

Protect a proxy host with HTTP Basic Auth:

1. Open the proxy host settings
2. Enable **Basic Auth**
3. Add username / password pairs

Credentials are stored hashed in the database and written to an Nginx `.htpasswd` file.

### Security Headers

Enable per-proxy security headers with a single toggle:

| Header | Value set |
|--------|-----------|
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` |
| `X-Frame-Options` | `SAMEORIGIN` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | Configurable per-host |

You can override the `Content-Security-Policy` value from the proxy host settings.

### Rate Limiting

Set per-IP request rate limits:

- **Rate** — requests per second or minute (e.g. `10r/s`, `100r/m`)
- **Burst** — allowed burst above the rate before requests are rejected
- **Response code** — 429 (default) or 503

### Dynamic Network Attachment

InfraPilot can attach the Nginx container to a Docker network on-the-fly so it can reach your container by its Docker DNS name, without exposing ports to the host.

1. Open the proxy host settings
2. Under **Network**, select the Docker network your container is on
3. Save — the agent attaches Nginx to that network and reloads the config

This lets you proxy to `http://my-app:8080` without publishing port 8080 to the host.

## Nginx Config Preview

Before saving, click **Preview Config** to see the exact `nginx.conf` block that will be written. This is useful for debugging unexpected behavior.

## Editing and Deleting

- **Edit** a proxy host from the three-dot menu or by clicking its row
- **Disable** temporarily removes the Nginx config block without deleting the record
- **Delete** removes the proxy host and its Nginx config; SSL certificates are left on disk

## Troubleshooting

**502 Bad Gateway**
- Confirm the upstream container is running
- Confirm the container is on a network reachable by Nginx (use dynamic network attachment)
- Check Nginx logs: Traffic → Logs

**Certificate not issued**
- Confirm port 80 is open on the server firewall
- Confirm the domain resolves to this server's IP (`dig A yourdomain.com`)
- Check `LETSENCRYPT_EMAIL` is set correctly
- Try `LETSENCRYPT_STAGING=true` first to avoid rate limits

**403 on allowlisted IP**
- Check if your IP is being NATed (you may need to allowlist the NAT gateway)
- Verify the CIDR notation is correct (e.g. `10.0.0.0/8` not `10.0.0.0/255.0.0.0`)
