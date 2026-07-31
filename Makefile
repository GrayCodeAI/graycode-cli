# Canonical GrayCodeAI Makefile for Go binary repos.
# Source of truth: .shared-templates/Makefile.binary.tmpl at the eco root.
# Placeholders rendered per repo: hawk, ..

# ---------------------------------------------------------------------------
# Project metadata
# ---------------------------------------------------------------------------
NAME      := hawk
MAIN_PKG  := ./cmd/hawk

# ---------------------------------------------------------------------------
# Versioning — sourced from VERSION file; falls back to git describe.
# See https://github.com/GrayCodeAI/hawk/blob/main/docs/versioning.md.
# ---------------------------------------------------------------------------
VERSION ?= $(shell v=$$(cat VERSION 2>/dev/null | head -n1 | tr -d '[:space:]'); if [ -n "$$v" ]; then echo "$$v"; else git describe --tags --always --dirty 2>/dev/null || echo "dev"; fi)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildDate=$(DATE)

# ---------------------------------------------------------------------------
# Tooling — pinned, install if missing.
# ---------------------------------------------------------------------------
GOBIN_DIR    := $(shell go env GOPATH)/bin
GOLANGCI     := $(GOBIN_DIR)/golangci-lint
GOFUMPT      := $(GOBIN_DIR)/gofumpt
GOIMPORTS    := $(GOBIN_DIR)/goimports
GOVULNCHECK  := $(GOBIN_DIR)/govulncheck
GORELEASER   := $(GOBIN_DIR)/goreleaser

# ---------------------------------------------------------------------------
# Phony declarations (alphabetical).
# ---------------------------------------------------------------------------
.PHONY: all bench boundaries build check-replace ci clean contracts-guard ecosystem-guard eyrie-client-guard eyrie-engine-guard peer-guard internal-layers-guard submodule-release-parity cover cover-new fmt help install lint lint-fix \
        release security setup smoke path sync-external test test-10x test-live test-new test-race tidy version vet

check-replace: ## Fail if go.mod has local replace directives (run before tagging)
	@bash scripts/check-no-replace-directives.sh

all: lint test build ## Default — lint, test, build.

# ---------------------------------------------------------------------------
# Build / install / release.
# ---------------------------------------------------------------------------
build: ## Build the binary into bin/$(NAME).
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME) $(MAIN_PKG)

install: ## Install the binary to $GOBIN.
	CGO_ENABLED=0 go install -trimpath -ldflags="$(LDFLAGS)" $(MAIN_PKG)

release: ## Cut a release via goreleaser (requires a clean tree + tag).
	@command -v $(GORELEASER) >/dev/null 2>&1 || (echo "install: go install github.com/goreleaser/goreleaser/v2@latest" && exit 1)
	$(GORELEASER) release --clean

# ---------------------------------------------------------------------------
# Tests.
# ---------------------------------------------------------------------------
test: ## Run unit tests.
	go test ./... -count=1 -timeout=120s

test-race: ## Run unit tests with the race detector.
	go test ./... -race -count=1 -timeout=300s

test-10x: ## Run tests 10 times to surface flakes.
	go test ./... -race -count=10 -timeout=600s

test-new: ## Run only the Round 2 ecosystem packages (fast iteration).
	go test -race -count=1 -timeout=60s ./internal/safewrite/... ./internal/jsonc/... ./internal/providers/... ./internal/session/... ./internal/permissions/...

test-live: ## Run opt-in live integration tests (requires real LLM credentials).
	@echo "Running live integration tests — requires OPENCODEGO_API_KEY"
	OPENCODEGO_API_KEY=$${OPENCODEGO_API_KEY:-$$(grep -v '^#' .envrc 2>/dev/null | grep OPENCODEGO_API_KEY | head -1 | cut -d= -f2-)} go test -tags=live_test -count=1 -timeout=300s ./cmd/...

cover: ## Generate a coverage report (coverage.out + coverage.html).
	go test ./... -race -coverprofile=coverage.out -covermode=atomic -timeout=180s
	@go tool cover -func=coverage.out | grep "^total:"
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

cover-new: ## Coverage report for Round 2 ecosystem packages only.
	go test -cover -timeout=30s ./internal/safewrite/... ./internal/jsonc/... ./internal/providers/... ./internal/session/... ./internal/permissions/...

bench: ## Run benchmarks.
	go test ./... -bench=. -benchmem -count=3 -timeout=300s

# ---------------------------------------------------------------------------
# Quality gates.
# ---------------------------------------------------------------------------
fmt: ## Format source files (gofumpt + goimports).
	@command -v $(GOFUMPT)   >/dev/null 2>&1 || (echo "install: go install mvdan.cc/gofumpt@latest"   && exit 1)
	@command -v $(GOIMPORTS) >/dev/null 2>&1 || (echo "install: go install golang.org/x/tools/cmd/goimports@latest" && exit 1)
	$(GOFUMPT) -w .
	$(GOIMPORTS) -w .

