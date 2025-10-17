# Grafana Dashboard Import Guide
**Quick reference for importing pre-built dashboards**

After `tilt up` and Grafana is running at http://localhost:3100, follow these steps to import monitoring dashboards.

---

## Dashboard List

### 1. ClickHouse Monitoring
**Dashboard ID**: [14192](https://grafana.com/grafana/dashboards/14192-clickhouse/)

**Metrics Included:**
- Query execution rate
- Insert rate
- Concurrent queries (current vs limit)
- Memory usage
- CPU usage
- Merge operations
- Part count

**Import Steps:**
1. Open http://localhost:3100
2. Click "Dashboards" (left sidebar) → "Import"
3. Enter `14192` in the "Import via grafana.com" field
4. Click "Load"
5. Select "Prometheus" as datasource
6. Click "Import"

---

### 2. Cassandra Monitoring
**Dashboard ID**: [10849](https://grafana.com/grafana/dashboards/10849-cassandra-dashboard/)

**Metrics Included:**
- Cluster health status
- Read/Write latency (P50, P95, P99)
- Compaction operations
- Cache hit rates
- Storage usage
- JVM heap memory

**Import Steps:**
1. Go to Dashboards → Import
2. Enter `10849`
3. Click "Load"
4. Select "Prometheus" as datasource
5. Click "Import"

---

### 3. Elasticsearch Monitoring
**Dashboard ID**: [9746](https://grafana.com/grafana/dashboards/9746-elasticsearch-example/)

**Metrics Included:**
- Cluster status (green/yellow/red)
- Indexing rate
- Search rate
- Index size
- JVM memory usage
- Query latency

**Import Steps:**
1. Go to Dashboards → Import
2. Enter `9746`
3. Click "Load"
4. Select "Prometheus" as datasource
5. Click "Import"

---

### 4. Temporal Server Monitoring
**Source**: [Official Temporal Dashboards (GitHub)](https://github.com/temporalio/dashboards)

**Recommended Dashboard**: `server-general.json`

**Metrics Included:**
- Workflow execution rate
- Task queue depth
- Service latency (P50, P95, P99)
- RPC errors
- Resource usage per service

**Import Steps:**
1. Download the dashboard JSON:
   ```bash
   cd /home/overlordyorch/Development/CanopyX/docs/grafana/dashboards
   curl -O https://raw.githubusercontent.com/temporalio/dashboards/main/server/server-general.json
   ```

2. In Grafana:
   - Go to Dashboards → Import
   - Click "Upload JSON file"
   - Select the downloaded `server-general.json`
   - Select "Prometheus" as datasource
   - Click "Import"

**Alternative**: Copy the JSON URL and paste it in the "Import via panel json" field

---

## Custom CanopyX Overview Dashboard

A custom dashboard for CanopyX is also available at:
```
docs/grafana/dashboards/canopyx-overview.json
```

**Import Steps:**
1. Go to Dashboards → Import
2. Click "Upload JSON file"
3. Select `canopyx-overview.json`
4. Select "Prometheus" as datasource
5. Click "Import"

**Panels Included:**
- Temporal workflow throughput
- Temporal queue depth
- Temporal CPU/Memory usage
- ClickHouse query rate
- ClickHouse concurrent queries
- Indexer CPU/Memory usage

---

## Quick Import All (Shell Commands)

Run these commands to download all Temporal dashboards:

```bash
# Create dashboards directory
mkdir -p docs/grafana/dashboards

# Download Temporal Server dashboards
cd docs/grafana/dashboards
curl -O https://raw.githubusercontent.com/temporalio/dashboards/main/server/server-general.json
curl -O https://raw.githubusercontent.com/temporalio/dashboards/main/server/server-resource-usage.json

# Return to project root
cd ../../../
```

Then import each JSON file via Grafana UI.

---

## Dashboard Organization

After importing, organize your dashboards:

1. **Temporal** folder:
   - Server General
   - Server Resource Usage

2. **Databases** folder:
   - ClickHouse
   - Cassandra
   - Elasticsearch

3. **CanopyX** folder:
   - CanopyX Overview

**To create folders:**
- Go to Dashboards → Browse
- Click "New" → "New folder"
- Drag dashboards into folders

---

## Troubleshooting

### Dashboard shows "No data"

**Check Prometheus targets:**
```bash
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'
```

All targets should show `"health": "up"`.

**Check if metrics exist:**
```bash
# Temporal metrics
curl 'http://localhost:9090/api/v1/query?query=temporal_workflow_start_total' | jq

# ClickHouse metrics
curl 'http://localhost:9090/api/v1/query?query=ClickHouseMetrics_Query' | jq
```

### Wrong datasource selected

1. Go to dashboard settings (⚙️ icon)
2. Click "Variables" or "JSON Model"
3. Update datasource to "Prometheus"
4. Save dashboard

### Metrics have different names

Some dashboards may use different metric naming conventions. Update queries:

**Example**: If dashboard uses `up` but you need `container_cpu_usage_seconds_total`:
1. Edit panel
2. Update query to match your metric names
3. Save

---

## Additional Dashboards

### Kubernetes Cluster Monitoring

If you installed kube-prometheus-stack (not our current setup), these are useful:

- **Dashboard ID 15757**: Kubernetes Cluster Overview
- **Dashboard ID 15758**: Kubernetes Pod Monitoring
- **Dashboard ID 15759**: Kubernetes Node Monitoring

For our simpler setup, we can query Kubernetes metrics directly via Prometheus.

---

## Next Steps

1. Import all 4 main dashboards (ClickHouse, Cassandra, Elasticsearch, Temporal)
2. Import custom CanopyX Overview dashboard
3. Create folders to organize dashboards
4. Set default dashboard in Grafana settings
5. Configure refresh intervals (default: 5s for CanopyX Overview)

---

**End of Guide**