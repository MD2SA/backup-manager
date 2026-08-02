# Backup Manager

A self-hosted service that automates PostgreSQL backups with scheduling, retention policies, cloud storage, restore workflows, and monitoring. It is designed to be deployed as a Docker service alongside existing applications.

> **Status:** 🚧 Active development

---

## Features

*   **Automated Scheduling**: Cron-based backup triggers per profile.
*   **Multi-Destination Storage**: Simultaneous backup to multiple providers (S3, Local Filesystem, etc.).
*   **Multi-Channel Notifications**: Real-time alerts via Discord, Email, and more.
*   **Flexible Retention Policies**: GFS (Grandfather-Father-Son) retention management to save storage space.
*   **Backup Verification**: Automatic integrity checks using checksums and archive validation.
*   **Restore Workflows**: Integrated database restoration from existing backups.
*   **REST API**: Fully documented API for management and monitoring.
*   **Modular Architecture**: Easily extendable with new storage or notification adapters.

---

## Project Structure

*   [**Architecture Overview**](docs/guides/ARCHITECTURE.md)
*   [**Configuration Guide**](docs/guides/CONFIGURATION.md)
*   [**API Reference**](docs/guides/API.md)

```text
.
├── cmd/                   # Application entry points
├── internal/
│   ├── api/               # REST API layer (Handlers, DTOs)
│   ├── app/               # App initialization and wiring (DI)
│   ├── backup/            # Core Backup Engine (Runner, Pipeline, Stages)
│   ├── repository/        # Data access layer (Postgres + SQLC)
│   ├── service/           # Domain business logic
│   ├── storage/           # Storage provider implementations
│   ├── notification/      # Notification provider implementations
│   ├── retention/         # Retention policy engine
│   └── verification/      # Integrity verification logic
├── sql/                   # SQL migrations and queries
└── docs/                  # Documentation and OpenAPI specs
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

## Production Deployment

### Running with Docker

Backup Manager is designed to be deployed as a containerized microservice. The provided `Dockerfile` uses a multi-stage build to keep the image small and secure (running as a non-root user).

1. **Build the image:**
   ```bash
   docker build -t backup-manager:latest .
   ```

2. **Configure environment:**
   Ensure your `.env` file has the correct `APP_METADATA_DB_*` (for internal state) and `APP_TARGET_DB_*` (the database to backup) settings.

3. **Deploy with Docker Compose:**
   For production, the base configuration starts only the manager and its metadata database:
   ```bash
   docker-compose up -d
   ```

### Local Development & Testing

To test the full pipeline locally with a sample target database, use the provided development stack:

```bash
make docker-dev-up
```

This will start:
1. **Backup Manager:** The microservice.
2. **Metadata DB:** To store internal state.
3. **Target DB:** A sample database to be backed up.

To stop the development stack:
```bash
make docker-dev-down
```

### Important Deployment Notes

* **Security:** The application runs as `appuser` (UID 100). Ensure any mounted volumes have correct permissions if using local storage.
* **Database Compatibility:** The image includes `postgresql16-client`. This is compatible with PostgreSQL 13 through 17.
* **Automatic Migrations:** The container automatically runs database migrations on the metadata database during startup. If migrations fail, the container will exit with an error.

---

## Contributing

This project is currently under active development and is not accepting external contributions yet.

Contribution guidelines will be added once the project reaches a more stable stage.

---

## License

This project is currently under development. License information will be added before the first stable release.
