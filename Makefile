APP_NAME=backup-manager

# Enable Docker BuildKit for faster, professional builds
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Load environment variables from .env file (Docker Compose / host settings).
# They are Make variables only; recipes opt in explicitly, so `make run` does
# not put database passwords into the application's environment.
ifneq (,$(wildcard ./.env))
    include .env
endif

# Tools
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

SWAG ?= $(LOCALBIN)/swag
SQLC ?= $(LOCALBIN)/sqlc
GOOSE ?= $(LOCALBIN)/goose
AIR ?= $(LOCALBIN)/air
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make init       - Generate config.yaml and secrets.env"
	@echo "  make init-docker - Generate config for docker-compose.yml"
	@echo "  make init-dev   - Generate config for the dev stack (target-db)"
	@echo "  make config-check - Validate the configuration (no DB connection)"
	@echo "  make docker-config-check - Validate inside the Compose stack"
	@echo "  make run        - Run the API server"
	@echo "  make dev        - Run the API server with live reload (air)"
	@echo "  make build      - Build the application"
	@echo "  make test       - Run fast tests"
	@echo "  make test-integration - Run PostgreSQL integration tests (BACKUP_MANAGER_TEST_DSN)"
	@echo "  make recovery-drill - Run the container disaster-recovery drill"
	@echo "  make fmt        - Format Go code"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make swagger    - Generate Swagger documentation"
	@echo "  make sqlc       - Generate SQLC code"
	@echo "  make migrate    - Apply database migrations"
	@echo "  make docker-dev-up   - Start full dev stack (App + Metadata DB + Target DB)"
	@echo "  make docker-dev-down - Stop and remove dev stack"
	@echo "  make tools      - Install all required tools locally"
	@echo "  make clean      - Remove build artifacts"

init:
	go run ./cmd/api init

init-docker:
	POSTGRES_USER="$(POSTGRES_USER)" POSTGRES_PASSWORD="$(POSTGRES_PASSWORD)" POSTGRES_DB="$(POSTGRES_DB)" \
		go run ./cmd/api init --docker

init-dev:
	POSTGRES_USER="$(POSTGRES_USER)" POSTGRES_PASSWORD="$(POSTGRES_PASSWORD)" POSTGRES_DB="$(POSTGRES_DB)" TARGET_DB_NAME="$(TARGET_DB_NAME)" \
		go run ./cmd/api init --dev

config-check:
	go run ./cmd/api config check

docker-config-check:
	docker compose run --rm backup-manager config check --connect

run:
	go run ./cmd/api serve

dev: $(AIR)
	$(AIR)

build:
	go build -o bin/$(APP_NAME)-api ./cmd/api

test:
	go test ./...

test-integration:
	@test -n "$(BACKUP_MANAGER_TEST_DSN)" || (echo "Set BACKUP_MANAGER_TEST_DSN to a disposable PostgreSQL DSN"; exit 1)
	BACKUP_MANAGER_TEST_DSN="$(BACKUP_MANAGER_TEST_DSN)" go test \
		./internal/backup ./internal/auth ./internal/repository ./internal/database ./internal/recovery \
		-run 'Integration|InstanceLock' -count=1

recovery-drill:
	./scripts/recovery-drill.sh

fmt:
	go fmt ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

swagger: $(SWAG)
	# swag v1.16.6 cannot parse Go 1.27's standard library; use the project's
	# Go 1.26 toolchain to regenerate docs if your local Go is newer.
	GOTOOLCHAIN=go1.26.5 go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/api/main.go -o docs --parseDependency --parseInternal

sqlc: $(SQLC)
	$(SQLC) generate

migrate:
	go run ./cmd/api migrate

docker-dev-up:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build

docker-dev-down:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml down

tools: $(SWAG) $(SQLC) $(GOOSE) $(AIR) $(GOLANGCI_LINT)

$(SWAG): $(LOCALBIN)
	test -s $(LOCALBIN)/swag || GOBIN=$(LOCALBIN) go install github.com/swaggo/swag/cmd/swag@v1.16.6

$(SQLC): $(LOCALBIN)
	test -s $(LOCALBIN)/sqlc || GOBIN=$(LOCALBIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

$(GOOSE): $(LOCALBIN)
	test -s $(LOCALBIN)/goose || GOBIN=$(LOCALBIN) go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

$(AIR): $(LOCALBIN)
	test -s $(LOCALBIN)/air || GOBIN=$(LOCALBIN) go install github.com/air-verse/air@v1.67.3

$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

clean:
	rm -rf bin
