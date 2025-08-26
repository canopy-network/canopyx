# Use bash for nicer conditionals
SHELL := /bin/bash

# Project coords
PKG := github.com/canopy-network/canopyx
GO  ?= go

# Versions (bump here when needed)
LINT_VERSION ?= v1.60.3
PNPM_VERSION ?= 10.13.1
NODE_REQUIRED ?= 22

# Default API endpoint (override with `make register-chain API=http://...`)
API ?= http://localhost:3000
ADMIN_TOKEN ?= devtoken

# -------------------------------------------------
# Docker image builds
# -------------------------------------------------
DOCKER ?= docker
IMAGE_PREFIX ?= canopyx
# Optionally override version/tag: make docker-admin TAG=v0.1.0
TAG ?= latest

# ---- kind + local registry ---------------------------------------------------
KIND           ?= kind
KIND_CLUSTER   ?= kind
REG_NAME       ?= kind-registry
REG_PORT       ?= 5001
REG_IMAGE      ?= registry:2

# ---- Tilt --------------------------------------------------------------------
TILT           ?= tilt
TILTFILE       ?= Tiltfile
TILT_PORT      ?= 10350
# Comma-separated profiles: e.g. TILT_PROFILES=clickhouse,temporal
TILT_PROFILES  ?=
# Extra args you might want to pass (e.g., --namespace=my-ns)
TILT_ARGS      ?=

# ----------------------------
# Help
# ----------------------------
.PHONY: help
help:
	@echo "Targets:"
	@echo "  make tools              - install/ensure pnpm, golangci-lint, goimports"
	@echo "  make deps               - go mod tidy + pnpm install (web/admin)"
	@echo ""
	@echo "  make fmt                - go fmt + goimports"
	@echo "  make fmt-fix            - golangci-lint --fix (safe quick fixes)"
	@echo "  make lint               - golangci-lint run"
	@echo "  make lint-fix           - golangci-lint run --fix"
	@echo ""
	@echo "  make build-web          - build Next.js admin (static export)"
	@echo "  make build-go           - build all Go binaries"
	@echo "  make build              - full build (web + go)"
	@echo "  make test               - run go tests"
	@echo "  make clean              - remove build artifacts"
	@echo "  make upgrade-admin      - bump core admin deps (Next/ESLint/etc.)"
	@echo ""
	@echo "  make dev-web            - run Next.js dev server (PORT=3003, API at localhost:3000)"
	@echo "  make register-chain     - register a chain via admin API (requires ADMIN_TOKEN)"
	@echo ""
	@echo "  make docker-admin       - build admin docker image"
	@echo "  make docker-indexer     - build indexer docker image"
	@echo "  make docker-controller  - build controller docker image"
	@echo "  make docker-query       - build query docker image"
	@echo "  make docker-reporter    - build reporter docker image"
	@echo "  make docker-all         - build all docker images (tag=$(TAG), prefix=$(IMAGE_PREFIX))"
	@echo ""
	@echo "  make kind-up            - ensure local registry + kind cluster, wire together"
	@echo "  make kind-down          - delete kind cluster and local registry"
	@echo "  make kind-recreate      - recreate cluster and registry from scratch"
	@echo "  make kind-ensure-registry- ensure registry is running"
	@echo "  make kind-ensure-cluster - ensure kind cluster exists"
	@echo "  make kind-link-registry  - link registry into kind network + patch hosts.toml"
	@echo "  make kind-doc-registry   - publish ConfigMap for local-registry-hosting"
	@echo "  make kind-registry-info  - print usage info for tagging/pushing images"
	@echo "  make kind-disk-report  - show Docker disk usage + per-node containerd usage"
	@echo "  make kind-inode-report - show inode usage inside kind nodes"
	@echo "  make kind-clean        - delete cluster/registry and prune Docker (safe; no volumes unless VOLUMES=1)"
	@echo "  make kind-purge        - ⚠️ full Docker prune including volumes (use FORCE=1 to skip prompt)"
	@echo ""
	@echo "  make metrics-server-install - install metrics-server (kind-friendly flags)"
	@echo "  make metrics-server-status  - show metrics-server API/pod status, sample metrics"
	@echo "  make metrics-server-uninstall - uninstall metrics-server"
	@echo ""
	@echo "  make tilt-up            - start Tilt (interactive HUD)"
	@echo "  make tilt-up-stream     - start Tilt (stream logs, no HUD)"
	@echo "  make tilt-down          - stop Tilt and clean resources"
	@echo "  make tilt-status        - show Tilt status"
	@echo "  make tilt-logs          - follow Tilt logs"
	@echo "  make tilt-open          - open Tilt Web UI in browser"
	@echo "  make tilt-ci            - run Tilt in CI mode (build/deploy/test once)"
	@echo "  make tilt-restart       - restart Tilt (down then up)"
	@echo "  make tilt-kubecontext   - ensure kubectl context matches kind cluster"