vet: ## Run go vet.
	go vet ./...

contracts-guard: ## Fail on any legacy imports of removed hawk/shared/types.
	bash ./scripts/check-shared-types-imports.sh

ecosystem-guard: ## Fail if external ecosystem repos import hawk/internal or removed hawk/shared/types.
	bash ./scripts/check-ecosystem-boundaries.sh

eyrie-client-guard: ## Fail on any production eyrie/client import.
	bash ./scripts/check-eyrie-client-imports.sh

eyrie-engine-guard: ## Require all production Eyrie imports to use the stable engine facade.
	bash ./scripts/check-eyrie-engine-boundary.sh

peer-guard: ## Fail if support engines import each other instead of depending only on Hawk contracts.
	bash ./scripts/check-support-repo-coupling.sh

internal-layers-guard: ## Enforce one-way dependencies across stable Hawk internal layers.
	bash ./scripts/check-internal-layer-imports.sh

boundaries: contracts-guard ecosystem-guard eyrie-client-guard eyrie-engine-guard peer-guard internal-layers-guard ## Alias for all boundary guards (matches `make boundaries` in engine repos).

submodule-release-parity: ## Verify every go.mod ecosystem version resolves to its pinned Gitlink.
	bash ./scripts/check-submodule-release-parity.sh

lint: ## Run golangci-lint.
	@command -v $(GOLANGCI) >/dev/null 2>&1 || (echo "install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest" && exit 1)
	$(GOLANGCI) run ./... --timeout=5m

lint-fix: ## Run golangci-lint with --fix.
	@command -v $(GOLANGCI) >/dev/null 2>&1 || (echo "install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest" && exit 1)
	$(GOLANGCI) run ./... --fix --timeout=5m

security: ## Run govulncheck.
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || (echo "install: go install golang.org/x/vuln/cmd/govulncheck@latest" && exit 1)
	$(GOVULNCHECK) ./...

tidy: ## Sync workspace modules and verify checksums.
	go work sync
	go mod verify

# ---------------------------------------------------------------------------
# Composite gate used by CI and pre-push.
# ---------------------------------------------------------------------------
ci: tidy fmt vet boundaries lint test-race security ## Run everything CI runs.
	@echo "All CI checks passed."

smoke: ## Quick build + doctor + ecosystem verification.
	./scripts/smoke-hawk.sh

path: ## Verify developer path (setup, security, milestone tests).
	./scripts/verify-developer-path.sh

# ---------------------------------------------------------------------------
# Misc.
# ---------------------------------------------------------------------------
version: ## Print the version that will be embedded.
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(DATE)"

clean: ## Remove build artefacts.
	rm -rf bin/ dist/ coverage.out coverage.html
	go clean -testcache

# ---------------------------------------------------------------------------
# Setup — bootstrap local development environment.
# ---------------------------------------------------------------------------
FOUNDATION_REPOS := hawk-core-contracts
ECO_REPOS := eyrie inspect sight tok trace yaad
WORKSPACE_REPOS := $(FOUNDATION_REPOS) $(ECO_REPOS)

