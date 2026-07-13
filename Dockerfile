# Build stage
# TODO(supply-chain): pin base images by digest (tag@sha256:…) — tags are
# mutable and an upstream re-push silently changes the build. Applies to the
# alpine runtime stage below as well.
FROM golang:1.26.5-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

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
RUN rm -f go.work go.work.sum && \
    { echo "go 1.26.5"; echo; echo "use ."; echo; echo "replace ("; \
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
FROM alpine:3.23

RUN apk add --no-cache ca-certificates git bash tini && \
    adduser -D -u 1000 -h /home/hawk hawk

COPY --from=builder /build/hawk /usr/local/bin/hawk
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

USER hawk
WORKDIR /workspace

ENTRYPOINT ["tini", "--", "hawk"]
CMD ["--help"]
