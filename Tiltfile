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
components = cfg.get('components', {})
resources_cfg = cfg.get('resources', {}).get(profile, {})
monitoring_cfg = cfg.get('monitoring', {})
ports = cfg.get('ports', {})
paths_cfg = cfg.get('paths', {})
dev_cfg = cfg.get('dev', {})

print("Tilt Profile: %s" % profile)
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

k8s_attach(
    name="clickhouse-web",
    obj="deployment/clickhouse-hdx-oss-v2-app",
    port_forwards=["%s:3000" % get_port('clickhouse_web', 8081)],
    resource_deps=["clickhouse"],
    labels=['clickhouse'],
)

k8s_attach(
    name="clickhouse-server",
    obj="deployment/clickhouse-hdx-oss-v2-clickhouse",
    port_forwards=["%s:8123" % get_port('clickhouse_server', 8123), "%s:9000" % get_port('clickhouse_native', 9000)],
    resource_deps=["clickhouse"],
    labels=['clickhouse'],
)

k8s_attach(
    name="clickhouse-mongodb",
    obj="deployment/clickhouse-hdx-oss-v2-mongodb",
    port_forwards=["27017:27017"],
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
if temporal_cfg.get('cassandra_replicas'):
    temporal_flags.append('--set=cassandra.config.cluster_size=%s' % temporal_cfg['cassandra_replicas'])
    temporal_flags.append('--set=cassandra.replicas=%s' % temporal_cfg['cassandra_replicas'])
if temporal_cfg.get('elasticsearch_replicas'):
    temporal_flags.append('--set=elasticsearch.replicas=%s' % temporal_cfg['elasticsearch_replicas'])

# Cassandra resource limits
cass_mem_limit = temporal_cfg.get('cassandra_memory_limit', '8G')
cass_heap = temporal_cfg.get('cassandra_heap_size', '8G')
temporal_flags.append('--set=cassandra.config.max_heap_size=%s' % cass_heap)

# Elasticsearch resource limits
if temporal_cfg.get('elasticsearch_memory_limit'):
    temporal_flags.append('--set=elasticsearch.resources.limits.memory=%s' % temporal_cfg['elasticsearch_memory_limit'])
if temporal_cfg.get('elasticsearch_cpu_limit'):
    temporal_flags.append('--set=elasticsearch.resources.limits.cpu=%s' % temporal_cfg['elasticsearch_cpu_limit'])

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
        labels=['monitoring'],
    )

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
                dashboard_content = read_file(dashboard_path)
                dashboard_configmap['data'][dashboard_name] = dashboard_content

        grafana_objects.append(dashboard_configmap)

    k8s_yaml(encode_yaml_stream(grafana_objects))

    k8s_resource(
        'grafana',
        port_forwards='%s:3000' % get_port('grafana', 3100),
        resource_deps=['prometheus'],
        labels=['monitoring'],
    )
else:
    print("Monitoring disabled - skipping Prometheus and Grafana deployment")

# ------------------------------------------
# ADMIN API (Always Required)
# ------------------------------------------

