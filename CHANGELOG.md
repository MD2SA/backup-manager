# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.1] - 2026-11-11

### Security
- **Dependency and image hardening:** build with Go 1.26.11, upgrade
  `go-chi/chi` to 5.4.0, refresh Alpine packages during the image build, and
  run `govulncheck` plus Trivy image scanning in CI.

## [1.0.0] - 2026-11-11

### Breaking
- **Declarative configuration:** the service no longer reads `APP_*`
  environment variables. Configuration now lives in `config.yaml` (non-secret)
  and `secrets.env` (mode `0600`). Generate both with
  `backup-manager init`. See `docs/guides/CONFIGURATION.md` for the complete
  migration table.
- **CLI:** `backup-manager` now exposes explicit commands: `serve` (default),
  `init`, `config check`, `migrate`, `keygen`, `verify`, `recover` and `version`.
  `metadata-restore` remains as a deprecated alias for `recover metadata`.
- **Automatic migrations:** `serve` applies pending migrations in-process
  (with a 60-second database wait) instead of relying on the Goose CLI in the
  container entrypoint.
- **Docker entrypoint:** the image no longer reads database environment
  variables or runs migrations in shell. Mount `config.yaml` at
  `/etc/backup-manager/config.yaml` and `secrets.env` next to it.

### Added
- **Docker Hub mirror:** stable releases are also published to Docker Hub when
  the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository secrets are set.
- **Release pipeline hardening:** explicit `latest` tagging, SBOM and
  provenance attestations, multi-architecture images for `linux/amd64` and
  `linux/arm64`, and a release guide covering tags, GHCR, verification and
  rollback.
- **Documentation site and repository polish:** MkDocs Material site with
  GitHub Pages deployment, a rewritten landing README, a docs index,
  contributing and security policies, pull-request template and issue forms.
- **PostgreSQL 13–18 compatibility:** the runtime image now ships the
  PostgreSQL 18 client, `config check --connect` validates that `pg_dump`,
  `pg_restore` and `psql` are new enough for the target server, and CI runs
  restore/recovery integration tests against PostgreSQL 13 through 18.
- **Operation timeouts:** `operations.backup_timeout` and
  `operations.restore_timeout` bound dump/restore work; a timed-out operation is
  marked failed instead of blocking the serial queue forever.
- **Overdue notifications:** a periodic health check sends a Discord `overdue`
  notification when a profile stops producing successful backups, repeats at
  most once per day and clears the episode when the profile recovers.
- **Production hardening:** the single-instance advisory lock is now acquired
  before migrations, so two replicas cannot race the schema version table.
  Backups left in `pending`/`running` by a crash are marked failed on startup,
  and local storage uploads are written to a temporary file and atomically
  renamed into place.
- **Recovery engine tests:** recovery orchestration now lives in
  `internal/recovery` with a thin CLI, unit tests for snapshot discovery,
  target guards, execution resolution and provider fallback, plus a
  PostgreSQL integration test that destroys the metadata and target databases
  and verifies restored checksums, sequences, indexes and foreign keys.
- **Multi-passphrase metadata decryption:** metadata snapshots encrypted with
  either the profile encryption passphrase or `METADATA_BACKUP_PASSPHRASE` are
  now tried automatically during recovery.
- **S3 recovery integration:** an in-memory S3-compatible server exercises
  upload, metadata recovery and data recovery through the real AWS SDK.
- **Container recovery drill:** `scripts/recovery-drill.sh` and
  `make recovery-drill` exercise the real image end to end, including metadata
  and target database loss, Age decryption, checksum/schema comparison and the
  safety guards. It runs on critical pull requests and weekly in CI.
- **Recovery CLI:** `backup-manager recover metadata` lists and restores the
  newest metadata snapshot from local storage, an explicit file, or S3/S3-
  compatible storage (`--s3-bucket`, `--s3-endpoint`, `--s3-path-style`).
  `--dry-run` validates without writing and `--replace` allows an in-place
  restore. Supports `--list`, `--profile-id` and passphrase/Age overrides.
- **Data recovery CLI:** `backup-manager recover data` restores the latest
  successful execution of a profile, a specific execution or a local artifact.
  It verifies checksums, decrypts/decompresses, refuses to target the metadata
  database, and supports `--target-dsn`, `--verify-only`,
  `--terminate-connections` and `--confirm`.
