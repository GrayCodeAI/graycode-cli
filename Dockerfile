# Build stage
FROM golang:1.26.3-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Clone eyrie (unpublished dependency with local-only packages)
RUN git clone --depth=1 https://github.com/GrayCodeAI/eyrie.git /eyrie

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Add replace after source copy so it doesn't get overwritten
RUN echo "" >> go.mod && echo "replace github.com/GrayCodeAI/eyrie => /eyrie" >> go.mod && go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=mod \
    -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o hawk ./cmd/hawk

# Runtime stage — minimal image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates git bash tini && \
    adduser -D -u 1000 -h /home/hawk hawk

COPY --from=builder /build/hawk /usr/local/bin/hawk
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

USER hawk
WORKDIR /workspace

ENTRYPOINT ["tini", "--", "hawk"]
CMD ["--help"]
