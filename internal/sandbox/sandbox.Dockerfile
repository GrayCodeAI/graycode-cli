FROM node:22-bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        git \
        jq \
        python3 \
        ripgrep \
        tree && \
    rm -rf /var/lib/apt/lists/*

USER node
WORKDIR /workspace

CMD ["sleep", "infinity"]
