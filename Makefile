# Arenile — gestionale open source per stabilimenti balneari (Coste360 SRL).
#
# This Makefile is the ONLY supported interface for building, testing and
# checking the project (spec 27.2). Humans, CI and coding agents all use the
# same targets. Run `make` or `make help` for the list.
#
# Compatible with GNU Make >= 3.81 (the version shipped with macOS): no
# .ONESHELL, no .SHELLFLAGS. Multi-line recipes use explicit `set -euo pipefail`.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

MODULE                := github.com/coste360/arenile
VERSION               ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE                 ?= ghcr.io/coste360/arenile
GOLANGCI_LINT_VERSION ?= v2.15.0
POSTGRES_IMAGE        ?= postgres:17-alpine
# Go packages for test/test-db/lint; narrow it for focused runs: make test PKG=./internal/money/...
PKG                   ?= ./...

BIN_DIR       := $(CURDIR)/bin
GOLANGCI_LINT := $(shell if [ -x "$(BIN_DIR)/golangci-lint" ]; then echo "$(BIN_DIR)/golangci-lint"; else command -v golangci-lint 2>/dev/null; fi)

# Local development defaults (never use these values in production).
DEV_DATA_DIR := $(CURDIR)/tmp/data
DEV_SECRET   := YXJlbmlsZS1kZXYtb25seS1zZWNyZXQta2V5LTMyYiE=
DEV_ENV      := ARENILE_DATA_DIR=$(DEV_DATA_DIR) \
                ARENILE_SECRET_KEY=$(DEV_SECRET) \
                ARENILE_ADDR=:8080 \
                ARENILE_BASE_URL=http://localhost:8080 \
                ARENILE_LOG_LEVEL=debug \
                ARENILE_AUTO_MIGRATE=true

# Optional parts of the tree: targets degrade gracefully when they are absent
# (useful while the repository is being bootstrapped).
HAS_WEB     := $(wildcard web/package.json)
HAS_WEB_LCK := $(wildcard web/package-lock.json)
HAS_E2E     := $(wildcard e2e/package.json)
HAS_E2E_LCK := $(wildcard e2e/package-lock.json)
HAS_API     := $(wildcard api/openapi.yaml)

GO_GENERATED  := internal/http/api/api.gen.go
WEB_GENERATED := web/shared/api/generated
STATIC_DIR    := internal/http/static

LDFLAGS := -s -w -X main.version=$(VERSION)

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

.PHONY: help
help: ## Mostra questo aiuto (target predefinito)
	@printf 'Arenile — comandi disponibili (versione %s)\n\n' '$(VERSION)'
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
	@printf '\nVariabili utili: PKG=%s VERSION=%s IMAGE=%s GOLANGCI_LINT_VERSION=%s\n' '$(PKG)' '$(VERSION)' '$(IMAGE)' '$(GOLANGCI_LINT_VERSION)'

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

.PHONY: setup
setup: ## Installa dipendenze Go e Node, golangci-lint in ./bin, browser Playwright
	go mod download
	@mkdir -p "$(BIN_DIR)"
	@if [ -x "$(BIN_DIR)/golangci-lint" ] && "$(BIN_DIR)/golangci-lint" version 2>/dev/null | grep -q "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))"; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) already installed in ./bin"; \
	else \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) into ./bin (built with the local Go toolchain)"; \
		GOBIN="$(BIN_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
ifneq ($(HAS_WEB),)
ifneq ($(HAS_WEB_LCK),)
	npm --prefix web ci
else
	@echo "web/package-lock.json not found: falling back to npm install"
	npm --prefix web install
endif
else
	@echo "web/ not present yet: skipping frontend dependencies"
endif
ifneq ($(HAS_E2E),)
ifneq ($(HAS_E2E_LCK),)
	npm --prefix e2e ci
else
	npm --prefix e2e install
endif
	cd e2e && npx playwright install chromium
else
	@echo "e2e/ not present yet: skipping Playwright"
endif

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

