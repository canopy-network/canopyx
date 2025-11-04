# Development Guide

## Prerequisites

- Go 1.22+
- Node 22+ with pnpm
- Docker Desktop
- Kind, k3d, or Minikube (choose one)
- Tilt (https://tilt.dev)

```bash
make tools  # Install pnpm, golangci-lint, goimports
make help   # See all available commands
```

## Quick Start

```bash
# 1. Start local Kubernetes + local registry
make kind-up
# OR: k3d cluster create / minikube start

# 2. Deploy everything (ClickHouse, Temporal, Admin, Controller)
make tilt-up

# Access UI at http://localhost:3003
```

That's it. Tilt deploys the full stack to Kubernetes and auto-rebuilds on code changes.

## What Tilt Does

Tilt deploys to Kubernetes using Helm charts and Kustomize manifests:

**Always deployed:**
- ClickHouse (database)
- Temporal (workflow engine)
- Redis (pub/sub)
- Admin API + UI
- Controller (manages indexer deployments)

**Optional (configure in `tilt-config.yaml`):**
- Canopy node(s) - local blockchain for testing
- Monitoring - Prometheus + Grafana

**Auto-spawned by controller:**
- Indexer workers (one deployment per registered chain)

## Configuration

Copy `tilt-config.default.yaml` → `tilt-config.yaml` and customize:

**Profiles:** `minimal` | `development` | `production`

**Component toggles:**
```yaml
components:
  monitoring: false        # Prometheus + Grafana
  canopy: "off"           # "off" | "single" | "dual"
  admin_web: true         # Next.js UI
```

**Canopy modes:**
- `off` - Use remote RPC endpoints
- `single` - Single local node (Chain ID 1)
- `dual` - Two nodes for DEX testing (Chain ID 1 + 2)

## Common Commands

**Tilt:**
```bash
make tilt-up            # Deploy stack (interactive)
make tilt-down          # Stop and cleanup
make tilt-status        # Show resource status
make tilt-logs          # Stream all logs
```

**Build:**
```bash
make build              # Build Go + Next.js
make build-go           # Go binaries only
make docker-all         # Build all Docker images
```

**Development:**
```bash
make dev-web            # Next.js dev server at :3003
make fmt                # Format code
make lint               # Run linter
```

**Testing:**
```bash
make test-unit          # Fast tests (no deps)
make test-integration   # Integration tests (needs Tilt)
```

**Kind cluster:**
```bash
make kind-up            # Create cluster + registry
make kind-down          # Delete cluster
make kind-clean         # Delete + Docker cleanup
```

Run `make help` for the full list.

## Development Workflows

### Standard: Everything in Kubernetes

Start Tilt and work through the UI. All services auto-rebuild on code changes.

```bash
make tilt-up
# Open http://localhost:3003
# Edit code → auto-rebuild → refresh browser
```

### Advanced: Local Go Services

Run Go services locally for faster iteration (bypasses Docker builds):

```bash
make tilt-up  # Starts infra only

# In separate terminals:
ADMIN_TOKEN=devtoken go run ./cmd/admin       # Admin API
CHAIN_ID=1 go run ./cmd/indexer               # Indexer worker
go run ./cmd/controller                       # Controller

# UI still runs in K8s, proxies to localhost:3000
```

**Note:** When using local Go services, the Tilt-deployed versions will crash-loop. This is expected - Tilt can't detect you're running locally.

### UI Development

Hot reload via Tilt (recommended):
```bash
make tilt-up
# Edit web/admin/** → auto-rebuild in cluster
```

Or run Next.js dev server locally:
```bash
make dev-web  # http://localhost:3003, proxies API to localhost:3000
```

## Access Points

**Default ports:**
- Admin UI: http://localhost:3003
- Admin API: http://localhost:3000
- Temporal UI: http://localhost:8080
- ClickHouse: localhost:9000 (native), localhost:8123 (HTTP)
- Prometheus: http://localhost:9090 (if monitoring enabled)
- Grafana: http://localhost:3100 (if monitoring enabled)

Change ports in `tilt-config.yaml` under `ports:`.

## Testing

**Unit tests** - No dependencies:
```bash
make test-unit
make test-unit RUN=TestValidatorActivity
```

**Integration tests** - Requires ClickHouse:
```bash
make tilt-up
make test-integration
make test-integration TEST_PKG=./tests/integration/db
```

## Troubleshooting

**Tilt won't start:**
```bash
kubectl config current-context  # Verify local cluster
make tilt-down && make tilt-up
```

**Port conflicts:**
```bash
lsof -i :3003
kill -9 <PID>
```

**Build failures:**
```bash
make clean
go clean -cache
make build
```

**Canopy node not deploying:**
Check `paths.canopy_source` in `tilt-config.yaml` points to Canopy repo.

**Out of disk space:**
```bash
make kind-disk-report   # Check usage
make kind-clean         # Clean Docker (safe)
make kind-purge         # Nuclear option (removes volumes)
```

## Project Structure

```
Makefile            # Build, test, Tilt, Kind, Docker commands
Tiltfile            # Kubernetes deployment logic
tilt-config.yaml    # Tilt configuration (gitignored)

cmd/                # Service entrypoints
app/                # Service implementations
pkg/                # Shared libraries
web/admin/          # Next.js UI
tests/              # Unit + integration tests
deploy/             # Helm values + K8s manifests
```
