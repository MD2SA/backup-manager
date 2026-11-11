# syntax=docker/dockerfile:1

FROM golang:1.26.5-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

RUN apk add --no-cache git

# Set Go Proxy for faster and more reliable downloads
ENV GOPROXY=https://proxy.golang.org,direct

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/MD2SA/backup-manager/internal/version.Version=${VERSION} \
      -X github.com/MD2SA/backup-manager/internal/version.Commit=${COMMIT} \
      -X github.com/MD2SA/backup-manager/internal/version.Date=${BUILD_DATE}" \
    -o /bin/backup-manager ./cmd/api

FROM alpine:3.21

# OCI Labels
LABEL org.opencontainers.image.title="Backup Manager" \
      org.opencontainers.image.description="Automated PostgreSQL backup service with multi-provider support" \
      org.opencontainers.image.source="https://github.com/MD2SA/backup-manager" \
      org.opencontainers.image.vendor="MD2SA" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache \
    postgresql-client \
    ca-certificates \
    tzdata \
    su-exec \
    shadow && \
    addgroup -S -g 101 appgroup && \
    adduser -S -u 100 -G appgroup appuser && \
    mkdir -p /backups /etc/backup-manager && \
    chown -R appuser:appgroup /backups /etc/backup-manager

ENV APP_CONFIG_PATH=/etc/backup-manager/config.yaml \
    APP_MIGRATIONS_DIR=/app/sql/migrations

WORKDIR /app

COPY --from=builder /bin/backup-manager /usr/local/bin/backup-manager
COPY --chown=appuser:appgroup sql/migrations ./sql/migrations
COPY --chown=appuser:appgroup scripts/entrypoint.sh ./scripts/entrypoint.sh

RUN chmod +x ./scripts/entrypoint.sh

VOLUME ["/backups"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/scripts/entrypoint.sh"]
