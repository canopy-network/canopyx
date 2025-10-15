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

# Add postgresql helm release
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

# Add PostgreSQL for Temporal (Temporal chart doesn't include PostgreSQL)
# Note: PVC retention policy set to Delete for clean state on tilt down
helm_resource(
  name='temporal-postgresql',
  chart='bitnami/postgresql',
  release_name='temporal-postgresql',
  flags=[
    '--set', 'auth.username=temporal',
    '--set', 'auth.password=temporal',
    '--set', 'auth.database=temporal',
    '--set', 'primary.persistence.enabled=true',
    '--set', 'primary.persistence.size=10Gi',
    '--set', 'primary.persistentVolumeClaimRetentionPolicy.enabled=true',
    '--set', 'primary.persistentVolumeClaimRetentionPolicy.whenDeleted=Delete',
    '--set', 'primary.persistentVolumeClaimRetentionPolicy.whenScaled=Retain',
    '--set', 'resources.limits.memory=512Mi',
    '--set', 'resources.limits.cpu=500m',
    '--set', 'resources.requests.memory=256Mi',
    '--set', 'resources.requests.cpu=250m',
  ],
  pod_readiness='wait',
  labels=['database'],
  resource_deps=['helm-repo-bitnami']
)

k8s_attach(
    name="temporal-postgresql-server",
    obj="statefulset/temporal-postgresql",
    port_forwards=["5432:5432"],
    resource_deps=["temporal-postgresql"],
    labels=['database'],
)

# Add temporal helm release
helm_resource(
  name='temporal',
  chart='temporal/temporal',
  release_name='temporal',
  flags=[
    '--values=./deploy/helm/temporal-values.yaml'
  ],
  pod_readiness='wait',
  resource_deps=['temporal-postgresql'],
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
    resource_deps=["clickhouse-server"],
    pod_readiness='wait',
)

# ------------------------------------------
# ADMIN WEB (Next.js Frontend)
# ------------------------------------------

# Build the admin web frontend image with hot reload support
docker_build(
    "localhost:5001/canopyx-admin-web",
    "./web/admin",
    dockerfile="./web/admin/Dockerfile",
    build_args={
        "NEXT_PUBLIC_API_BASE": "http://localhost:3000",
        "NEXT_PUBLIC_QUERY_SERVICE_URL": "http://localhost:3001",
    },
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
          if curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '{"chain_id":"canopy_local","chain_name":"Canopy Local","rpc_endpoints":["http://canopy-node.default.svc.cluster.local:50002"], "image":"localhost:5001/canopyx-indexer:dev"}' 2>/dev/null; then
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
