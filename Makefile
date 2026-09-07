APP_NAME=backup-manager

# Enable Docker BuildKit for faster, professional builds
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Load environment variables from .env file
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Database URL construction for migrations (Internal state)
DB_USER ?= $(APP_METADATA_DB_USER)
DB_PASSWORD ?= $(APP_METADATA_DB_PASSWORD)
DB_HOST ?= $(APP_METADATA_DB_HOST)
DB_PORT ?= $(APP_METADATA_DB_PORT)
DB_NAME ?= $(APP_METADATA_DB_DBNAME)
DB_SSLMODE ?= $(APP_METADATA_DB_SSLMODE)

ifneq ($(DB_USER),)
DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
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
	@echo "  make run        - Run the API server"
	@echo "  make dev        - Run the API server with live reload (air)"
	@echo "  make build      - Build the application"
	@echo "  make test       - Run tests"
	@echo "  make fmt        - Format Go code"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make swagger    - Generate Swagger documentation"
	@echo "  make sqlc       - Generate SQLC code"
	@echo "  make migrate    - Run database migrations"
	@echo "  make docker-dev-up   - Start full dev stack (App + Metadata DB + Target DB)"
	@echo "  make docker-dev-down - Stop and remove dev stack"
	@echo "  make tools      - Install all required tools locally"
	@echo "  make clean      - Remove build artifacts"

run:
	go run ./cmd/api

dev: $(AIR)
	$(AIR)

build:
	go build -o bin/$(APP_NAME)-api ./cmd/api

test:
	go test ./...

fmt:
	go fmt ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

swagger: $(SWAG)
	$(SWAG) init -g cmd/api/main.go -o docs --parseDependency --parseInternal

sqlc: $(SQLC)
	$(SQLC) generate

migrate: $(GOOSE)
	$(GOOSE) -dir sql/migrations postgres "$(DATABASE_URL)" up

docker-dev-up:
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build

docker-dev-down:
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml down

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
