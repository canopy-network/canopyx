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

# Add temporal helm release
helm_resource(
  name='temporal',
  chart='temporal/temporal',
  release_name='temporal',
  flags=[
    '--values=./deploy/helm/temporal-values.yaml'
  ],
  pod_readiness='wait',
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
# ADMIN
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
)

# ------------------------------------------
# INDEXER
# ------------------------------------------

# Build an image with indexer binary

# Stable tag to share with the controller
idx_repo = "localhost:5001/canopyx-indexer:dev"

# Only include the files that affect the indexer image to avoid rebuilding on every change
idx_deps = [
  "Dockerfile.indexer",
  "cmd/indexer",
  "pkg",
  "go.mod",
  "go.sum",
]

local_resource(
  name = "build:indexer-stable",
  cmd  = "docker build -f Dockerfile.indexer -t %s . && docker push %s" % (idx_repo, idx_repo),
  deps = idx_deps,
  # optional: set to TRIGGER_MODE_MANUAL if you don’t want auto rebuild
  trigger_mode = TRIGGER_MODE_AUTO,
  labels = ["Indexer"],
)

# ------------------------------------------
# TRIGGER DEFAULT CHAIN
# ------------------------------------------

# Run curl AFTER the "admin" resource is deployed and marked healthy
local_resource(
    name="add-canopy-mainnet",
    cmd="curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '{\"chain_id\":\"canopy_mainnet\",\"chain_name\":\"Canopy MainNet\",\"rpc_endpoints\":[\"https://node1.canopy.us.nodefleet.net/rpc\"]}' || exit 1",
    deps=[],
    labels=['no-op'],
    resource_deps=["canopyx-admin"],  # 👈 waits for admin resource to be healthy
    allow_parallel=False,     # run sequentially, after admin
    auto_init=True,           # runs on startup
)

local_resource(
    name="add-canopy-canary",
    cmd="curl -X POST -f http://localhost:3000/api/chains -H 'Authorization: Bearer devtoken' -d '{\"chain_id\":\"canopy_canary\",\"chain_name\":\"Canopy CanaryNet\",\"rpc_endpoints\":[\"https://node2.canopy.us.nodefleet.net/rpc\"]}' || exit 1",
    deps=[],
    labels=['no-op'],
    resource_deps=["canopyx-admin"],  # 👈 waits for admin resource to be healthy
    allow_parallel=False,     # run sequentially, after admin
    auto_init=True,           # runs on startup
)

# ------------------------------------------
# HANDLE DEFAULT CHAIN AS RESOURCE (need research/dev)
# ------------------------------------------

# Logical resource in Tilt - TODO: research if this will be allowed by tilt, since it require an object, but the deployment does not exists yet...
k8s_attach(
    name="indexer-mainnet",
    obj="deployment/canopyx-indexer-canopy-mainnet",
    resource_deps=["canopyx-controller", "add-canopy-mainnet"],
    labels=['indexers'],
)

k8s_attach(
    name="indexer-canary",
    obj="deployment/canopyx-indexer-canopy-canary",
    resource_deps=["canopyx-controller", "add-canopy-canary"],
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
