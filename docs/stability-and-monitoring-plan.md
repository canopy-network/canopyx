# Stability & Monitoring Enhancement Plan
**Date**: October 17, 2025
**Goal**: Achieve 100% production-grade stability with comprehensive monitoring

---

## Current Stability Status

### ✅ Excellent Baseline
- **Zero pod restarts** during sustained load (450k+ workflows)
- **Zero OOMKilled errors** with 2Gi indexer memory
- **Zero rate limit errors** in client logs
- **Sustained throughput** at 3,500 wf/sec without degradation

### ⚠️ Stability Gaps to Address
1. **No resource limits on Temporal services** - Risk of CPU/memory hogging
2. **No pod disruption budgets** - Risk during node maintenance/upgrades
3. **Inconsistent health probes** - Some services lack proper liveness checks
4. **No monitoring/alerting** - Blind to degradation until failure
5. **Single-node Cassandra** - No fault tolerance for storage layer

---

## Phase 1: Resource Management (Immediate)

### 1.1 Add Resource Limits to Temporal Services

**Why**: Prevent resource exhaustion and enable predictable scheduling

**Changes to `deploy/helm/temporal-cassandra-elasticsearch-values.yaml`**:

```yaml
server:
  replicaCount: 3

  frontend:
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 2000m
        memory: 1Gi

  history:
    resources:
      requests:
        cpu: 2000m      # History is CPU-intensive
        memory: 2Gi
      limits:
        cpu: 4000m      # Allow bursting to 4 cores
        memory: 4Gi

  matching:
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 2000m
        memory: 1Gi

  worker:
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 1000m
        memory: 1Gi

# Web UI
web:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
```

**Expected Impact**:
- Kubernetes can make intelligent scheduling decisions
- Prevents any single pod from starving others
- Enables horizontal pod autoscaling (HPA) in future
- OOMKiller will be predictable and monitorable

### 1.2 Add Resource Limits to Cassandra

```yaml
cassandra:
  resources:
    requests:
      cpu: 4000m      # 4 cores baseline
      memory: 12Gi    # Adequate for 16GB heap + overhead
    limits:
      cpu: 8000m      # Allow bursting to 8 cores
      memory: 20Gi    # Prevent OOM on node
```

### 1.3 Add Resource Limits to ClickHouse

```yaml
clickhouse:
  resources:
    requests:
      cpu: 2000m
      memory: 4Gi
    limits:
      cpu: 4000m
      memory: 8Gi
```

### 1.4 Add Resource Limits to Elasticsearch

```yaml
elasticsearch:
  resources:
    requests:
      cpu: 1000m
      memory: 2Gi
    limits:
      cpu: 2000m
      memory: 4Gi
```

---

## Phase 2: High Availability (HA)

### 2.1 Pod Disruption Budgets (PDBs)

**Why**: Ensure minimum availability during:
- Node drains (maintenance)
- Cluster upgrades
- Voluntary pod evictions

**Create `deploy/k8s/pdb.yaml`**:

```yaml
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: temporal-frontend-pdb
spec:
  minAvailable: 2  # Keep at least 2 of 3 replicas running
  selector:
    matchLabels:
      app.kubernetes.io/component: frontend
      app.kubernetes.io/instance: temporal

---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: temporal-history-pdb
spec:
  minAvailable: 2  # Keep at least 2 of 3 replicas running
  selector:
    matchLabels:
      app.kubernetes.io/component: history
      app.kubernetes.io/instance: temporal

---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: temporal-matching-pdb
spec:
  minAvailable: 2  # Keep at least 2 of 3 replicas running
  selector:
    matchLabels:
      app.kubernetes.io/component: matching
      app.kubernetes.io/instance: temporal

---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: canopyx-indexer-pdb
spec:
  minAvailable: 3  # Keep at least 3 of 6 replicas running
  selector:
    matchLabels:
      app: indexer
      managed-by: canopyx-controller
```

### 2.2 Anti-Affinity Rules

**Why**: Spread pods across nodes for fault tolerance

