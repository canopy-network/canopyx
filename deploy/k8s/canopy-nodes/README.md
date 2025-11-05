# Dual Canopy Node Deployment

This directory contains Kubernetes manifests for deploying two Canopy blockchain nodes for DEX and liquidity pools testing.

## Quick Start

### 1. Enable in tilt-config.yaml

```yaml
components:
  canopy: "dual"   # Enable dual-node setup
                   # Options: "off" | "single" | "dual"
```

### 2. Start Tilt

```bash
make tilt-up
```

## Architecture

- **Node 1** (Chain ID 1) - Root Chain
  - RPC: http://node-1.default.svc.cluster.local:50002
  - Ports: 50000-50003, 9001, 9090
  - Pool ID: 32769

- **Node 2** (Chain ID 2) - Subchain
  - RPC: http://node-2.default.svc.cluster.local:40002
  - Ports: 40000-40003, 9001, 9091
  - Pool ID: 32768

## Structure

```
canopy-nodes/
├── base/
│   ├── node-1-configmap-config.yaml    # Node 1 config
│   ├── node-1-configmap-genesis.yaml   # Node 1 genesis
│   ├── node-1-configmap-keystore.yaml  # Node 1 keys
│   ├── node-1-deployment.yaml          # Node 1 deployment
│   ├── node-1-service.yaml             # Node 1 service
│   ├── node-2-configmap-config.yaml    # Node 2 config
│   ├── node-2-configmap-genesis.yaml   # Node 2 genesis
│   ├── node-2-configmap-keystore.yaml  # Node 2 keys
│   ├── node-2-deployment.yaml          # Node 2 deployment
│   ├── node-2-service.yaml             # Node 2 service
│   └── kustomization.yaml              # Base kustomization
└── overlays/
    └── local/
        └── kustomization.yaml          # Local overlay
```

## Testing

### Check Node Status

```bash
# Node 1
curl http://localhost:50002/v1/query/height

# Node 2
curl http://localhost:40002/v1/query/height
```

### Check P2P Connectivity

```bash
# From Node 1
kubectl exec -it deployment/node-1 -- curl http://node-2.default.svc.cluster.local:40002/v1/query/height

# From Node 2
kubectl exec -it deployment/node-2 -- curl http://node-1.default.svc.cluster.local:50002/v1/query/height
```

### View Logs

```bash
# Node 1
kubectl logs -f deployment/node-1

# Node 2
kubectl logs -f deployment/node-2
```

### Check Indexing

```bash
# List indexers
kubectl get deployments -l managed-by=canopyx-controller,app=indexer

# Check Chain 1 indexer
kubectl logs -l app=indexer,chain-id=1 -f

# Check Chain 2 indexer
kubectl logs -l app=indexer,chain-id=2 -f
```

## Configuration Source

All configurations are generated from:
- `/home/overlordyorch/Development/canopy/.docker/volumes/node_1/`
- `/home/overlordyorch/Development/canopy/.docker/volumes/node_2/`

## Troubleshooting

### Nodes Can't Find Each Other
- Check P2P port (9001) is accessible
- Verify DNS resolution: `kubectl exec deployment/node-1 -- nslookup node-2.default.svc.cluster.local`
- Check `dialPeers` configuration in ConfigMaps

### Port Conflicts
- Ensure local ports 50000-50003 and 40000-40003 are available
- Check Tilt port forwards with `tilt resources`
