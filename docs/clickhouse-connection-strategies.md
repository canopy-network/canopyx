# ClickHouse Connection Strategies Configuration Guide

## Overview

The ClickHouse client now supports configurable connection strategies via the `CLICKHOUSE_CONN_STRATEGY` environment variable. This allows different services to optimize for their specific access patterns.

## Available Strategies

| Strategy | Behavior | Use Case | Consistency Model |
|----------|----------|----------|-------------------|
| `in_order` | Always connects to first replica, falls back to others only on failure | Indexer (read-after-write within same service) | Strong (same-replica) |
| `round_robin` | Distributes connections evenly across all replicas | SuperApp/API (high read throughput) | Strong (via insert_quorum) |
| `random` | Randomly selects replica for each connection | SuperApp/API (load balancing) | Strong (via insert_quorum) |

## Service Configurations

### 1. Indexer (Admin & Controller)

**Strategy**: `in_order` (default)

**Why**: The indexer writes and reads its own data within the same service. Using `in_order` provides read-after-write consistency without any ClickHouse Keeper overhead.

**Configuration**:
```yaml
# deploy/k8s/admin/base/deployment.yaml
# deploy/k8s/controller/base/deployment.yaml
env:
  # Connection strategy (defaults to in_order, can omit this variable)
  - { name: CLICKHOUSE_CONN_STRATEGY, value: "in_order" }

  # Multiple replicas for failover
  # Note: Service names follow Altinity operator pattern: chi-{installation}-{cluster}-{shard}-{replica}
  - { name: CLICKHOUSE_ADDR, value: "clickhouse://canopyx:canopyx@chi-canopyx-canopyx-0-0.default.svc.cluster.local:9000,chi-canopyx-canopyx-0-1.default.svc.cluster.local:9000?sslmode=disable" }
```

**Expected Behavior**:
- All connections go to replica-0
- Automatic failover to replica-1 if replica-0 is down
- Zero Keeper coordination overhead
- Read-after-write consistency guaranteed

---

### 2. SuperApp Backend (Future)

**Strategy**: `round_robin` or `random`

**Why**: The SuperApp only reads data (written by the indexer). Using `round_robin` or `random` distributes read load across both replicas for 2× throughput.

**Key Insight**: This is safe because the indexer uses `insert_quorum=auto`, which ensures all data is on BOTH replicas before the write completes. Therefore, any replica can serve consistent reads.

**Configuration**:
```yaml
# superapp/deploy/k8s/deployment.yaml
env:
  # Use round_robin for even load distribution
  - { name: CLICKHOUSE_CONN_STRATEGY, value: "round_robin" }

  # OR use random for random load balancing
  # - { name: CLICKHOUSE_CONN_STRATEGY, value: "random" }

  # Multiple replicas for load distribution
  # Note: Service names follow Altinity operator pattern: chi-{installation}-{cluster}-{shard}-{replica}
  - { name: CLICKHOUSE_ADDR, value: "clickhouse://canopyx:canopyx@chi-canopyx-canopyx-0-0.default.svc.cluster.local:9000,chi-canopyx-canopyx-0-1.default.svc.cluster.local:9000?sslmode=disable" }

  # Large connection pool for high read throughput
  - { name: CLICKHOUSE_MAX_OPEN_CONNS, value: "100" }
  - { name: CLICKHOUSE_MAX_IDLE_CONNS, value: "50" }

dnsConfig:
  options:
    - name: ndots
      value: "1"  # DNS optimization for FQDN lookups
```

