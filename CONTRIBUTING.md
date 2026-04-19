# Contributing to InfraPilot

Thank you for your interest in contributing to InfraPilot! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)
- [Community](#community)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. We expect all contributors to:

- Be respectful of differing viewpoints and experiences
- Accept constructive criticism gracefully
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

- **Go 1.22+** — Backend and Agent
- **Node.js 20+** — Frontend
- **pnpm** — Package manager for frontend
- **Docker 24.0+** — Container runtime
- **Docker Compose v2** — Multi-container orchestration

### Development Setup

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/YOUR_USERNAME/infrapilot-ce.git
   cd infrapilot-ce
   ```

2. **Start the full development environment**
   ```bash
   # Local dev with hot reload (recommended)
   ./scripts/dev.sh dev

   # Or fully containerised (no host ports)
   ./scripts/dev.sh up
   ```

3. **Access the application** (local dev mode)
   - Dashboard: http://localhost:3000
   - Backend API: http://localhost:8080/api/v1
   - Storybook: http://localhost:6006

### Project Structure

```
infrapilot/
├── backend/           # Go API server (Gin)
├── agent/             # Go agent for Docker/Nginx management
├── frontend/          # Next.js 15 dashboard
├── proto/             # gRPC protocol definitions
├── deployments/       # Docker Compose files
├── scripts/           # Development helper scripts
└── docs/              # Documentation and epics
```

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in [Issues](https://github.com/infrapilothq/infrapilot-ce/issues)
2. If not, create a new issue with:
   - Clear, descriptive title
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (OS, Docker version, browser)
   - Relevant logs or screenshots

### Suggesting Features

1. Check [Discussions](https://github.com/infrapilothq/infrapilot-ce/discussions) for existing proposals
2. Create a new discussion in the "Ideas" category with:
   - Problem statement
   - Proposed solution
   - Alternatives considered
   - Any relevant context

### Contributing Code

1. **Find an issue to work on**
   - Look for issues labeled `good first issue` or `help wanted`
   - Comment on the issue to express interest
   - Wait for maintainer assignment to avoid duplicate work

2. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

3. **Make your changes**
   - Follow the coding standards below
   - Write tests for new functionality
   - Update documentation as needed

4. **Commit with conventional commits**
   ```bash
   git commit -m "feat: add container health monitoring"
   git commit -m "fix: resolve nginx reload race condition"
   git commit -m "docs: update API endpoint documentation"
   ```

5. **Push and create a Pull Request**
   ```bash
   git push origin feature/your-feature-name
   ```

## Pull Request Process

1. **Before submitting:**
   - Ensure all tests pass
   - Run linters (`go fmt`, `pnpm lint`)
   - Update relevant documentation
   - Rebase on latest `main` if needed

2. **PR Description should include:**
   - Summary of changes
   - Related issue number(s)
   - Testing performed
   - Screenshots for UI changes

3. **Review process:**
   - At least one maintainer approval required
   - All CI checks must pass
   - Address review feedback promptly

4. **After merge:**
   - Delete your feature branch
   - Celebrate your contribution!

## Coding Standards

### Go (Backend & Agent)

- Follow standard Go conventions
- Run `go fmt` before committing
- Use meaningful variable and function names
- Keep functions focused and testable
- Handle errors explicitly — no silent failures

```go
// Good
func GetContainer(ctx context.Context, id string) (*Container, error) {
    if id == "" {
        return nil, ErrInvalidContainerID
    }
    // ...
}

// Avoid
func GetContainer(id string) *Container {
    // Silent nil on error
}
```

### TypeScript (Frontend)

- Use TypeScript strict mode
- Prefer interfaces over type aliases
- Use functional components with hooks
- Follow the existing component structure

```typescript
// Good
interface ProxyHostProps {
  host: ProxyHost;
  onEdit: (id: string) => void;
}

export function ProxyHostCard({ host, onEdit }: ProxyHostProps) {
  // ...
}
```

### SQL

- Uppercase SQL keywords
- Use snake_case for identifiers
- Include comments for complex queries

```sql
-- Good
SELECT id, domain_name, upstream_url
FROM proxy_hosts
WHERE agent_id = $1
ORDER BY created_at DESC;
```

### Git Commits

We use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Purpose |
|--------|---------|
| `feat:` | New feature |
| `fix:` | Bug fix |
| `docs:` | Documentation only |
| `style:` | Code style (formatting, etc.) |
| `refactor:` | Code change that neither fixes nor adds |
| `test:` | Adding or updating tests |
| `chore:` | Maintenance tasks |

## Testing

### Backend Tests

```bash
cd backend
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/api/...
```

### Frontend Tests

```bash
cd frontend
pnpm test

# Watch mode
pnpm test:watch

# Coverage
pnpm test:coverage
```

### Integration Tests

```bash
# Start test environment
./scripts/dev.sh up

# Run integration tests
cd backend && go test -tags=integration ./...
```

## Documentation

- Add JSDoc comments for public TypeScript functions
- Document new API endpoints in the codebase
- Update the README for user-facing changes

## Development Tips

### Hot Reload

- Backend: Uses [Air](https://github.com/air-verse/air) for hot reload
- Frontend: Next.js built-in hot reload

### Debugging

```bash
# View Docker service logs
./scripts/dev.sh logs backend
./scripts/dev.sh logs          # all services

# View local dev logs (dev.sh dev mode)
./scripts/dev.sh dev:logs

# Reset database (destroys all data)
./scripts/dev.sh reset
```

### Proto Changes

When modifying `proto/agent/v1/agent.proto`:

```bash
./scripts/dev.sh proto
```

## Community

- **GitHub Issues** — Bug reports and tracked work
- **GitHub Discussions** — Questions, ideas, and community chat
- **Pull Requests** — Code contributions and reviews

## Recognition

Contributors are recognized in:
- GitHub Contributors page
- Release notes for significant contributions

Thank you for helping make InfraPilot better!
