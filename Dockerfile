# Build stage
# Supply-chain hardening: both stages are pinned by digest so a mutable tag
# cannot silently change the build.
FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

RUN apk upgrade --no-cache && \
    apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# GrayCodeAI sibling modules are unpublished at their current code (the public proxy
# froze v0.1.0 at old commits). Resolve them locally via a generated go.work
# (use . + replace => ./external/<repo>), bypassing the proxy/sumdb entirely.
ENV GOPRIVATE=github.com/GrayCodeAI/* \
    GONOSUMDB=github.com/GrayCodeAI/* \
    GONOSUMCHECK=1

# Build-time provenance (passed by .github/workflows/docker.yml or `docker build
# --build-arg VERSION=... --build-arg COMMIT=... --build-arg BUILD_DATE=...`).
# Default to "dev"/"none"/"unknown" so plain `docker build .` still produces a
# runnable image — matching the cmd/hawk/main.go ldflags fallbacks.
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

COPY . .

# external/<repo> are committed submodules pinned to the integrated revisions
# (populated by `submodules: recursive` in .github/workflows/docker.yml, or
# `git submodule update --init --recursive` for a local `docker build`). Build
# against those pinned checkouts via a generated go.work — the committed
# go.work/go.work.sum are excluded by .dockerignore, and the public proxy froze
# v0.1.0 at older commits. Do NOT run 'go mod download' first.
#
# main.Version / main.Commit / main.BuildDate are baked in from the ARGs above;
# this is the only correct source — `git describe` would always return empty
# because `.dockerignore` excludes `.git/` from the build context.
#
# The cache mounts persist the Go module and compile caches between CI runs
# (exported via cache-to: type=gha,mode=max), so the per-commit ldflags change
# only re-links the binary instead of cold-compiling the whole dependency tree.
# Requires BuildKit (default for Docker Desktop 23+ and every CI builder).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    rm -f go.work go.work.sum && \
    { echo "go 1.26.6"; echo; echo "use ."; echo; echo "replace ("; \
      for repo in hawk-core-contracts eyrie inspect sight tok trace yaad; do \
        echo "	github.com/GrayCodeAI/${repo} => ./external/${repo}"; \
      done; echo ")"; } > go.work && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o hawk ./cmd/hawk

# Runtime stage — Alpine (hawk requires git + bash for workspace operations; distroless excluded)
FROM alpine:3.23.5@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates git bash tini && \
    adduser -D -u 1000 -h /home/hawk hawk

COPY --from=builder /build/hawk /usr/local/bin/hawk
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

USER hawk
WORKDIR /workspace

ENTRYPOINT ["tini", "--", "hawk"]
CMD ["--help"]
