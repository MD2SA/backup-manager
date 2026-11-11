# Configuration Guide

Backup Manager reads its configuration from two files:

| File | Purpose | Recommended mode |
|---|---|---|
| `config.yaml` | Non-secret settings (server, storage, databases, retention defaults) | `0644` |
| `secrets.env` | Passwords, API keys and encryption keys | `0600` |

The application does **not** load `.env` and does **not** read settings from
process environment variables. Secrets never appear in `docker inspect` or in
the environment of child processes such as `pg_dump`.

The only environment variables the application understands are paths and
deployment details:

| Variable | Meaning |
|---|---|
| `APP_CONFIG_PATH` | Path to `config.yaml` (default `./config.yaml`) |
| `APP_SECRETS_FILE` | Overrides `secrets_file` from the config (useful for Docker/Kubernetes secrets) |
| `APP_MIGRATIONS_DIR` | Path to SQL migrations (default `sql/migrations`) |
| `PUID` / `PGID` | Container UID/GID overrides (entrypoint only) |

---

## Quick start

### Generate the files

```bash
backup-manager init
```

The wizard prompts for the databases, storage path, environment and encryption
mode, then writes `config.yaml` and `secrets.env` (mode `0600`). It generates:

- the bootstrap admin API key (`ADMIN_KEY`),
- the provider-config encryption key (`CONFIG_ENCRYPT_KEY`),
- an Age keypair (default) or an encryption passphrase,
- a metadata snapshot passphrase when embedding is enabled.

Useful presets:

```bash
cp .env.example .env   # set POSTGRES_PASSWORD and other Compose values
make init-docker       # metadata host "metadata-db", storage /backups, temp /tmp
make init-dev          # docker preset + target host "target-db"
```

The presets read `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` and
`TARGET_DB_NAME` from `.env` when run through the Makefile, so the generated
configuration matches the bundled Compose databases.

Because the Docker preset uses `storage.path: /backups`, run the connectivity
check inside the container where that path is mounted:

```bash
make docker-config-check
```

Non-interactive example:

```bash
backup-manager init --non-interactive \
  --metadata-host localhost --metadata-password '...' \
  --target-host postgres.example.com --target-password '...' \
  --storage-path /backups --env production --encryption keypair
```

### Validate before starting

```bash
backup-manager config check
backup-manager config check --connect   # also pings both databases
```

`config check` validates the schema, secret references, file permissions,
required binaries (`pg_dump`, `psql`, `pg_restore`), directory writability,
Age key consistency and cron syntax. It prints a redacted summary and never
prints secret values.

### Start

```bash
backup-manager serve
```

`serve` applies pending migrations automatically. Use `--no-migrate` to skip
that step, or run `backup-manager migrate` separately.

---

## `config.yaml` reference

```yaml
version: 1

server:
  port: "8080"
  env: "production"              # development | staging | production
  log_level: "info"              # debug | info | warn | error
  cors_origins:
    - "https://backup.example.com"
  trusted_proxies:
    - "172.16.0.0/12"
  rate_limit:
    requests: 100
    window: "1m"

storage:
  path: "/backups"
  temp_dir: "/tmp"

secrets_file: "secrets.env"

secrets:
  metadata_db_password: "METADATA_DB_PASSWORD"
  target_db_password: "TARGET_DB_PASSWORD"
  age_private_key: "AGE_PRIVATE_KEY"
  encryption_passphrase: "ENCRYPTION_PASSPHRASE"
  admin_key: "ADMIN_KEY"
  config_encrypt_key: "CONFIG_ENCRYPT_KEY"
  metadata_backup_passphrase: "METADATA_BACKUP_PASSPHRASE"

metadata_database:
  host: "metadata-db"
  port: "5432"
  user: "backup_manager"
  dbname: "backup_manager"
  sslmode: "require"

target_database:
  host: "postgres.example.com"
  port: "5432"
  user: "backup_reader"
  dbname: "app"
  sslmode: "require"

encryption:
  age_public_key: "age1..."
  allow_unencrypted_backups: false

metadata_backup:
  schedule: "0 3 * * *"
  retention: 14
  embed: true

operations:
  backup_timeout: "6h"
  restore_timeout: "2h"

health:
  overdue_factor: 2.0
  overdue_grace: "15m"
  notify_interval: "15m"
  notify_repeat: "24h"

audit:
  retention_days: 90
  cleanup_schedule: "0 4 * * *"
```

