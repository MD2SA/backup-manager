# Backup Manager

A self-hosted service that automates PostgreSQL backups with scheduling, retention policies, cloud storage, restore workflows, and monitoring.

> **Status:** 🚧 Active development

---

## 🚀 Getting Started

You can run Backup Manager either as a pre-built Docker image or directly from the source code.

### Option A: Docker Image (Recommended)

This is the easiest way to get started. No need to install Go or build dependencies.

1.  **Create a `docker-compose.yml`**:
    ```yaml
    services:
      backup-manager:
        image: md2sa/backup-manager:latest
        ports:
          - "8080:8080"
        environment:
          - APP_BACKUP_PATH=./backups
          - APP_METADATA_DB_URL=postgres://...
          - APP_TARGET_DB_HOST=...
          - APP_ADMIN_KEY=your-secure-api-key # Optional but recommended
          # - APP_ENCRYPTION_PASSPHRASE=... (Optional for simple encryption)
        volumes:
          - ./backups:/backups
    ```
2.  **Start the service**:
    ```bash
    docker-compose up -d
    ```

### Option B: From Source

Ideal for development or custom deployments.

1.  **Clone and install tools**:
    ```bash
    git clone https://github.com/MD2SA/backup-manager.git
    cd backup-manager
    make tools
    ```
2.  **Configure and Run**:
    ```bash
    cp .env.example .env
    # Edit .env with your database details
    make migrate
    make dev
    ```

---

## 🔐 Security

### API Authentication
To protect your API, set the `APP_ADMIN_KEY` environment variable. Once set, all requests to the `/api/v1` endpoints must include the `X-API-Key` header:

```bash
curl -H "X-API-Key: your-secure-api-key" http://localhost:8080/api/v1/profiles
```

If no key is set, the application will run in **Insecure Mode** and display a warning on startup.

### Backup Encryption
Backup Manager features professional, end-to-end encryption using the **Age** standard. You can choose between two modes:

### 1. Simple Mode (Passphrase)
Just set the `APP_ENCRYPTION_PASSPHRASE` environment variable. This is the easiest way to secure your backups.

### 2. Pro Mode (X25519 Keypair)
For maximum security, you can use a public/private key pair. Generate them using the built-in utility:

**For Docker users**:
```bash
docker run --rm md2sa/backup-manager:latest keygen
```

**For Source users**:
```bash
go run ./cmd/api keygen
```

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

* **Auto-Adaptive Identity:** The Docker image automatically detects the owner of the mounted `/backups` volume and runs with those permissions. This ensures created backups are owned by your host user without manual configuration. You can still override this using `PUID` and `PGID` environment variables.
* **Database Compatibility:** The image includes `postgresql16-client`. This is compatible with PostgreSQL 13 through 17.
* **Automatic Migrations:** The container automatically runs database migrations on the metadata database during startup. If migrations fail, the container will exit with an error.

---

## Contributing

This project is currently under active development and is not accepting external contributions yet.

Contribution guidelines will be added once the project reaches a more stable stage.

---

## License

This project is currently under development. License information will be added before the first stable release.