# ----------------------------
# Tool checks (runtime, no stale vars)
# ----------------------------
.PHONY: check-node
check-node:
	@if ! command -v node >/dev/null 2>&1; then \
		echo "❌ node not found. Please install Node >= $(NODE_REQUIRED)"; exit 1; \
	fi
	@nv=$$(node -v | sed 's/^v\([0-9]*\).*/\1/'); \
	if [ "$$nv" -lt "$(NODE_REQUIRED)" ]; then \
		echo "❌ Node >= $(NODE_REQUIRED) required. Found: $$(node -v)"; exit 1; \
	fi
	@echo "✅ Node $$(node -v)"

.PHONY: check-pnpm
check-pnpm: check-node
	@if ! command -v pnpm >/dev/null 2>&1; then \
		echo "ℹ️  pnpm not found — enabling via corepack..."; \
		if ! command -v corepack >/dev/null 2>&1; then \
			echo "❌ corepack not found (Node >= 16 should have it). Install pnpm manually."; exit 1; \
		fi; \
		corepack enable; \
		corepack prepare pnpm@$(PNPM_VERSION) --activate; \
	fi
	@echo "✅ pnpm $$(pnpm -v)"

.PHONY: check-golangci
check-golangci:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "ℹ️  Installing golangci-lint $(LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		  | sh -s -- -b $$($(GO) env GOPATH)/bin $(LINT_VERSION); \
	fi
	@echo "✅ golangci-lint $$(golangci-lint version | head -n1 || echo installed)"

.PHONY: check-goimports
check-goimports:
	@if ! command -v goimports >/dev/null 2>&1; then \
		echo "ℹ️  installing goimports..."; \
		$(GO) install golang.org/x/tools/cmd/goimports@latest; \
	fi
	@echo "✅ goimports at $$(command -v goimports)"

.PHONY: tools
tools: check-pnpm check-golangci check-goimports

# ----------------------------
# Dependencies
# ----------------------------
.PHONY: deps
deps: tools
	@$(GO) mod tidy
	@cd web/admin && pnpm install

# ----------------------------
# Formatting & Lint
# ----------------------------
.PHONY: fmt
fmt: check-goimports
	@$(GO) fmt ./...
	@goimports -w .

# Quick safe auto-fixes via golangci-lint
.PHONY: fmt-fix
fmt-fix: check-golangci
	@golangci-lint run --fix || true

.PHONY: lint
lint: check-golangci
	@golangci-lint cache clean
	@golangci-lint run

.PHONY: lint-fix
lint-fix: check-golangci
	@golangci-lint cache clean
	@golangci-lint run --fix || true

# ----------------------------
# Build
# ----------------------------
.PHONY: build-web
build-web: deps
	@cd web/admin && pnpm build

.PHONY: build-go
build-go:
	@mkdir -p bin
	@$(GO) build -trimpath -o bin/admin     ./cmd/admin
	@$(GO) build -trimpath -o bin/controller ./cmd/controller
	@$(GO) build -trimpath -o bin/indexer   ./cmd/indexer
	@$(GO) build -trimpath -o bin/reporter  ./cmd/reporter
	@$(GO) build -trimpath -o bin/query     ./cmd/query

.PHONY: build
build: build-web build-go
	@echo "✅ Build complete."

# ----------------------------
# Test & Clean
# ----------------------------
.PHONY: test
test:
	@$(GO) test ./...

.PHONY: clean
clean:
	@rm -rf web/admin/out web/admin/.next
	@rm -rf bin
	@find . -name "*.test" -type f -delete
	@echo "🧹 Cleaned."