.PHONY: dev
dev: ## Avvia backend (ricarica automatica se c'e' air) e Vite per le due SPA
	@mkdir -p "$(DEV_DATA_DIR)" "$(CURDIR)/tmp/bin"
	@set -euo pipefail; \
	trap 'trap - INT TERM EXIT; kill 0 2>/dev/null || true' INT TERM EXIT; \
	if command -v air >/dev/null 2>&1; then \
		echo "dev: backend with auto-reload (air)"; \
		$(DEV_ENV) air \
			--build.cmd "go build -o ./tmp/bin/arenile ./cmd/arenile" \
			--build.bin "./tmp/bin/arenile serve" \
			--build.include_ext "go,sql,yaml,html,tmpl,json" \
			--build.exclude_dir "web,e2e,node_modules,tmp,bin,docs,deploy,.git" \
			--build.exclude_regex "_test\\.go$$" & \
	else \
		echo "dev: air not found, running plain 'go run' (no auto-reload; install with: go install github.com/air-verse/air@latest)"; \
		$(DEV_ENV) go run ./cmd/arenile serve & \
	fi; \
	if [ -f web/package.json ]; then \
		npm --prefix web run dev & \
	else \
		echo "dev: web/ not present, frontend dev server skipped"; \
	fi; \
	wait

.PHONY: seed
seed: ## Crea il database locale con lo stabilimento dimostrativo
	@mkdir -p "$(DEV_DATA_DIR)"
	$(DEV_ENV) go run ./cmd/arenile seed demo

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------

.PHONY: gen
gen: ## Rigenera server Go e tipi TypeScript da api/openapi.yaml
ifneq ($(HAS_API),)
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
ifneq ($(HAS_WEB),)
	npm --prefix web run gen
endif
else
	@echo "api/openapi.yaml not present yet: nothing to generate"
endif

.PHONY: gen-check
gen-check: gen ## Verifica che il codice generato sia allineato a quello versionato
	@set -euo pipefail; \
	paths=""; \
	for p in $(GO_GENERATED) $(WEB_GENERATED); do [ -e "$$p" ] && paths="$$paths $$p"; done; \
	if [ -z "$$paths" ]; then echo "gen-check: no generated files yet"; exit 0; fi; \
	if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then echo "gen-check: not a git repository, skipped"; exit 0; fi; \
	changes="$$(git status --porcelain --untracked-files=all -- $$paths)"; \
	if [ -n "$$changes" ]; then \
		echo "gen-check: generated code is out of date or not committed:"; \
		echo "$$changes"; \
		git --no-pager diff --stat -- $$paths; \
		echo "Run 'make gen' and commit the result."; \
		exit 1; \
	fi; \
	echo "gen-check: generated code is up to date"

# ---------------------------------------------------------------------------
# Quality
# ---------------------------------------------------------------------------

.PHONY: lint
lint: ## golangci-lint, ESLint, Prettier in verifica, tsc --noEmit
ifneq ($(GOLANGCI_LINT),)
	$(GOLANGCI_LINT) run $(PKG)
else
	@echo "WARNING: golangci-lint not found (run 'make setup'): falling back to 'go vet'" >&2
	go vet $(PKG)
endif
ifneq ($(HAS_WEB),)
	npm --prefix web run lint
	npm --prefix web run format:check
	npm --prefix web run typecheck
endif

.PHONY: fmt
fmt: ## Formatta Go (gofumpt, goimports) e frontend (Prettier)
	@set -euo pipefail; \
	files="$$(find . -type f -name '*.go' ! -name '*.gen.go' \
		! -path './web/*' ! -path './e2e/*' ! -path './bin/*' ! -path './tmp/*' ! -path '*/node_modules/*' ! -path './.git/*')"; \
	if [ -n "$$files" ]; then \
		echo "$$files" | xargs go tool gofumpt -w; \
		echo "$$files" | xargs go tool goimports -local $(MODULE) -w; \
	fi
ifneq ($(HAS_WEB),)
	npm --prefix web run format
endif

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

.PHONY: test
test: ## Test unitari Go e Vitest
	go test $(PKG)
ifneq ($(HAS_WEB),)
	npm --prefix web run test
endif

