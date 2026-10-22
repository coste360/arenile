# syntax=docker/dockerfile:1
#
# Arenile — immagine di produzione multi-stage e multi-architettura (spec 24.2).
#
#   docker build -f deploy/Dockerfile --build-arg VERSION=v0.1.0 -t ghcr.io/coste360/arenile:dev .
#   docker buildx build -f deploy/Dockerfile --platform linux/amd64,linux/arm64 .
#
# The web and build stages always run on the build platform ($BUILDPLATFORM)
# and Go cross-compiles for $TARGETOS/$TARGETARCH, so arm64 images (Raspberry Pi)
# are built without QEMU emulation of the toolchains.

ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

# ---------------------------------------------------------------------------
# 1. Frontend: admin SPA, booking SPA and embed script
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web
WORKDIR /src
ENV CI=true \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_AUDIT=false
# Manifests first so that "npm ci" is cached until dependencies change.
COPY web/package.json web/package-lock.json ./web/
COPY web/admin/package.json ./web/admin/
COPY web/booking/package.json ./web/booking/
COPY web/shared/package.json ./web/shared/
RUN --mount=type=cache,target=/root/.npm \
    npm --prefix web ci
COPY web ./web
COPY api ./api
# Produces web/admin/dist and web/booking/dist (including embed.js).
RUN npm --prefix web run build

# ---------------------------------------------------------------------------
# 2. Backend: static Go binary with the SPAs embedded
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# Replace whatever is in the static dirs (only .gitkeep, see .dockerignore)
# with the fresh frontend build.
COPY --from=web /src/web/admin/dist   ./internal/http/static/admin
COPY --from=web /src/web/booking/dist ./internal/http/static/booking
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/arenile ./cmd/arenile
# Data directory owned by the distroless "nonroot" user (uid/gid 65532), so the
# /data volume is writable without running as root.
RUN mkdir -p /out/data && chown 65532:65532 /out/data && chmod 0750 /out/data

# ---------------------------------------------------------------------------
# 3. Runtime: distroless, no shell, no package manager, non-root
# ---------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE}
ARG VERSION=dev
LABEL org.opencontainers.image.title="Arenile" \
      org.opencontainers.image.description="Gestionale gratuito per stabilimenti balneari (codice sorgente disponibile, BSL 1.1): prenotazioni online senza commissioni, mappa ombrelloni, cassa." \
      org.opencontainers.image.vendor="Coste360 SRL" \
      org.opencontainers.image.licenses="BUSL-1.1" \
      org.opencontainers.image.source="https://github.com/coste360/arenile" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=build /out/arenile /arenile
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME ["/data"]
ENV ARENILE_DATA_DIR=/data \
    ARENILE_ADDR=:8080
EXPOSE 8080
# Health is checked by the orchestrator via GET /healthz: the distroless image
# has no curl/wget.
HEALTHCHECK NONE
USER nonroot:nonroot
ENTRYPOINT ["/arenile", "serve"]
