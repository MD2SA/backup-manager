# syntax=docker/dockerfile:1

# Stage 1: Build the Go application and tools
FROM golang:1.26.5-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Set Go Proxy for faster and more reliable downloads
ENV GOPROXY=https://proxy.golang.org,direct

# Copy dependency files
COPY go.mod go.sum ./

# Download dependencies with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source code
COPY . .

# Build the application with static linking and build cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /bin/backup-manager ./cmd/api

# Install Goose for migrations
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

# Stage 2: Production image
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache \
    postgresql16-client \
    ca-certificates \
    tzdata

# Create a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy the binary and goose from the builder stage
COPY --from=builder /bin/backup-manager /usr/local/bin/backup-manager
COPY --from=builder /go/bin/goose /usr/local/bin/goose

# Copy migrations and entrypoint script
COPY sql/migrations ./sql/migrations
COPY scripts/entrypoint.sh ./scripts/entrypoint.sh

# Ensure the entrypoint script is executable and owned by the non-root user
RUN chmod +x ./scripts/entrypoint.sh && \
    chown -R appuser:appgroup /app

# Switch to the non-root user
USER appuser

# Expose the API port
EXPOSE 8080

# Use the entrypoint script to handle migrations and startup
ENTRYPOINT ["./scripts/entrypoint.sh"]
