FROM golang:1.26.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/backup-manager ./cmd/api

RUN go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

FROM alpine:3.20

RUN apk add --no-cache \
    postgresql16-client \
    ca-certificates \
    tzdata

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /bin/backup-manager /usr/local/bin/backup-manager
COPY --from=builder /go/bin/goose /usr/local/bin/goose

COPY sql/migrations ./sql/migrations
COPY scripts/entrypoint.sh ./scripts/entrypoint.sh

RUN chmod +x ./scripts/entrypoint.sh && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["./scripts/entrypoint.sh"]
