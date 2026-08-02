# syntax=docker/dockerfile:1

FROM golang:1.26.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Set Go Proxy for faster and more reliable downloads
ENV GOPROXY=https://proxy.golang.org,direct

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
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
    mkdir -p /backups && \
    chown -R appuser:appgroup /backups

ENV APP_PORT=8080 \
    APP_LOG_LEVEL=info \
    APP_TEMP_DIR=/tmp \
    APP_STORAGE_PATH=/backups

WORKDIR /app

COPY --from=builder /bin/backup-manager /usr/local/bin/backup-manager
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --chown=appuser:appgroup sql/migrations ./sql/migrations
COPY --chown=appuser:appgroup scripts/entrypoint.sh ./scripts/entrypoint.sh

RUN chmod +x ./scripts/entrypoint.sh

VOLUME ["/backups"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${APP_PORT}/api/v1/health || exit 1

ENTRYPOINT ["./scripts/entrypoint.sh"]
