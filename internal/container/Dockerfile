FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget jq tree ripgrep fd-find make gcc g++ \
    python3 python3-pip python3-venv \
    nodejs npm \
    ca-certificates openssh-client unzip xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/fdfind /usr/bin/fd

# Install Go
RUN curl -fsSL https://go.dev/dl/go1.26.3.linux-$(dpkg --print-architecture).tar.gz \
    | tar -C /usr/local -xz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/root/go"
ENV PATH="${GOPATH}/bin:${PATH}"
ENV TERM=xterm-256color
ENV LANG=C.UTF-8