**Add to Temporal Helm values**:

```yaml
server:
  frontend:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/component: frontend
              topologyKey: kubernetes.io/hostname

  history:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/component: history
              topologyKey: kubernetes.io/hostname

  matching:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/component: matching
              topologyKey: kubernetes.io/hostname
```

**Note**: Using `preferredDuringScheduling` instead of `requiredDuringScheduling` allows deployment on single-node dev clusters while enforcing on multi-node prod.

### 2.3 Health Probe Tuning

**Current State**: Temporal services have basic probes from Helm chart
**Action**: Review and tune timeouts/thresholds

**Example tuning for History service** (in Helm values):

```yaml
server:
  history:
    livenessProbe:
      initialDelaySeconds: 30
      periodSeconds: 10
      timeoutSeconds: 5
      failureThreshold: 3      # Allow 3 failures = 30s before restart

    readinessProbe:
      initialDelaySeconds: 10
      periodSeconds: 5
      timeoutSeconds: 3
      failureThreshold: 2      # Remove from service after 10s
```

---

## Phase 3: Monitoring & Observability

### 3.1 Install Prometheus + Grafana Stack

**Using kube-prometheus-stack Helm chart**:

```bash
# Add Prometheus community Helm repo
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Create namespace
kubectl create namespace monitoring

# Install kube-prometheus-stack
helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring \
  -f deploy/helm/prometheus-values.yaml
```

**Create `deploy/helm/prometheus-values.yaml`**:

```yaml
# Prometheus configuration
prometheus:
  prometheusSpec:
    retention: 7d  # Keep 7 days of metrics
    storageSpec:
      volumeClaimTemplate:
        spec:
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 50Gi  # Adequate for 7 days

    # Scrape Temporal metrics
    additionalScrapeConfigs:
      - job_name: 'temporal-services'
        kubernetes_sd_configs:
          - role: pod
            namespaces:
              names:
                - default
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_instance]
            action: keep
            regex: temporal
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
            action: keep
            regex: true
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
            action: replace
            target_label: __address__
            regex: ([^:]+)(?::\d+)?;(\d+)
            replacement: $1:$2

# Grafana configuration
grafana:
  enabled: true
  adminPassword: "admin"  # Change in production
  persistence:
    enabled: true
    size: 10Gi

  # Pre-configure datasource
  datasources:
    datasources.yaml:
      apiVersion: 1
      datasources:
        - name: Prometheus
          type: prometheus
          url: http://prometheus-kube-prometheus-prometheus:9090
          isDefault: true

  # Load dashboards from ConfigMaps
  dashboardProviders:
    dashboardproviders.yaml:
      apiVersion: 1
      providers:
        - name: 'default'
          orgId: 1
          folder: ''
          type: file
          disableDeletion: false
          editable: true
          options:
            path: /var/lib/grafana/dashboards/default

# Node exporter (for node metrics)
nodeExporter:
  enabled: true

# Kube-state-metrics (for k8s resource metrics)
kubeStateMetrics:
  enabled: true

# Alertmanager (for alerting)
alertmanager:
  enabled: true
  config:
    global:
      resolve_timeout: 5m
    route:
      group_by: ['alertname', 'cluster', 'service']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      receiver: 'default'
    receivers:
      - name: 'default'
        # Add webhook, email, or Slack config here
```

### 3.2 Enable Temporal Metrics Export

**Temporal already exposes Prometheus metrics by default!**

Verify with:
```bash
kubectl port-forward -n default svc/temporal-frontend 9090:9090
curl http://localhost:9090/metrics
```

**Add Prometheus scrape annotations to Temporal Helm values**:

```yaml
server:
  frontend:
    podAnnotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
      prometheus.io/path: "/metrics"

  history:
    podAnnotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
      prometheus.io/path: "/metrics"

  matching:
    podAnnotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
      prometheus.io/path: "/metrics"

  worker:
    podAnnotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
      prometheus.io/path: "/metrics"
```