# ----------------------------
# Admin upgrades
# ----------------------------
.PHONY: upgrade-admin
upgrade-admin: check-pnpm
	@cd web/admin && pnpm up next@^15 eslint@^9 eslint-config-next@^15 autoprefixer@^10 postcss@^8 typescript@^5 --latest
	@echo "✅ Admin deps upgraded. Run 'make deps && make build-web'."


# ----------------------------
# Admin API calls
# ----------------------------

.PHONY: register-chain
register-chain:
	@if [ -z "$(ADMIN_TOKEN)" ]; then \
		echo "❌ ADMIN_TOKEN not set. Usage: make register-chain ADMIN_TOKEN=xxx"; \
		exit 1; \
	fi
	@echo "➡️  Registering chain canopy-mainnet at $(API)..."
	curl -sS -X POST "$(API)/api/chains" \
	  -H "Authorization: Bearer $(ADMIN_TOKEN)" \
	  -H "Content-Type: application/json" \
	  -d '{"chain_id":"canopy-mainnet","chain_name":"Canopy Mainnet","rpc_endpoints":["https://rpc.node1.canopy.us.nodefleet.net"]}' \
	  | jq .

# ----------------------------
# Admin Web dev
# ----------------------------

.PHONY: dev-web
dev-web: check-pnpm
	@cd web/admin && \
	NEXT_PUBLIC_API_BASE=http://localhost:3000 PORT=3003 pnpm run dev

# ----------------------------
# Docker
# ----------------------------

.PHONY: docker-admin
docker-admin:
	$(DOCKER) build \
		-f Dockerfile.admin \
		--build-arg NEXT_PUBLIC_API_BASE=http://localhost:3000 \
		-t $(IMAGE_PREFIX)/admin:$(TAG) .

.PHONY: docker-indexer
docker-indexer:
	$(DOCKER) build \
		-f Dockerfile.indexer \
		-t $(IMAGE_PREFIX)/indexer:$(TAG) .

.PHONY: docker-controller
docker-controller:
	$(DOCKER) build \
		-f Dockerfile.controller \
		-t $(IMAGE_PREFIX)/controller:$(TAG) .

.PHONY: docker-query
docker-query:
	$(DOCKER) build \
		-f Dockerfile.query \
		-t $(IMAGE_PREFIX)/query:$(TAG) .

.PHONY: docker-reporter
docker-reporter:
	$(DOCKER) build \
		-f Dockerfile.reporter \
		-t $(IMAGE_PREFIX)/reporter:$(TAG) .

.PHONY: docker-all
docker-all: docker-admin docker-indexer docker-controller docker-query docker-reporter
	@echo "✅ All Docker images built with tag $(TAG)"

# ----------------------------
# Kind
# ----------------------------

# If you're on kind v0.27.0+ you can drop the containerd patch completely.
define KIND_CONTAINERD_PATCH
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
endef
export KIND_CONTAINERD_PATCH

.PHONY: kind-up kind-down kind-recreate kind-ensure-registry kind-ensure-cluster kind-link-registry kind-doc-registry kind-registry-info

kind-up: kind-ensure-registry kind-ensure-cluster kind-link-registry kind-doc-registry kind-registry-info ## Start/ensure local registry and kind cluster wired together

kind-down: ## Delete kind cluster and local registry
	@echo ">> Deleting kind cluster $(KIND_CLUSTER) (if exists)"
	@$(KIND) delete cluster --name $(KIND_CLUSTER) 2>/dev/null || true
	@echo ">> Removing registry container $(REG_NAME) (if exists)"
	@docker rm -f $(REG_NAME) 2>/dev/null || true

kind-recreate: kind-down kind-up ## Recreate both cluster and registry from scratch

kind-ensure-registry:
	@echo ">> Ensuring local registry $(REG_NAME) on 127.0.0.1:$(REG_PORT)"
	@if [ "$$(docker inspect -f '{{.State.Running}}' $(REG_NAME) 2>/dev/null || true)" != "true" ]; then \
		echo ">> Starting $(REG_NAME) ..."; \
		docker run -d --restart=always \
			-p 127.0.0.1:$(REG_PORT):5000 \
			--network bridge \
			--name $(REG_NAME) $(REG_IMAGE); \
	else \
		echo ">> Registry already running, skipping"; \
	fi

# --- Kind tuned kubelet config (3 nodes: 1 cp + 2 workers) ---
# --- Kind tuned kubelet config (3 nodes) + containerd certs.d for local registry ---
define KIND_TUNED_CFG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

