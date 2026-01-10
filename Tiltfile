load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://k8s_attach', 'k8s_attach')
load('ext://restart_process', 'docker_build_with_restart')

# ------------------------------------------
# TILT SETTINGS
# ------------------------------------------

# Increase timeout for slow operations (like Helm installs on first run)
update_settings(k8s_upsert_timeout_secs=300)  # 5 minutes for apply operations

# ------------------------------------------
# CONFIGURATION SYSTEM
# ------------------------------------------

# Load YAML configuration
config_file = './tilt-config.yaml'
default_config_file = './tilt-config.default.yaml'

# Create default config if it doesn't exist
if not os.path.exists(config_file):
    print("No tilt-config.yaml found - creating default configuration")
    if os.path.exists(default_config_file):
        local('cp %s %s' % (default_config_file, config_file))
    else:
        fail("tilt-config.default.yaml not found - cannot create default configuration")

# Load configuration
cfg = read_yaml(config_file)
profile = cfg.get('profile', 'development')
mode = cfg.get('mode', 'local')
components = cfg.get('components', {})
resources_cfg = cfg.get('resources', {}).get(profile, {})
monitoring_cfg = cfg.get('monitoring', {})
ports = cfg.get('ports', {})
paths_cfg = cfg.get('paths', {})
dev_cfg = cfg.get('dev', {})
chains_cfg = cfg.get('chains', [])
env_cfg = cfg.get('env', {})

print("Tilt Profile: %s" % profile)
print("Tilt Mode: %s" % mode)
print("Components enabled: %s" % ', '.join([k for k, v in components.items() if v]))

# Ensure kind local registry is available for localhost:5001 images.
local_resource(
  'kind-registry-ready',
  'set -e; if kubectl config current-context 2>/dev/null | grep -q "^kind-"; then make kind-ensure-registry kind-link-registry kind-doc-registry; else echo "Skipping kind registry setup (non-kind kubecontext)"; fi',
  labels=['infra']
)

# Helper function to get port from config with default fallback
def get_port(service, default):
    return ports.get(service, default)

# ------------------------------------------
# HELM REPOSITORIES
# ------------------------------------------

helm_repo(
  name='altinity',
  url='https://docs.altinity.com/clickhouse-operator/',
  labels=['helm_repo'],
  resource_name='helm-repo-altinity'
)

helm_repo(
  name='temporal',
  url='https://go.temporal.io/helm-charts',
  labels=['helm_repo'],
  resource_name='helm-repo-temporal'
)

helm_repo(
  name='bitnami',
  url='https://charts.bitnami.com/bitnami',
  labels=['helm_repo'],
  resource_name='helm-repo-bitnami'
)

helm_repo(
  name='clickhouse-operator',
  url='https://helm.altinity.com',
  labels=['helm_repo'],
  resource_name='helm-repo-clickhouse-operator'
)

# ------------------------------------------
# CLICKHOUSE (Always Required)
# ------------------------------------------
# ClickHouse is always deployed via Altinity ClickHouse Operator

print("Deploying ClickHouse via Altinity ClickHouse Operator")

# Generate dynamic storage and resource patch based on tilt-config.yaml
clickhouse_cfg = resources_cfg.get('clickhouse', {})
hot_storage = clickhouse_cfg.get('hot_storage', '2Gi')
warm_storage = clickhouse_cfg.get('warm_storage', '2Gi')
cold_storage = clickhouse_cfg.get('cold_storage', '5Gi')
log_storage = clickhouse_cfg.get('log_storage', '1Gi')
# CPU/Memory resources for ClickHouse pods
ch_cpu_limit = clickhouse_cfg.get('cpu_limit', '2000m')
ch_cpu_request = clickhouse_cfg.get('cpu_request', '1000m')
ch_memory_limit = clickhouse_cfg.get('memory_limit', '4Gi')
ch_memory_request = clickhouse_cfg.get('memory_request', '2Gi')

# Build storage and resource patch YAML
storage_patch = """# Auto-generated patch from tilt-config.yaml
# Profile: {profile} | CPU: {cpu_req}/{cpu_lim} | Memory: {mem_req}/{mem_lim}
# Storage: Hot={hot} | Warm={warm} | Cold={cold} | Log={log}
apiVersion: clickhouse.altinity.com/v1
kind: ClickHouseInstallation
metadata:
  name: canopyx
spec:
  templates:
    podTemplates:
      - name: clickhouse-pod
        spec:
          containers:
            - name: clickhouse
              image: clickhouse/clickhouse-server:25.8
              env:
                - name: KUBERNETES_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.namespace
              volumeMounts:
                - name: hot-volume
                  mountPath: /var/lib/clickhouse/hot
                - name: warm-volume
                  mountPath: /var/lib/clickhouse/warm
                - name: cold-volume
                  mountPath: /var/lib/clickhouse/cold
              resources:
                requests:
                  memory: "{mem_req}"
                  cpu: "{cpu_req}"
                limits:
                  memory: "{mem_lim}"
                  cpu: "{cpu_lim}"
  defaults:
    templates:
      volumeClaimTemplates:
        - name: hot-volume
          spec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: {hot}

        - name: warm-volume
          spec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: {warm}

        - name: cold-volume
          spec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: {cold}

        - name: log-volume
          spec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: {log}
""".format(
    profile=profile,
    hot=hot_storage, warm=warm_storage, cold=cold_storage, log=log_storage,
    cpu_lim=ch_cpu_limit, cpu_req=ch_cpu_request,
    mem_lim=ch_memory_limit, mem_req=ch_memory_request
)

# Write patch file using Python
local_resource(
  'generate-clickhouse-storage-patch',
  """python3 -c "
import os
os.makedirs('deploy/k8s/clickhouse/overlays/local', exist_ok=True)
with open('deploy/k8s/clickhouse/overlays/local/pvc-sizes.yaml', 'w') as f:
    f.write('''%s''')
" """ % storage_patch,
  labels=['clickhouse']
)

print("  ClickHouse resources (profile=%s): CPU=%s/%s, Memory=%s/%s" % (profile, ch_cpu_request, ch_cpu_limit, ch_memory_request, ch_memory_limit))
print("  ClickHouse storage: hot=%s, warm=%s, cold=%s, log=%s" % (hot_storage, warm_storage, cold_storage, log_storage))