- **Metadata snapshot accuracy:** embedded snapshots are now taken after the
  execution row is finalized, so a recovered metadata database contains the
  correct success status, checksum and storage path of the backup that carried
  the snapshot.
- **Recovery drills:** `backup-manager verify` validates one or more executions
  and can apply an artifact to an empty scratch database.
- **Recursive local listing:** local storage can list nested prefixes for
  metadata recovery without changing the existing shallow `List` contract.
- **`config.yaml` + `secrets.env`:** strict YAML schema (`version: 1`), unknown
  fields fail fast, and secrets are read directly from a file instead of the
  process environment.
- **`backup-manager init`:** interactive/non-interactive generator for config,
  secrets, Age keypair, admin key and provider-config key. Includes `--docker`
  and `--dev` presets.
- **`backup-manager config check`:** validates schema, secret references, file
  permissions, required binaries, writable directories, Age key consistency,
  cron syntax and (with `--connect`) database connectivity. Prints a redacted
  summary.
- **Public liveness/readiness endpoints:** `/healthz` and `/readyz` (the latter
  pings the metadata database). Container healthchecks no longer need an API key.
- **`GET /api/v1/profiles/{id}`**: fetch a profile with its linked providers.
- **404 responses** for missing profiles and executions.
- **Idempotency:** `X-Idempotency-Key` on `POST /profiles/{id}/run` and
  `POST /executions/{id}/restore`.
- **Correlation and rate limiting:** `X-Request-ID` on every response, configurable
  CORS/trusted proxies and per-client rate limits.
- **CI/CD:** GitHub Actions workflow with gofmt, vet, lint, race tests, build,
  configuration smoke test, and Docker image publishing to GHCR.
- **Reliable asynchronous restores:** new `restores` table and `RestoreRunner`
  with `pending/running/success/failed` states, logs, provider/path audit,
  checksum verification, gzip decompression, streaming Age decryption,
  `psql ON_ERROR_STOP` + `--single-transaction`, and target preflight checks.
- **Per-provider storage copies:** new `execution_storage` table records every
  uploaded copy, so restores use the correct provider and retention can delete
  all copies before removing an execution.
- **Restore API:** `POST /executions/{id}/restore` now returns `202` with a
  restore ID; `GET /restores/{id}`, `GET /executions/{id}/restores` and
  `GET /restores` expose progress and history.
- **Restore safety:** `confirm: true` is required for a real restore;
  `verify_only` validates an artifact without touching the target; non-empty
  target databases and active connections are rejected unless explicitly
  terminated.
- **Backup portability:** new dumps use `--no-owner --no-acl`, and the target
  `sslmode` is applied to `pg_dump`, `psql` and `pg_restore`.
- **Multi-user authentication:** new `users` and `api_keys` tables with SHA-256
  key hashes, expiry and immediate revocation. Every user has the single admin
  role; keys provide identity, attribution and revocation.
- **Bootstrap and recovery:** `ADMIN_KEY` seeds `admin@localhost` on first boot
  and keeps working as that admin's key. CLI `user create` / `key create`
  provide lockout recovery.
- **Audit log:** mutating API requests are recorded with user, key, IP, request
  ID and status; `GET /audit` exposes the log.
- **User/key API and self-service rotation:** `/auth/me`, `/auth/keys`,
  `/users`, `/users/{id}/keys` and revocation endpoints.
- **Per-user controls:** rate limiting per authenticated user and idempotency
  keys scoped by user; restore jobs record `requested_by_user_id` and
  `requested_by_key_id`.
- **Multiple scheduled profiles:** every profile can now be enabled
  independently. The scheduler keeps one cron entry per profile and reconciles
  changes immediately after create/update/activate/deactivate/delete.
- **Serial backup queue:** different profiles can be queued and run one at a
  time; the same profile cannot be queued twice. This supports hourly S3 +
  daily NAS-style policies without dropping triggers.
- **Scheduling API:** `/profiles/{id}/activate` and
  `/profiles/{id}/deactivate` affect only the selected profile. Health summary
  reports the earliest next run across all enabled profiles.
