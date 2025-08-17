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

# ----------------------------
# Help
# ----------------------------
.PHONY: help
help:
	@echo "Targets:"
	@echo "  make tools            - install/ensure pnpm, golangci-lint, goimports"
	@echo "  make deps             - go mod tidy + pnpm install (web/admin)"
	@echo "  make fmt              - go fmt + goimports"
	@echo "  make fmt-fix          - golangci-lint --fix (safe quick fixes)"
	@echo "  make lint             - golangci-lint run"
	@echo "  make lint-fix         - golangci-lint run --fix"
	@echo "  make build-web        - build Next.js admin (static export)"
	@echo "  make build-go         - build all Go binaries"
	@echo "  make build            - full build (web + go)"
	@echo "  make test             - run go tests"
	@echo "  make clean            - remove build artifacts"
	@echo "  make upgrade-admin    - bump core admin deps (Next/ESLint/etc.)"
	@echo ""
	@echo "  make dev-web          - run Next.js dev server (PORT=3003, API at localhost:3000)"
	@echo "  make register-chain   - register a chain via admin API (requires ADMIN_TOKEN)"
	@echo ""
	@echo "  make docker-admin     - build admin docker image"
	@echo "  make docker-indexer   - build indexer docker image"
	@echo "  make docker-controller- build controller docker image"
	@echo "  make docker-query     - build query docker image"
	@echo "  make docker-reporter  - build reporter docker image"
	@echo "  make docker-all       - build all docker images (tag=$(TAG), prefix=$(IMAGE_PREFIX))"


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