# Step 1: Deploy ClickHouse Operator (installs CRDs)
helm_resource(
  name='clickhouse-operator',
  chart='clickhouse-operator/altinity-clickhouse-operator',
  release_name='clickhouse-operator',
  flags=['--set=metrics.enabled=true'],
  pod_readiness='wait',
  resource_deps=['helm-repo-clickhouse-operator'],
  labels=['clickhouse']
)

# Step 1.5: Wait 30s for operator to register CRDs
local_resource(
  'clickhouse-operator-ready',
  'echo "Waiting 30s for ClickHouse operator CRDs to be registered..." && sleep 30',
  resource_deps=['clickhouse-operator', 'generate-clickhouse-storage-patch'],
  labels=['clickhouse']
)

# Step 2 & 3: Deploy ClickHouseKeeperInstallation and ClickHouseInstallation
# Using kustomize overlay with dynamically generated storage patch from tilt-config.yaml
k8s_yaml(kustomize('./deploy/k8s/clickhouse/overlays/local'))

k8s_resource(
  objects=['canopyx-keeper:ClickHouseKeeperInstallation:default'],
  new_name='clickhouse-keeper',
  resource_deps=['clickhouse-operator-ready'],
  labels=['clickhouse']
)

k8s_resource(
  objects=['canopyx:ClickHouseInstallation:default'],
  new_name='clickhouse',
  resource_deps=['clickhouse-keeper'],
  labels=['clickhouse'],
  port_forwards=[
    "%s:8123" % get_port('clickhouse_server', 8123),
    "%s:9000" % get_port('clickhouse_native', 9000)
  ]
)

# Attach to individual ClickHouse replica pods for debugging replication
# These pods are created by the clickhouse-operator
k8s_attach('clickhouse-replica-0', 'pod/chi-canopyx-canopyx-0-0-0')
k8s_resource(
  'clickhouse-replica-0',
  resource_deps=['clickhouse'],
  labels=['clickhouse'],
  port_forwards=[
    "%s:8123" % get_port('clickhouse_replica_0_http', 8124),
    "%s:9000" % get_port('clickhouse_replica_0_native', 9001)
  ]
)

k8s_attach('clickhouse-replica-1', 'pod/chi-canopyx-canopyx-0-1-0')
k8s_resource(
  'clickhouse-replica-1',
  resource_deps=['clickhouse'],
  labels=['clickhouse'],
  port_forwards=[
    "%s:8123" % get_port('clickhouse_replica_1_http', 8125),
    "%s:9000" % get_port('clickhouse_replica_1_native', 9002)
  ]
)

# Attach to individual ClickHouse keeper replicas pods for log tracking
# These pods are created by the clickhouse-operator
k8s_attach('clickhouse-keeper-0', 'pod/chk-canopyx-keeper-canopyx-0-0-0')
k8s_resource(
  'clickhouse-keeper-0',
  resource_deps=['clickhouse'],
  labels=['clickhouse']
)
# Attach to individual ClickHouse keeper replicas pods for log tracking
# These pods are created by the clickhouse-operator
k8s_attach('clickhouse-keeper-1', 'pod/chk-canopyx-keeper-canopyx-0-1-0')
k8s_resource(
  'clickhouse-keeper-1',
  resource_deps=['clickhouse'],
  labels=['clickhouse']
)
# Attach to individual ClickHouse keeper replicas pods for log tracking
# These pods are created by the clickhouse-operator
k8s_attach('clickhouse-keeper-2', 'pod/chk-canopyx-keeper-canopyx-0-2-0')
k8s_resource(
  'clickhouse-keeper-2',
  resource_deps=['clickhouse'],
  labels=['clickhouse']
)

# Cleanup ClickHouse PVCs on tilt down (prevents stale replica metadata)
k8s_custom_deploy(
  'cleanup-clickhouse-data',
  apply_cmd='true',
  delete_cmd='''
    echo "Cleaning up ClickHouse persistent data..." && \
    echo "Deleting ClickHouse StatefulSets..." && \
    kubectl delete statefulset -l clickhouse.altinity.com/chi=canopyx --ignore-not-found --wait && \
    echo "Deleting ClickHouse Keeper StatefulSets..." && \
    kubectl delete statefulset -l clickhouse.altinity.com/chk=canopyx-keeper --ignore-not-found --wait && \
    echo "Force-removing ClickHouse installation finalizers..." && \
    kubectl patch clickhouseinstallation canopyx -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true && \
    kubectl patch clickhousekeeperinstallation canopyx-keeper -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true && \
    echo "Waiting for ClickHouse installations to be fully removed..." && \
    kubectl wait --for=delete clickhouseinstallation/canopyx --timeout=30s 2>/dev/null || true && \
    kubectl wait --for=delete clickhousekeeperinstallation/canopyx-keeper --timeout=30s 2>/dev/null || true && \
    echo "Deleting ClickHouse Keeper PVCs (must wait to clear replica metadata)..." && \
    kubectl delete pvc -l clickhouse-keeper.altinity.com/chk=canopyx-keeper --ignore-not-found --wait && \
    echo "Deleting ClickHouse data PVCs (background)..." && \
    kubectl delete pvc -l clickhouse.altinity.com/chi=canopyx --ignore-not-found --wait=false && \
    echo "ClickHouse cleanup complete (data PVCs deleting in background)"
  ''',
  deps=[],
)

k8s_resource('cleanup-clickhouse-data', resource_deps=['clickhouse', 'clickhouse-keeper'], labels=['no-op'])

# ------------------------------------------
# TEMPORAL (Always Required)
# ------------------------------------------
# Temporal is always deployed - resource limits controlled via profile configuration

# Build Temporal resource limit flags from profile
temporal_cfg = resources_cfg.get('temporal', {})
temporal_flags = ['--values=./deploy/helm/temporal-cassandra-elasticsearch-values.yaml']

# Temporal replica counts
if temporal_cfg.get('history_replicas'):
    temporal_flags.append('--set=server.replicaCount=%s' % temporal_cfg['history_replicas'])