### 3.3 Enable Cassandra Metrics Export

**Install Cassandra Prometheus Exporter as sidecar**:

```yaml
cassandra:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9500"

  # Add exporter sidecar (if not included in chart)
  extraContainers:
    - name: cassandra-exporter
      image: criteord/cassandra_exporter:latest
      ports:
        - containerPort: 9500
          name: metrics
      env:
        - name: CASSANDRA_HOST
          value: localhost
        - name: JMX_PORT
          value: "7199"
```

**Alternative**: Use Instaclustr Cassandra Exporter
https://github.com/instaclustr/cassandra-exporter

### 3.4 Enable ClickHouse Metrics Export

ClickHouse has built-in Prometheus metrics endpoint at `/metrics` on port 9363.

**Add to ClickHouse Helm values**:

```yaml
clickhouse:
  service:
    ports:
      - name: metrics
        port: 9363
        targetPort: 9363

  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9363"
    prometheus.io/path: "/metrics"
```

### 3.5 Grafana Dashboard: Temporal Overview

**Create `deploy/grafana/temporal-dashboard.json`** with panels for:

1. **Workflow Execution Rate**
   - Query: `rate(temporal_workflow_start_total[5m])`
   - Shows workflows/sec across all task queues

2. **Workflow Success/Failure Rate**
   - Success: `rate(temporal_workflow_completed_total[5m])`
   - Failure: `rate(temporal_workflow_failed_total[5m])`

3. **Task Queue Depth**
   - Workflow tasks: `temporal_task_queue_workflow_tasks_pending`
   - Activity tasks: `temporal_task_queue_activity_tasks_pending`

4. **Service RPC Latency**
   - P50: `histogram_quantile(0.5, temporal_rpc_latency_bucket)`
   - P99: `histogram_quantile(0.99, temporal_rpc_latency_bucket)`

5. **Service CPU Usage**
   - Query: `rate(container_cpu_usage_seconds_total{pod=~"temporal-.*"}[5m])`

6. **Service Memory Usage**
   - Query: `container_memory_working_set_bytes{pod=~"temporal-.*"}`

7. **Rate Limit Errors**
   - Query: `rate(temporal_rpc_errors_total{error_type="ResourceExhausted"}[5m])`

8. **History Shard Distribution**
   - Query: `temporal_history_shard_ownership_count`

### 3.6 Grafana Dashboard: Cassandra

**Create `deploy/grafana/cassandra-dashboard.json`** with panels for:

1. **Cluster Health**
   - Nodes up: `cassandra_cluster_node_count`
   - Node status: `cassandra_node_state`

2. **Write Performance**
   - Write latency P99: `cassandra_table_write_latency_99p`
   - Write throughput: `rate(cassandra_table_write_count[5m])`

3. **Read Performance**
   - Read latency P99: `cassandra_table_read_latency_99p`
   - Read throughput: `rate(cassandra_table_read_count[5m])`

4. **Thread Pools**
   - Mutation pending: `cassandra_threadpool_pending_tasks{pool="MutationStage"}`
   - Read pending: `cassandra_threadpool_pending_tasks{pool="ReadStage"}`
   - Blocked: `cassandra_threadpool_blocked_tasks`

5. **Compaction Status**
   - Pending compactions: `cassandra_table_pending_compactions`
   - Compaction rate: `rate(cassandra_table_compacted_bytes[5m])`

6. **Storage Metrics**
   - Disk usage: `cassandra_table_disk_space_used_bytes`
   - SSTable count: `cassandra_table_sstables_count`

7. **JVM Metrics**
   - Heap usage: `cassandra_jvm_heap_memory_usage`
   - GC pause time: `rate(cassandra_jvm_gc_pause_seconds_sum[5m])`

### 3.7 Grafana Dashboard: ClickHouse

**Create `deploy/grafana/clickhouse-dashboard.json`** with panels for:

1. **Query Performance**
   - Query rate: `rate(ClickHouseProfileEvents_Query[5m])`
   - Query duration P99: `histogram_quantile(0.99, ClickHouseProfileEvents_QueryTimeMicroseconds_bucket)`