### Top-level fields

| Field | Default | Description |
|---|---|---|
| `version` | — (required) | Schema version. Only `1` is accepted; unknown versions refuse to start. |
| `secrets_file` | `secrets.env` | Secrets file path, resolved relative to `config.yaml` unless absolute. |

### `server`

| Field | Default | Description |
|---|---|---|
| `port` | `"8080"` | HTTP listen port. |
| `env` | `development` | `production`/`prod` enables the production safety rules. |
| `log_level` | `info` | `debug`, `info`, `warn` or `error`. |
| `cors_origins` | `["http://localhost:3000"]` | Browser origins allowed by CORS. |
| `trusted_proxies` | `[]` | IPs/CIDRs whose `X-Forwarded-For` is trusted. Bare IPs become `/32`. |
| `rate_limit.requests` | `100` | Requests allowed per window, per client IP/key. |
| `rate_limit.window` | `1m` | Window duration (`30s`, `1m`, `1h`, ...). |

### `storage`

| Field | Default | Description |
|---|---|---|
| `path` | `/backups` | Root directory for the local storage provider and metadata backups. Use an absolute path in production. |
| `temp_dir` | `/tmp` | Staging directory for dumps, encryption and restore downloads. |

### `metadata_database`

The manager's own state. All fields are required:

`host`, `port`, `user`, `dbname`, `sslmode` (and the password from the secrets
file). This database must exist before startup; migrations run automatically.

### `target_database`

The PostgreSQL database to back up. Either configure **all** fields or leave
them empty:

`host`, `port`, `user`, `dbname`, `sslmode` (and the password from the secrets
file). A partially configured target database fails validation. Backups and
restores report a clear infrastructure error while it is empty.

Optional restore credentials:

```yaml
target_database:
  restore_user: "restore_admin"
  restore_password_secret: "TARGET_DB_RESTORE_PASSWORD"
```

When `restore_user` is set, `pg_dump` continues to use the backup user while
`psql`/`pg_restore` use the restore user. Without it, both use the same
credentials.

`sslmode` is applied to `pg_dump`, `psql` and `pg_restore` (via `PGSSLMODE`).

**Restore requires an empty target database.** A target with user tables,
views or sequences is rejected with `409` before a restore job is queued. This
keeps restores predictable and avoids a failed `psql` halfway through. New
backups are dumped with `--no-owner --no-acl` for portability.

The target database must not be the same as `metadata_database`; the service
refuses to start if they point to the same host, port and database name.

### `encryption`

| Field | Default | Description |
|---|---|---|
| `age_public_key` | empty | Age X25519 recipient used for backup encryption. |
| `allow_unencrypted_backups` | `false` | Production guard. Set `true` only if you deliberately accept unencrypted backups. |

A passphrase can be used instead of (or alongside) the Age keypair by setting
the `ENCRYPTION_PASSPHRASE` secret. If both are present, the passphrase takes
precedence when encrypting.

### `metadata_backup`

| Field | Default | Description |
|---|---|---|
| `schedule` | empty (disabled) | 5-field cron for the local metadata self-backup. |
| `retention` | `14` | Snapshots to keep per local directory / per profile per provider. |
| `embed` | `true` | Upload an encrypted metadata snapshot with every backup execution. |

### `operations`

| Field | Default | Description |
|---|---|---|
| `backup_timeout` | `6h` | Maximum duration of one backup. Use `0` to disable. |
| `restore_timeout` | `2h` | Maximum duration of one restore. Use `0` to disable. |

Timeouts cover the dump/download/decrypt/apply path but not the final status
update, so a timed-out execution is still recorded as `failed`. Set a larger
value for very large databases, or `0` if an operation must never be killed.

### `health`

