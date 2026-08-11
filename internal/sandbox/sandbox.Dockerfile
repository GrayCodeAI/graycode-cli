FROM node:22-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436

RUN npm install --global npm@12.0.2 && \
    npm cache clean --force && \
    rm -rf /root/.npm && \
    apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
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
