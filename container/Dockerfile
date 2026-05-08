FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    wget \
    jq \
    tree \
    ripgrep \
    fd-find \
    make \
    gcc \
    python3 \
    python3-pip \
    nodejs \
    npm \
    ca-certificates \
    openssh-client \
    && rm -rf /var/lib/apt/lists/*

# Create symlinks for common tool names
RUN ln -sf /usr/bin/fdfind /usr/bin/fd

# Set reasonable defaults
ENV TERM=xterm-256color
ENV LANG=C.UTF-8

WORKDIR /workspace
