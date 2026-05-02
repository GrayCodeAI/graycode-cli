# Build stage
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=$(git describe --tags --always)" -o hawk .

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates git bash

WORKDIR /app
COPY --from=builder /build/hawk /usr/local/bin/hawk

# Create non-root user
RUN adduser -D -u 1000 hawk
USER hawk

ENTRYPOINT ["hawk"]
CMD ["--help"]