# Enable containerd to read registry host aliases from /etc/containerd/certs.d
containerdConfigPatches:
- |
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"

nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: KubeletConfiguration
    imageGCHighThresholdPercent: 70
    imageGCLowThresholdPercent: 50
    evictionHard:
      nodefs.available: "5%"
      nodefs.inodesFree: "5%"

- role: worker
  kubeadmConfigPatches:
  - |
    kind: KubeletConfiguration
    imageGCHighThresholdPercent: 70
    imageGCLowThresholdPercent: 50
    evictionHard:
      nodefs.available: "5%"
      nodefs.inodesFree: "5%"

- role: worker
  kubeadmConfigPatches:
  - |
    kind: KubeletConfiguration
    imageGCHighThresholdPercent: 70
    imageGCLowThresholdPercent: 50
    evictionHard:
      nodefs.available: "5%"
      nodefs.inodesFree: "5%"
endef
export KIND_TUNED_CFG

kind-ensure-cluster:
	@echo ">> Ensuring kind cluster $(KIND_CLUSTER)"
	@if ! $(KIND) get clusters | grep -qx "$(KIND_CLUSTER)"; then \
		echo ">> Creating cluster $(KIND_CLUSTER) with tuned kubelet GC + containerd certs.d"; \
		printf "%s" "$$KIND_TUNED_CFG" | $(KIND) create cluster --name $(KIND_CLUSTER) --config=-; \
	else \
		echo ">> Cluster already exists, skipping create"; \
	fi


# Teach each node to reach the registry via hosts.toml and ensure the registry container is on the 'kind' network
kind-link-registry:
	@echo ">> Linking registry to kind network (if not already)"
	@if [ "$$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' $(REG_NAME))" = "null" ]; then \
		docker network connect "kind" "$(REG_NAME)"; \
	fi
	@echo ">> Writing hosts.toml into each node so localhost:$(REG_PORT) resolves to the registry"
	@REGISTRY_DIR="/etc/containerd/certs.d/localhost:$(REG_PORT)"; \
	for node in $$($(KIND) get nodes --name $(KIND_CLUSTER)); do \
		docker exec "$$node" mkdir -p "$$REGISTRY_DIR"; \
		printf '%s\n' '[host."http://$(REG_NAME):5000"]' | docker exec -i "$$node" tee "$$REGISTRY_DIR/hosts.toml" >/dev/null; \
	done

# Advertise the local registry location in kube-public (KEP-1755)
kind-doc-registry:
	@echo ">> Documenting local registry in kube-public/local-registry-hosting"
	@printf "%s\n" \
	"apiVersion: v1" \
	"kind: ConfigMap" \
	"metadata:" \
	"  name: local-registry-hosting" \
	"  namespace: kube-public" \
	"data:" \
	"  localRegistryHosting.v1: |" \
	"    host: \"localhost:$(REG_PORT)\"" \
	"    help: \"https://kind.sigs.k8s.io/docs/user/local-registry/\"" \
	| kubectl apply -f -
	@echo ">> Done"

.PHONY: kind-registry-info

kind-registry-info: ## Show how to tag/push images into the local kind-registry
	@echo ""
	@echo "============================================================"
	@echo " Local kind-registry information"
	@echo "============================================================"
	@echo ""
	@echo " On your host, build & tag images like:"
	@echo ""
	@echo "   docker build -t localhost:$(REG_PORT)/myapp:dev ."
	@echo "   docker push localhost:$(REG_PORT)/myapp:dev"
	@echo ""
	@echo " Inside the kind cluster, the same image will be available as:"
	@echo ""
	@echo "   kind-registry:5000/myapp:dev"
	@echo ""
	@echo " This works because we wrote hosts.toml in each node and"
	@echo " published the ConfigMap kube-public/local-registry-hosting."
	@echo ""
	@echo "============================================================"
	@echo ""

# ----------------------------
# Metrics Server (for HPA)
# ----------------------------
.PHONY: metrics-server-install metrics-server-status metrics-server-uninstall

