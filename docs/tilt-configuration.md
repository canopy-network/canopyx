# Tilt Configuration System

## Overview

The CanopyX Tilt setup now uses a flexible configuration system (`tilt-config.yaml`) that allows developers to customize their local environment based on their needs and available resources.

## Configuration File

### Location
- **Default template**: `tilt-config.default.yaml` (version controlled)
- **Your config**: `tilt-config.yaml` (gitignored, created automatically)

### First Time Setup

When you run `tilt up` for the first time:
1. Tilt checks for `tilt-config.yaml`
2. If not found, it copies `tilt-config.default.yaml` to `tilt-config.yaml`
3. You can then edit `tilt-config.yaml` to customize your setup

## Profiles

Three profiles are available to match different use cases:

### Minimal Profile
**Best for**: Laptop development, UI work, API testing
**Resources**: ~4GB RAM, 2 CPU cores
**Components**: Core services only, no monitoring, no local blockchain node

```yaml
profile: minimal
components:
  monitoring: false
  canopy_node: false
  query: false  # Optional: disable if not needed
```

### Development Profile (Default)
**Best for**: Full-stack development, workflow testing
**Resources**: ~8GB RAM, 4 CPU cores
**Components**: Full stack with minimal replicas

```yaml
profile: development
components:
  monitoring: false  # Enable only when debugging
  canopy_node: false # Enable if testing against local node
```

### Production Profile
**Best for**: Performance testing, benchmarking, demos
**Resources**: ~16GB RAM, 8+ CPU cores
**Components**: Production-like setup with replication and monitoring

```yaml
profile: production
components:
  monitoring: true   # REQUIRED for benchmarking
  canopy_node: true  # Optional based on test requirements
```

## Components

### Always Required (Cannot Disable)
- **ClickHouse**: Database for blocks/transactions
- **Temporal**: Workflow orchestration (with Cassandra + Elasticsearch)
- **Admin API**: Required for controller
- **Controller**: K8s orchestrator that spawns indexers
- **Indexer**: Build process (deployment managed by controller)

**Note**: These components scale via resource limits in the profile, not on/off toggles.

### Optional Components

#### Monitoring (`components.monitoring`)
**What it includes**: Prometheus + Grafana
**Resource cost**: ~1GB RAM
**Enable when**:
- Performance testing or benchmarking
- Debugging workflow/indexing issues
- Demos or presentations

**Disable when**:
- Normal development work
- Limited laptop resources
- Not actively monitoring metrics

#### Canopy Node (`components.canopy_node`)
**What it includes**: Local Canopy blockchain node
**Resource cost**: ~2GB RAM, +1 CPU core
**Enable when**:
- Testing against local blockchain
- Developing blockchain-specific features
- Need full control over chain state

**Disable when**:
- Using remote RPC endpoints
- Only working on indexer/UI features
- Limited resources

**Requires**: `paths.canopy_source` must point to valid Canopy repository

#### Query API (`components.query`)
**What it includes**: Public query API service
**Disable if**: Only working on admin/indexer, not using query features

#### Admin Web (`components.admin_web`)
**What it includes**: Next.js admin UI
**Disable if**: Only using API directly, not working on UI

## Configuration Sections

### Paths
Configure paths to external repositories:

```yaml
paths:
  canopy_source: "../canopy"  # Path to Canopy blockchain source
```

Supports:
- Relative paths: `"../canopy"`
- Absolute paths: `"/home/user/go/src/canopy"`
- Home directory: `"~/projects/canopy"`

### Resource Limits by Profile

Control replica counts and resource limits:

```yaml
resources:
  development:
    clickhouse:
      cpu_limit: "2000m"
      memory_limit: "4Gi"

    temporal:
      history_replicas: 1
      matching_replicas: 1
      frontend_replicas: 1
      worker_replicas: 1
      cassandra_replicas: 1

    indexer:
      min_replicas: 1
      max_replicas: 3
```

### Port Forwarding

Customize local ports to avoid conflicts:

```yaml
ports:
  clickhouse_web: 8081
  clickhouse_server: 8123
  temporal_web: 8080
  temporal_frontend: 7233
  prometheus: 9090
  grafana: 3100
  admin: 3000
  query: 3001
  admin_web: 3003
```

### Monitoring Configuration

When `components.monitoring = true`:

```yaml
monitoring:
  dashboards:
    - docs/grafana/dashboards/canopyx-overview.json
    # Add more dashboards here

  extra_scrape_configs: []
    # Add custom Prometheus scrape configs
```

### Development Settings

```yaml
dev:
  auto_rebuild: true              # Auto-rebuild on code changes
  auto_register_chain: true       # Auto-register local Canopy chain
  indexer_tag: "localhost:5001/canopyx-indexer:dev"
  indexer_build_mode: "auto"      # or "manual"
```

## Example Configurations

### Scenario: Laptop Developer (Limited Resources)

```yaml
profile: minimal
components:
  monitoring: false
  canopy_node: false
  query: false        # If you don't need query API
  admin_web: true     # Keep if working on UI
```

**Result**: ~4GB RAM, only essential services

### Scenario: Full-Stack Developer

```yaml
profile: development
components:
  monitoring: false   # Enable only when debugging
  canopy_node: true   # If testing against local node
  query: true
  admin_web: true
paths:
  canopy_source: "../canopy"
```

