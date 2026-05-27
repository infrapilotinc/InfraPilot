# CI/CD

GitHub Actions workflows live in [`.github/workflows/`](../.github/workflows).

## Workflows

| Workflow | File | Trigger | Does |
|----------|------|---------|------|
| CI | `ci.yml` | PRs + pushes to `main`/`dev` | `go vet`/`go build` for backend & agent; pnpm lint + build for the frontend |
| Publish images | `publish.yml` | push to `main`, `v*` tags, manual dispatch | builds all images and pushes them to GHCR |

## Publish flow

1. `docker/login-action` logs in to `ghcr.io` using `GITHUB_TOKEN`
   (`packages: write` permission).
2. The version is resolved: a `v*` tag → that version; `main` → `latest`;
   manual dispatch → the supplied input.
3. [`scripts/build-and-publish.sh`](../scripts/build-and-publish.sh) runs with
   `PUSH_DOCKERHUB=false`, so it pushes to **GHCR only**.

The resulting `ghcr.io/infrapilothq/infrapilot-ce-*` packages are **public**.
Package visibility is configured once in the GHCR package settings — the
workflow does not change it.

## Adding Docker Hub publishing in CI (optional)

The build script still supports Docker Hub. To also push there from CI, add
`DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` secrets, a second `docker/login-action`
step, and drop the `PUSH_DOCKERHUB=false` override.

## Platform-wide coordination

This repo is a submodule of the `infrapilot-platform` meta-repo, which can fan
out releases across all four repos. See that repo's `docs/CI-CD.md`.
