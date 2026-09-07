FROM node:22-bookworm-slim@sha256:4d676821dff059fd00d277ee4261ef34ea712317fed0737c03941481b5760c96

# Configure package sources explicitly rather than relying on the base image's
# snapshot-pinned debian.sources, which can lag the security suite (libssh2-1
# shipped 1.10.0-3+b1 until DSA-6365-1 published 1.10.0-3+deb12u1). Pinning
# bookworm-security here keeps every build on current security fixes and never
# serves a stale cached apt layer.
RUN printf '%s\n' \
        'Types: deb' \
        'URIs: http://deb.debian.org/debian' \
        'Suites: bookworm bookworm-updates' \
        'Components: main' \
        'Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg' \
        '' \
        'Types: deb' \
        'URIs: http://deb.debian.org/debian-security' \
        'Suites: bookworm-security' \
        'Components: main' \
        'Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg' \
        > /etc/apt/sources.list.d/debian.sources && \
    npm install --global npm@12.0.2 && \
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
