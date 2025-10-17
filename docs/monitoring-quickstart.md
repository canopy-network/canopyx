# Monitoring Quick Start Guide
**Date**: October 17, 2025
**For**: Local development environment

---

## What's Installed

Prometheus + Grafana monitoring stack is now integrated into the Tiltfile and will deploy automatically with `tilt up`.

### Components
- **Prometheus**: Metrics collection and storage (2 days retention)
- **Grafana**: Visualization dashboards
- **Node Exporter**: Node-level metrics (CPU, memory, disk)
- **Kube State Metrics**: Kubernetes resource metrics

### Access URLs
- **Grafana**: http://localhost:3100 (admin/admin)
- **Prometheus**: http://localhost:9090

---

## Quick Start

### 1. Start the Stack
```bash
tilt up
```

Tilt will automatically:
- Add prometheus-community Helm repo
- Install kube-prometheus-stack
- Wait for pods to be ready
- Port-forward Grafana (3100) and Prometheus (9090)

### 2. Access Grafana
```bash
open http://localhost:3100
# Login: admin / admin
```

### 3. Explore Pre-Installed Dashboards
Grafana comes with several built-in dashboards:
- **Kubernetes / Compute Resources / Cluster** - Overall cluster metrics
- **Kubernetes / Compute Resources / Namespace (Pods)** - Per-namespace pod metrics
- **Node Exporter Full** - Detailed node metrics

---

## Next Steps

### Add Temporal Metrics Scraping

Temporal services already expose Prometheus metrics on port 9090 at `/metrics`.

**Option 1: Add ServiceMonitor (Recommended)**

Create `deploy/k8s/monitoring/temporal-servicemonitor.yaml`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: temporal-services
  labels:
    app.kubernetes.io/name: temporal
spec:
  selector:
    matchLabels:
      app.kubernetes.io/instance: temporal
  endpoints:
    - port: metrics
      path: /metrics
      interval: 30s
```

Then add to Tiltfile:
```python
k8s_yaml('./deploy/k8s/monitoring/temporal-servicemonitor.yaml')
```

**Option 2: Add Pod Annotations (Simpler)**

Update `deploy/helm/temporal-cassandra-elasticsearch-values.yaml`:

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

### Import Temporal Dashboard

1. Go to Grafana → Dashboards → Import
2. Use dashboard ID: **10867** (Temporal Server Metrics)
3. Select Prometheus datasource
4. Click Import

Or create custom queries:
```promql
# Workflow execution rate
rate(temporal_workflow_start_total[5m])

# Task queue depth
temporal_task_queue_workflow_tasks_pending

# RPC latency P99
histogram_quantile(0.99, temporal_rpc_latency_bucket)

# Rate limit errors
rate(temporal_rpc_errors_total{error_type="ResourceExhausted"}[5m])
```

### Add Cassandra Monitoring

**Install Cassandra Exporter** (Optional):

Add to `deploy/helm/temporal-cassandra-elasticsearch-values.yaml`:

```yaml
cassandra:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9500"

  # Add exporter sidecar
  extraContainers:
    - name: cassandra-exporter
      image: criteord/cassandra_exporter:latest
      ports:
        - containerPort: 9500
          name: metrics
      env:
        - name: CASSANDRA_HOST
          value: localhost
```

Then import Cassandra dashboard (ID: **11323**) in Grafana.

### Add ClickHouse Monitoring

ClickHouse exposes metrics on port 9363 at `/metrics` by default.

**Enable scraping** in `deploy/helm/clickhouse-values.yaml`:

```yaml
clickhouse:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9363"
    prometheus.io/path: "/metrics"
```

Then import ClickHouse dashboard (ID: **14192**) in Grafana.

---

## Useful Prometheus Queries

### System Health
```promql
# All pods running
up{job="kubernetes-pods"}

# Pod restart count
increase(kube_pod_container_status_restarts_total[1h])

# Memory usage by pod
container_memory_working_set_bytes{pod=~"temporal.*"}

# CPU usage by pod
rate(container_cpu_usage_seconds_total{pod=~"temporal.*"}[5m])
```

### Temporal Metrics
```promql
# Workflow throughput
sum(rate(temporal_workflow_start_total[5m])) by (task_queue)

# Success rate
sum(rate(temporal_workflow_completed_total[5m])) / sum(rate(temporal_workflow_start_total[5m]))

# Queue backlog
temporal_task_queue_workflow_tasks_pending

# History service CPU (approximation)
rate(container_cpu_usage_seconds_total{pod=~"temporal-history.*"}[5m])
```

### Indexer Metrics
```promql
# Indexer pod count
count(kube_pod_info{pod=~"canopyx-indexer.*"})

# Indexer memory usage
container_memory_working_set_bytes{pod=~"canopyx-indexer.*"}

# Indexer OOM kills
kube_pod_container_status_last_terminated_reason{pod=~"canopyx-indexer.*", reason="OOMKilled"}
```

---

## Creating Custom Dashboards

### Example: Temporal Overview Dashboard

1. Go to Grafana → Dashboards → New → New Dashboard
2. Add panels with these queries:

**Panel 1: Workflow Execution Rate**
```promql
sum(rate(temporal_workflow_start_total[5m]))
```

**Panel 2: Task Queue Depth**
```promql
temporal_task_queue_workflow_tasks_pending
```

**Panel 3: History Service CPU**
```promql
rate(container_cpu_usage_seconds_total{pod=~"temporal-history.*"}[5m])
```

**Panel 4: Service Memory**
```promql
container_memory_working_set_bytes{pod=~"temporal.*"} / 1024 / 1024 / 1024
```

3. Save dashboard with name "Temporal Overview"

---

## Troubleshooting

### Grafana Not Accessible
```bash
# Check Grafana pod status
kubectl get pods -l app.kubernetes.io/name=grafana

# Check logs
kubectl logs -l app.kubernetes.io/name=grafana --tail=50

# Verify port-forward
kubectl port-forward svc/prometheus-grafana 3100:80
```

### Prometheus Not Scraping
```bash
# Check Prometheus targets
open http://localhost:9090/targets

# Look for errors in target health status
# Common issues:
# - Pods not labeled for scraping
# - Metrics port not exposed
# - Firewall/network policy blocking
```

### High Resource Usage
The monitoring stack uses:
- Prometheus: ~2GB RAM, 0.5-1 CPU
- Grafana: ~256MB RAM, 0.1 CPU
- Exporters: ~512MB RAM total, 0.2 CPU

To reduce:
```bash
# Reduce retention (default: 2 days)
--set prometheus.prometheusSpec.retention=1d

# Reduce scrape frequency (default: 30s)
--set prometheus.prometheusSpec.scrapeInterval=60s

# Reduce storage size
--set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=10Gi
```

---

## Production Considerations

When moving to production, consider:

1. **Persistent Storage**: Use proper storage class with backups
2. **High Availability**: Run 2+ Prometheus replicas with remote write
3. **Alerting**: Enable Alertmanager and configure notifications (Slack, PagerDuty)
4. **Security**:
   - Change Grafana admin password
   - Enable TLS for Grafana
   - Add authentication for Prometheus
5. **Data Retention**: Adjust based on compliance requirements
6. **Remote Storage**: Use Thanos or Cortex for long-term storage

---

## References

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Temporal Metrics](https://docs.temporal.io/self-hosted-guide/monitoring)
- [Full Stability Plan](./stability-and-monitoring-plan.md)

---

**End of Quick Start**