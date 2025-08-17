load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://k8s_attach', 'k8s_attach')

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
