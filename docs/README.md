# InfraPilot (Community Edition) — Documentation

Open-source, self-hosted infrastructure control plane for Docker, NGINX and
modern DevOps operations. This is the **public** edition; see
[`infrapilot-ee`](https://github.com/infrapilothq/infrapilot-ee) for the
Enterprise Edition.

## Index

### Platform
- [Architecture](ARCHITECTURE.md) — components, images and how they fit together
- [Configuration](CONFIGURATION.md) — environment variables and settings
- [Stacks](STACKS.md) — Docker Compose stack management
- [Proxy](PROXY.md) — NGINX / reverse-proxy and traffic exposure
- [Analytics](ANALYTICS.md) — log analytics
- [Alerts](ALERTS.md) — alerting system

### Operating it
- [Development](DEVELOPMENT.md) — local dev setup
- [Deployment](DEPLOYMENT.md) — building and publishing images, running in prod
- [CI/CD](CI-CD.md) — GitHub Actions workflows and the GHCR release flow

## Repository map

This repo is one of four that make up the InfraPilot platform:

| Repo | Visibility | Purpose |
|------|-----------|---------|
| [`InfraPilot`](https://github.com/infrapilothq/InfraPilot) (this repo) | public | Community Edition control plane |
| [`infrapilot-ee`](https://github.com/infrapilothq/infrapilot-ee) | private | Enterprise Edition |
| [`infrapilot.org`](https://github.com/infrapilothq/infrapilot.org) | private | Marketing site, license issuance, account portal, CLI |
| [`homebrew-tap`](https://github.com/infrapilothq/homebrew-tap) | public | Homebrew formula for the `infrapilot` CLI |

They are aggregated for coordinated development and release in the
`infrapilot-platform` meta-repository (git submodules).
