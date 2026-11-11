# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Breaking
- **Declarative configuration:** the service no longer reads `APP_*`
  environment variables. Configuration now lives in `config.yaml` (non-secret)
  and `secrets.env` (mode `0600`). Generate both with
  `backup-manager init`. See `docs/guides/CONFIGURATION.md` for the complete
  migration table.
- **CLI:** `backup-manager` now exposes explicit commands: `serve` (default),
  `init`, `config check`, `migrate`, `keygen`, `metadata-restore` and `version`.
- **Automatic migrations:** `serve` applies pending migrations in-process
  (with a 60-second database wait) instead of relying on the Goose CLI in the
  container entrypoint.
- **Docker entrypoint:** the image no longer reads database environment
  variables or runs migrations in shell. Mount `config.yaml` at
  `/etc/backup-manager/config.yaml` and `secrets.env` next to it.

### Added
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