| Field | Default | Description |
|---|---|---|
| `overdue_factor` | `2.0` | A profile is overdue when the last successful backup is older than `maxGap × factor + grace`. |
| `overdue_grace` | `15m` | Extra tolerance before marking a profile overdue. |
| `notify_interval` | `15m` | How often to check for overdue profiles. `0` disables built-in overdue notifications. |
| `notify_repeat` | `24h` | Minimum interval between repeated notifications for the same overdue episode. |

`maxGap` is sampled from the profile's cron expression (hourly ≈ 1h, daily ≈
24h, weekly ≈ 7d, monthly ≈ 31d, yearly ≈ 365d).

When a profile becomes overdue, Backup Manager sends an `overdue` Discord
notification to the profile's notification providers. The notification repeats
at most once per `notify_repeat`; when the profile recovers, the episode is
cleared and a future overdue state notifies again.

### `audit`

| Field | Default | Description |
|---|---|---|
| `retention_days` | `90` | Audit entries older than this are deleted. `0` disables cleanup. |
| `cleanup_schedule` | `0 4 * * *` | Cron expression for the audit cleanup job. |

---

## S3-compatible storage

When creating an S3 storage provider through the API, the `config` object
supports custom endpoints and path-style access:

```json
{
  "region": "us-east-1",
  "bucket": "backups",
  "access_key": "...",
  "secret_key": "...",
  "endpoint": "https://minio.example.com:9000",
  "force_path_style": true,
  "use_ssl": true
}
```

`endpoint` may include the scheme; otherwise `use_ssl` decides between
`https://` and `http://`. This supports AWS S3, MinIO, Cloudflare R2 and other
S3-compatible services.

## Restore behavior

Restores are asynchronous and use the target database configured above.

- `POST /api/v1/executions/{id}/restore` returns `202` with a restore ID.
- `GET /api/v1/restores/{id}` returns status, logs, provider, path and whether
  the checksum was verified.
- A real restore requires `{"confirm": true}`.
- `{"verify_only": true}` downloads, decrypts, verifies the checksum and
  detects the format without touching the target.
- `{"terminate_connections": true}` allows the job to disconnect other clients
  from the target database. Default is `false`.
- A non-empty target returns `409` immediately.
- The checksum recorded at backup time is verified before any SQL is executed.
- `psql` runs with `ON_ERROR_STOP=1` and `--single-transaction`; a failure rolls
  the restore back.
- gzip, plain SQL and PostgreSQL custom-format artifacts are supported.

Only one restore runs at a time, and backups and restores share a target
database gate so a `pg_dump` cannot overlap a restore.

---

## `secrets.env` reference

The file is a simple `KEY=VALUE` list. Blank lines and lines starting with `#`
are ignored. Values containing spaces or `#` may be double-quoted.

| Key | Required | Description |
|---|---|---|
| `METADATA_DB_PASSWORD` | yes | Password for `metadata_database`. |
| `TARGET_DB_PASSWORD` | if target configured | Password for `target_database`. |
| `ADMIN_KEY` | first boot / recovery | Bootstrap admin API key (`X-API-Key`). On the first start it seeds the `admin@localhost` user and remains valid until revoked. After an active user exists it can be removed from the secrets file. Generate with `openssl rand -base64 32`. |
| `CONFIG_ENCRYPT_KEY` | in production | AES-256-GCM key for provider credentials at rest. Changing it invalidates existing provider configs. |
| `AGE_PRIVATE_KEY` | for Age restores | Age X25519 private key (`AGE-SECRET-KEY-...`). |
| `ENCRYPTION_PASSPHRASE` | optional | Passphrase used when a profile enables passphrase encryption. |
| `METADATA_BACKUP_PASSPHRASE` | recommended with `embed: true` | Passphrase for embedded/local metadata snapshots. |

The names above are defaults. You can point each logical secret to a different
key using the `secrets:` mapping in `config.yaml`.

### File permissions

```bash
chmod 600 secrets.env
```

`config check` warns if the secrets file is readable by group or others.

---

## Docker and Docker Compose

### Compose mounts

