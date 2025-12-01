load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://k8s_attach', 'k8s_attach')
load('ext://restart_process', 'docker_build_with_restart')

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

# Helper function to get port from config with default fallback
def get_port(service, default):
    return ports.get(service, default)

# ------------------------------------------
# HELM REPOSITORIES
# ------------------------------------------

helm_repo(
  name='hyperdx',
  url='https://hyperdxio.github.io/helm-charts',
  labels=['helm_repo'],
  resource_name='helm-repo-hyperdx'
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

# ------------------------------------------
# CLICKHOUSE (Always Required)
# ------------------------------------------
# ClickHouse is always deployed - resource limits controlled via profile configuration

# Build ClickHouse resource limit flags from profile
clickhouse_cfg = resources_cfg.get('clickhouse', {})
clickhouse_flags = ['--values=./deploy/helm/clickhouse-values.yaml']

if clickhouse_cfg.get('cpu_limit'):
    clickhouse_flags.append('--set=clickhouse.resources.limits.cpu=%s' % clickhouse_cfg['cpu_limit'])
if clickhouse_cfg.get('memory_limit'):
    clickhouse_flags.append('--set=clickhouse.resources.limits.memory=%s' % clickhouse_cfg['memory_limit'])
if clickhouse_cfg.get('cpu_request'):
    clickhouse_flags.append('--set=clickhouse.resources.requests.cpu=%s' % clickhouse_cfg['cpu_request'])
if clickhouse_cfg.get('memory_request'):
    clickhouse_flags.append('--set=clickhouse.resources.requests.memory=%s' % clickhouse_cfg['memory_request'])

print("ClickHouse resources: CPU=%s, Memory=%s" % (
    clickhouse_cfg.get('cpu_limit', 'default'),
    clickhouse_cfg.get('memory_limit', 'default')
))

helm_resource(
  name='clickhouse',
  chart='hyperdx/hdx-oss-v2',
  release_name='clickhouse',
  flags=clickhouse_flags,
  pod_readiness='wait',
  labels=['clickhouse']
)

# Patch ClickHouse ConfigMap with increased limits after Helm install
local_resource(
  'clickhouse-config-patch',
  cmd='''
    echo "Patching ClickHouse ConfigMap with increased concurrency limits..."
    kubectl get configmap clickhouse-hdx-oss-v2-clickhouse-config -o yaml | \\
      sed 's/<max_concurrent_queries>100<\\/max_concurrent_queries>/<max_concurrent_queries>2000<\\/max_concurrent_queries>/' | \\
      sed 's/<max_connections>4096<\\/max_connections>/<max_connections>3000<\\/max_connections>/' | \\
      kubectl apply -f - && \\
    echo "Restarting ClickHouse pod to apply new configuration..." && \\
    kubectl delete pod -l app.kubernetes.io/name=clickhouse --wait && \\
    echo "ClickHouse configuration updated successfully!"
  ''',
  resource_deps=['clickhouse'],
  labels=['clickhouse'],
)

# Apply ClickHouse storage tiering configuration
local_resource(
  'clickhouse-storage-config',
  cmd='''
    echo "Applying ClickHouse storage tiering configuration (hot/warm/cold)..."
    kubectl apply -f ./deploy/k8s/clickhouse/configmap.yaml && \\
    kubectl patch deployment clickhouse-hdx-oss-v2-clickhouse -p '{"spec":{"template":{"spec":{"volumes":[{"name":"storage-config","configMap":{"name":"clickhouse-storage-config"}}],"containers":[{"name":"clickhouse","volumeMounts":[{"name":"storage-config","mountPath":"/etc/clickhouse-server/config.d/storage-policy.xml","subPath":"storage-policy.xml"}]}]}}}}' && \\
    echo "Storage tiering configured: hot (30d) -> warm (180d) -> cold (permanent)" && \\
    echo "Restarting ClickHouse to apply storage configuration..." && \\
    kubectl delete pod -l app.kubernetes.io/name=clickhouse --wait && \\
    echo "ClickHouse storage configuration applied successfully!"
  ''',
  resource_deps=['clickhouse-config-patch'],
  labels=['clickhouse'],
)

# Patch ClickHouse deployment with Prometheus scrape annotations
local_resource(
  'clickhouse-prometheus-patch',
  cmd='''
    echo "Adding Prometheus scrape annotations to ClickHouse deployment..."
    kubectl patch deployment clickhouse-hdx-oss-v2-clickhouse -p '{"spec":{"template":{"metadata":{"annotations":{"prometheus.io/scrape":"true","prometheus.io/port":"9363","prometheus.io/path":"/metrics"}}}}}' && \\
    echo "Prometheus annotations applied successfully!"
  ''',
  resource_deps=['clickhouse-storage-config'],
  labels=['clickhouse'],
)

# HyperDX web UI, MongoDB, and OTEL collector are disabled in clickhouse-values.yaml
# Only attach to the ClickHouse server itself
k8s_attach(
    name="clickhouse-server",
    obj="deployment/clickhouse-hdx-oss-v2-clickhouse",
    port_forwards=["%s:8123" % get_port('clickhouse_server', 8123), "%s:9000" % get_port('clickhouse_native', 9000)],
    resource_deps=["clickhouse"],
    labels=['clickhouse'],
)

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
    obj="statefulset/redis-master",
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
            break

    k8s_yaml(encode_yaml_stream(admin_objects))

    k8s_resource(
        "canopyx-admin",
        port_forwards=["%s:3000" % get_port('admin', 3000)],
        labels=['apps'],
        resource_deps=["clickhouse-server", "temporal-frontend"],
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
            break

    k8s_yaml(encode_yaml_stream(controller_objects))

    # Build resource_deps dynamically based on what's enabled
    controller_deps = ["clickhouse-server", "temporal-frontend"]
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
# CANOPY LOCAL NODES (Optional - Dual Node Setup)
# ------------------------------------------

# Get configured Canopy source path
canopy_path = paths_cfg.get('canopy_source', '../canopy')
# Note: Tilt/Starlark doesn't have os.path.expanduser, so use absolute paths in config or relative paths

# Get Canopy deployment mode: "off" | "single" | "dual"
canopy_mode = components.get('canopy', 'off')

if canopy_mode == 'dual':
    if os.path.exists(canopy_path):
        print("Canopy dual-node setup enabled - found source at %s" % canopy_path)

        # Build single image for both nodes
        docker_build(
            "localhost:5001/canopy-node",
            canopy_path,
            dockerfile=canopy_path + "/.docker/Dockerfile",
            live_update=[],
        )

        # Deploy both nodes
        k8s_yaml(kustomize("./deploy/k8s/canopy-nodes/overlays/local"))

        # Node 1 resource (Chain ID 1)
        k8s_resource(
            "node-1",
            objects=[
                "node-1-config:configmap",
                "node-1-genesis:configmap",
                "node-1-keystore:configmap"
            ],
            port_forwards=[
                "%s:50000" % get_port('canopy1_wallet', 50000),
                "%s:50001" % get_port('canopy1_explorer', 50001),
                "%s:50002" % get_port('canopy1_rpc', 50002),
                "%s:50003" % get_port('canopy1_admin', 50003),
                "%s:9001" % get_port('canopy1_p2p', 9001),
            ],
            labels=['blockchain'],
            pod_readiness='wait',
        )

        # Node 2 resource (Chain ID 2)
        k8s_resource(
            "node-2",
            objects=[
                "node-2-config:configmap",
                "node-2-genesis:configmap",
                "node-2-keystore:configmap"
            ],
            port_forwards=[
                "%s:40000" % get_port('canopy2_wallet', 40000),
                "%s:40001" % get_port('canopy2_explorer', 40001),
                "%s:40002" % get_port('canopy2_rpc', 40002),
                "%s:40003" % get_port('canopy2_admin', 40003),
                "%s:9001" % get_port('canopy2_p2p', 9002),
            ],
            labels=['blockchain'],
            pod_readiness='wait',
        )

        # Node 3 resource (Chain ID 1 - second validator)
        k8s_resource(
            "node-3",
            objects=[
                "node-3-config:configmap",
                "node-3-genesis:configmap",
                "node-3-keystore:configmap"
            ],
            port_forwards=[
                "%s:30000" % get_port('canopy3_wallet', 30000),
                "%s:30001" % get_port('canopy3_explorer', 30001),
                "%s:30002" % get_port('canopy3_rpc', 30002),
                "%s:30003" % get_port('canopy3_admin', 30003),
                "%s:9003" % get_port('canopy3_p2p', 9003),
            ],
            labels=['blockchain'],
            pod_readiness='wait',
        )
    else:
        fail("Canopy dual nodes enabled but source not found at: %s\nPlease update paths.canopy_source in tilt-config.yaml" % canopy_path)

elif canopy_mode == 'single':
    if os.path.exists(canopy_path):
        print("Canopy single node enabled - found source at %s" % canopy_path)

        docker_build(
            "localhost:5001/canopy-node",
            canopy_path,
            dockerfile=canopy_path + "/.docker/Dockerfile",
            live_update=[],
        )

        k8s_yaml(kustomize("./deploy/k8s/canopy-node/overlays/local"))

        k8s_resource(
            "canopy-node",
            objects=["canopy-node-data:persistentvolumeclaim", "canopy-node-config:configmap", "canopy-node-genesis:configmap", "canopy-node-keystore:configmap"],
            port_forwards=[
                "%s:50000" % get_port('canopy_wallet', 50000),
                "%s:50001" % get_port('canopy_explorer', 50001),
                "%s:50002" % get_port('canopy_rpc', 50002),
                "%s:50003" % get_port('canopy_admin_rpc', 50003),
                "%s:9001" % get_port('canopy_p2p', 9001),
                "%s:6060" % get_port('canopy_debug', 6060)
            ],
            labels=['blockchain'],
            pod_readiness='wait',
        )
    else:
        fail("Canopy single node enabled but source not found at: %s\nPlease update paths.canopy_source in tilt-config.yaml" % canopy_path)

elif canopy_mode == 'off':
    print("Canopy nodes disabled")
else:
    fail("Invalid canopy mode: '%s'. Must be 'off', 'single', or 'dual'" % canopy_mode)

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
# TRIGGER DEFAULT CHAIN (Conditional on Canopy node)
# ------------------------------------------

# Auto-register dual nodes
if components.get('canopy', 'off') == 'dual' and os.path.exists(canopy_path):
    if dev_cfg.get('auto_register_chain', True) and components.get('admin', True):
        print("Auto-registering dual Canopy chains")

        # Build resource_deps dynamically
        register_deps = ["node-1", "node-2",
        #"node-3"
        ]
        if components.get('admin', True):
            register_deps.append("canopyx-admin")

        local_resource(
            name="register-canopy-chains",
            cmd="""
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

            # Wait for Canopy Node 1 to produce at least one block
            echo "Waiting for Canopy Node 1 to produce blocks..."
            for i in {1..60}; do
              CHAIN_ID=$(curl -s -X POST http://node-1.default.svc.cluster.local:50002/v1/query/cert-by-height \\
                -H 'Content-Type: application/json' \\
                -d '{"height":0}' 2>/dev/null | grep -o '"chainId":[0-9]*' | cut -d: -f2)
              if [ ! -z "$CHAIN_ID" ] && [ "$CHAIN_ID" != "0" ]; then
                echo "✓ Node 1 is ready (Chain ID: $CHAIN_ID)"
                break
              fi
              echo "  Waiting for node 1... attempt $i/60"
              sleep 2
            done

            # Wait for Canopy Node 2 to produce at least one block
            echo "Waiting for Canopy Node 2 to produce blocks..."
            for i in {1..60}; do
              CHAIN_ID=$(curl -s -X POST http://node-2.default.svc.cluster.local:40002/v1/query/cert-by-height \\
                -H 'Content-Type: application/json' \\
                -d '{"height":0}' 2>/dev/null | grep -o '"chainId":[0-9]*' | cut -d: -f2)
              if [ ! -z "$CHAIN_ID" ] && [ "$CHAIN_ID" != "0" ]; then
                echo "✓ Node 2 is ready (Chain ID: $CHAIN_ID)"
                break
              fi
              echo "  Waiting for node 2... attempt $i/60"
              sleep 2
            done

            # Register Chain 1 (Root Chain) - NO chain_id field, let controller discover it
            echo ""
            echo "Registering Canopy Chain 1..."
            if curl -X POST -f http://localhost:3000/api/chains \\
              -H 'Authorization: Bearer devtoken' \\
              -H 'Content-Type: application/json' \\
              -d '{"rpc_endpoints":["http://node-1.default.svc.cluster.local:50002"], "chain_name":"Canopy Chain 1","image":"localhost:5001/canopyx-indexer:dev","min_replicas":1,"max_replicas":2}' 2>/dev/null; then
              echo "✓ Successfully registered Canopy Chain 1"
            else
              echo "✗ Failed to register Canopy Chain 1 (may already exist or still starting)"
            fi

            # Wait a bit before registering second chain
            sleep 3

            # Register Chain 2 (Subchain) - NO chain_id field, let controller discover it
            echo "Registering Canopy Chain 2..."
            if curl -X POST -f http://localhost:3000/api/chains \\
              -H 'Authorization: Bearer devtoken' \\
              -H 'Content-Type: application/json' \\
              -d '{"rpc_endpoints":["http://node-2.default.svc.cluster.local:40002"], "chain_name":"Canopy Chain 2","image":"localhost:5001/canopyx-indexer:dev","min_replicas":1,"max_replicas":2}' 2>/dev/null; then
              echo "✓ Successfully registered Canopy Chain 2"
            else
              echo "✗ Failed to register Canopy Chain 2 (may already exist or still starting)"
            fi

            echo ""
            echo "Chain registration complete!"
            """,
            deps=[],
            labels=['setup'],
            resource_deps=register_deps,
            allow_parallel=False,
            auto_init=True,
        )

        # Only attach indexers if controller is enabled
        if components.get('controller', True):
            # Attach to Chain 1 indexer (root chain)
            k8s_attach(
                name="indexer-chain-1",
                obj="deployment/canopyx-indexer-1",
                resource_deps=["canopyx-controller", "register-canopy-chains"],
                labels=['indexers'],
            )

            # Attach to Chain 2 indexer (subchain)
            k8s_attach(
                name="indexer-chain-2",
                obj="deployment/canopyx-indexer-2",
                resource_deps=["canopyx-controller", "register-canopy-chains"],
                labels=['indexers'],
            )
    elif not components.get('admin', True):
        print("Auto-register chains disabled - admin API required")
    else:
        print("Auto-register chains disabled in config")

# Auto-register single node (legacy support)
elif components.get('canopy', 'off') == 'single' and os.path.exists(canopy_path):
    if dev_cfg.get('auto_register_chain', True) and components.get('admin', True):
        print("Auto-registering local Canopy chain")

        # Build resource_deps dynamically
        single_deps = ["canopy-node"]
        if components.get('admin', True):
            single_deps.append("canopyx-admin")

        local_resource(
            name="add-canopy-local",
            cmd="""
            for i in {1..30}; do
              if curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '{"chain_id":"canopy_local","chain_name":"Canopy Local","rpc_endpoints":["http://canopy-node.default.svc.cluster.local:50002"], "image":"localhost:5001/canopyx-indexer:dev","min_replicas":1,"max_replicas":2}' 2>/dev/null; then
                echo "Successfully registered canopy_local chain"
                exit 0
              fi
              echo "Waiting for admin API to be ready... attempt $i/30"
              sleep 2
            done
            echo "Failed to register chain after 30 attempts"
            exit 1
            """,
            deps=[],
            labels=['no-op'],
            resource_deps=single_deps,
            allow_parallel=False,
            auto_init=True,
        )

        # Only attach indexer if controller is enabled
        if components.get('controller', True):
            k8s_attach(
                name="indexer-local",
                obj="deployment/canopyx-indexer-canopy-local",
                resource_deps=["canopyx-controller", "add-canopy-local"],
                labels=['indexers'],
            )
    elif not components.get('admin', True):
        print("Auto-register chain disabled - admin API required")
    else:
        print("Auto-register chain disabled in config")

# ------------------------------------------
# AUTO-REGISTER EXTERNAL CHAINS
# ------------------------------------------
# Register external Canopy networks from config
if chains_cfg and len(chains_cfg) > 0 and components.get('admin', True):
    print("Auto-registering %d external Canopy chain(s)" % len(chains_cfg))

    for chain in chains_cfg:
        chain_id = chain.get('chain_id')
        chain_name = chain.get('chain_name')
        rpc_endpoints = chain.get('rpc_endpoints', [])
        min_replicas = chain.get('min_replicas', 1)
        max_replicas = chain.get('max_replicas', 3)
        image = chain.get('image', idx_repo)  # Default to dev indexer image

        if not chain_id or not chain_name or not rpc_endpoints:
            fail("Invalid chain configuration: chain_id, chain_name, and rpc_endpoints are required")

        # Build JSON payload for chain registration manually (Starlark doesn't have json.encode)
        rpc_endpoints_str = ', '.join(['"%s"' % ep for ep in rpc_endpoints])
        chain_payload = '{\"chain_id\":\"%s\",\"chain_name\":\"%s\",\"rpc_endpoints\":[%s],\"image\":\"%s\",\"min_replicas\":%d,\"max_replicas\":%d}' % (
            chain_id, chain_name, rpc_endpoints_str, image, min_replicas, max_replicas
        )

        resource_name = "add-chain-%s" % chain_id

        print("Configuring auto-registration for chain: %s (%s)" % (chain_name, chain_id))

        # Build resource_deps dynamically
        chain_deps = []
        if components.get('admin', True):
            chain_deps.append("canopyx-admin")

        local_resource(
            name=resource_name,
            cmd="""
            for i in {1..30}; do
              if curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '%s' 2>/dev/null; then
                echo "Successfully registered %s chain"
                exit 0
              fi
              echo "Waiting for admin API to be ready... attempt $i/30"
              sleep 2
            done
            echo "Failed to register %s chain after 30 attempts"
            exit 1
            """ % (chain_payload, chain_name, chain_name),
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

            k8s_attach(
                name="indexer-%s" % chain_id,
                obj="deployment/canopyx-indexer-%s" % chain_id,
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
    kubectl delete pvc -l app=cassandra --ignore-not-found && \
    kubectl delete pvc -l app=elasticsearch-master --ignore-not-found && \
    echo "Temporal data cleanup initiated (will complete in background)"
  ''',
  deps=[],
)

k8s_resource('cleanup-temporal-data', resource_deps=['temporal'], labels=['no-op'])
