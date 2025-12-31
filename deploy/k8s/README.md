# CanopyX Kubernetes Deployment

This directory contains Kubernetes manifests for deploying CanopyX components.

## Prerequisites

The following infrastructure must be deployed before applying the CanopyX manifests:

### 1. Redis (via OT Container Kit Redis Operator)

```bash
# Add Helm repo
helm repo add ot-helm https://ot-container-kit.github.io/helm-charts
helm repo update

# Install Redis Operator
helm install redis-operator ot-helm/redis-operator \
  --namespace canopy-dev \
  --kubeconfig /etc/rancher/k3s/k3s.yaml

# Install Redis Standalone
helm install redis ot-helm/redis \
  --namespace canopy-dev \
  --kubeconfig /etc/rancher/k3s/k3s.yaml
```

**Service Endpoint:** `redis.canopy-dev.svc.cluster.local:6379`

### 2. Temporal (via Temporal Helm Chart)

```bash
# Add Helm repo
helm repo add temporal https://go.temporal.io/helm-charts
helm repo update

# Install Temporal with Cassandra + Elasticsearch
helm install temporal temporal/temporal \
  --namespace canopy-dev \
  --kubeconfig /etc/rancher/k3s/k3s.yaml \
  --values ../helm/temporal-cassandra-elasticsearch-values.yaml \
  --timeout 10m
```

**Service Endpoint:** `temporal-frontend.canopy-dev.svc.cluster.local:7233`

### 3. ClickHouse (via HyperDX Helm Chart)

ClickHouse should already be deployed. If not:

```bash
helm install clickhouse hdx/hdx-oss-v2 \
  --namespace canopy-dev \
  --kubeconfig /etc/rancher/k3s/k3s.yaml
```

**Service Endpoint:** `clickhouse-hdx-oss-v2-clickhouse.canopy-dev.svc.cluster.local:9000`

## Directory Structure

```
deploy/k8s/
├── admin/                    # canopyx-admin API
│   ├── base/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── kustomization.yaml
│   └── overlays/
│       ├── local/            # Local development (Tilt)
│       └── dev/              # canopy-dev namespace
│
├── admin-web/                # canopyx-admin-web UI
│   ├── base/
│   └── overlays/
│       ├── local/
│       ├── shared/
│       └── dev/
│
├── admin-web-proxy/          # nginx proxy for unified routing
│   ├── base/
│   └── overlays/
│
├── controller/               # canopyx-controller (K8s orchestrator)
│   ├── base/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── rbac.yaml
│   │   └── kustomization.yaml
│   └── overlays/
│       ├── local/
│       └── dev/
│
├── clickhouse/               # ClickHouse (Altinity Operator)
├── prometheus/               # Prometheus monitoring
├── grafana/                  # Grafana dashboards
└── canopy-rpc-mock/          # Mock RPC for testing
```

## Deployment

### Deploy to Dev Environment (canopy-dev namespace)

```bash
# Deploy admin
kubectl apply -k admin/overlays/dev

# Deploy controller
kubectl apply -k controller/overlays/dev

# Deploy admin-web
kubectl apply -k admin-web/overlays/dev
```

### Deploy to Local Environment (Tilt)

Use `tilt up` from the repository root - it handles all deployments automatically.

## Environment-Specific Configuration

### Dev Environment (canopy-dev namespace)

| Service | Endpoint |
|---------|----------|
| ClickHouse | `clickhouse-hdx-oss-v2-clickhouse.canopy-dev.svc.cluster.local:9000` |
| Temporal | `temporal-frontend.canopy-dev.svc.cluster.local:7233` |
| Redis | `redis.canopy-dev.svc.cluster.local:6379` |
| Admin API | `canopyx-admin.canopy-dev.svc.cluster.local:3000` |

### Local Environment (default namespace)

| Service | Endpoint |
|---------|----------|
| ClickHouse | `chi-canopyx-canopyx-0-0.default.svc.cluster.local:9000` |
| Temporal | `temporal-frontend.default.svc.cluster.local:7233` |
| Redis | `redis-master.default.svc.cluster.local:6379` |
| Admin API | `canopyx-admin.default.svc.cluster.local:3000` |

## Helm Values

Helm chart values are stored in `deploy/helm/`:

- `redis-values.yaml` - Redis configuration
- `temporal-cassandra-elasticsearch-values.yaml` - Temporal with Cassandra + Elasticsearch

## Updating Deployments

After making changes to manifests:

```bash
# Rebuild and push images
docker build -t canopyx-admin:latest -f Dockerfile.admin .
docker build -t canopyx-controller:latest -f Dockerfile.controller .
docker build -t canopyx-admin-web:latest ./web/admin

# Import to k3s
docker save canopyx-admin:latest | sudo k3s ctr images import -
docker save canopyx-controller:latest | sudo k3s ctr images import -
docker save canopyx-admin-web:latest | sudo k3s ctr images import -

# Apply manifests
kubectl apply -k admin/overlays/dev
kubectl apply -k controller/overlays/dev
kubectl apply -k admin-web/overlays/dev

# Restart deployments
kubectl rollout restart deployment/canopyx-admin -n canopy-dev
kubectl rollout restart deployment/canopyx-controller -n canopy-dev
kubectl rollout restart deployment/canopyx-admin-web -n canopy-dev
```