```yaml
services:
  backup-manager:
    image: ghcr.io/md2sa/backup-manager:latest
    environment:
      APP_CONFIG_PATH: /etc/backup-manager/config.yaml
    volumes:
      - ./config.yaml:/etc/backup-manager/config.yaml:ro
      - ./secrets.env:/etc/backup-manager/secrets.env:ro
      - ./backups:/backups
```

`secrets_file: secrets.env` is resolved relative to
`/etc/backup-manager`, so the second mount is exactly where the application
looks for it.

### Docker/Kubernetes secrets as a single file

If your platform mounts secrets as files, mount the whole `secrets.env` as one
secret and point `APP_SECRETS_FILE` at it:

```yaml
environment:
  APP_SECRETS_FILE: /run/secrets/backup-manager-secrets
```

This keeps secrets out of environment variables and `docker inspect`. If you
prefer one file per secret, create separate files and use a small entrypoint
script to assemble them into a single `secrets.env` before starting the
container; the application intentionally supports one secrets file.

### Bundled metadata database

The Compose example's `metadata-db` container uses `POSTGRES_USER`,
`POSTGRES_PASSWORD` and `POSTGRES_DB` from `.env`. They must match
`metadata_database` in `config.yaml` and `METADATA_DB_PASSWORD` in
`secrets.env`. For production, prefer an external managed PostgreSQL and remove
the bundled service.

---

## Production checklist

1. `server.env: "production"`.
2. `CONFIG_ENCRYPT_KEY` and an encryption mode configured; the service refuses
   to start otherwise. `ADMIN_KEY` is required until an active user exists.
3. `secrets.env` mode `0600`, owned by the service user, excluded from Git.
4. Absolute `storage.path` and `storage.temp_dir`, on a persistent volume.
5. `sslmode: "require"` (or stricter) for both databases.
6. A dedicated backup role on the target database with read-only access.
7. TLS terminated by a reverse proxy; `cors_origins` restricted and
   `trusted_proxies` set to the proxy IPs/CIDRs.
8. Pinned image tag and a tested restore procedure.
9. `config check --connect` run against the real environment before rollout.
10. A secure offline copy of `secrets.env`, especially `AGE_PRIVATE_KEY`.
11. Monitor `/readyz`, backup failures and the timestamp of the last successful
    backup.

---

## Authentication and users

Backup Manager has a **single admin role**: every authenticated user can use the
full API. Users exist to give each person or automation an independent key that
can be revoked without impacting anyone else.

- On first start with `ADMIN_KEY` set, the service creates `admin@localhost` and
  stores the hash of `ADMIN_KEY` as its bootstrap API key. Existing clients keep
  using the same key.
- After that, manage users and keys through `/api/v1/users`,
  `/api/v1/auth/keys` and `/api/v1/audit`, or from the host:

  ```bash
  backup-manager user create --config ... --email admin2@example.com --name "Admin 2"
  backup-manager key create --config ... --email admin2@example.com --name recovery
  ```

- API keys are stored as SHA-256 hashes and shown only once.
- Keys can expire and be revoked immediately.
- The last active user cannot be deactivated.
- Mutating API requests are written to the audit log with user, key, IP,
  request ID and status.
- If no users and no `ADMIN_KEY` exist in development, the service starts in
  insecure mode with a warning. In production, at least one active user must
  exist or `ADMIN_KEY` must be set for the first bootstrap.

## Migrating from environment variables (schema 0.1)

The previous releases configured everything through `APP_*` environment
variables. They are no longer read. Generate a new config with `init` and move
the values:

| Old environment variable | New location |
|---|---|
| `APP_PORT` | `server.port` |
| `APP_ENV` | `server.env` |
| `APP_LOG_LEVEL` | `server.log_level` |
| `APP_CORS_ORIGINS` | `server.cors_origins` |
| `APP_TRUSTED_PROXIES` | `server.trusted_proxies` |
| `APP_RATE_LIMIT_REQUESTS` | `server.rate_limit.requests` |
| `APP_RATE_LIMIT_WINDOW` | `server.rate_limit.window` |
| `APP_STORAGE_PATH` | `storage.path` |
| `APP_TEMP_DIR` | `storage.temp_dir` |
| `APP_METADATA_DB_*` | `metadata_database.*` + `METADATA_DB_PASSWORD` |
| `APP_TARGET_DB_*` | `target_database.*` + `TARGET_DB_PASSWORD` |
| `APP_ADMIN_KEY` | `ADMIN_KEY` secret |
| `APP_CONFIG_ENCRYPT_KEY` | `CONFIG_ENCRYPT_KEY` secret |
| `APP_AGE_PUBLIC_KEY` | `encryption.age_public_key` |
| `APP_AGE_PRIVATE_KEY` | `AGE_PRIVATE_KEY` secret |
| `APP_ENCRYPTION_PASSPHRASE` | `ENCRYPTION_PASSPHRASE` secret |
| `APP_METADATA_BACKUP_SCHEDULE` | `metadata_backup.schedule` |
| `APP_METADATA_BACKUP_RETENTION` | `metadata_backup.retention` |
| `APP_METADATA_BACKUP_EMBED` | `metadata_backup.embed` |
| `APP_METADATA_BACKUP_PASSPHRASE` | `METADATA_BACKUP_PASSPHRASE` secret |

`APP_BACKUP_PATH` remains a Docker Compose variable (host path mounted at
`/backups`); it is not application configuration.

Deploy the new config before upgrading the image. The metadata database schema
does not change, so existing profiles and execution history are preserved.

---

## Scheduling profiles

A profile is scheduled when `enabled=true` in its configuration. **Several
profiles may be enabled at the same time**, each with its own cron expression,
storage providers and retention policy. This supports patterns such as:

- hourly backups to S3 with a short retention;
- a daily backup to a NAS or local disk with a long retention;
- a weekly encrypted archive.

Triggered backups are queued and executed serially against the single target
database. The queue accepts different profiles; the same profile cannot be
queued twice while pending or running.

Use `POST /api/v1/profiles/{id}/activate` and
`POST /api/v1/profiles/{id}/deactivate` (or `enabled` on create/update) to
change scheduling. Changes are applied to the running scheduler immediately.

New backup objects are stored under
`<storage.path>/<profile-slug>-<profile-id-prefix>/...`, keeping profiles with
similar names isolated.

## Metadata disaster recovery

Backup Manager protects its own state in two complementary ways.

### 1. Embedded in every backup (default)

Each backup execution uploads an encrypted snapshot of the metadata database to
every storage provider of the profile:

```text
meta/<profile-id>/metadata-<timestamp>.dump.age
```

Configured by:

```yaml
metadata_backup:
  embed: true
  retention: 14
```

The snapshot is encrypted with the profile's backup keys (passphrase or Age
public key), falling back to `METADATA_BACKUP_PASSPHRASE`. If neither is
available, the metadata snapshot is skipped with a warning and the data backup
continues normally. Snapshots are never uploaded in plaintext.

### 2. Scheduled local self-backup

```yaml
metadata_backup:
  schedule: "0 3 * * *"
  retention: 14
```

Snapshots are written to `<storage.path>/metadata/metadata-<timestamp>.dump` and
encrypted with `METADATA_BACKUP_PASSPHRASE` when set. This is a local copy; sync
it off the host or rely on the embedded snapshots for remote coverage.

### 3. Restoring the metadata database

Use `recover metadata` before starting the service. It searches local snapshots
by default, can download from S3, and validates/decrypts before writing.

```bash
# List local snapshots (embedded meta/<profile-id>/ + scheduled metadata/)
backup-manager recover metadata --config /etc/backup-manager/config.yaml --list

# Newest local snapshot
backup-manager recover metadata --config /etc/backup-manager/config.yaml

# One profile only
backup-manager recover metadata --config ... --profile-id <profile-uuid>

# Specific snapshot, validate only, or restore over a non-empty database
backup-manager recover metadata --config ... --file snapshot.dump.age
backup-manager recover metadata --config ... --file snapshot.dump.age --dry-run
backup-manager recover metadata --config ... --file snapshot.dump.age --replace

# Directly from S3/S3-compatible storage
backup-manager recover metadata --config ... \
  --s3-bucket my-backups --s3-prefix meta/ \
  --s3-access-key ... --s3-secret-key ...
```