2. **Insert Performance**
   - Insert rate: `rate(ClickHouseProfileEvents_InsertedRows[5m])`
   - Insert bytes/sec: `rate(ClickHouseProfileEvents_InsertedBytes[5m])`

3. **Connection Pool**
   - Active connections: `ClickHouseMetrics_TCPConnection`
   - Max connections: `ClickHouseAsyncMetrics_MaxPartCountForPartition`

4. **Merge Activity**
   - Merges in progress: `ClickHouseMetrics_Merge`
   - Background merges: `ClickHouseMetrics_BackgroundMergesAndMutationsPoolTask`

5. **Table Metrics (per chain)**
   - Rows per table: `ClickHouseAsyncMetrics_TotalRowsOfMergeTreeTables`
   - Parts per partition: `ClickHouseAsyncMetrics_MaxPartCountForPartition`

6. **Resource Usage**
   - CPU usage: `rate(ClickHouseProfileEvents_OSCPUVirtualTimeMicroseconds[5m])`
   - Memory usage: `ClickHouseMetrics_MemoryTracking`

7. **Query Queue**
   - Queued queries: `ClickHouseMetrics_QueryPreempted`
   - Rejected queries: `rate(ClickHouseProfileEvents_QueryRejected[5m])`

### 3.8 Grafana Dashboard: CanopyX Application

**Create `deploy/grafana/canopyx-dashboard.json`** with panels for:

1. **Indexing Progress (per chain)**
   - Current indexed height
   - Target height (chain head)
   - Gap between indexed and target
   - Indexing rate (blocks/sec)

2. **Indexer Health**
   - Replica count over time
   - Pod restart count
   - Memory usage per replica
   - CPU usage per replica

3. **Controller Metrics**
   - Scaling decisions over time
   - Queue backlog depth
   - Queue backlog age
   - Desired vs actual replicas

4. **Database Write Performance**
   - ClickHouse insert rate (blocks/sec)
   - Transaction insert rate
   - Failed writes

5. **RPC Health**
   - RPC call rate per chain
   - RPC error rate
   - RPC latency P95

---

## Phase 4: Alerting Rules

### 4.1 Critical Alerts

**Create `deploy/prometheus/alerts.yaml`**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-alerts
  namespace: monitoring
