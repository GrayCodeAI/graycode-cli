FROM node:22-bookworm-slim@sha256:4d676821dff059fd00d277ee4261ef34ea712317fed0737c03941481b5760c96

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
