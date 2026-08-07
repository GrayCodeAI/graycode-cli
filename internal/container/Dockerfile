FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# ───── 1. Base system + existing tools ─────
# gpg and gnupg are needed early for keyring-based repo setup (gh, docker, terraform).
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget jq tree ripgrep fd-find make gcc g++ \
    python3 python3-pip python3-venv \
    nodejs npm \
    ca-certificates openssh-client unzip xz-utils \
    gpg gnupg gettext-base \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/fdfind /usr/bin/fd

# ───── 2. Go ─────
RUN curl -fsSL https://go.dev/dl/go1.26.3.linux-$(dpkg --print-architecture).tar.gz \
    | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/root/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# ───── 3. GitHub CLI (gh) ─────
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# ───── 4. Docker CLI (client only) ─────
RUN install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    | tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update && apt-get install -y docker-ce-cli docker-buildx-plugin docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

# ───── 5. Terraform ─────
RUN wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com noble main" \
    | tee /etc/apt/sources.list.d/hashicorp.list \
    && apt-get update && apt-get install -y terraform \
    && rm -rf /var/lib/apt/lists/*

# ───── 6. kubectl + helm ─────
RUN curl -fsSL "https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/$(dpkg --print-architecture)/kubectl" \
    -o /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl \
    && curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# ───── 7. Language runtimes: Java, Ruby, Rust, .NET ─────
# Java (OpenJDK 21) + Ruby
RUN apt-get update && apt-get install -y --no-install-recommends \
    openjdk-21-jdk ruby-bundler \
    && rm -rf /var/lib/apt/lists/*

# Rust (rustup minimal: rustc, cargo, rust-std, rustfmt, clippy)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable --profile minimal
ENV PATH="/root/.cargo/bin:${PATH}"

# .NET 8 SDK
RUN wget https://dot.net/v1/dotnet-install.sh -O /tmp/dotnet-install.sh \
    && chmod +x /tmp/dotnet-install.sh \
    && /tmp/dotnet-install.sh --channel 8.0 --install-dir /usr/share/dotnet \
    && ln -sf /usr/share/dotnet/dotnet /usr/local/bin/dotnet \
    && rm /tmp/dotnet-install.sh

# ───── 8. Editors, signing, utilities ─────
RUN apt-get update && apt-get install -y --no-install-recommends \
    vim nano \
    gpg pass zip rsync less htop \
    && rm -rf /var/lib/apt/lists/*

# ───── 9. Database clients ─────
# PostgreSQL, MySQL, Redis
RUN apt-get update && apt-get install -y --no-install-recommends \
    postgresql-client mysql-client redis-tools \
    && rm -rf /var/lib/apt/lists/*

# MS SQL Server tools (sqlcmd, bcp)
RUN curl https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor -o /usr/share/keyrings/microsoft-prod.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/microsoft-prod.gpg] https://packages.microsoft.com/ubuntu/24.04/prod noble main" \
    > /etc/apt/sources.list.d/mssql-release.list \
    && apt-get update && ACCEPT_EULA=Y apt-get install -y msodbcsql18 mssql-tools18 unixodbc-dev \
    && rm -rf /var/lib/apt/lists/*
ENV PATH="/opt/mssql-tools18/bin:${PATH}"

# ───── 10. Modern CLI enhancements + shell ─────
# bat (cat with syntax highlighting) — Ubuntu 24.04 has it
RUN apt-get update && apt-get install -y --no-install-recommends bat \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/batcat /usr/bin/bat

# delta (git diff viewer) — install via cargo (Rust already present)
RUN cargo install git-delta

# zoxide (smart cd) — installer puts it in ~/.local/bin
RUN curl -fsSL https://raw.githubusercontent.com/ajeetdsouza/zoxide/main/install.sh | bash
ENV PATH="/root/.local/bin:${PATH}"

# direnv
RUN apt-get update && apt-get install -y --no-install-recommends direnv \
    && rm -rf /var/lib/apt/lists/*

# starship (prompt)
RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y

# zsh
RUN apt-get update && apt-get install -y --no-install-recommends zsh \
    && rm -rf /var/lib/apt/lists/*

# ───── 11. Environment ─────
ENV TERM=xterm-256color
ENV LANG=C.UTF-8
ENV EDITOR=vim
