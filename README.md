# Backup Manager

A self-hosted service that automates PostgreSQL backups with scheduling, retention policies, cloud storage, restore workflows, and monitoring.

> **Status:** active development. The API is stable enough for self-hosted production use; see the production checklist in the configuration guide.

---

## Features

- **Scheduled PostgreSQL backups** via cron, plus manual triggers through the API.
- **Compression and verification**: plain SQL or gzip, archive validation and SHA-256 checksums.
- **End-to-end encryption** with [Age](https://age-encryption.org/): X25519 keypairs or passphrases.
- **Multiple storage destinations** per profile: local filesystem and S3-compatible object storage.
- **GFS retention** (hourly, daily, weekly, monthly, yearly) with pinned backups.
- **Restore workflows** with automatic decryption.
- **Notifications** via Discord webhooks.
- **Metadata disaster recovery**: encrypted snapshots of the manager's own state embedded in every backup, plus an optional local self-backup and a `recover metadata` command with local/S3 discovery.
- **Production hardening**: API key auth, per-IP rate limiting, CORS, trusted proxies, request IDs, encrypted provider credentials at rest.
- **Declarative configuration**: one `config.yaml` for non-secret settings and one `secrets.env` for secrets, with strict validation.
- **Keyset pagination**: every list endpoint supports `limit` and an opaque `cursor`, with `has_more`, `next_cursor` and a `Link` header.
- **Operational visibility**: per-profile overdue detection, authenticated Prometheus metrics, build version endpoint and configurable audit-log retention.
- **Verification and recovery CLI**: `backup-manager verify` runs restore drills, while `backup-manager recover metadata|data` rebuilds the manager state and restores data after host loss.

---

## Quick start (Docker Compose)

Requirements: Docker with Compose.

```bash
git clone https://github.com/MD2SA/backup-manager.git
cd backup-manager

cp .env.example .env
# Edit .env: set POSTGRES_PASSWORD (and APP_BACKUP_PATH if needed).

# Build the image and generate config.yaml + secrets.env.
docker build -t backup-manager:latest .
docker run --rm -it -u "$(id -u):$(id -g)" \
  -v "$PWD:/work" -w /work \
  backup-manager:latest init --docker

# The wizard asks for the metadata DB password. Use the same value as
# POSTGRES_PASSWORD in .env. It writes config.yaml and secrets.env (0600).

docker compose up -d
```

Check it:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
docker compose run --rm backup-manager config check --connect
```

Then create a retention policy, a storage provider, and a profile through the API. The generated admin key is printed by `init` and stored as `ADMIN_KEY` in `secrets.env`.

> The bundled `metadata-db` uses `POSTGRES_*` from `.env`. Those values must match `metadata_database` in `config.yaml` and `METADATA_DB_PASSWORD` in `secrets.env`. `init --docker` uses `metadata-db` as the database host for you.

---

## Quick start (from source)

Requirements: Go 1.26+, PostgreSQL 13–17, `pg_dump`/`psql`/`pg_restore` in `PATH`.

```bash
git clone https://github.com/MD2SA/backup-manager.git
cd backup-manager

# Generate config.yaml + secrets.env. For a local PostgreSQL use localhost and
# a relative storage path, for example:
go run ./cmd/api init --metadata-host localhost --storage-path ./backups

# Validate before starting (add --connect to test database connectivity):
go run ./cmd/api config check --connect

# Apply migrations and start:
go run ./cmd/api serve
```

The server applies pending migrations automatically on startup. To run them
separately, use `go run ./cmd/api migrate`.

Alternatively, use the Makefile:

```bash
make init           # interactive setup
make config-check   # validate config.yaml + secrets.env
make run            # serve (migrations run automatically)
make dev            # live reload with Air
```

---

## Configuration

Backup Manager uses two files:

| File | Purpose | Permissions |
|---|---|---|
| `config.yaml` | All non-secret settings: server, storage, databases, retention defaults | `0644` |
| `secrets.env` | Passwords, API keys and encryption keys | `0600` (never commit) |

`config.yaml` references secret names under `secrets:`; the values are read from
`secrets.env`. The application never loads `.env` and never reads configuration
from process environment variables (except the optional `APP_CONFIG_PATH` and
`APP_SECRETS_FILE` path overrides).

Generate a working pair with:

```bash
backup-manager init
```

Validate them with:

```bash
backup-manager config check --connect
```

See [**Configuration Guide**](docs/guides/CONFIGURATION.md) for the complete
reference, Docker secrets examples, production checklist, and migration from the
old environment-variable setup.

A commented template is available at [`config.example.yaml`](config.example.yaml)
and [`secrets.example.env`](secrets.example.env).

---

## Docker

The image is available at `ghcr.io/md2sa/backup-manager` once published by CI.
The production Compose stack starts:

1. `metadata-db` — internal PostgreSQL for the manager's own state (no host port).
2. `backup-manager` — the API, with `config.yaml` and `secrets.env` mounted read-only.

```yaml
services:
  backup-manager:
    image: ghcr.io/md2sa/backup-manager:latest
    ports:
      - "8080:8080"
    environment:
      APP_CONFIG_PATH: /etc/backup-manager/config.yaml
    volumes:
      - ./config.yaml:/etc/backup-manager/config.yaml:ro
      - ./secrets.env:/etc/backup-manager/secrets.env:ro
      - ./backups:/backups
```

The container healthcheck probes `/healthz` (public, no API key). Readiness at
`/readyz` checks the metadata database.

For local development with a target database:

```bash
make docker-dev-up
```

This uses `docker-compose.yml` + `docker-compose.dev.yml`. Generate the config
with the `--dev` preset so it points at `metadata-db` and `target-db`:

```bash
make init-dev
```

---

## Security

- **API authentication:** every `/api/v1` request requires `X-API-Key`.
  Keys belong to users, are stored as SHA-256 hashes, and can be revoked
  individually. The service has a **single admin role**: every authenticated
  user has full access, but each person has their own key for attribution and
  revocation. `/healthz` and `/readyz` remain public for orchestrators.
- **Bootstrap admin:** on first start, `ADMIN_KEY` becomes the first admin
  user's API key (`admin@localhost`). Existing clients keep working. After
  that, create additional users and keys through `/api/v1/users`,
  `/api/v1/auth/keys`, or the CLI (`backup-manager user create` /
  `backup-manager key create`).
- **Audit log:** mutating API calls are recorded with the user, key, IP,
  request ID and response status. Inspect it at `GET /api/v1/audit`.
- **Production gate:** with `server.env: production` the service refuses to
  start without `CONFIG_ENCRYPT_KEY` and backup encryption. `ADMIN_KEY` is
  required until an active user exists; after bootstrap it can be removed.
- **Provider credentials at rest:** S3 credentials and Discord webhook URLs are
  AES-256-GCM encrypted in the metadata database when
  `CONFIG_ENCRYPT_KEY` is set. Production requires it.
- **Backup encryption:** Age passphrase or X25519 keypair. Production requires
  one of them unless you explicitly set
  `encryption.allow_unencrypted_backups: true`.
- **Secrets:** kept out of `config.yaml`, out of the process environment, and out
  of `docker inspect`. Mount `secrets.env` read-only with mode `0600`.
- **Reverse proxy:** terminate TLS in front of the service (Caddy, nginx,
  Traefik). Set `server.cors_origins` and `server.trusted_proxies` for your
  deployment.

Generate encryption keys with:

```bash
backup-manager keygen
```

Store the private key in `secrets.env` as `AGE_PRIVATE_KEY` and keep a secure
offline copy. **Without it you cannot restore Age-encrypted backups.**

## Users and API keys

Every user has the same admin role. The difference is accountability: each
person or automation gets its own key, which can be revoked without affecting
anyone else.

```bash
# First admin (from ADMIN_KEY) is created automatically on first start.
curl -H "X-API-Key: $ADMIN_KEY" http://localhost:8080/api/v1/auth/me

# Create a user and a key for them.
curl -H "X-API-Key: $ADMIN_KEY" -H 'Content-Type: application/json' \
  -d '{"email":"ops@example.com","name":"Ops Team"}' \
  http://localhost:8080/api/v1/users

curl -H "X-API-Key: $ADMIN_KEY" -H 'Content-Type: application/json' \
  -d '{"name":"ci"}' \
  http://localhost:8080/api/v1/users/$USER_ID/keys

# The plaintext key is returned exactly once. Revoke it later with:
curl -H "X-API-Key: $ADMIN_KEY" -X POST \
  http://localhost:8080/api/v1/users/$USER_ID/keys/$KEY_ID/revoke
```

Users can also create and revoke their own keys via `/api/v1/auth/keys`.
Lockout recovery without the API:

```bash
backup-manager user create --config /etc/backup-manager/config.yaml \
  --email admin2@example.com --name "Admin 2"
backup-manager key create --config /etc/backup-manager/config.yaml \
  --email admin2@example.com --name recovery
```

---

## Architecture

Backup Manager is a **modular monolith**:

```text
cmd/api                  CLI (serve, init, config check, migrate, keygen, verify, recover)
internal/
├── api/                 REST API (handlers, DTOs, middleware, router)
├── app/                 Composition root and graceful shutdown
├── backup/              Backup pipeline, stages, runner and restore
├── config/              config.yaml + secrets.env loader, validation, init, check
├── database/            pgx pool and automatic Goose migrations
├── metarestore/         Metadata snapshot restore CLI
├── monitor/             Health summary
├── notification/        Notification providers (Discord)
├── repository/          Data access (PostgreSQL + sqlc)
├── retention/           GFS retention engine
├── scheduler/           Cron scheduler
├── selfbackup/          Local metadata self-backup
├── service/             Domain services
├── storage/             Storage providers (local, S3)
└── verification/        Archive and checksum verification
sql/                     Migrations and sqlc queries
docs/                    Architecture, configuration, API and Swagger
```

See [**Architecture**](docs/guides/ARCHITECTURE.md) and [**API Reference**](docs/guides/API.md).

### Backup lifecycle

1. A cron trigger or `POST /api/v1/profiles/{id}/run` enqueues the profile.
2. The serial runner creates an execution and runs the pipeline:
   `pg_dump` → verification/checksum → optional Age encryption → upload to every
   storage provider → GFS retention → cleanup.
3. The execution result is persisted and notifications are sent.
4. An encrypted metadata snapshot is uploaded to the same providers (unless
   disabled).
5. Restores are asynchronous jobs: download → decrypt → checksum → target
   preflight → `psql`/`pg_restore` → status and logs.

### Scheduling profiles

`enabled=true` means a profile is scheduled. You can enable several profiles at
once — for example hourly backups to S3 with short retention and a daily backup
to a NAS with long retention:

```bash
curl -X POST -H "X-API-Key: $ADMIN_KEY" http://localhost:8080/api/v1/profiles/$ID/activate
curl -X POST -H "X-API-Key: $ADMIN_KEY" http://localhost:8080/api/v1/profiles/$ID/deactivate
```

Each profile has its own cron expression, storage providers and retention
policy. Triggered backups are queued and executed serially, so two profiles
firing together do not overlap.

### Restore

```bash
# Validate a backup without touching the target database
curl -X POST -H "X-API-Key: $ADMIN_KEY" -H 'Content-Type: application/json' \
  -d '{"verify_only":true}' \
  http://localhost:8080/api/v1/executions/$EXECUTION_ID/restore

# Real restore into an empty target database
curl -X POST -H "X-API-Key: $ADMIN_KEY" -H 'Content-Type: application/json' \
  -d '{"confirm":true}' \
  http://localhost:8080/api/v1/executions/$EXECUTION_ID/restore

# Poll the job
curl -H "X-API-Key: $ADMIN_KEY" \
  http://localhost:8080/api/v1/restores/$RESTORE_ID
```

A restore requires `confirm: true`, verifies the recorded checksum, supports
gzip and Age-encrypted artifacts, and refuses to run against a non-empty target
database by default. Use `terminate_connections: true` if the target has active
clients that should be disconnected. See the
[API reference](docs/guides/API.md#restores).

### Technology stack

| Component | Technology |
|---|---|
| Language | Go |
| HTTP router | Chi |
| API docs | OpenAPI / Swagger |
| Database | PostgreSQL |
| Database access | pgx + sqlc |
| Migrations | Goose (embedded in the binary) |
| Logging | slog (JSON) |
| Configuration | YAML + secrets file |
| Validation | go-playground/validator |
| Hot reload | Air |
| Linting | golangci-lint |
| Containers | Docker & Docker Compose |

---

## Production checklist

- [ ] Generate `config.yaml` and `secrets.env` with `init`; never commit either.
- [ ] `chmod 600 secrets.env`.
- [ ] Keep `secrets.env` and the Age private key in a secure backup.
- [ ] Run `config check --connect` against the real metadata and target databases.
- [ ] Set `server.env: production`; the service then requires a config
      encryption key and backup encryption. Keep `ADMIN_KEY` for the first boot
      (and as a recovery key) until an active user exists.
- [ ] Put the service behind a TLS-terminating proxy; set `cors_origins` and
      `trusted_proxies`.
- [ ] Use a dedicated PostgreSQL role with read access for backups and, ideally,
      a separate `restore_user` with write access. Keep a separate metadata
      database. The target must be empty when restoring.
- [ ] Pin the container image to a version tag, not `latest`.
- [ ] Run one Backup Manager replica; the advisory lock refuses a second one.
- [ ] Monitor `/readyz`, `/api/v1/metrics`, backup failures and overdue profiles.
- [ ] Run `backup-manager verify` on recent backups and test a real restore
      into an empty scratch database before relying on the backups.
- [ ] Keep an off-host recovery kit: config, secrets, encryption keys, S3
      credentials and the image tag.

---

## Metadata disaster recovery

Every backup also uploads an encrypted snapshot of the manager's own state
(profiles, providers, history) to the profile's storage providers. This is
controlled by:

```yaml
metadata_backup:
  embed: true
  retention: 14
  schedule: "0 3 * * *"   # optional local self-backup
```

The snapshot passphrase is `METADATA_BACKUP_PASSPHRASE` in `secrets.env`.
Restore it before starting the service:

```bash
# List local snapshots (embedded meta/<profile>/ and scheduled metadata/)
backup-manager recover metadata --config /etc/backup-manager/config.yaml --list

# Restore the newest local snapshot into the configured metadata database
backup-manager recover metadata --config /etc/backup-manager/config.yaml

# Validate a specific snapshot, or restore over a non-empty database
backup-manager recover metadata --config ... --file snapshot.dump.age --dry-run
backup-manager recover metadata --config ... --file snapshot.dump.age --replace

# Recover directly from S3 when the host or metadata database is gone
backup-manager recover metadata --config ... \
  --s3-bucket my-backups --s3-prefix meta/ \
  --s3-access-key "$AWS_ACCESS_KEY_ID" --s3-secret-key "$AWS_SECRET_ACCESS_KEY"
```

Then restore data into an empty target database:

```bash
# Latest successful backup of a profile
backup-manager recover data --config ... --profile <profile-id> --confirm

# Specific execution, or a local artifact
backup-manager recover data --config ... --execution <execution-id> --confirm
backup-manager recover data --config ... --file backup.sql.gz --confirm

# Verify without touching the target
backup-manager recover data --config ... --execution <id> --verify-only
```

`recover data` accepts `--target-dsn` for a scratch or replacement database and
`--terminate-connections` when clients must be disconnected. Keep a **recovery
kit** off-host: `config.yaml`, `secrets.env`, the config encryption key, the Age
private key or metadata passphrase, S3 credentials and the container image tag.

See the [configuration guide](docs/guides/CONFIGURATION.md#metadata-disaster-recovery)
and the [operations runbook](docs/guides/OPERATIONS.md#recovery-after-host-loss)
for details.

---

## Development

```bash
make tools          # install Air, sqlc, goose, swag and golangci-lint
make init           # generate local config.yaml + secrets.env
make config-check   # validate configuration
make test           # fast go test ./...
make test-integration # PostgreSQL integration tests (set BACKUP_MANAGER_TEST_DSN)
make recovery-drill # container disaster-recovery drill (Docker)
make lint           # golangci-lint
make fmt            # gofmt
make swagger        # regenerate OpenAPI docs
make sqlc           # regenerate database code
make migrate        # apply migrations
```

Swagger UI is served at `/swagger/index.html` when the server is running.
See [docs/guides/TESTING.md](docs/guides/TESTING.md) for the test layers and
the recovery integration test.

---

## Contributing

The project is under active development. Contribution guidelines will be added
once the API and configuration format stabilize.

---

## License

MIT. See [LICENSE](LICENSE).