S3 flags may also come from `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
`AWS_REGION` and `AWS_ENDPOINT_URL`. For MinIO or R2 add `--s3-endpoint`,
`--s3-path-style` and, for plain HTTP, `--s3-use-ssl=false`. Prefer environment
variables over command-line keys, which are visible in the process list.

Supported formats are auto-detected: age (current), legacy `enc_v1`, and
plaintext. Age snapshots require `METADATA_BACKUP_PASSPHRASE` or
`AGE_PRIVATE_KEY`; pass `--passphrase`/`--identity` to override the config.
By default the command refuses to restore into a database that already contains
user objects.

`metadata-restore` remains as a deprecated alias for `recover metadata`.

### 4. Restoring data after host loss

`recover data` works from the metadata database or from a local artifact. It
verifies the checksum, decrypts, decompresses and applies the backup to an
**empty** target.

```bash
# Latest successful execution of a profile
backup-manager recover data --config ... --profile <profile-uuid> --confirm

# Specific execution (optionally force one storage provider)
backup-manager recover data --config ... --execution <execution-uuid> --confirm
backup-manager recover data --config ... --execution <uuid> --provider-id <provider-uuid> --confirm

# Local artifact when the metadata database is unavailable
backup-manager recover data --config ... --file backup.sql.gz --confirm

# Validate only; never touches the target
backup-manager recover data --config ... --execution <uuid> --verify-only
```

Use `--target-dsn` to restore into a scratch/replacement database instead of
`target_database` from the config, and `--terminate-connections` when clients
must be disconnected. The command refuses to target the metadata database.

### 5. Recovery kit

Keep these off-host, encrypted, and accessible without the failed server:

- `config.yaml` and `secrets.env` (or the secret manager equivalents);
- `CONFIG_ENCRYPT_KEY` (provider credentials are encrypted with it);
- the Age private key or `METADATA_BACKUP_PASSPHRASE`;
- S3 endpoint, bucket and credentials, if used;
- the container image tag and this CLI.

Without the encryption key or passphrase, encrypted metadata and data snapshots
cannot be recovered. Test the kit periodically with `recover data --verify-only`
and a restore into an empty scratch database.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| `unsupported config version 0` | Add `version: 1` at the top of `config.yaml`. |
| `field X not found` | The YAML has a typo or an unsupported field. The error names the exact key. |
| `secret "Y" is missing or empty in ...` | Add `Y=...` to `secrets.env`. |
| `secrets file ... is readable by group/others` | `chmod 600 secrets.env`. |
| `target database is not configured` | Fill `target_database` or ignore until you are ready to back up. |
| `production requires backup encryption` | Configure `encryption.age_public_key` or `ENCRYPTION_PASSPHRASE`, or explicitly set `encryption.allow_unencrypted_backups: true`. |
| `age_public_key does not match ...` | The private key in `AGE_PRIVATE_KEY` belongs to a different keypair. |
| `metadata database not reachable after 60s` | Check `metadata_database` and the network; migrations retry for 60 seconds. |
| `config.yaml not found` inside Docker | Mount it at `/etc/backup-manager/config.yaml` or set `APP_CONFIG_PATH`. |
| `password authentication failed` after changing `POSTGRES_PASSWORD` | The bundled PostgreSQL volume was initialized with the old password. Run `docker compose down -v` once (this deletes the metadata database) or change the password inside PostgreSQL. |
| `No metadata snapshots found` | Check `storage.path`, or pass `--storage-path`, `--file`, or the correct `--s3-bucket`/`--s3-prefix`. Use `--list` to inspect what is visible. |
| `target database contains data; pass --replace` | Metadata recovery refuses a non-empty database by design. Use an empty database or `--replace` after taking a copy. |
| `Refusing to restore without --confirm` | Add `--confirm` for a real data restore, or use `--verify-only` to validate only. |