setup: ## Set up local development environment (go.work + external repos).
	@echo "=== Setting up hawk development environment ==="
	@mkdir -p external
	@for repo in $(WORKSPACE_REPOS); do \
		dest="external/$$repo"; \
		if [ -d "$$dest/.git" ]; then \
			echo "✓ $$dest already exists"; \
		else \
			commit=$$(git ls-tree HEAD "$$dest" 2>/dev/null | awk '{print $$3}' || true); \
			if [ -n "$$commit" ]; then \
				echo "Cloning $$repo at pinned commit $$commit..."; \
				git clone "https://github.com/GrayCodeAI/$$repo.git" "$$dest" 2>/dev/null && \
				(cd "$$dest" && git checkout --quiet "$$commit") || \
				echo "  ⚠ Could not clone $$repo at pinned commit $$commit"; \
			else \
				ref=$$(git branch --show-current 2>/dev/null || echo main); \
				if ! git ls-remote --heads "https://github.com/GrayCodeAI/$$repo.git" "$$ref" | grep -q .; then \
					ref=main; \
				fi; \
				echo "Cloning $$repo at branch $$ref..."; \
				git clone --depth=1 --branch "$$ref" "https://github.com/GrayCodeAI/$$repo.git" "$$dest" 2>/dev/null || \
				echo "  ⚠ Could not clone $$repo (may not exist yet or no access)"; \
			fi; \
		fi; \
	done
	@echo "Generating go.work..."
	@echo "go 1.26.5" > go.work
	@echo "" >> go.work
	@echo "use ." >> go.work
	@echo "" >> go.work
	@echo "replace (" >> go.work
	@for repo in $(WORKSPACE_REPOS); do \
		if [ -d "external/$$repo" ]; then \
			echo "	github.com/GrayCodeAI/$$repo => ./external/$$repo" >> go.work; \
		fi; \
	done
	@echo ")" >> go.work
	@go work sync
	@echo "✓ go.work generated and synced"
	@echo ""
	@echo "=== Environment check ==="
	@echo "Go version: $$(go version)"
	@echo "GOPATH:     $$(go env GOPATH)"
	@echo "GOBIN:      $$(go env GOPATH)/bin"
	@echo ""
	@echo "=== Installing development tools ==="
	@command -v $(GOFUMPT)   >/dev/null 2>&1 || go install mvdan.cc/gofumpt@latest || echo "  ⚠ Could not install gofumpt"
	@command -v $(GOIMPORTS) >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest || echo "  ⚠ Could not install goimports"
	@command -v $(GOLANGCI)  >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest || echo "  ⚠ Could not install golangci-lint"
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest || echo "  ⚠ Could not install govulncheck"
	@command -v lefthook >/dev/null 2>&1 || go install github.com/evilmartians/lefthook@latest || echo "  ⚠ Could not install lefthook"
	@echo "✓ All tools installed"
	@echo ""
	@echo "=== Installing git hooks ==="
	@lefthook install || echo "  ⚠ lefthook install failed (run 'make hooks' manually)"
	@echo ""
	@echo "=== Setup complete! ==="
	@echo "Run 'make ci' to verify everything works."

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Compatibility matrix (hawk-specific extension to the canonical template).
# Validates compatibility-matrix.json and reports the resolved versions for
# a chosen matrix entry. Wired into the compatibility-test workflow.
# ---------------------------------------------------------------------------
.PHONY: compat-test compat-check compat-drift

compat-test: ## Validate testdata/compatibility-matrix.json and report the 'next' matrix.
	@go run ./cmd/compat-test -matrix=next -file=testdata/compatibility-matrix.json

compat-check: ## Strict validation — non-zero exit if any component lacks a version.
	@go run ./cmd/compat-test -matrix=next -strict -file=testdata/compatibility-matrix.json

compat-drift: ## Advisory: report pin drift between hawk's go.mod and external/ submodules. Never fails.
	@go run ./cmd/compat-test -check-external -file=testdata/compatibility-matrix.json

.PHONY: hooks sync-submodules sync-submodule-versions sync-clone
hooks: ## Install git hooks via lefthook (formatting, linting, conventional commits).
	@command -v lefthook >/dev/null 2>&1 || (echo "install: go install github.com/evilmartians/lefthook@latest" && exit 1)
	lefthook install

sync-submodules: ## Fetch and checkout latest origin/main for all external/ submodules.
	git submodule foreach 'git fetch origin && git checkout origin/main 2>/dev/null || git checkout origin/HEAD'
	@echo "Submodule heads:"
	@git submodule status

sync-submodule-versions: ## After advancing submodules, bump go.mod requires to match each gitlink.
	@chmod +x scripts/sync-submodule-versions.sh
	@./scripts/sync-submodule-versions.sh

sync-external: ## Read-only drift report: external/<repo> pin vs sibling ../<repo> HEAD.
	@bash ./scripts/sync-external.sh

sync-clone: ## Hard-reset hawk and submodules to origin/main (post history rewrite).
	@chmod +x scripts/sync-clone.sh scripts/commit-clean.sh
	@./scripts/sync-clone.sh
# === Cross-platform binary targets (add after existing 'build' target) ===

.PHONY: build-all build-static size-check

build-all: ## Build for all platforms (darwin/linux/windows × amd64/arm64)
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-darwin-arm64 $(MAIN_PKG)
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-linux-amd64 $(MAIN_PKG)
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-linux-arm64 $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-windows-amd64.exe $(MAIN_PKG)

build-static: ## Build fully static binaries for Linux (musl-compatible)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-linux-amd64-static $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(NAME)-linux-arm64-static $(MAIN_PKG)

size-check: build ## Report binary size and warn if over threshold (80MB, matching CI).
	@SIZE=$$(stat -f%z bin/$(NAME) 2>/dev/null || stat -c%s bin/$(NAME) 2>/dev/null); \
	MB=$$(echo "scale=1; $$SIZE / 1048576" | bc); \
	echo "Binary size: $${MB} MB"; \
	if [ $$SIZE -gt 83886080 ]; then echo "::warning::Binary size $${MB} MB exceeds 80 MB threshold (CI gate)"; fi