# Force single-replica configuration for Cassandra and Elasticsearch
# This prevents shard allocation errors on single-node Kubernetes clusters
# Even in production profile, we enforce simple replication to avoid 503 errors
temporal_flags.append('--set=cassandra.config.cluster_size=1')
temporal_flags.append('--set=cassandra.replicas=1')
temporal_flags.append('--set=cassandra.affinity=null')
temporal_flags.append('--set=elasticsearch.replicas=1')
temporal_flags.append('--set=elasticsearch.antiAffinity=soft')

# Cassandra resource limits
cass_mem_limit = temporal_cfg.get('cassandra_memory_limit', '4G')
cass_heap = temporal_cfg.get('cassandra_heap_size', '2G')
temporal_flags.append('--set=cassandra.config.max_heap_size=%s' % cass_heap)
temporal_flags.append('--set=cassandra.resources.limits.memory=%s' % cass_mem_limit)
temporal_flags.append('--set=cassandra.resources.requests.memory=%s' % cass_mem_limit)

# Elasticsearch resource limits
if temporal_cfg.get('elasticsearch_memory_limit'):
    temporal_flags.append('--set=elasticsearch.resources.limits.memory=%s' % temporal_cfg['elasticsearch_memory_limit'])
    # Set requests to match limits to avoid validation errors
    temporal_flags.append('--set=elasticsearch.resources.requests.memory=%s' % temporal_cfg['elasticsearch_memory_limit'])