- **Storage isolation:** new backup paths include a short profile ID, avoiding
  collisions between profiles with similar names.
- **Keyset pagination:** all list endpoints now accept `limit` (default 50,
  max 200) and an opaque `cursor`, returning `{items, next_cursor, has_more}`
  and a `Link` header. The old bare-array list responses are replaced.
- **Pagination indexes:** added `(created_at, id)` / `(name, id)` indexes for
  executions, restores, audit, users, profiles, providers, policies and API keys.
- **Overdue profiles:** `health.overdue_factor`/`overdue_grace`, real
  `overdue_profiles` counts and per-profile status in `/health/summary`.
- **Prometheus metrics:** authenticated `/api/v1/metrics` with backup, restore,
  queue, scheduler, health, auth and HTTP series.
- **Audit retention:** configurable `audit.retention_days` and cron cleanup.
- **Build version:** `/version` plus Docker/CI build metadata.
- **Operations runbook:** `docs/guides/OPERATIONS.md` with alerts, key rotation,
  upgrades and incident checklist.
- **Verification CLI:** `backup-manager verify` validates one or more executions,
  with optional application to an empty scratch database for real restore drills.
- **Separate restore credentials:** optional `target_database.restore_user` and
  `restore_password_secret`; backups use the read user and restores use the
  privileged user.
- **S3-compatible endpoints:** `endpoint`, `force_path_style` and `use_ssl` are
  now honored; `Exists` no longer reports auth/network errors as "not found".
- **Single-instance guard:** a PostgreSQL advisory lock prevents two service
  instances from scheduling duplicate backups.
- **Observability assets:** Prometheus alert rules and a Grafana dashboard under
  `deploy/observability/`.

### Security
- **Secrets isolation:** secrets are not loaded into the process environment,
  so they do not appear in `/proc`, child processes (`pg_dump`, `psql`) or
  `docker inspect`.
- **Production gate:** with `server.env: production` the service refuses to
  start without `ADMIN_KEY`, `CONFIG_ENCRYPT_KEY` and a backup encryption mode
  (unless `encryption.allow_unencrypted_backups: true`).
- **Provider configs encrypted at rest:** S3 credentials and Discord webhook
  URLs are AES-256-GCM encrypted in the metadata database when
  `CONFIG_ENCRYPT_KEY` is set.
- **Provider secrets masked in API responses:** `secret_key`, `access_key`, and
  `webhook_url` are returned as `********`. Updating a provider with the masked
  placeholder preserves the stored secret.
- **Request timeouts** added to the HTTP server, and graceful shutdown now stops
  the scheduler and metadata self-backup before closing the database.

### Changed
- **CORS and proxies:** configured via `server.cors_origins` and
  `server.trusted_proxies` (comma-separated lists in YAML).
- **Metadata self-backup:** configured via `metadata_backup.schedule`,
  `metadata_backup.retention`, `metadata_backup.embed`, and the
  `METADATA_BACKUP_PASSPHRASE` secret.
- **Metadata restore CLI:** now accepts `--config`; the rest of the flags are
  unchanged (`--file`, `--passphrase`, `--identity`, `--replace`, `--dry-run`).
- **Docker Compose:** mounts `config.yaml`/`secrets.env` read-only and keeps the
  metadata database internal. `.env` is now Compose-only (host paths, UID/GID,
  bundled database credentials).
- **Documentation:** README, configuration guide and API reference rewritten for
  the new model, with a production checklist and troubleshooting section.
- **Restore semantics:** restores are now asynchronous and require
  `{"confirm": true}`. A target database must be empty; use `verify_only` to
  validate a backup safely.
- **Retention:** the engine is provider-aware, only considers successful
  backups for GFS, deletes every stored copy before removing an execution, and
  skips executions with an active restore.

### Fixed
- **Docker healthcheck:** it now probes the public `/healthz` endpoint instead
  of an authenticated `/api/v1` route.
- **License consistency:** README now states MIT, matching `LICENSE`.

## [0.1.0] - Initial development release

Baseline: automated PostgreSQL backup service with scheduling, retention (GFS), multi-provider storage (local/S3), Discord notifications, Age encryption, restore workflows, and health monitoring.