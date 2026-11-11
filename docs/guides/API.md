# API Reference

Backup Manager provides a RESTful API for management and monitoring. By default, the API is available at `http://localhost:8080/api/v1`.

## Documentation (Swagger)

The project includes built-in Swagger documentation. To view it:
1. Start the application.
2. Navigate to `http://localhost:8080/swagger/index.html`.

## Key Endpoints

### Health & Monitoring
*   `GET /healthz`: Public liveness check (returns "OK", no API key). Used by the container healthcheck.
*   `GET /readyz`: Public readiness check; pings the metadata database and returns 503 when unavailable.
*   `GET /api/v1/health`: Authenticated health check (returns "OK").
*   `GET /api/v1/health/summary`: Summary of system health and backup status.

### Profiles
*   `GET /profiles`: List all backup profiles.
*   `POST /profiles`: Create a new profile.
*   `GET /profiles/{id}`: Get profile details (includes linked providers).
*   `PUT /profiles/{id}`: Update an existing profile.
*   `DELETE /profiles/{id}`: Delete a profile.
*   `POST /profiles/{id}/run`: Trigger a backup execution immediately (asynchronous).
*   `POST /profiles/{id}/activate`: Enable scheduled backups for this profile.
*   `POST /profiles/{id}/deactivate`: Disable scheduled backups for this profile.
*   `GET /profiles/{id}/executions`: List backup history for a specific profile.

`enabled=true` means the profile is scheduled. Several profiles may be enabled
at the same time, each with its own schedule, storage providers and retention
policy. Triggered backups are queued and executed serially.`

### Executions
*   `GET /executions`: List all backup executions in the system.
*   `GET /executions/{id}`: Get detailed logs and status of a specific execution.
*   `POST /executions/{id}/restore`: Enqueue an asynchronous restore (see below).
*   `GET /executions/{id}/restores`: Restore history for an execution.
*   `POST /executions/{id}/pin`: Pin an execution to prevent automatic cleanup (if retention supports it).

### Restores
*   `GET /restores`: List all restore jobs.
*   `GET /restores/{id}`: Get restore status, logs, provider, checksum verification and errors.

Restores are **asynchronous**. `POST /executions/{id}/restore` returns `202`
immediately with a restore ID; poll `GET /restores/{id}` for progress.

```json
{
  "confirm": true,
  "verify_only": false,
  "provider_id": "optional-storage-provider-uuid",
  "terminate_connections": false
}
```

*   `confirm: true` is required for a real restore. Use `verify_only: true` to
    download, decrypt and verify the artifact without touching the target DB.
*   The target database must be **empty**. A non-empty target returns `409`
    before the job is queued.
*   `terminate_connections: true` allows the restore job to terminate other
    client connections to the target database. Default is `false`.
*   `provider_id` selects a specific stored copy. Without it, the restore tries
    the recorded copies (newest upload first) and falls back to the legacy
    execution path for old backups.
*   The job verifies the recorded SHA-256 checksum before applying anything.
*   `psql` runs with `ON_ERROR_STOP=1` and `--single-transaction`; a failure
    rolls the restore back.
*   Supported artifacts: plain SQL, gzip, PostgreSQL custom format, and Age
    encrypted variants.

### Storage Providers
*   `GET /storage-providers`: List all configured storage providers.
*   `POST /storage-providers`: Add a new storage provider (e.g., S3, Local).
*   `PUT /storage-providers/{id}`: Update storage provider configuration.
*   `DELETE /storage-providers/{id}`: Remove a storage provider.

### Notification Providers
*   `GET /notification-providers`: List all configured notification providers.
*   `POST /notification-providers`: Add a new notification provider (e.g., Discord).
*   `PUT /notification-providers/{id}`: Update notification provider configuration.
*   `DELETE /notification-providers/{id}`: Remove a notification provider.

### Retention Policies
*   `GET /retention-policies`: List all GFS retention policies.
*   `POST /retention-policies`: Define a new retention policy.
*   `PUT /retention-policies/{id}`: Update an existing policy.
*   `DELETE /retention-policies/{id}`: Remove a retention policy.

## Authentication

All `/api/v1` endpoints require the `X-API-Key` header, including
`/api/v1/health`. The public endpoints `/healthz` and `/readyz` are intended for
container and orchestrator probes and do not require a key.

```bash
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/v1/profiles
```

*   Keys belong to users and are stored as SHA-256 hashes. The plaintext key is
    shown only once when created.
*   The service has a **single admin role**: every authenticated user has full
    access. Separate keys provide attribution, rotation and revocation.
*   On first start, `ADMIN_KEY` bootstraps the first admin
    (`admin@localhost`) and remains valid as that admin's API key.
*   Keys can have an expiry (`expires_in_days`) and can be revoked immediately.
*   When `server.env` is `production`, the service **fails to start** without
    an admin key.
*   In development, if no users and no `ADMIN_KEY` exist, the service runs in
    **Insecure Mode** with a warning.

### Users and API keys

*   `GET /auth/me`: current user and key prefix.
*   `GET /auth/keys`: list your keys (metadata only).
*   `POST /auth/keys`: create a key for yourself; returns the plaintext once.
*   `POST /auth/keys/{id}/revoke`: revoke one of your keys.
*   `GET /users`: list users.
*   `POST /users`: create a user (`email`, `name`).
*   `GET /users/{id}`, `PUT /users/{id}`, `DELETE /users/{id}`: read, update or
    deactivate a user. Deactivation revokes all of that user's keys.
*   `GET /users/{id}/keys`: list a user's keys.
*   `POST /users/{id}/keys`: create a key for a user; returns the plaintext once.
*   `POST /users/{id}/keys/{keyID}/revoke`: revoke a user's key.
*   `GET /audit`: global audit log (limit/offset). Mutating requests are recorded
    automatically with user, key, IP, request ID and status.

The last active user cannot be deactivated. Use the CLI for lockout recovery:

```bash
backup-manager user create --config ... --email admin2@example.com --name "Admin 2"
backup-manager key create --config ... --email admin2@example.com --name recovery
```

### Idempotency

`POST /profiles/{id}/run` and `POST /executions/{id}/restore` accept an optional
`X-Idempotency-Key` header. Replaying the same method + path + authenticated
user + key within 24 hours returns the original response without re-triggering
the operation. Keys are scoped per user.

### Request Correlation

Every response includes an `X-Request-ID` header. If you send your own
`X-Request-ID`, it is echoed back; otherwise a unique ID is generated. Reference
it when reporting issues.

### Provider Secrets

Storage provider (`secret_key`, `access_key`) and notification provider
(`webhook_url`) values are **masked** in API responses. When updating a provider,
send the masked placeholder (`********`) in fields you want unchanged; the stored
value is preserved.