data:
  alerts.yml: |
    groups:
      - name: temporal-critical
        interval: 30s
        rules:
          # Temporal service down
          - alert: TemporalServiceDown
            expr: up{job="temporal-services"} == 0
            for: 1m
            labels:
              severity: critical
            annotations:
              summary: "Temporal {{ $labels.app_kubernetes_io_component }} service is down"
              description: "{{ $labels.pod }} has been down for more than 1 minute"

          # History service CPU saturation
          - alert: TemporalHistoryCPUSaturation
            expr: rate(container_cpu_usage_seconds_total{pod=~"temporal-history.*"}[5m]) > 3.5
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Temporal History service CPU saturated"
              description: "{{ $labels.pod }} is using {{ $value }} cores (>3.5)"

          # Task queue backlog growing
          - alert: TemporalQueueBacklogGrowing
            expr: increase(temporal_task_queue_workflow_tasks_pending[10m]) > 10000
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Task queue backlog growing rapidly"
              description: "Queue {{ $labels.task_queue }} grew by {{ $value }} tasks in 10 minutes"

          # Rate limit errors spiking
          - alert: TemporalRateLimitErrors
            expr: rate(temporal_rpc_errors_total{error_type="ResourceExhausted"}[5m]) > 10
            for: 2m
            labels:
              severity: warning
            annotations:
              summary: "High rate of rate limit errors"
              description: "{{ $value }} rate limit errors/sec on {{ $labels.service }}"

      - name: cassandra-critical
        interval: 30s
        rules:
          # Cassandra node down
          - alert: CassandraNodeDown
            expr: cassandra_node_state == 0
            for: 1m
            labels:
              severity: critical
            annotations:
              summary: "Cassandra node is down"
              description: "Cassandra node {{ $labels.instance }} is down"

          # Write latency degradation
          - alert: CassandraHighWriteLatency
            expr: cassandra_table_write_latency_99p > 100
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Cassandra write latency high"
              description: "P99 write latency is {{ $value }}ms (threshold: 100ms)"

          # Thread pool saturation
          - alert: CassandraThreadPoolSaturated
            expr: cassandra_threadpool_pending_tasks{pool="MutationStage"} > 100
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Cassandra mutation thread pool saturated"
              description: "{{ $value }} pending mutation tasks"

          # Compaction falling behind
          - alert: CassandraCompactionLag
            expr: cassandra_table_pending_compactions > 100
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "Cassandra compaction falling behind"
              description: "{{ $value }} pending compactions on {{ $labels.keyspace }}.{{ $labels.table }}"

      - name: clickhouse-critical
        interval: 30s
        rules:
          # ClickHouse service down
          - alert: ClickHouseDown
            expr: up{job="clickhouse"} == 0
            for: 1m
            labels:
              severity: critical
            annotations:
              summary: "ClickHouse is down"
              description: "ClickHouse instance {{ $labels.instance }} is down"

          # Too many rejected queries
          - alert: ClickHouseQueriesRejected
            expr: rate(ClickHouseProfileEvents_QueryRejected[5m]) > 1
            for: 2m
            labels:
              severity: warning
            annotations:
              summary: "ClickHouse rejecting queries"
              description: "{{ $value }} queries/sec rejected (concurrency limit hit)"

          # Merge queue growing
          - alert: ClickHouseMergeQueueGrowing
            expr: ClickHouseMetrics_BackgroundMergesAndMutationsPoolTask > 50
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "ClickHouse merge queue growing"
              description: "{{ $value }} pending merge tasks"

          # Too many parts per partition
          - alert: ClickHouseTooManyParts
            expr: ClickHouseAsyncMetrics_MaxPartCountForPartition > 300
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "ClickHouse has too many parts"
              description: "{{ $value }} parts in a partition (threshold: 300)"

      - name: indexer-critical
        interval: 30s
        rules:
          # Indexer pods restarting
          - alert: IndexerPodsRestarting
            expr: increase(kube_pod_container_status_restarts_total{pod=~"canopyx-indexer.*"}[10m]) > 0
            labels:
              severity: warning
            annotations:
              summary: "Indexer pod restarting"
              description: "{{ $labels.pod }} has restarted {{ $value }} times in 10 minutes"

          # Indexer OOMKilled
          - alert: IndexerOOMKilled
            expr: kube_pod_container_status_last_terminated_reason{pod=~"canopyx-indexer.*", reason="OOMKilled"} == 1
            labels:
              severity: critical
            annotations:
              summary: "Indexer pod OOMKilled"
              description: "{{ $labels.pod }} was killed due to out of memory"

          # Indexing stalled
          - alert: IndexingStalledForChain
            expr: increase(canopyx_indexed_height[5m]) == 0 and canopyx_indexed_height < canopyx_target_height - 100
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Indexing stalled for {{ $labels.chain_id }}"
              description: "No progress in 5 minutes, gap is {{ $value }} blocks"
```

### 4.2 Configure Alertmanager

**Add Slack webhook** (example in `prometheus-values.yaml`):

```yaml
alertmanager:
  config:
    receivers:
      - name: 'slack-notifications'
        slack_configs:
          - api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'
            channel: '#canopyx-alerts'
            title: '{{ .GroupLabels.alertname }}'
            text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

    route:
      receiver: 'slack-notifications'
      group_by: ['alertname', 'cluster']
      group_wait: 10s
      group_interval: 5m
      repeat_interval: 12h
      routes:
        - match:
            severity: critical
          receiver: 'slack-notifications'
          repeat_interval: 1h  # More frequent for critical