docker_build_with_restart(
    "localhost:5001/canopyx-admin",
    ".",
    dockerfile="./Dockerfile.admin",
    entrypoint=["/app/admin"],
    live_update=[sync("bin/admin", "/app/admin")],
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
        break

k8s_yaml(encode_yaml_stream(admin_objects))

k8s_resource(
    "canopyx-admin",
    port_forwards=["%s:3000" % get_port('admin', 3000)],
    labels=['apps'],
    resource_deps=["clickhouse-server", "temporal-frontend"],
    pod_readiness='wait',
)

# ------------------------------------------
# QUERY API (Optional)
# ------------------------------------------

if components.get('query', True):
    print("Query API enabled")

    docker_build_with_restart(
        "localhost:5001/canopyx-query",
        ".",
        dockerfile="./Dockerfile.query",
        entrypoint=["/app/query"],
        live_update=[sync("bin/query", "/app/query")],
    )

    # Load query manifests and apply resource limits from profile
    query_objects = decode_yaml_stream(kustomize("./deploy/k8s/query/overlays/local"))

    for o in query_objects:
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopyx-query':
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'query':
                    query_cpu = canopyx_cfg.get('query_cpu_limit', '1000m')
                    query_mem = canopyx_cfg.get('query_memory_limit', '1Gi')
                    container['resources'] = {
                        'limits': {
                            'cpu': query_cpu,
                            'memory': query_mem
                        },
                        'requests': {
                            'cpu': str(int(query_cpu.replace('m', '')) // 2) + 'm' if 'm' in query_cpu else str(float(query_cpu) / 2),
                            'memory': str(int(query_mem.replace('Gi', '')) // 2) + 'Gi' if 'Gi' in query_mem else str(int(query_mem.replace('Mi', '')) // 2) + 'Mi'
                        }
                    }
                    print("CanopyX Query resources: CPU=%s, Memory=%s" % (query_cpu, query_mem))
            break

    k8s_yaml(encode_yaml_stream(query_objects))

    k8s_resource(
        "canopyx-query",
        port_forwards=["%s:3001" % get_port('query', 3001)],
        labels=['apps'],
        resource_deps=["clickhouse-server", "canopyx-admin"],
        pod_readiness='wait',
    )
else:
    print("Query API disabled")

# ------------------------------------------
# ADMIN WEB (Optional)
# ------------------------------------------

if components.get('admin_web', True):
    print("Admin Web UI enabled")

    docker_build(
        "localhost:5001/canopyx-admin-web",
        "./web/admin",
        dockerfile="./web/admin/Dockerfile",
        live_update=[
            fall_back_on(['./web/admin/package.json', './web/admin/pnpm-lock.yaml']),
            sync('./web/admin/app', '/app/app'),
            sync('./web/admin/public', '/app/public'),
        ],
    )

    # Load admin-web manifests and apply resource limits from profile
    admin_web_objects = decode_yaml_stream(kustomize("./deploy/k8s/admin-web/overlays/local"))

    for o in admin_web_objects:
        if o.get('kind') == 'Deployment' and o.get('metadata', {}).get('name') == 'canopyx-admin-web':
            containers = o.get('spec', {}).get('template', {}).get('spec', {}).get('containers', [])
            for container in containers:
                if container.get('name') == 'admin-web':
                    # Note: admin_web_memory_limit should be higher (512Mi+) for Next.js dev mode with hot reload
                    admin_web_cpu = canopyx_cfg.get('admin_web_cpu_limit', '500m')
                    admin_web_mem = canopyx_cfg.get('admin_web_memory_limit', '512Mi')
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

    k8s_resource(
        "canopyx-admin-web",
        port_forwards=["%s:3003" % get_port('admin_web', 3003)],
        labels=['apps'],
        resource_deps=["canopyx-admin"],
        pod_readiness='wait',
    )
else:
    print("Admin Web UI disabled")

# ------------------------------------------
# CONTROLLER (Always Required)
# ------------------------------------------

docker_build_with_restart(
    "localhost:5001/canopyx-controller",
    ".",
    dockerfile="./Dockerfile.controller",
    entrypoint=["/app/controller"],
    live_update=[sync("bin/controller", "/app/controller")],
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
        break

k8s_yaml(encode_yaml_stream(controller_objects))

k8s_resource(
    "canopyx-controller",
    labels=['apps'],
    objects=["canopyx-controller:serviceaccount", "canopyx-controller:role", "canopyx-controller:rolebinding"],
    resource_deps=["clickhouse-server", "temporal-frontend", "canopyx-admin"],
    pod_readiness='wait',
)

# ------------------------------------------
# CANOPY LOCAL NODE (Optional)
# ------------------------------------------

# Get configured Canopy source path
canopy_path = paths_cfg.get('canopy_source', '../canopy')
# Expand ~ to home directory
canopy_path = os.path.expanduser(canopy_path)

if components.get('canopy_node', False):
    if os.path.exists(canopy_path):
        print("Canopy node enabled - found source at %s" % canopy_path)

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
                "%s:6060" % get_port('canopy_debug', 6060),
                "%s:9090" % get_port('canopy_metrics', 9090),
            ],
            labels=['blockchain'],
            pod_readiness='wait',
        )
    else:
        fail("Canopy node enabled but source not found at: %s\nPlease update paths.canopy_source in tilt-config.yaml" % canopy_path)
else:
    print("Canopy node disabled")

# ------------------------------------------
# INDEXER (Always Required)
# ------------------------------------------

# Use indexer tag from config
idx_repo = dev_cfg.get('indexer_tag', 'localhost:5001/canopyx-indexer:dev')
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
  labels = ["Indexer"],
)

# ------------------------------------------
# TRIGGER DEFAULT CHAIN (Conditional on Canopy node)
# ------------------------------------------

if components.get('canopy_node', False) and os.path.exists(canopy_path):
    if dev_cfg.get('auto_register_chain', True):
        print("Auto-registering local Canopy chain")

        local_resource(
            name="add-canopy-local",
            cmd="""
            for i in {1..30}; do
              if curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '{"chain_id":"canopy_local","chain_name":"Canopy Local","rpc_endpoints":["https://node1.canopy.us.nodefleet.net/rpc"], "image":"localhost:5001/canopyx-indexer:dev","min_replicas":1,"max_replicas":6}' 2>/dev/null; then
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
            resource_deps=["canopyx-admin", "canopy-node"],
            allow_parallel=False,
            auto_init=True,
        )

        k8s_attach(
            name="indexer-local",
            obj="deployment/canopyx-indexer-canopy-local",
            resource_deps=["canopyx-controller", "add-canopy-local"],
            labels=['indexers'],
        )
    else:
        print("Auto-register chain disabled in config")

# Cleanup resource for controller-spawned deployments
k8s_custom_deploy(
  'cleanup-controller-spawns',
  apply_cmd='true',
  delete_cmd='kubectl delete deployment -l managed-by=canopyx-controller -l app=indexer --ignore-not-found --wait',
  deps=[],
)

k8s_resource('cleanup-controller-spawns', resource_deps=['canopyx-controller'], labels=['no-op'])