**Expected Behavior**:
- Queries distributed ~50/50 across both replicas
- 2× read throughput compared to single replica
- Strong consistency (thanks to indexer's insert_quorum)
- Zero Keeper coordination overhead

---

## How It Works

### Architecture

```
┌─────────────────────────────────────────────────┐
│                   INDEXER                        │
│  Strategy: in_order                              │
│  Writes: insert_quorum=auto (waits for 2/2)     │
│                                                  │
│              ┌──────────────┐                    │
│              │   Keeper     │                    │
│              └──────┬───────┘                    │
│                     │                            │
│          ┌──────────┴──────────┐                 │
│          ▼                     ▼                 │
│     ┌─────────┐          ┌─────────┐            │
│     │Replica-0│          │Replica-1│            │
│     └────┬────┘          └────┬────┘            │
└──────────┼──────────────────────┼───────────────┘
           │                     │
           │  Both replicas      │
           │  have same data     │
           │  (quorum writes)    │
           │                     │
┌──────────┴─────────────────────┴───────────────┐
│                  SUPERAPP                       │
│  Strategy: round_robin                          │
│  Reads: Distributed across both replicas       │
│  No Keeper coordination needed!                │
└─────────────────────────────────────────────────┘
```

### Why This Is Safe

1. **Indexer writes with insert_quorum=auto**
   - Waits for 2/2 replicas to acknowledge
   - Data guaranteed on BOTH replicas before success

2. **SuperApp reads with round_robin**
   - Both replicas have identical data (from quorum writes)
   - No possibility of stale reads
   - No Keeper coordination needed
   - 2× throughput

3. **No race conditions**
   - Indexer's PromoteData uses insert_quorum
   - SuperApp will never read promoted data until it's on both replicas
   - Perfect consistency without sequential_consistency overhead

---

## Connection Strategy Implementation

### Code (pkg/db/clickhouse/client.go)

```go
// Parse connection strategy from environment
connStrategy := parseConnOpenStrategy(utils.Env("CLICKHOUSE_CONN_STRATEGY", "in_order"))

options := &clickhouse.Options{
    Addr: replicas,  // Multiple replica addresses
    ConnOpenStrategy: connStrategy,  // Configurable strategy
    // ... other options
}
```

### Strategy Parser

```go
func parseConnOpenStrategy(strategy string) clickhouse.ConnOpenStrategy {
    switch strings.ToLower(strings.TrimSpace(strategy)) {
    case "round_robin", "roundrobin":
        return clickhouse.ConnOpenRoundRobin
    case "random":
        return clickhouse.ConnOpenRandom
    case "in_order", "inorder", "":
        return clickhouse.ConnOpenInOrder
    default:
        return clickhouse.ConnOpenInOrder  // Safe default
    }
}
```

---

## Performance Comparison

### Indexer (in_order)

| Metric | Value | Notes |
|--------|-------|-------|
| Write latency | +50-100ms | insert_quorum overhead |
| Read latency | ~16ms | Single replica, FINAL queries |
| Read throughput | Single replica capacity | Consistent performance |
| Keeper overhead | None | No coordination on reads |

### SuperApp (round_robin)

| Metric | Value | Notes |
|--------|-------|-------|
| Write latency | N/A | SuperApp doesn't write |
| Read latency | ~8-16ms | Distributed across 2 replicas |
| Read throughput | **2× single replica** | True load balancing |
| Keeper overhead | None | No coordination needed |

---

## Monitoring Connection Strategy

The connection strategy is logged on startup:

```
INFO  ClickHouse connection pool configured
  database=chain_1
  component=indexer_chain
  replicas=[clickhouse-canopyx-0...:9000, clickhouse-canopyx-1...:9000]
  conn_strategy=in_order
  max_open_conns=40
  max_idle_conns=15
  conn_max_lifetime=5m0s
```

Check your logs to verify the strategy is correctly configured.

---

## Testing Different Strategies

### Test Query Distribution

```bash
# Run queries from SuperApp
for i in {1..100}; do
  curl "http://superapp/api/blocks/${RANDOM}"
done

# Check which replicas served the queries
# On ClickHouse, run:
SELECT
    hostname(),
    count() as query_count
FROM system.query_log
WHERE type = 'QueryFinish'
  AND event_time >= now() - INTERVAL 1 MINUTE
  AND query LIKE '%FROM blocks%'
GROUP BY hostname();

# Expected with round_robin: ~50/50 distribution
# Expected with in_order: 100% on replica-0
# Expected with random: ~50/50 distribution (slightly less even than round_robin)
```

### Test Consistency

```go
// Write via indexer
indexer.InsertAndPromote(ctx, block)

// IMMEDIATELY read from SuperApp (might hit different replica)
// Should NEVER fail because insert_quorum guarantees data on both replicas
superapp.GetBlock(ctx, blockHeight)
```

---

## Migration Guide

### From: Single Replica / Headless Service
### To: Multi-Replica with Configurable Strategy

1. **Update CLICKHOUSE_ADDR** to include multiple replicas:
   ```yaml
   # OLD (headless service):
   - { name: CLICKHOUSE_ADDR, value: "clickhouse://canopyx:canopyx@clickhouse-canopyx.default.svc.cluster.local:9000" }

   # NEW (explicit replica services - Altinity operator pattern):
   # Format: chi-{installation}-{cluster}-{shard}-{replica}.{namespace}.svc.cluster.local
   - { name: CLICKHOUSE_ADDR, value: "clickhouse://canopyx:canopyx@chi-canopyx-canopyx-0-0.default.svc.cluster.local:9000,chi-canopyx-canopyx-0-1.default.svc.cluster.local:9000?sslmode=disable" }
   ```

2. **Add CLICKHOUSE_CONN_STRATEGY** for non-default strategies:
   ```yaml
   # For SuperApp (reads only):
   - { name: CLICKHOUSE_CONN_STRATEGY, value: "round_robin" }

   # For Indexer (writes + reads):
   # Omit this variable (defaults to in_order)
   ```

3. **Add DNS optimization**:
   ```yaml
   dnsConfig:
     options:
       - name: ndots
         value: "1"
   ```

4. **Deploy and monitor**:
   - Check logs for `conn_strategy` value
   - Monitor query distribution (see Testing section)
   - Verify no stale reads occur

---

## Troubleshooting

### Issue: All queries hitting single replica with round_robin

**Cause**: Only one replica address in CLICKHOUSE_ADDR

**Fix**: Ensure CLICKHOUSE_ADDR has comma-separated replicas:
```
clickhouse://user:pass@replica-0:9000,replica-1:9000
```

### Issue: Stale reads with round_robin

**Cause**: Indexer not using insert_quorum

**Fix**: Ensure indexer uses `PrepareBatchWithQuorum()` or `WithInsertQuorum(ctx)`:
```go
// Staging writes MUST use quorum
batch, err := db.PrepareBatchWithQuorum(ctx, query)

// Promotion MUST use quorum
ctx = clickhouse.WithInsertQuorum(ctx)
db.Exec(ctx, promotionQuery)
```

### Issue: High latency with round_robin

**Cause**: One replica is slow/unhealthy

**Fix**: Check replica health:
```sql
SELECT
    replica_name,
    absolute_delay,
    queue_size
FROM system.replicas
WHERE database = 'chain_1';
```

---

## Summary

- **Indexer**: Use `in_order` (default) for read-after-write consistency
- **SuperApp**: Use `round_robin` or `random` for read distribution
- **Key**: indexer's insert_quorum makes round_robin safe for readers
- **Result**: 2× read throughput without consistency sacrifice