load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://k8s_attach', 'k8s_attach')
load('ext://restart_process', 'docker_build_with_restart')

# Add bitnami repo
helm_repo(
  name='hyperdx',
  url='https://hyperdxio.github.io/helm-charts',
  labels=['helm_repo'],
  resource_name='helm-repo-hyperdx'
)

# Add temporal repo
helm_repo(
  name='temporal',
  url='https://go.temporal.io/helm-charts',
  labels=['helm_repo'],
  resource_name='helm-repo-temporal'
)

# Add bitnami repo for PostgreSQL
helm_repo(
  name='bitnami',
  url='https://charts.bitnami.com/bitnami',
  labels=['helm_repo'],
  resource_name='helm-repo-bitnami'
)

helm_resource(
  name='clickhouse',
  chart='hyperdx/hdx-oss-v2',
  release_name='clickhouse',
  flags=[
    '--values=./deploy/helm/clickhouse-values.yaml'
  ],
  pod_readiness='wait',
  labels=['clickhouse']
)

# Patch ClickHouse ConfigMap with increased limits after Helm install
# This is required because HyperDX chart uses hardcoded config.xml from data/ directory
local_resource(
  'clickhouse-config-patch',
  cmd='''
    echo "Patching ClickHouse ConfigMap with increased concurrency limits..."
    kubectl get configmap clickhouse-hdx-oss-v2-clickhouse-config -o yaml | \
      sed 's/<max_concurrent_queries>100<\\/max_concurrent_queries>/<max_concurrent_queries>2000<\\/max_concurrent_queries>/' | \
      sed 's/<max_connections>4096<\\/max_connections>/<max_connections>3000<\\/max_connections>/' | \
      kubectl apply -f - && \
    echo "Restarting ClickHouse pod to apply new configuration..." && \
    kubectl delete pod -l app.kubernetes.io/name=clickhouse --wait && \
    echo "ClickHouse configuration updated successfully!"
  ''',
  resource_deps=['clickhouse'],
  labels=['clickhouse'],
)

k8s_attach(
    name="clickhouse-web",
    obj="deployment/clickhouse-hdx-oss-v2-app",
    port_forwards=["8081:3000"],
    resource_deps=["clickhouse"],
    labels=['clickhouse'],
)

