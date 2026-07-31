# Backup Manager

A self-hosted service that automates PostgreSQL backups with scheduling, retention policies, cloud storage, restore workflows, and monitoring. It is designed to be deployed as a Docker service alongside existing applications.

> **Status:** 🚧 Active development

---

## Features

### Current

* REST API built with Go and Chi
* PostgreSQL persistence using `pgx` + `sqlc`
* Modular Monolith architecture
* Structured logging using `slog`
* Database migrations using Goose
* OpenAPI/Swagger API documentation
* Request validation
* Dependency injection and application lifecycle management

### Planned

* Automated backup scheduling
* Backup execution pipeline with compression support
* Backup verification
* GFS retention policies
* Local filesystem and cloud storage providers
* Notification providers (Discord, Slack, Email, etc.)
* Backup execution history and monitoring
* Profile management (have pre-configured retention policies, scheduling etc)
* Database restore workflows
* Configuration export/import

---

## Project Structure

```text
.
├── cmd/                   # Entry points (API server, CLI)
├── internal/
│   ├── api/               # HTTP layer (Handlers, Router, Middleware)
│   ├── app/               # Dependency injection & application wiring
│   ├── backup/            # Backup engine (Runner, Pipelines, Stages)
│   ├── repository/        # Database access (PostgreSQL + SQLC)
│   ├── service/           # Business logic orchestration
│   ├── storage/           # Storage providers (Local, S3...)
│   ├── notification/      # Notification providers (Discord...)
│   ├── retention/         # GFS retention engine
│   ├── verification/      # Backup integrity verification
│   └── pkg/               # Shared utilities
├── sql/                   # Migrations and SQLC queries
└── docs/                  # Swagger / OpenAPI documentation
```

---

## Architecture

Backup Manager follows a **Modular Monolith** architecture.

Each package represents a business domain with a single responsibility. Business logic is kept independent from infrastructure, making it straightforward to introduce new storage providers, notification providers, or verification strategies without impacting the rest of the application.

Core domains include:

* **Backup** – Coordinates the backup pipeline
* **Storage** – Uploads and manages backup artifacts
* **Retention** – Applies GFS retention policies
* **Verification** – Validates backup integrity
* **Notifications** – Sends backup events
* **Repository** – Database persistence
* **Service** – Coordinates business operations
* **API** – Exposes REST endpoints

---

## Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| HTTP Router | Chi |
| API Documentation | OpenAPI / Swagger |
| Database | PostgreSQL |
| Database Access | pgx + sqlc |
| Migrations | Goose |
| Logging | slog |
| Configuration | Environment Variables |
| Validation | go-playground/validator |
| CLI | Cobra |
| Hot Reload | Air |
| Linting | golangci-lint |
| Containers | Docker & Docker Compose |

---

## Getting Started

### Requirements

* Go 1.26+
* PostgreSQL
* Docker (optional)

### Clone

```bash
git clone https://github.com/MD2SA/backup-manager.git

cd backup-manager
```

### Install development tools

Backup Manager uses local development tools for code generation, migrations, documentation, hot reload, and linting.

Install them with:

```bash
make tools
```

This installs:

* Air (hot reload)
* SQLC (database code generation)
* Goose (database migrations)
* Swag (OpenAPI documentation)
* golangci-lint (code quality checks)

### Configure environment

Create your local environment file:

```bash
cp .env.example .env
```

Update the required database and application settings.

### Run database migrations

```bash
make migrate
```

### Run the API

For normal execution:

```bash
make run
```

For development with hot reload:

```bash
make dev
```

### Generate code

Generate SQLC code:

```bash
make sqlc
```

Generate Swagger/OpenAPI documentation:

```bash
make swagger
```

### Other commands

```bash
make test      # Run tests
make fmt       # Format Go code
make lint      # Run golangci-lint
make build     # Build API binary
make build-cli # Build CLI binary
make clean     # Remove build artifacts
```

### Health Check

```http
GET /api/v1/health
```

Response:

```text
OK
```

---

## Contributing

This project is currently under active development and is not accepting external contributions yet.

Contribution guidelines will be added once the project reaches a more stable stage.

---

## License

This project is currently under development. License information will be added before the first stable release.