if temporal_cfg.get('elasticsearch_cpu_limit'):
    temporal_flags.append('--set=elasticsearch.resources.limits.cpu=%s' % temporal_cfg['elasticsearch_cpu_limit'])
    # Set CPU request to half of limit for better scheduling
    cpu_value = temporal_cfg['elasticsearch_cpu_limit']
    if 'm' in cpu_value:
        cpu_request = str(int(cpu_value.replace('m', '')) // 2) + 'm'
    else:
        cpu_request = str(float(cpu_value) / 2)
    temporal_flags.append('--set=elasticsearch.resources.requests.cpu=%s' % cpu_request)

print("Temporal resources: History replicas=%s, Cassandra replicas=%s, ES replicas=%s" % (
    temporal_cfg.get('history_replicas', 'default'),
    temporal_cfg.get('cassandra_replicas', 'default'),
    temporal_cfg.get('elasticsearch_replicas', 'default')
))

helm_resource(
  name='temporal',
  chart='temporal/temporal',
  release_name='temporal',
  flags=temporal_flags,
  pod_readiness='wait',
  resource_deps=['helm-repo-temporal'],
  labels=['temporal']
)

k8s_attach(
    name="temporal-web",
    obj="deployment/temporal-web",
    port_forwards=["%s" % get_port('temporal_web', 8080)],
    resource_deps=["temporal"],
    labels=['temporal'],
)

k8s_attach(
    name="temporal-frontend",
    obj="deployment/temporal-frontend",
    port_forwards=["%s" % get_port('temporal_frontend', 7233)],
    resource_deps=["temporal"],
    labels=['temporal'],
)

k8s_attach(
    name="temporal-worker",
    obj="deployment/temporal-worker",
    port_forwards=["%s" % get_port('temporal_worker', 7239), "6939"],
    resource_deps=["temporal"],
    labels=['temporal'],
)

# Attach to Cassandra for monitoring (deployed by Temporal Helm chart)
k8s_attach(
    name="temporal-cassandra",
    obj="statefulset/temporal-cassandra",
    port_forwards=[
        "%s:9042" % get_port('cassandra_cql', 9042),      # CQL native protocol
        "%s:7199" % get_port('cassandra_jmx', 7199),      # JMX
    ],
    resource_deps=["temporal"],
    labels=['temporal'],
)

# Attach to Cassandra for monitoring (deployed by Temporal Helm chart)
k8s_attach(
    name="temporal-elasticsearch",
    obj="statefulset/elasticsearch-master",
    port_forwards=[
        "%s:9200" % get_port('elasticsearch_http', 9200)      # http
    ],
    resource_deps=["temporal"],
    labels=['temporal'],
)

# ------------------------------------------
# REDIS (Always Required)
# ------------------------------------------
# Redis is used for real-time event notifications (Pub/Sub)

helm_resource(
  name='redis',
  chart='bitnami/redis',
  release_name='redis',
  flags=['--values=./deploy/helm/redis-values.yaml'],
  pod_readiness='wait',
  resource_deps=['helm-repo-bitnami'],
  labels=['redis']
)

k8s_attach(
    name="redis-master",
    obj="pod/redis-master-0",
    port_forwards=["%s:6379" % get_port('redis', 6379)],
    resource_deps=["redis"],
    labels=['redis'],
)

# Cleanup Redis data on tilt down
k8s_custom_deploy(
  'cleanup-redis-data',
  apply_cmd='true',
  delete_cmd='''
    echo "Cleaning up Redis persistent data..." && \
    kubectl delete pvc -l app.kubernetes.io/name=redis --ignore-not-found --wait && \
    echo "Redis data cleanup complete"
  ''',
  deps=[],
)

k8s_resource('cleanup-redis-data', resource_deps=['redis'], labels=['no-op'])

# ------------------------------------------
# MONITORING (Optional - Prometheus + Grafana)
# ------------------------------------------

if components.get('monitoring', False):
    print("Monitoring enabled - deploying Prometheus and Grafana")

    # Get resource limits from config
    prom_cpu = resources_cfg.get('monitoring', {}).get('prometheus_cpu_limit', '1000m')
    prom_mem = resources_cfg.get('monitoring', {}).get('prometheus_memory_limit', '1Gi')
    grafana_cpu = resources_cfg.get('monitoring', {}).get('grafana_cpu_limit', '500m')
    grafana_mem = resources_cfg.get('monitoring', {}).get('grafana_memory_limit', '512Mi')

    print("Prometheus resources: CPU=%s, Memory=%s" % (prom_cpu, prom_mem))
    print("Grafana resources: CPU=%s, Memory=%s" % (grafana_cpu, grafana_mem))

    # Load Prometheus manifests and apply resource limits + config from profile
    prom_objects = decode_yaml_stream(kustomize("./deploy/k8s/prometheus/overlays/local"))

    scrape_interval = monitoring_cfg.get('prometheus', {}).get('scrape_interval', '15s')
    retention = monitoring_cfg.get('prometheus', {}).get('retention', '2d')

    for o in prom_objects:
        # Update ConfigMap with scrape interval
        if o.get('kind') == 'ConfigMap' and o.get('metadata', {}).get('name') == 'prometheus-config':
            config_data = o.get('data', {}).get('prometheus.yml', '')
            # Update scrape interval
            config_data = config_data.replace('scrape_interval: 15s', 'scrape_interval: %s' % scrape_interval)
            config_data = config_data.replace('evaluation_interval: 15s', 'evaluation_interval: %s' % scrape_interval)
            o['data']['prometheus.yml'] = config_data

        # Update Deployment with resource limits and retention
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'prometheus':
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'prometheus':
                    # Update retention arg
                    args = container.get('args', [])
                    for i, arg in enumerate(args):
                        if arg.startswith('--storage.tsdb.retention.time='):
                            args[i] = '--storage.tsdb.retention.time=%s' % retention

                    # Set resource limits
                    container['resources'] = {
                        'limits': {
                            'cpu': prom_cpu,
                            'memory': prom_mem
                        },
                        'requests': {
                            'cpu': str(int(prom_cpu.replace('m', '')) // 2) + 'm' if 'm' in prom_cpu else str(float(prom_cpu) / 2),
                            'memory': str(int(prom_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in prom_mem else str(int(prom_mem.replace('Mi', '')) // 2) + 'Mi'
                        }
                    }

    k8s_yaml(encode_yaml_stream(prom_objects))

    k8s_resource(
        'prometheus',
        port_forwards='%s:9090' % get_port('prometheus', 9090),
        objects=[
            'prometheus:serviceaccount',
            'prometheus:clusterrole',
            'prometheus:clusterrolebinding',
            'prometheus-config:configmap'
        ],
        labels=['monitoring'],
    )

    k8s_custom_deploy(
      'cleanup-prometheus-data',
      apply_cmd='true',
      delete_cmd='''
        echo "Cleaning up Prometheus persistent data..." && \
        kubectl delete pvc -l app.kubernetes.io/name=prometheus --ignore-not-found --wait
        echo "Prometheus data cleanup complete"
      ''',
      deps=[],
    )

    k8s_resource('cleanup-prometheus-data', resource_deps=['prometheus'], labels=['no-op'])

    # Load Grafana manifests and apply resource limits
    grafana_objects = decode_yaml_stream(kustomize("./deploy/k8s/grafana/overlays/local"))

    # Handle dashboard injection
    dashboard_files = monitoring_cfg.get('dashboards', [])

    for o in grafana_objects:
        # Update Deployment with resource limits
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'grafana':
            spec = o.get('spec', {}).get('template', {}).get('spec', {})
            containers = spec.get('containers', [])

            for container in containers:
                if container.get('name') == 'grafana':
                    # Set resource limits
                    container['resources'] = {
                        'limits': {
                            'cpu': grafana_cpu,
                            'memory': grafana_mem
                        },
                        'requests': {
                            'cpu': str(int(grafana_cpu.replace('m', '')) // 2) + 'm' if 'm' in grafana_cpu else str(float(grafana_cpu) / 2),
                            'memory': str(int(grafana_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in grafana_mem else str(int(grafana_mem.replace('Mi', '')) // 2) + 'Mi'
                        }
                    }

                    # Add dashboard volume mount if dashboards configured
                    if dashboard_files:
                        volumeMounts = container.get('volumeMounts', [])
                        volumeMounts.append({
                            'name': 'dashboards',
                            'mountPath': '/var/lib/grafana/dashboards'
                        })
                        container['volumeMounts'] = volumeMounts

            # Add dashboard volume if dashboards configured
            if dashboard_files:
                volumes = spec.get('volumes', [])
                volumes.append({
                    'name': 'dashboards',
                    'configMap': {
                        'name': 'grafana-dashboards'
                    }
                })
                spec['volumes'] = volumes

    # Create dashboard ConfigMap if needed
    if dashboard_files:
        dashboard_configmap = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {
                'name': 'grafana-dashboards'
            },
            'data': {}
        }

        for dashboard_path in dashboard_files:
            if os.path.exists(dashboard_path):
                dashboard_name = os.path.basename(dashboard_path)
                print("Loading Grafana dashboard: %s" % dashboard_path)
                dashboard_content = str(read_file(dashboard_path))
                dashboard_configmap['data'][dashboard_name] = dashboard_content

        grafana_objects.append(dashboard_configmap)

    k8s_yaml(encode_yaml_stream(grafana_objects))

    k8s_resource(
        'grafana',
        port_forwards='%s:3000' % get_port('grafana', 3100),
        objects=[
            'grafana-dashboards-config:configmap',
            'grafana-datasources:configmap',
            'grafana-dashboards:configmap'  # Only present if dashboards configured
        ],
        resource_deps=['prometheus'],
        labels=['monitoring'],
    )
else:
    print("Monitoring disabled - skipping Prometheus and Grafana deployment")

# ------------------------------------------
# ADMIN API (Optional)
# ------------------------------------------

if components.get('admin', True):
    print("Admin API enabled")

    # Choose build mode based on mode setting
    if mode == 'local':
        # Local mode: Enable hot reload with live binary sync
        docker_build_with_restart(
            "localhost:5001/canopyx-admin",
            ".",
            dockerfile="./Dockerfile.admin",
            entrypoint=["/app/admin"],
            live_update=[sync("bin/admin", "/app/admin")],
            ignore=[
                './web',
                './scripts',
                './docs',
                './deploy',
                './tests',
                './*.md',
                './Tiltfile',
                './tilt-config.yaml',
                './tilt-config.default.yaml',
                './.git',
            ],
        )
    else:
        # Shared mode: Production build without hot reload
        docker_build(
            "localhost:5001/canopyx-admin",
            ".",
            dockerfile="./Dockerfile.admin",
            ignore=[
                './web',
                './scripts',
                './docs',
                './deploy',
                './tests',
                './*.md',
                './Tiltfile',
                './tilt-config.yaml',
                './tilt-config.default.yaml',
                './.git',
            ],
        )

    # Load admin manifests and apply resource limits from profile
    admin_objects = decode_yaml_stream(kustomize("./deploy/k8s/admin/overlays/local"))
    canopyx_cfg = resources_cfg.get('canopyx', {})

    for o in admin_objects:
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopyx-admin':
            # Apply resource limits from profile
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'admin':
                    admin_cpu = canopyx_cfg.get('admin_cpu_limit', '1000m')
                    admin_mem = canopyx_cfg.get('admin_memory_limit', '1Gi')
                    container['resources'] = {
                        'limits': {
                            'cpu': admin_cpu,
                            'memory': admin_mem
                        },
                        'requests': {
                            'cpu': str(int(admin_cpu.replace('m', '')) // 2) + 'm' if 'm' in admin_cpu else str(float(admin_cpu) / 2),
                            'memory': str(int(admin_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in admin_mem else str(int(admin_mem.replace('Mi', '')) // 2) + 'Mi'
                        }
                    }
                    print("CanopyX Admin resources: CPU=%s, Memory=%s" % (admin_cpu, admin_mem))

                    # Apply environment variable overrides from config
                    admin_env_overrides = env_cfg.get('admin', {})
                    if admin_env_overrides:
                        env_vars = container.get('env', [])
                        for env_var in env_vars:
                            env_name = env_var.get('name')
                            if env_name in admin_env_overrides:
                                env_var['value'] = str(admin_env_overrides[env_name])
                                print("CanopyX Admin env override: %s=%s" % (env_name, env_var['value']))

                    # Apply global ClickHouse connection strategy if configured
                    if env_cfg.get('clickhouse_conn_strategy'):
                        env_vars = container.get('env', [])
                        env_vars.append({'name': 'CLICKHOUSE_CONN_STRATEGY', 'value': env_cfg['clickhouse_conn_strategy']})
                        container['env'] = env_vars
                        print("CanopyX Admin ClickHouse strategy: %s" % env_cfg['clickhouse_conn_strategy'])
            break

    k8s_yaml(encode_yaml_stream(admin_objects))

    k8s_resource(
        "canopyx-admin",
        port_forwards=["%s:3000" % get_port('admin', 3000)],
        labels=['apps'],
        resource_deps=["clickhouse", "temporal-frontend"],
        pod_readiness='wait',
    )
else:
    print("Admin API disabled")

# ------------------------------------------
# QUERY API - REMOVED (Phase 4: Query Service Deprecation)
# ------------------------------------------
# Query service has been deprecated and removed.
# Essential endpoints migrated to admin service:
#   - /api/admin/entities (schema introspection)
#   - /api/admin/chains/{id}/schema (table schemas)
#   - /api/admin/ws (WebSocket real-time events)
# Entity query endpoints will be replaced by external service built by engineering team.

# ------------------------------------------
# ADMIN WEB (Optional)
# ------------------------------------------

if components.get('admin_web', True):
    print("Admin Web UI enabled")

    # Choose build mode based on mode setting
    if mode == 'local':
        # Local mode: Enable hot reload with live_update
        docker_build(
            "localhost:5001/canopyx-admin-web",
            "./web/admin",
            dockerfile="./web/admin/Dockerfile",
            ignore=[
                '../../app',
                '../../pkg',
                '../../scripts',
                '../../docs',
                '../../deploy',
                '../../tests',
                '../../*.md',
                '../../Tiltfile',
                '../../tilt-config.yaml',
                '../../tilt-config.default.yaml',
                '../../.git',
            ],
            live_update=[
                fall_back_on(['./web/admin/package.json', './web/admin/pnpm-lock.yaml']),
                sync('./web/admin/app', '/app/app'),
                sync('./web/admin/public', '/app/public'),
            ],
        )
    else:
        # Shared mode: Production build without hot reload
        docker_build(
            "localhost:5001/canopyx-admin-web",
            "./web/admin",
            dockerfile="./web/admin/Dockerfile",
            ignore=[
                '../../app',
                '../../pkg',
                '../../scripts',
                '../../docs',
                '../../deploy',
                '../../tests',
                '../../*.md',
                '../../Tiltfile',
                '../../tilt-config.yaml',
                '../../tilt-config.default.yaml',
                '../../.git',
            ],
        )

    # Load admin-web manifests and apply resource limits from profile
    # Use mode-specific overlay (local vs shared)
    # EXCEPTION: Production profile ALWAYS uses shared overlay (NODE_ENV=production)
    admin_web_overlay = "shared" if profile == 'production' else ("local" if mode == 'local' else "shared")
    admin_web_objects = decode_yaml_stream(kustomize("./deploy/k8s/admin-web/overlays/%s" % admin_web_overlay))

    for o in admin_web_objects:
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopyx-admin-web':
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'admin-web':
                    admin_web_cpu = canopyx_cfg.get('admin_web_cpu_limit', '500m')
                    admin_web_mem = canopyx_cfg.get('admin_web_memory_limit', '512Mi')

                    # For dev environments (minimal/development), don't set memory limit to allow Next.js builds with hot reload
                    # For production, apply memory limit for safety
                    if profile in ['minimal', 'development']:
                        container['resources'] = {
                            'limits': {
                                'cpu': admin_web_cpu,
                                # No memory limit for dev mode - Next.js can spike during builds
                            },
                            'requests': {
                                'cpu': str(int(admin_web_cpu.replace('m', '')) // 2) + 'm' if 'm' in admin_web_cpu else str(float(admin_web_cpu) / 2),
                                'memory': '256Mi'  # Baseline request for scheduling
                            }
                        }
                        print("CanopyX Admin Web resources: CPU=%s, Memory=unlimited (dev mode)" % admin_web_cpu)
                    else:
                        container['resources'] = {
                            'limits': {
                                'cpu': admin_web_cpu,
                                'memory': admin_web_mem
                            },
                            'requests': {
                                'cpu': str(int(admin_web_cpu.replace('m', '')) // 2) + 'm' if 'm' in admin_web_cpu else str(float(admin_web_cpu) / 2),
                                'memory': str(int(admin_web_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in admin_web_mem else str(int(admin_web_mem.replace('Mi', '')) // 2) + 'Mi'
                            }
                        }
                        print("CanopyX Admin Web resources: CPU=%s, Memory=%s" % (admin_web_cpu, admin_web_mem))
            break

    k8s_yaml(encode_yaml_stream(admin_web_objects))

    # Build admin_web resource_deps dynamically based on what's enabled
    admin_web_deps = []
    if components.get('admin', True):
        admin_web_deps.append("canopyx-admin")

    k8s_resource(
        "canopyx-admin-web",
        labels=['apps'],
        resource_deps=admin_web_deps if admin_web_deps else None,
        pod_readiness='wait',
    )

    # Admin Web Proxy - nginx routes all API requests
    # /api/query/ws  -> Query service WebSocket
    # /api/query/*   -> Query service REST API
    # /api/admin/*   -> Admin API
    # /api/*         -> Admin API (legacy)
    # /*             -> Admin Web (Next.js)
    print("Admin Web Proxy enabled - unified API routing via nginx")

    k8s_yaml(kustomize("./deploy/k8s/admin-web-proxy/overlays/local"))

    # Build proxy resource_deps dynamically
    proxy_deps = ["canopyx-admin-web"]
    if components.get('admin', True):
        proxy_deps.append("canopyx-admin")

    k8s_resource(
        "admin-web-proxy",
        port_forwards=["%s:80" % get_port('admin_web', 3003)],
        labels=['apps'],
        resource_deps=proxy_deps,
        pod_readiness='wait',
        objects=["admin-web-proxy-config:configmap"]
    )
else:
    print("Admin Web UI disabled")

# ------------------------------------------
# CONTROLLER (Optional)
# ------------------------------------------

if components.get('controller', True):
    print("Controller enabled")

    # Choose build mode based on mode setting
    if mode == 'local':
        # Local mode: Enable hot reload with live binary sync
        docker_build_with_restart(
            "localhost:5001/canopyx-controller",
            ".",
            dockerfile="./Dockerfile.controller",
            entrypoint=["/app/controller"],
            live_update=[sync("bin/controller", "/app/controller")],
            ignore=[
                './web',
                './scripts',
                './docs',
                './deploy',
                './tests',
                './*.md',
                './Tiltfile',
                './tilt-config.yaml',
                './tilt-config.default.yaml',
                './.git',
            ],
        )
    else:
        # Shared mode: Production build without hot reload
        docker_build(
            "localhost:5001/canopyx-controller",
            ".",
            dockerfile="./Dockerfile.controller",
            ignore=[
                './web',
                './scripts',
                './docs',
                './deploy',
                './tests',
                './*.md',
                './Tiltfile',
                './tilt-config.yaml',
                './tilt-config.default.yaml',
                './.git',
            ],
        )

    # Load controller manifests and apply resource limits from profile
    controller_objects = decode_yaml_stream(kustomize("./deploy/k8s/controller/overlays/local"))

    for o in controller_objects:
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopyx-controller':
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'controller':
                    controller_cpu = canopyx_cfg.get('controller_cpu_limit', '500m')
                    controller_mem = canopyx_cfg.get('controller_memory_limit', '512Mi')
                    container['resources'] = {
                        'limits': {
                            'cpu': controller_cpu,
                            'memory': controller_mem
                        },
                        'requests': {
                            'cpu': str(int(controller_cpu.replace('m', '')) // 2) + 'm' if 'm' in controller_cpu else str(float(controller_cpu) / 2),
                            'memory': str(int(controller_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in controller_mem else str(int(controller_mem.replace('Mi', '')) // 2) + 'Mi'
                        }
                    }
                    print("CanopyX Controller resources: CPU=%s, Memory=%s" % (controller_cpu, controller_mem))

                    # Apply environment variable overrides from config
                    controller_env_overrides = env_cfg.get('controller', {})
                    if controller_env_overrides:
                        env_vars = container.get('env', [])
                        for env_var in env_vars:
                            env_name = env_var.get('name')
                            if env_name in controller_env_overrides:
                                env_var['value'] = str(controller_env_overrides[env_name])
                                print("CanopyX Controller env override: %s=%s" % (env_name, env_var['value']))

                    # Apply global ClickHouse connection strategy if configured
                    # Controller passes this to indexer deployments via provider_k8s.go
                    if env_cfg.get('clickhouse_conn_strategy'):
                        env_vars = container.get('env', [])
                        env_vars.append({'name': 'CLICKHOUSE_CONN_STRATEGY', 'value': env_cfg['clickhouse_conn_strategy']})
                        container['env'] = env_vars
                        print("CanopyX Controller ClickHouse strategy: %s (will be passed to indexers)" % env_cfg['clickhouse_conn_strategy'])
            break

    k8s_yaml(encode_yaml_stream(controller_objects))

    # Build resource_deps dynamically based on what's enabled
    controller_deps = ["clickhouse", "temporal-frontend"]
    if components.get('admin', True):
        controller_deps.append("canopyx-admin")

    k8s_resource(
        "canopyx-controller",
        labels=['apps'],
        objects=["canopyx-controller:serviceaccount", "canopyx-controller:role", "canopyx-controller:rolebinding"],
        resource_deps=controller_deps,
        pod_readiness='wait',
    )
else:
    print("Controller disabled")

# ------------------------------------------
# CANOPY RPC MOCK (Optional - Mock Blockchain Nodes)
# ------------------------------------------

# Get configured Canopy RPC Mock source path
canopy_rpc_mock_path = paths_cfg.get('canopy_rpc_mock_source', '../canopy-rpc-mock')

# Check if canopy mock is enabled (boolean)
canopy_enabled = components.get('canopy', False)

if canopy_enabled:
    if os.path.exists(canopy_rpc_mock_path):
        print("Canopy RPC Mock enabled - found source at %s" % canopy_rpc_mock_path)

        # Get mock configuration from config
        mock_chains = paths_cfg.get('mock_chains', 2)
        mock_blocks = paths_cfg.get('mock_blocks', 100)
        mock_start_port = paths_cfg.get('mock_start_port', 60000)
        mock_start_chain_id = paths_cfg.get('mock_start_chain_id', 5)

        print("Mock config: %d chains, %d blocks per chain, starting at port %d (chain ID %d)" % (
            mock_chains, mock_blocks, mock_start_port, mock_start_chain_id
        ))

        # Build docker image for canopy-rpc-mock
        docker_build(
            "localhost:5001/canopy-rpc-mock",
            canopy_rpc_mock_path,
            dockerfile=canopy_rpc_mock_path + "/Dockerfile",
            live_update=[],
        )

        # Deploy canopy-rpc-mock with dynamic configuration
        mock_objects = decode_yaml_stream(kustomize("./deploy/k8s/canopy-rpc-mock/overlays/local"))

        for o in mock_objects:
            if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopy-rpc-mock':
                containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
                for container in containers:
                    if container.get('name') == 'canopy-rpc-mock':
                        # Update args with config values
                        container['args'] = [
                            "-chains", str(mock_chains),
                            "-blocks", str(mock_blocks),
                            "-start-port", str(mock_start_port),
                            "-start-chain-id", str(mock_start_chain_id),
                        ]

                        # Update ports dynamically
                        container_ports = []
                        for i in range(mock_chains):
                            chain_id = mock_start_chain_id + i
                            port = mock_start_port + i
                            container_ports.append({
                                'name': 'chain-%d' % chain_id,
                                'containerPort': port
                            })
                        container['ports'] = container_ports
                        break

            # Update Service ports dynamically
            if o.get('kind') == 'Service' and o.get('metadata', {}).get('name') == 'canopy-rpc-mock':
                service_ports = []
                for i in range(mock_chains):
                    chain_id = mock_start_chain_id + i
                    port = mock_start_port + i
                    service_ports.append({
                        'name': 'chain-%d' % chain_id,
                        'port': port,
                        'targetPort': port
                    })
                o['spec']['ports'] = service_ports

        k8s_yaml(encode_yaml_stream(mock_objects))

        # Build dynamic port forwards for all chains
        port_forwards = []
        for i in range(mock_chains):
            port = mock_start_port + i
            # For the first two chains, check if custom port is configured
            if i == 0:
                local_port = get_port('canopy_mock_chain_1', port)
            elif i == 1:
                local_port = get_port('canopy_mock_chain_2', port)
            else:
                # For additional chains, just use the default port
                local_port = port
            port_forwards.append("%s:%d" % (local_port, port))

        k8s_resource(
            "canopy-rpc-mock",
            port_forwards=port_forwards,
            labels=['blockchain'],
            pod_readiness='wait',
        )
    else:
        fail("Canopy RPC Mock enabled but source not found at: %s\nPlease update paths.canopy_rpc_mock_source in tilt-config.yaml" % canopy_rpc_mock_path)
else:
    print("Canopy RPC Mock disabled")

# ------------------------------------------
# INDEXER (Always Required)
# ------------------------------------------

# Use indexer tag from config
idx_repo = dev_cfg.get('indexer_tag', 'localhost:5001/canopyx-indexer:dev')

# In shared mode, always use auto trigger regardless of config
# In local mode, respect the config setting
if mode == 'shared':
    idx_build_mode = TRIGGER_MODE_AUTO
else:
    idx_build_mode = TRIGGER_MODE_AUTO if dev_cfg.get('indexer_build_mode', 'auto') == 'auto' else TRIGGER_MODE_MANUAL

idx_deps = [
  "Dockerfile.indexer",
  "app/indexer",
  "cmd/indexer",
  "pkg",
  "go.mod",
  "go.sum",
]

local_resource(
  name = "build:indexer-stable",
  cmd  = "docker build -f Dockerfile.indexer -t %s . && docker push %s && kubectl rollout restart deployment -l managed-by=canopyx-controller -l app=indexer || true" % (idx_repo, idx_repo),
  deps = idx_deps,
  trigger_mode = idx_build_mode,
  labels = ["indexer"],
)

# ------------------------------------------
# AUTO-REGISTER MOCK CHAINS (Conditional on Mock RPC)
# ------------------------------------------

# Auto-register mock RPC chains
if canopy_enabled and os.path.exists(canopy_rpc_mock_path):
    if dev_cfg.get('auto_register_chain', True) and components.get('admin', True):
        print("Auto-registering %d mock Canopy chain(s)" % mock_chains)

        # Build resource_deps dynamically
        mock_deps = ["canopy-rpc-mock"]
        if components.get('admin', True):
            mock_deps.append("canopyx-admin")

        # Generate registration script for all chains
        registration_cmds = []
        registration_cmds.append("""
        # Wait for admin API to be ready
        echo "Waiting for admin API..."
        for i in {1..30}; do
          if curl -f http://localhost:3000/api/health 2>/dev/null; then
            echo "✓ Admin API is ready"
            break
          fi
          echo "  Waiting for admin API... attempt $i/30"
          sleep 2
        done
        """)

        # Add registration for each chain
        for i in range(mock_chains):
            chain_id = mock_start_chain_id + i
            chain_port = mock_start_port + i
            chain_name = "Mock Chain %d" % chain_id

            registration_cmds.append("""
        # Wait for mock RPC chain %d
        echo "Waiting for %s on port %d..."
        for i in {1..30}; do
          if kubectl get endpoints canopy-rpc-mock -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; then
            echo "✓ %s is ready"
            break
          fi
          echo "  Waiting for mock RPC... attempt $i/30"
          sleep 2
        done

        # Register %s
        echo "Registering %s..."
        if curl -X POST -f http://localhost:3000/api/chains \\
          -H 'Authorization: Bearer devtoken' \\
          -H 'Content-Type: application/json' \\
          -d '{"chain_id":%d,"chain_name":"%s","rpc_endpoints":["http://canopy-rpc-mock.default.svc.cluster.local:%d"],"image":"%s","min_replicas":1,"max_replicas":2}' 2>/dev/null; then
          echo "✓ Successfully registered %s"
        else
          echo "✗ Failed to register %s (may already exist or still starting)"
        fi
        """ % (chain_id, chain_name, chain_port, chain_name, chain_name, chain_name, chain_id, chain_name, chain_port, idx_repo, chain_name, chain_name))

        registration_cmds.append("""
        echo ""
        echo "Mock chain registration complete!"
        """)

        local_resource(
            name="register-mock-chains",
            cmd=''.join(registration_cmds),
            deps=[],
            labels=['setup'],
            resource_deps=mock_deps,
            allow_parallel=False,
            auto_init=True,
        )

        # Only attach indexers if controller is enabled
        if components.get('controller', True):
            for i in range(mock_chains):
                chain_id = mock_start_chain_id + i
                k8s_attach(
                    name="indexer-chain-%d" % chain_id,
                    obj="deployment/canopyx-indexer-%d" % chain_id,
                    resource_deps=["canopyx-controller", "register-mock-chains"],
                    labels=['indexers'],
                )
    elif not components.get('admin', True):
        print("Auto-register mock chains disabled - admin API required")
    else:
        print("Auto-register mock chains disabled in config")

# ------------------------------------------
# AUTO-REGISTER EXTERNAL CHAINS
# ------------------------------------------
# Auto-register external chains from configuration
# Only rpc_endpoints is required - chain_id will be auto-detected from RPC
# and chain_name will default to "Chain {id}" if not provided.
if chains_cfg and len(chains_cfg) > 0 and components.get('admin', True):
    print("Auto-registering %d external Canopy chain(s)" % len(chains_cfg))

    for idx, chain in enumerate(chains_cfg):
        chain_id_config = chain.get('chain_id', 0)  # 0 means auto-detect from RPC
        chain_name = chain.get('chain_name', '')  # Empty string means use default "Chain {id}"
        rpc_endpoints = chain.get('rpc_endpoints', [])
        min_replicas = chain.get('min_replicas', 1)
        max_replicas = chain.get('max_replicas', 1)  # Changed default from 3 to 1
        image = chain.get('image', 'canopynetwork/canopyx-indexer:latest')  # Changed to public image

        # Only RPC endpoints are required
        if not rpc_endpoints or len(rpc_endpoints) == 0:
            fail("Invalid chain configuration: at least one rpc_endpoint is required")

        # Fetch actual chain_id from RPC if not provided in config
        actual_chain_id = chain_id_config
        if chain_id_config == 0:
            # Use helper script to fetch chain_id from RPC endpoint
            print("Fetching chain ID from RPC endpoint: %s" % rpc_endpoints[0])
            detected_id = str(local("bash scripts/get-chain-id.sh '%s'" % rpc_endpoints[0])).strip()
            actual_chain_id = int(detected_id)
            print("Detected chain ID: %d" % actual_chain_id)

        # Build JSON payload for chain registration manually (Starlark doesn't have json.encode)
        rpc_endpoints_str = ', '.join(['"%s"' % ep for ep in rpc_endpoints])
        # Always include the actual chain_id in the payload
        chain_payload = '{\"chain_id\":%d,\"chain_name\":\"%s\",\"rpc_endpoints\":[%s],\"image\":\"%s\",\"min_replicas\":%d,\"max_replicas\":%d}' % (
            actual_chain_id, chain_name, rpc_endpoints_str, image, min_replicas, max_replicas
        )

        # Use actual chain_id for resource naming
        resource_name = "add-chain-%d" % actual_chain_id

        chain_display = chain_name if chain_name else ("Chain %d" % actual_chain_id)
        print("Configuring auto-registration for chain: %s (ID: %d)" % (chain_display, actual_chain_id))

        # Build resource_deps dynamically
        chain_deps = []
        if components.get('admin', True):
            chain_deps.append("canopyx-admin")

        local_resource(
            name=resource_name,
            cmd="""
            for i in {1..30}; do
              RESPONSE=$(curl -X POST -f -s http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '%s' 2>/dev/null || echo "")
              if [ -n "$RESPONSE" ]; then
                # Extract chain_id from response to verify
                CHAIN_ID=$(echo "$RESPONSE" | grep -o '"chain_id":[0-9]*' | head -1 | cut -d':' -f2)
                echo "Successfully registered chain %s (ID: $CHAIN_ID)"
                exit 0
              fi
              echo "Waiting for admin API to be ready... attempt $i/30"
              sleep 2
            done
            echo "Failed to register chain %s after 30 attempts"
            exit 1
            """ % (chain_payload, chain_display, chain_display),
            deps=[],
            labels=['chains'],
            resource_deps=chain_deps if chain_deps else None,
            allow_parallel=False,
            auto_init=True,
        )

        # Only attach indexer if controller is enabled
        if components.get('controller', True):
            # Build indexer resource_deps dynamically
            indexer_deps = [resource_name]
            if components.get('controller', True):
                indexer_deps.append("canopyx-controller")

            # Use actual_chain_id (fetched from RPC) for deployment name
            k8s_attach(
                name="indexer-%d" % actual_chain_id,
                obj="deployment/canopyx-indexer-%d" % actual_chain_id,
                resource_deps=indexer_deps,
                labels=['indexers'],
            )
elif not components.get('admin', True):
    print("No external chains configured - admin API required for auto-registration")
else:
    print("No external chains configured for auto-registration")

# Cleanup resource for controller-spawned deployments (only if controller is enabled)
if components.get('controller', True):
    k8s_custom_deploy(
      'cleanup-controller-spawns',
      apply_cmd='true',
      delete_cmd='kubectl delete deployment -l managed-by=canopyx-controller -l app=indexer --ignore-not-found --wait',
      deps=[],
    )

    k8s_resource('cleanup-controller-spawns', resource_deps=['canopyx-controller'], labels=['no-op'])

# Cleanup persistent data which are not deleted by helm charts
# This ensures fresh data on each tilt up by removing old workflow/visibility data
k8s_custom_deploy(
  'cleanup-temporal-data',
  apply_cmd='true',
  delete_cmd='''
    echo "Cleaning up Temporal persistent data (Cassandra + Elasticsearch PVCs)..." && \
    kubectl delete pvc -l app=cassandra --ignore-not-found --wait=false && \
    kubectl delete pvc -l app=elasticsearch-master --ignore-not-found --wait=false && \
    echo "Temporal data cleanup initiated (will complete in background)"
  ''',
  deps=[],
)

k8s_resource('cleanup-temporal-data', resource_deps=['temporal'], labels=['no-op'])
