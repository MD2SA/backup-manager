# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- **Production gate:** the service now refuses to start with `APP_ENV=production` if `APP_ADMIN_KEY` is empty. Insecure mode is limited to development.
- **Provider configs encrypted at rest:** S3 credentials and Discord webhook URLs are AES-256-GCM encrypted in the metadata database when `APP_CONFIG_ENCRYPT_KEY` is set (plaintext in dev).
- **Provider secrets masked in API responses:** `secret_key`, `access_key`, and `webhook_url` are returned as `********`. Updating a provider with the masked placeholder preserves the stored secret.

### Added
- **Idempotency:** `X-Idempotency-Key` header on `POST /profiles/{id}/run` and `POST /executions/{id}/restore` prevents duplicate triggers within a 24-hour window.
- **Configurable CORS:** `APP_CORS_ORIGINS` (comma-separated) replaces the hardcoded `http://localhost:3000` origin.
- **Trusted proxies:** `APP_TRUSTED_PROXIES` (IPs or CIDRs) enables correct client-IP resolution through `X-Forwarded-For` for rate limiting and logging; direct connections still work.
- **X-Request-ID:** unique request ID generated (or echoed from the incoming header), returned in responses, and included in access logs for correlation.
- **Metadata self-backup:** scheduled `pg_dump` of the internal database (`APP_METADATA_BACKUP_SCHEDULE`), optional Age-style passphrase encryption (`APP_METADATA_BACKUP_PASSPHRASE`), and retention pruning (`APP_METADATA_BACKUP_RETENTION`, default 14).
- **Metadata embedded in backups:** each backup execution also uploads an encrypted metadata snapshot (`meta/<profile-id>/metadata-<timestamp>.dump.age`) to every storage provider of the profile (`APP_METADATA_BACKUP_EMBED`, default on). Encryption uses the profile's backup keys, falling back to `APP_METADATA_BACKUP_PASSPHRASE`; it is never uploaded in plaintext and never fails the data backup. Retention is isolated per profile/provider.
- **Production compose hardening:** the metadata database no longer publishes host ports, services use `restart: unless-stopped`, and resource limits are declared.
- **CI/CD:** GitHub Actions workflow running lint, vet, test (with `-race`), and build, plus Docker image publishing to GHCR with semantic version and branch tags.
- **Metadata restore CLI:** `backup-manager metadata-restore` decrypts (age/legacy `enc_v1`/plaintext) and restores a metadata snapshot into the database before boot; supports `--file`, `--passphrase`, `--identity`, `--replace`, and `--dry-run`. The local self-backup now encrypts with age (`EncryptWithPassphrase`), the same portable format used by the embedded snapshots.
- **License:** project is now MIT licensed and publishes a Changelog.

## [0.1.0] - Initial development release

Baseline: automated PostgreSQL backup service with scheduling, retention (GFS), multi-provider storage (local/S3), Discord notifications, Age encryption, restore workflows, and health monitoring.