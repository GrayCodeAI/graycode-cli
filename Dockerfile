# Build stage
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o hawk .

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