# ---- Metrics Server patches (jq program kept in a Make var to avoid quoting hell)
define JQ_MS_PATCH
.spec.template.spec.containers[0].args = [
  "--cert-dir=/tmp",
  "--secure-port=4443",
  "--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
  "--kubelet-use-node-status-port",
  "--metric-resolution=15s",
  "--kubelet-insecure-tls"
]
| .spec.template.spec.containers[0].ports = [
  {"name":"https","containerPort":4443,"protocol":"TCP"}
]
| .spec.template.spec.containers[0].livenessProbe = {
  "httpGet":{"path":"/livez","port":4443,"scheme":"HTTPS"},
  "initialDelaySeconds":20,"periodSeconds":10
}
| .spec.template.spec.containers[0].readinessProbe = {
  "httpGet":{"path":"/readyz","port":4443,"scheme":"HTTPS"},
  "initialDelaySeconds":10,"periodSeconds":10
}
endef
export JQ_MS_PATCH

.PHONY: metrics-server-install metrics-server-status
metrics-server-install: ## Install metrics-server (tuned for kind)
	@echo ">> Installing metrics-server"
	@kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	@echo ">> Patching metrics-server (args/ports/probes -> 4443, kind-friendly flags)"
	@kubectl -n kube-system get deploy metrics-server -o json \
	  | jq "$$JQ_MS_PATCH" \
	  | kubectl apply -f -
	@echo ">> Waiting for metrics-server deployment to be ready..."
	@kubectl -n kube-system rollout status deploy/metrics-server --timeout=120s
	@$(MAKE) metrics-server-status

metrics-server-status: ## Show metrics-server API and pod status
	@echo ">> APIService (should be Available=True)"
	@kubectl get apiservices.apiregistration.k8s.io v1beta1.metrics.k8s.io -o custom-columns=NAME:.metadata.name,AVAILABLE:.status.conditions[?(@.type==\"Available\")].status || true
	@echo ">> Pods:"
	@kubectl -n kube-system get pods -l k8s-app=metrics-server || true
	@echo ">> Sample metrics:"
	@kubectl top nodes || true
	@kubectl top pods -A | head -n 20 || true

metrics-server-uninstall: ## Uninstall metrics-server
	@echo ">> Uninstalling metrics-server"
	@kubectl delete -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml || true


# ----------------------------
# Kind / Docker cleanup & diagnostics
# ----------------------------
.PHONY: kind-disk-report kind-inode-report kind-clean kind-purge

# Show Docker disk usage plus space/usage inside each kind node
kind-disk-report: ## Report Docker disk usage and per-node containerd usage (space)
	@echo ">> Docker disk usage (host)"
	@docker system df -v || true
	@echo ""
	@echo ">> Kind node disk usage (inside containers)"
	@for n in $$($(KIND) get nodes --name $(KIND_CLUSTER) 2>/dev/null || true); do \
		echo "---- $$n: df -h"; \
		docker exec $$n sh -c 'df -h | sed "s/^/    /"'; \
		echo "---- $$n: du -sh /var/lib/containerd (this may take a bit)"; \
		docker exec $$n sh -c 'du -sh /var/lib/containerd 2>/dev/null || echo "    (no access or path missing)"'; \
	done

# Show inode usage inside each kind node (common cause of 'no space left on device')
kind-inode-report: ## Report inode usage inside kind nodes
	@for n in $$($(KIND) get nodes --name $(KIND_CLUSTER) 2>/dev/null || true); do \
		echo "---- $$n: df -i"; \
		docker exec $$n sh -c 'df -i | sed "s/^/    /"'; \
	done

# Clean unused Docker artifacts safely (keeps your running containers & images-in-use)
# Includes: delete cluster + registry (idempotent), then prune dangling images/networks/build cache.
kind-clean: ## Delete kind cluster + registry, prune Docker (keeps volumes unless VOLUMES=1)
	@echo ">> This will delete the kind cluster '$(KIND_CLUSTER)' and registry '$(REG_NAME)' and prune Docker."
	@if [ "$(FORCE)" != "1" ]; then \
		read -r -p "Proceed? [y/N] " ans; case $$ans in [yY]*) ;; *) echo "aborted."; exit 1;; esac; \
	fi
	@$(MAKE) kind-down
	@echo ">> Pruning Docker images/networks/build cache (dangling & unused)"
	@docker builder prune -af || true
	@docker image prune   -af || true
	@docker network prune -f  || true
	@if [ "$(VOLUMES)" = "1" ]; then \
		echo ">> Pruning Docker volumes (--volumes)"; \
		docker volume prune -f || true; \
	else \
		echo ">> Skipping volume prune (set VOLUMES=1 to remove unused volumes)"; \
	fi
	@echo ">> Done. Tip: run 'make kind-disk-report' to inspect current usage."