.PHONY: test-db
test-db: ## Test di integrazione su SQLite e PostgreSQL (container effimero)
	@set -euo pipefail; \
	if [ -n "$${ARENILE_TEST_POSTGRES_URL:-}" ]; then \
		echo "test-db: using ARENILE_TEST_POSTGRES_URL from the environment"; \
		go test -tags=integration $(PKG); \
		exit 0; \
	fi; \
	if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then \
		echo "WARNING: docker not available: running integration tests on SQLite only" >&2; \
		go test -tags=integration $(PKG); \
		exit 0; \
	fi; \
	cid="$$(docker run -d --rm \
		-e POSTGRES_USER=arenile -e POSTGRES_PASSWORD=arenile -e POSTGRES_DB=arenile_test \
		-p 127.0.0.1::5432 $(POSTGRES_IMAGE) \
		-c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=200)"; \
	trap 'docker rm -f "$$cid" >/dev/null 2>&1 || true' EXIT INT TERM; \
	port="$$(docker port "$$cid" 5432/tcp | head -n1 | sed 's/.*://')"; \
	echo "test-db: postgres container $${cid:0:12} on 127.0.0.1:$$port"; \
	ready=0; \
	for _ in $$(seq 1 60); do \
		if docker exec "$$cid" pg_isready -U arenile -d arenile_test -h 127.0.0.1 >/dev/null 2>&1; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ready" != 1 ]; then echo "test-db: postgres did not become ready" >&2; docker logs "$$cid" >&2 || true; exit 1; fi; \
	ARENILE_TEST_POSTGRES_URL="postgres://arenile:arenile@127.0.0.1:$$port/arenile_test?sslmode=disable" \
		go test -tags=integration $(PKG)

.PHONY: test-race
test-race: ## Test di concorrenza con -race (disponibilita', pagamenti)
	go test -race -run Concurrency ./internal/...

.PHONY: e2e
e2e: web-build ## Test end-to-end Playwright con server di prova e dati demo
ifneq ($(HAS_E2E),)
	cd e2e && npx playwright test
else
	@echo "e2e/ not present yet: skipping end-to-end tests"
endif

.PHONY: check
check: gen-check lint test ## gen + verifica codice generato + lint + test (< 2 min)

.PHONY: check-full
check-full: check test-db test-race e2e ## check + test-db + test-race + e2e (prima della PR)

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

.PHONY: web-build
web-build: ## Compila le SPA e le copia in internal/http/static/{admin,booking}
ifneq ($(HAS_WEB),)
	npm --prefix web run build
	@set -euo pipefail; \
	for app in admin booking; do \
		dest="$(STATIC_DIR)/$$app"; \
		mkdir -p "$$dest"; \
		find "$$dest" -mindepth 1 ! -name .gitkeep -exec rm -rf {} +; \
		if [ -d "web/$$app/dist" ]; then \
			cp -R "web/$$app/dist/." "$$dest/"; \
			echo "web-build: copied web/$$app/dist -> $$dest"; \
		else \
			echo "WARNING: web/$$app/dist not found after build" >&2; \
		fi; \
		touch "$$dest/.gitkeep"; \
	done
else
	@echo "web/ not present yet: skipping frontend build (the server serves a placeholder page)"
endif

.PHONY: build
build: web-build ## Compila il binario statico in bin/arenile
	@mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/arenile ./cmd/arenile

.PHONY: docker
docker: ## Costruisce l'immagine Docker locale ghcr.io/coste360/arenile:dev
	docker build -f deploy/Dockerfile --build-arg VERSION=$(VERSION) -t $(IMAGE):dev .

.PHONY: clean
clean: ## Rimuove binario, build del frontend e file temporanei (conserva ./bin/golangci-lint)
	rm -rf bin/arenile tmp dist coverage.out web/admin/dist web/booking/dist playwright-report test-results e2e/playwright-report e2e/test-results
	@for app in admin booking; do \
		if [ -d "$(STATIC_DIR)/$$app" ]; then find "$(STATIC_DIR)/$$app" -mindepth 1 ! -name .gitkeep -exec rm -rf {} +; fi; \
	done