k8s_attach(
    name="clickhouse-server",
    obj="deployment/clickhouse-hdx-oss-v2-clickhouse",
    port_forwards=["8123:8123", "9000:9000"],
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

# Add temporal helm release with Cassandra + Elasticsearch
# (PostgreSQL removed - using Cassandra to eliminate WAL write bottleneck)
helm_resource(
  name='temporal',
  chart='temporal/temporal',
  release_name='temporal',
  flags=[
    '--values=./deploy/helm/temporal-cassandra-elasticsearch-values.yaml'
  ],
  pod_readiness='wait',
  resource_deps=['helm-repo-temporal'],
  labels=['temporal']
)

k8s_attach(
    name="temporal-web",
    obj="deployment/temporal-web",
    port_forwards=["8080"],
    resource_deps=["temporal"],
    labels=['temporal'],
)

k8s_attach(
    name="temporal-frontend",
    obj="deployment/temporal-frontend",
    port_forwards=["7233"],
    resource_deps=["temporal"],
    labels=['temporal'],
)

k8s_attach(
    name="temporal-worker",
    obj="deployment/temporal-worker",
    port_forwards=["7239", "6939"],
    resource_deps=["temporal"],
    labels=['temporal'],
)

# ------------------------------------------
# MONITORING (Prometheus + Grafana)
# ------------------------------------------

# Prometheus - monitoring and metrics
k8s_yaml(blob('''
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s

    scrape_configs:
      # Scrape Temporal services
      - job_name: 'temporal-frontend'
        static_configs:
          - targets: ['temporal-frontend:9090']
        metrics_path: '/metrics'

      - job_name: 'temporal-history'
        static_configs:
          - targets: ['temporal-history-headless:9090']
        metrics_path: '/metrics'

      - job_name: 'temporal-matching'
        static_configs:
          - targets: ['temporal-matching-headless:9090']
        metrics_path: '/metrics'

      - job_name: 'temporal-worker'
        static_configs:
          - targets: ['temporal-worker:9090']
        metrics_path: '/metrics'

      # Scrape ClickHouse
      - job_name: 'clickhouse'
        static_configs:
          - targets: ['clickhouse-hdx-oss-v2-clickhouse:9363']
        metrics_path: '/metrics'

      # Scrape Kubernetes pods with prometheus.io annotations
      - job_name: 'kubernetes-pods'
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
            action: keep
            regex: true
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
            action: replace
            target_label: __metrics_path__
            regex: (.+)
          - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
            action: replace
            regex: ([^:]+)(?::[0-9]+)?;([0-9]+)
            replacement: $1:$2
            target_label: __address__
---
apiVersion: v1
kind: Pod
metadata:
  name: prometheus
  labels:
    app: prometheus
spec:
  serviceAccountName: prometheus
  containers:
  - name: prometheus
    image: prom/prometheus:latest
    args:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=2d'
    ports:
    - containerPort: 9090
    volumeMounts:
    - name: config
      mountPath: /etc/prometheus
  volumes:
  - name: config
    configMap:
      name: prometheus-config
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
spec:
  selector:
    app: prometheus
  ports:
  - port: 9090
    targetPort: 9090
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus
rules:
- apiGroups: [""]
  resources:
  - nodes
  - nodes/proxy
  - services
  - endpoints
  - pods
  verbs: ["get", "list", "watch"]
- apiGroups:
  - extensions
  resources:
  - ingresses
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: prometheus
subjects:
- kind: ServiceAccount
  name: prometheus
  namespace: default
'''))

k8s_resource(
  'prometheus',
  port_forwards='9090:9090',
  labels=['monitoring'],
)

# Grafana - visualization and dashboards
k8s_yaml(blob('''
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasources
data:
  prometheus.yml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        access: proxy
        url: http://prometheus:9090
        isDefault: true
        editable: true
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards-config
data:
  dashboards.yml: |
    apiVersion: 1
    providers:
      - name: 'CanopyX'
        orgId: 1
        folder: ''
        type: file
        disableDeletion: false
        updateIntervalSeconds: 10
        allowUiUpdates: true
        options:
          path: /var/lib/grafana/dashboards
---
apiVersion: v1
kind: Pod
metadata:
  name: grafana
  labels:
    app: grafana
spec:
  containers:
  - name: grafana
    image: grafana/grafana:latest
    ports:
    - containerPort: 3000
    env:
    - name: GF_AUTH_ANONYMOUS_ENABLED
      value: "true"
    - name: GF_AUTH_ANONYMOUS_ORG_ROLE
      value: "Admin"
    - name: GF_AUTH_DISABLE_LOGIN_FORM
      value: "true"
    volumeMounts:
    - name: datasources
      mountPath: /etc/grafana/provisioning/datasources
    - name: dashboards-config
      mountPath: /etc/grafana/provisioning/dashboards/dashboards.yml
      subPath: dashboards.yml
  volumes:
  - name: datasources
    configMap:
      name: grafana-datasources
  - name: dashboards-config
    configMap:
      name: grafana-dashboards-config
---
apiVersion: v1
kind: Service
metadata:
  name: grafana
spec:
  selector:
    app: grafana
  ports:
  - port: 3000
    targetPort: 3000
'''))

k8s_resource(
  'grafana',
  port_forwards='3100:3000',
  resource_deps=['prometheus'],
  labels=['monitoring'],
)

# ------------------------------------------
# ADMIN API (Go Backend)
# ------------------------------------------

# Build an image with a admin binary
docker_build_with_restart(
    "localhost:5001/canopyx-admin",
    ".",
    dockerfile="./Dockerfile.admin",
    entrypoint=["/app/admin"],
    live_update=[sync("bin/admin", "/app/admin")],
)

k8s_yaml(kustomize("./deploy/k8s/admin/overlays/local"))

k8s_resource(
    "canopyx-admin",
    port_forwards=["3000:3000"],
    labels=['apps'],
    resource_deps=["clickhouse-server", "temporal-frontend"],
    pod_readiness='wait',  # Wait for readiness probe to pass
)

# ------------------------------------------
# QUERY API (Go Backend)
# ------------------------------------------

# Build an image with query binary
docker_build_with_restart(
    "localhost:5001/canopyx-query",
    ".",
    dockerfile="./Dockerfile.query",
    entrypoint=["/app/query"],
    live_update=[sync("bin/query", "/app/query")],
)

k8s_yaml(kustomize("./deploy/k8s/query/overlays/local"))

k8s_resource(
    "canopyx-query",
    port_forwards=["3001:3001"],
    labels=['apps'],
    resource_deps=["clickhouse-server", "canopyx-admin"],
    pod_readiness='wait',
)

# ------------------------------------------
# ADMIN WEB (Next.js Frontend)
# ------------------------------------------

# Build the admin web frontend image with hot reload support
# No build args needed - backend URLs are configured via runtime env vars in k8s deployment
docker_build(
    "localhost:5001/canopyx-admin-web",
    "./web/admin",
    dockerfile="./web/admin/Dockerfile",
    live_update=[
        # Fall back to full rebuild if dependencies change (must be first)
        fall_back_on(['./web/admin/package.json', './web/admin/pnpm-lock.yaml']),
        # Sync app directory changes - triggers Next.js hot reload
        sync('./web/admin/app', '/app/app'),
        # Sync public directory changes
        sync('./web/admin/public', '/app/public'),
    ],
)

k8s_yaml(kustomize("./deploy/k8s/admin-web/overlays/local"))

k8s_resource(
    "canopyx-admin-web",
    port_forwards=["3003:3003"],
    labels=['apps'],
    resource_deps=["canopyx-admin"],
    pod_readiness='wait',  # Wait for readiness probe to pass
)

# ------------------------------------------
# CONTROLLER
# ------------------------------------------

# Build an image with a indexer binary
docker_build_with_restart(
    "localhost:5001/canopyx-controller",
    ".",
    dockerfile="./Dockerfile.controller",
    entrypoint=["/app/controller"],
    live_update=[sync("bin/controller", "/app/controller")],
)

k8s_yaml(kustomize("./deploy/k8s/controller/overlays/local"))

k8s_resource(
    "canopyx-controller",
    labels=['apps'],
    objects=["canopyx-controller:serviceaccount", "canopyx-controller:role", "canopyx-controller:rolebinding"],
    resource_deps=["clickhouse-server", "temporal-frontend", "canopyx-admin"],
    pod_readiness='wait',  # Wait for readiness probe to pass
)

# ------------------------------------------
# CANOPY LOCAL NODE (Optional)
# ------------------------------------------

# Only include Canopy node if the ../canopy directory exists
canopy_path = '../canopy'
if os.path.exists(canopy_path):
    print("Found Canopy source at %s - including local node in deployment" % canopy_path)

    # Build Canopy node image using the Dockerfile from the canopy project
    docker_build(
        "localhost:5001/canopy-node",
        canopy_path,
        dockerfile=canopy_path + "/.docker/Dockerfile",
        # Watch for changes in canopy source
        live_update=[],  # Full rebuild on any change - can optimize later
    )

    # Deploy Canopy node manifests
    k8s_yaml(kustomize("./deploy/k8s/canopy-node/overlays/local"))

    # Configure Canopy node resource with port forwards
    k8s_resource(
        "canopy-node",
        objects=["canopy-node-data:persistentvolumeclaim", "canopy-node-config:configmap", "canopy-node-genesis:configmap", "canopy-node-keystore:configmap"],
        port_forwards=[
            "50000:50000",  # Wallet
            "50001:50001",  # Explorer
            "50002:50002",  # RPC
            "50003:50003",  # Admin RPC
            "9001:9001",    # TCP P2P
            "6060:6060",    # Debug
            "9090:9090",    # Metrics
        ],
        labels=['blockchain'],
        pod_readiness='wait',  # Wait for readiness probe to pass
    )
else:
    print("Canopy source not found at %s - skipping local node deployment" % canopy_path)
    print("To use local Canopy node, clone it to: %s" % canopy_path)

# ------------------------------------------
# INDEXER
# ------------------------------------------

# Build an image with indexer binary

# Stable tag to share with the controller
idx_repo = "localhost:5001/canopyx-indexer:dev"

# Only include the files that affect the indexer image to avoid rebuilding on every change
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
  # optional: set to TRIGGER_MODE_MANUAL if you don't want auto rebuild
  trigger_mode = TRIGGER_MODE_AUTO,
  labels = ["Indexer"],
)

# ------------------------------------------
# TRIGGER DEFAULT CHAIN (Conditional on Canopy local node)
# ------------------------------------------

# Only register local Canopy chain if the canopy source exists
if os.path.exists(canopy_path):
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
        resource_deps=["canopyx-admin", "canopy-node"],  # Wait for both admin and canopy node
        allow_parallel=False,
        auto_init=True,
    )

    # ------------------------------------------
    # HANDLE LOCAL CHAIN INDEXER
    # ------------------------------------------

    # Attach to indexer deployment created by the controller for local node
    k8s_attach(
        name="indexer-local",
        obj="deployment/canopyx-indexer-canopy-local",
        resource_deps=["canopyx-controller", "add-canopy-local"],
        labels=['indexers'],
    )

# This resource only exists so `tilt down` deletes the child made by the controller
k8s_custom_deploy(
  'cleanup-controller-spawns',
  apply_cmd='true',  # no-op on tilt up
  delete_cmd='kubectl delete deployment -l managed-by=canopyx-controller -l app=indexer --ignore-not-found --wait',
  deps=[],
)

# ensure deletion happens after the controller is torn down
k8s_resource('cleanup-controller-spawns', resource_deps=['canopyx-controller'], labels=['no-op'])
