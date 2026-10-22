# Development container for Arenile (spec 27.10): Go, Node, jq, golangci-lint
# and the system libraries Playwright's Chromium needs.
FROM mcr.microsoft.com/devcontainers/base:bookworm

ARG GO_VERSION=1.27.2
ARG GOLANGCI_LINT_VERSION=v2.15.0
ARG TARGETARCH

# jq, sqlite3 CLI, postgres client and Chromium runtime libraries for Playwright.
RUN apt-get update \
    && export DEBIAN_FRONTEND=noninteractive \
    && apt-get install -y --no-install-recommends \
        jq make sqlite3 postgresql-client ca-certificates curl \
        libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libdbus-1-3 \
        libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 \
        libpango-1.0-0 libcairo2 libasound2 libatspi2.0-0 libx11-xcb1 libxshmfence1 \
        fonts-liberation fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

# Go toolchain (exact version from go.mod).
RUN arch="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -C /usr/local -xz
ENV GOROOT=/usr/local/go \
    GOPATH=/go \
    PATH=/usr/local/go/bin:/go/bin:${PATH}
RUN mkdir -p /go && chown -R vscode:vscode /go

# golangci-lint built with the same Go toolchain as the project, so it can lint
# a module declaring go ${GO_VERSION}. `make setup` also installs it in ./bin.
USER vscode
RUN GOBIN=/go/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && go install github.com/air-verse/air@v1.67.4 \
    && go clean -cache
USER root
