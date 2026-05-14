NAME := hawk
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.BuildDate=$(DATE)

.PHONY: all build test lint fmt vet security bench clean install release help

all: lint test build ## Default: lint, test, build

build: ## Build binary
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/$(NAME) .

test: ## Run tests with race detector
	go test ./... -race -count=1 -timeout=120s

test-coverage: ## Run tests with coverage report
	go test ./... -race -coverprofile=coverage.out -covermode=atomic -timeout=120s
	go tool cover -func=coverage.out | grep "^total:"

test-10x: ## Run tests 10 times to catch flakes
	go test ./... -race -count=10 -timeout=600s

lint: ## Run linter
	golangci-lint run ./... --timeout=5m

fmt: ## Format code
	gofumpt -w .
	goimports -w .

vet: ## Run go vet
	go vet ./...

security: ## Run security scanner
	govulncheck ./...

bench: ## Run benchmarks
	go test ./... -bench=. -benchmem -count=3 -timeout=300s

clean: ## Clean build artifacts
	rm -rf bin/ coverage.out

install: ## Install locally
	go install -ldflags="$(LDFLAGS)" .

release: ## Create release (requires goreleaser)
	goreleaser release --clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
