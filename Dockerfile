# Build stage
FROM golang:1.26.4-alpine AS builder

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

# Clone every sibling hawk imports into ./external, then generate a go.work that
# resolves them locally. NOTE: the committed go.work/go.work.sum are excluded by
# .dockerignore, so we must (re)create the workspace here. Do NOT run
# 'go mod download' first — the frozen-proxy v0.1.0 fails checksum verification.
#
# main.Version / main.Commit / main.BuildDate are baked in from the ARGs above;
# this is the only correct source — `git describe` would always return empty
# because `.dockerignore` excludes `.git/` from the build context.
RUN rm -rf external go.work go.work.sum && mkdir -p external && \
    for repo in eyrie inspect sight tok trace yaad; do \
      git clone --depth=1 "https://github.com/GrayCodeAI/${repo}.git" "external/${repo}"; \
    done && \
    { echo "go 1.26.4"; echo; echo "use ."; echo; echo "replace ("; \
      for repo in eyrie inspect sight tok trace yaad; do \
        echo "	github.com/GrayCodeAI/${repo} => ./external/${repo}"; \
      done; echo ")"; } > go.work && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o hawk ./cmd/hawk

# Runtime stage — Alpine (hawk requires git + bash for workspace operations; distroless excluded)
FROM alpine:3.21

RUN apk add --no-cache ca-certificates git bash tini && \
    adduser -D -u 1000 -h /home/hawk hawk

COPY --from=builder /build/hawk /usr/local/bin/hawk
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

USER hawk
WORKDIR /workspace

ENTRYPOINT ["tini", "--", "hawk"]
CMD ["--help"]
