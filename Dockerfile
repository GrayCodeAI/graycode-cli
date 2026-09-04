# Build stage
# Supply-chain hardening: both stages are pinned by digest so a mutable tag
# cannot silently change the build.
FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS builder

RUN apk upgrade --no-cache && \
    apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# GrayCodeAI engine modules are published and pinned in go.mod at tagged or
# commit-pseudo versions, resolved from the module proxy. The committed
# go.work (which references sibling checkouts ../<repo>) is excluded from the
# build context, so build in module mode (no go.work) against those pins.
# (No GOPRIVATE: proxy-first resolution serves the immutable pins; direct-VCS
# fallback remains available for anything the proxy lacks.)

# Build-time provenance (passed by .github/workflows/docker.yml or `docker build
# --build-arg VERSION=... --build-arg COMMIT=... --build-arg BUILD_DATE=...`).
# Default to "dev"/"none"/"unknown" so plain `docker build .` still produces a
# runnable image — matching the cmd/graycode/main.go ldflags fallbacks.
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

COPY . .

# Build against the engine versions pinned in go.mod via the module proxy. The
# committed go.work/go.work.sum (sibling-checkout based) are excluded by
# .dockerignore and must not be present for a module-mode build.
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
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o graycode ./cmd/graycode

# Runtime stage — Alpine (graycode requires git + bash for workspace operations; distroless excluded)
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates git bash tini && \
    adduser -D -u 1000 -h /home/graycode graycode

COPY --from=builder /build/graycode /usr/local/bin/graycode
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

USER graycode
WORKDIR /workspace

ENTRYPOINT ["tini", "--", "graycode"]
CMD ["--help"]
