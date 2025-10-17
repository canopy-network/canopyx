# Temporal Kubernetes Worker Controller Analysis

## Overview

This document analyzes whether the official Temporal Kubernetes Worker Controller could replace CanopyX's current custom controller for managing indexer workers.

**Documentation Sources:**
- https://docs.temporal.io/production-deployment/worker-deployments
- https://docs.temporal.io/production-deployment/worker-deployments/kubernetes-controller

---

## Current CanopyX Architecture

### Custom Controller (app/controller/)
- **Purpose**: Auto-scales indexer deployments based on Temporal queue depth
- **Implementation**: Kubernetes controller pattern with leader election
- **Scaling Logic**:
  - Monitors `index:<chainID>` task queue depth via Temporal API
  - Scales indexer deployments from min_replicas to max_replicas based on backlog
  - 1 deployment per chain (isolation model)
- **Features**:
  - Queue depth monitoring
  - Per-chain deployment management
  - Min/max replica configuration per chain
  - Automatic cleanup when chains are deleted

### Current Pain Points
- Custom scaling logic needs maintenance
- Queue depth monitoring requires custom Temporal API calls
- Need to maintain controller infrastructure (leader election, etc.)

---

## Temporal Kubernetes Worker Controller

### How It Works

The official Temporal Worker Controller is designed for **Worker Versioning** use cases, not queue-based autoscaling. It:

1. Tracks active workflow versions deployed
2. Creates versioned Kubernetes Deployment resources
3. Manages deployment lifecycles (creation, updates, deletion)
4. Routes workflows to specific worker versions using "pinning"
5. Supports "rainbow deployments" (multiple versions coexist)

### Key Features

- **Automatic Version Registration**: Detects new worker versions and registers them
- **Versioned Deployments**: Creates separate K8s Deployments for each worker version
- **Rollout Strategies**: Manual, AllAtOnce, Progressive
- **Gate Workflows**: Validate new versions before routing production traffic
- **Resource Cleanup**: Auto-deletes resources when versions are drained
- **Autoscaling**: Coming soon (not available yet)

### Setup Requirements

- Tag workers following Worker Versioning guidance
- Install via Helm chart (version 1.0.0+)
- Configure rollout strategies
- Deploy to K8s cluster with Temporal Server access

---

## Comparison: Custom vs. Temporal Controller

| Aspect | CanopyX Custom Controller | Temporal K8s Controller |
|--------|---------------------------|-------------------------|
| **Primary Purpose** | Queue-based autoscaling | Worker version management |
| **Scaling Trigger** | Task queue depth | Not applicable (versioning focus) |
| **Per-Chain Isolation** | ✅ Yes (1 deployment per chain) | ❌ Not designed for this |
| **Dynamic Scaling** | ✅ Yes (based on backlog) | ⚠️ Coming soon (autoscaling) |
| **Version Management** | ❌ No | ✅ Yes (rainbow deployments) |
| **Rollout Strategies** | ❌ No | ✅ Yes (Manual, AllAtOnce, Progressive) |
| **Maintenance Burden** | ⚠️ High (custom code) | ✅ Low (official support) |

---

## Analysis: Could It Replace Current Controller?

### ❌ Not Currently Suitable

The Temporal Kubernetes Worker Controller **cannot replace** the current custom controller because:

1. **Different Problem Space**:
   - Temporal controller: Manages worker **versions** (blue/green deployments, canary releases)
   - CanopyX controller: Manages worker **scaling** based on queue depth

2. **No Queue-Based Autoscaling**:
   - The official controller does NOT monitor task queue depth
   - It does NOT dynamically scale replicas based on backlog
   - Autoscaling is "coming soon" but not available yet

3. **Not Designed for Per-Chain Isolation**:
   - CanopyX uses 1 deployment per chain (`indexer-<chainID>`)
   - Temporal controller manages versioned deployments, not per-queue deployments
   - Would need significant adaptation to support per-chain architecture

4. **Missing Core Requirement**:
   - CanopyX needs: "Scale indexer-mainnet from 1→6 replicas when queue depth > threshold"
   - Temporal controller provides: "Deploy v1.2.0 alongside v1.1.0 and route workflows"

---

## Potential Future Consideration

### When Autoscaling Feature Arrives

Once the Temporal controller adds autoscaling support, it **might** be worth revisiting if:

1. ✅ It supports scaling based on task queue depth
2. ✅ It allows per-queue deployment management (e.g., separate deployments for different queues)
3. ✅ It supports min/max replica configuration
4. ✅ It can handle CanopyX's multi-chain isolation model

### Hybrid Approach

A possible future architecture could combine both:

- **Temporal Controller**: Manage worker versions and rollouts (for safe upgrades)
- **Custom Logic**: Continue queue-based scaling (until official support arrives)

However, this adds complexity rather than reducing it.

---

## Recommendation

### Keep Current Custom Controller

**Rationale**:

1. **Solves Different Problem**: The Temporal controller is for version management, not queue-based autoscaling
2. **Core Feature Missing**: No queue-depth-based scaling (coming soon ≠ available)
3. **Architecture Mismatch**: Not designed for per-chain deployment isolation
4. **Current Solution Works**: Custom controller successfully handles scaling logic

### Potential Improvements to Current Controller

Instead of replacing it, consider:

1. **Monitor Official Autoscaling**: Watch for autoscaling feature release
2. **Simplify Current Logic**: Review if scaling algorithm can be optimized
3. **Add Metrics**: Export queue depth and scaling decisions to Prometheus
4. **Documentation**: Document scaling behavior and thresholds
5. **Version Management**: Use Temporal's Worker Versioning for safe rollouts (separate concern)

### Future Re-evaluation

Re-assess when Temporal releases autoscaling feature:
- Test if it supports per-queue scaling
- Evaluate if it can replace custom logic
- Consider migration path if suitable

---

## Conclusion

The Temporal Kubernetes Worker Controller is a valuable tool for **managing worker versions and safe rollouts**, but it **does not replace** CanopyX's custom controller for **queue-based autoscaling**.

The custom controller should be retained for now, as it solves a different problem (dynamic scaling based on workload) than what the Temporal controller addresses (version management and deployment orchestration).

**Status**: ❌ Not a suitable replacement
**Action**: Keep current custom controller, monitor Temporal autoscaling development