```

---

## Phase 5: Production Readiness Checklist

### 5.1 Storage Resilience
- [ ] Cassandra 3-node cluster (RF=3)
- [ ] Elasticsearch 3-node cluster
- [ ] PVC snapshots/backups configured
- [ ] Data retention policies defined

### 5.2 Network & Security
- [ ] Network policies to restrict pod-to-pod traffic
- [ ] TLS for Temporal gRPC connections
- [ ] TLS for Cassandra client connections
- [ ] Secrets management (not in Helm values)
- [ ] Rate limiting at ingress/API gateway level

### 5.3 Disaster Recovery
- [ ] Backup strategy for Cassandra (snapshots)
- [ ] Backup strategy for ClickHouse (exports)
- [ ] RTO/RPO defined
- [ ] DR runbook documented
- [ ] Regular restore testing

### 5.4 Operational Readiness
- [ ] Runbooks for common failures
- [ ] On-call rotation defined
- [ ] Escalation policies
- [ ] SLO/SLA definitions
- [ ] Capacity planning model

---

## Implementation Priority

### Immediate (This Week)
1. ✅ **Add resource limits to Temporal services** - Prevents resource hogging
2. ✅ **Create PodDisruptionBudgets** - Ensures availability during maintenance
3. ✅ **Install Prometheus + Grafana** - Visibility into system health

### Short-term (Next Week)
4. ⚠️ **Create Temporal dashboard** - Monitor workflow throughput/errors
5. ⚠️ **Create Cassandra dashboard** - Monitor storage layer performance
6. ⚠️ **Create ClickHouse dashboard** - Monitor database writes
7. ⚠️ **Configure basic alerting** - Get notified of critical issues

### Medium-term (Next Month)
8. 📋 **Scale Cassandra to 3 nodes** - Production-grade storage resilience
9. 📋 **Add anti-affinity rules** - Improve fault tolerance
10. 📋 **Implement backup strategy** - Data protection
11. 📋 **Write runbooks** - Operational documentation

---

## Expected Outcomes

After implementing this plan:

### Stability Improvements
- **99.9% uptime** during normal operations
- **Zero unplanned downtime** due to resource exhaustion
- **Graceful degradation** during node failures/maintenance
- **Predictable behavior** under load

### Observability Improvements
- **Real-time visibility** into all system components
- **<5 minute MTTD** (mean time to detect) for issues
- **<15 minute MTTR** (mean time to recover) for common failures
- **Proactive alerts** before user-facing impact

### Operational Improvements
- **Confident deployments** with rollback capabilities
- **Data-driven capacity planning** using historical metrics
- **Faster incident response** with clear dashboards
- **Knowledge sharing** through runbooks

---

## Cost Estimate (Monitoring Infrastructure)

**Additional resources required**:
- Prometheus: 2 CPU, 8GB RAM, 50GB storage
- Grafana: 1 CPU, 2GB RAM, 10GB storage
- Alertmanager: 0.5 CPU, 1GB RAM, 5GB storage
- Exporters (Cassandra/ClickHouse): 0.5 CPU, 512MB RAM per exporter

**Total overhead**: ~4 CPU, ~12GB RAM, ~70GB storage

**AWS pricing estimate** (for reference):
- EKS worker node (m5.xlarge: 4 vCPU, 16GB) = ~$140/month
- EBS storage (70GB) = ~$7/month

**Recommendation**: Use a dedicated monitoring node or add to existing nodes with headroom.

---

## References

### Documentation
- [Temporal Metrics](https://docs.temporal.io/self-hosted-guide/monitoring)
- [Cassandra Metrics](https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html)
- [ClickHouse Metrics](https://clickhouse.com/docs/en/operations/system-tables/metrics)
- [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)

### Dashboards
- [Temporal Grafana Dashboard](https://github.com/temporalio/dashboards)
- [Cassandra Grafana Dashboard](https://grafana.com/grafana/dashboards/11323-cassandra/)
- [ClickHouse Grafana Dashboard](https://grafana.com/grafana/dashboards/14192-clickhouse/)

---

**End of Plan**