**Result**: ~8-10GB RAM, full development environment

### Scenario: Performance Testing / Benchmarking

```yaml
profile: production
components:
  monitoring: true    # MUST be enabled for metrics
  canopy_node: true   # If testing local node
  query: true
  admin_web: true
```

**Result**: ~16GB RAM, production-like setup with monitoring

### Scenario: Demo / Presentation

```yaml
profile: production
components:
  monitoring: true    # Show off the dashboards!
  canopy_node: true   # Show local blockchain
  query: true
  admin_web: true
```

**Result**: Full stack with all bells and whistles

## Accessing Services

Once Tilt is running, access services at:

- **ClickHouse UI**: http://localhost:8081 (or your configured port)
- **Temporal UI**: http://localhost:8080
- **Admin API**: http://localhost:3000
- **Admin Web UI**: http://localhost:3003
- **Query API**: http://localhost:3001
- **Prometheus**: http://localhost:9090 (if monitoring enabled)
- **Grafana**: http://localhost:3100 (if monitoring enabled)
- **Canopy RPC**: http://localhost:50002 (if canopy_node enabled)

## Troubleshooting

### "No tilt-config.yaml found"
This is normal on first run. Tilt will create it automatically from the default template.

### Port Conflicts
If you see port binding errors, edit `tilt-config.yaml` and change the conflicting ports.

### Out of Resources
Switch to a lighter profile:
```bash
# Edit tilt-config.yaml
profile: minimal
```

Then restart Tilt:
```bash
tilt down
tilt up
```

### Canopy Node Not Starting
Check:
1. `components.canopy_node = true` in config
2. `paths.canopy_source` points to valid Canopy repository
3. Canopy directory exists and contains `.docker/Dockerfile`

## Advanced Customization

### Adding Custom Prometheus Scrape Configs

```yaml
monitoring:
  extra_scrape_configs:
    - job_name: 'my-custom-service'
      static_configs:
        - targets: ['my-service:9090']
```

### Adding More Grafana Dashboards

1. Place dashboard JSON in `docs/grafana/dashboards/`
2. Add to config:

```yaml
monitoring:
  dashboards:
    - docs/grafana/dashboards/canopyx-overview.json
    - docs/grafana/dashboards/my-custom-dashboard.json
```

## Implementation Status

✅ **Completed** (2025-10-17)

All core features have been implemented:

- [x] Configuration loading and validation
- [x] Profile selection (minimal, development, production)
- [x] Component toggles (monitoring, canopy_node, query, admin_web)
- [x] Monitoring wrapped in conditional
- [x] Grafana dashboard injection from config files
- [x] Enhanced Prometheus with all service monitoring
  - Temporal services (frontend, history, matching, worker)
  - ClickHouse
  - Cassandra (Temporal persistence)
  - Elasticsearch (Temporal visibility)
  - CanopyX services via Kubernetes service discovery
  - Kubernetes pods with prometheus.io annotations
- [x] Port configuration applied throughout Tiltfile
- [x] Optional service conditionals (query, admin_web)
- [x] Canopy source path configuration with expansion
- [x] Development settings (auto_register_chain, indexer_tag, indexer_build_mode)

### Resource Limits (Completed)

✅ **Implemented** - Resource limits are now dynamically applied from profile configuration:

**Helm Charts (ClickHouse, Temporal)**
- ClickHouse: CPU/memory limits and requests via `--set` flags
- Temporal: Cassandra replicas, heap size, and memory limits
- Temporal: Elasticsearch replicas, CPU/memory limits
- Applied automatically based on selected profile

**Kustomize Deployments (CanopyX Services)**
- Admin API: CPU/memory limits from `canopyx.admin_*`
- Query API: CPU/memory limits from `canopyx.query_*`
- Controller & Admin Web: Limits defined but not yet applied (future enhancement)
- Uses `decode_yaml_stream()` / `encode_yaml_stream()` pattern for dynamic patching

**How It Works**
```starlark
# Load Kustomize output
admin_objects = decode_yaml_stream(kustomize("./deploy/k8s/admin/overlays/local"))

# Find and patch Deployment
for o in admin_objects:
    if o['kind'] == 'Deployment' and o['metadata']['name'] == 'canopyx-admin':
        container['resources'] = {
            'limits': {'cpu': admin_cpu, 'memory': admin_mem},
            'requests': {'cpu': admin_cpu // 2, 'memory': admin_mem // 2}
        }

# Re-encode and apply
k8s_yaml(encode_yaml_stream(admin_objects))
```

**Profile Resource Allocations**

| Component | Minimal | Development | Production |
|-----------|---------|-------------|------------|
| **ClickHouse** | 2Gi / 1CPU | 4Gi / 2CPU | 8Gi / 4CPU |
| **Cassandra** | 6G heap, 1 replica | 12G heap, 1 replica | 24G heap, 3 replicas |
| **Elasticsearch** | 1Gi / 1CPU, 1 replica | 2Gi / 2CPU, 1 replica | 4Gi / 4CPU, 3 replicas |
| **Admin API** | 512Mi / 0.5CPU | 1Gi / 1CPU | 2Gi / 2CPU |
| **Query API** | 512Mi / 0.5CPU | 1Gi / 1CPU | 2Gi / 2CPU |
| **Total Estimate** | ~4GB RAM | ~8GB RAM | ~16GB+ RAM |