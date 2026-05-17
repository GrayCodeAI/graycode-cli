---
name: docker-deploy
description: Docker build optimization, multi-stage builds, and deployment workflows
version: "1.0.0"
author: graycode
license: MIT
category: ops
tags: ["docker", "deploy", "containers"]
allowed-tools: Read Write Bash
---

# Docker Deploy

## When to Use
- Writing or optimizing Dockerfiles
- Setting up multi-stage builds
- Reducing image size
- Configuring docker-compose for development or production

## Workflow
1. Analyze the project's language and dependencies
2. Choose appropriate base image (alpine when possible)
3. Use multi-stage builds to separate build and runtime
4. Order layers for optimal caching (dependencies before source)
5. Add .dockerignore for build context optimization
6. Set non-root user for security

## Patterns

### Multi-stage Go build
```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app .

FROM alpine:3.19
COPY --from=build /bin/app /bin/app
USER nobody
ENTRYPOINT ["/bin/app"]
```

### Layer caching
```dockerfile
# Dependencies change less often — cache this layer
COPY package.json package-lock.json ./
RUN npm ci --production

# Source changes frequently — this layer rebuilds
COPY . .
RUN npm run build
```

## Verification
- Image size is reasonable (< 100MB for Go, < 200MB for Node)
- No secrets in the image (check with `docker history`)
- Runs as non-root user
- Health check is configured