# Nuclear option: remove EVERYTHING unused from Docker (images, containers, networks, volumes, build cache).
# Also removes kindest/node images so next 'kind-up' pulls fresh.
kind-purge: ## ⚠️ Aggressive cleanup: prune ALL unused Docker data (includes volumes). Use FORCE=1 to skip prompt.
	@echo ">> WARNING: This will prune ALL unused Docker data, including volumes."
	@if [ "$(FORCE)" != "1" ]; then \
		read -r -p "Proceed with FULL prune? [y/N] " ans; case $$ans in [yY]*) ;; *) echo "aborted."; exit 1;; esac; \
	fi
	@$(MAKE) kind-down
	@echo ">> Full Docker prune (images, containers, networks, volumes, build cache)"
	@docker system prune -af --volumes || true
	@echo ">> Removing cached kindest/node images (optional, harmless if none)"
	@docker images 'kindest/node' -q | xargs -r docker rmi -f || true
	@echo ">> Done. You can now recreate with 'make tilt-up' or 'make kind-up'."


# ----------------------------
# Tilt
# ----------------------------

# Build profile flags for Tilt (empty if no profiles)
# shellwords-safe: split on comma
profile_flags = $(if $(strip $(TILT_PROFILES)),$(foreach p,$(subst $(comma), ,$(TILT_PROFILES)),-p $(p)),)

# Current kube context check (helps avoid pointing Tilt at the wrong cluster)
KUBE_CONTEXT_EXPECTED ?= kind-$(KIND_CLUSTER)

.PHONY: tilt-up tilt-up-stream tilt-down tilt-logs tilt-status tilt-open tilt-ci tilt-restart tilt-kubecontext

# Interactive Tilt (recommended for local dev).
# Ensures local registry + kind cluster first.
tilt-up: kind-up tilt-kubecontext ## Start Tilt (interactive HUD) after ensuring kind+registry
	@echo ">> Starting Tilt (interactive)"
	@$(TILT) up -f $(TILTFILE) --port $(TILT_PORT) \
		$(profile_flags) $(TILT_ARGS)

# Non-interactive streaming mode (good for running via scripts/CI-like terminals).
tilt-up-stream: kind-up tilt-kubecontext ## Start Tilt (non-interactive, streams logs, exits on ctrl-c)
	@echo ">> Starting Tilt (stream mode, no HUD)"
	@$(TILT) up -f $(TILTFILE) --hud=false --stream --port $(TILT_PORT) \
		$(profile_flags) $(TILT_ARGS)

# Clean everything that Tilt applied in this repo.
tilt-down: ## Stop Tilt and delete resources it manages
	@echo ">> Tilt down"
	@$(TILT) down -f $(TILTFILE) $(profile_flags) $(TILT_ARGS) || true

# Show current Tilt status (resource health/sync info)
tilt-status: ## Show Tilt status
	@$(TILT) status

# Stream logs for all resources managed by Tilt
tilt-logs: ## Follow Tilt logs
	@$(TILT) logs -f

# Open the Tilt Web UI (needs Tilt daemon or 'tilt up' already running)
tilt-open: ## Open Tilt Web UI in your browser
	@$(TILT) open --browser --port $(TILT_PORT) || true

# CI-style build (no live dev, exits non-zero on failure)
tilt-ci: kind-up tilt-kubecontext ## Run Tilt in CI mode (build/deploy/test once)
	@echo ">> Running 'tilt ci'"
	@$(TILT) ci -f $(TILTFILE) $(profile_flags) $(TILT_ARGS)

# Convenience: restart everything (down then up)
tilt-restart: tilt-down tilt-up ## Restart Tilt (interactive)

# Sanity check to ensure kubectl context matches the intended kind cluster
tilt-kubecontext:
	@ctx="$$(kubectl config current-context 2>/dev/null || true)"; \
	if [ "$$ctx" != "$(KUBE_CONTEXT_EXPECTED)" ]; then \
		echo ">> Switching kubectl context to $(KUBE_CONTEXT_EXPECTED)"; \
		kubectl config use-context "$(KUBE_CONTEXT_EXPECTED)"; \
	fi
