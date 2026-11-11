# Operations Runbook

This guide covers day-to-day operation, monitoring, upgrades and recovery.

## Health and readiness

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Public liveness probe. |
| `GET /readyz` | Public readiness probe; pings the metadata database. |
| `GET /version` | Public build version, commit and Go runtime. |
| `GET /api/v1/health` | Authenticated simple health check. |
| `GET /api/v1/health/summary` | Active/overdue profiles, next scheduled run and last execution status. |

`health/summary` returns a `scheduled_profiles` array. A profile is overdue when
its last successful backup is older than:

```text
maxGap(schedule) * health.overdue_factor + health.overdue_grace
```

Defaults: factor `2.0`, grace `15m`. A newly enabled profile is not marked
overdue until one expected interval plus grace has passed.

## Metrics

Prometheus scrapes `GET /api/v1/metrics` with the API key:

```yaml
scrape_configs:
  - job_name: backup-manager
    metrics_path: /api/v1/metrics
    static_configs:
      - targets: ["backup-manager:8080"]
    authorization:
      type: APIKey
      credentials: <admin-key>
```

Important series:

- `backup_manager_backups_total{profile_id,profile_name,status}`
- `backup_manager_backup_duration_seconds`
- `backup_manager_backup_bytes`
- `backup_manager_last_success_timestamp_seconds`
- `backup_manager_restores_total{status}`
- `backup_manager_restore_duration_seconds`
- `backup_manager_queue_depth`
- `backup_manager_scheduler_jobs`
- `backup_manager_active_profiles`
- `backup_manager_overdue_profiles`
- `backup_manager_auth_failures_total`
- `backup_manager_http_requests_total{method,route,status}`
- `backup_manager_audit_deleted_total`

### Alert examples

```yaml
groups:
  - name: backup-manager
    rules:
      - alert: BackupManagerOverdue
        expr: backup_manager_overdue_profiles > 0
        for: 10m
        labels: { severity: warning }
        annotations:
          summary: "{{ $value }} perfil(es) de backup overdue"

      - alert: BackupManagerBackupFailed
        expr: increase(backup_manager_backups_total{status="failed"}[1h]) > 0
        for: 5m
        labels: { severity: critical }

      - alert: BackupManagerRestoreFailed
        expr: increase(backup_manager_restores_total{status="failed"}[1h]) > 0
        for: 5m
        labels: { severity: critical }

      - alert: BackupManagerAuthFailures
        expr: increase(backup_manager_auth_failures_total[5m]) > 20
        for: 5m
        labels: { severity: warning }
```

## Audit retention

Configured under `audit`:

```yaml
audit:
  retention_days: 90
  cleanup_schedule: "0 4 * * *"
```

The cleanup runs once at startup and then on the cron. Set
`retention_days: 0` to disable it.

## Backups and restores

- Each enabled profile runs its own `pg_dump` and has its own storage and
  retention policy.
- Backups are queued and executed serially.
- Restores are asynchronous and require an empty target database.
- Use `verify_only` to validate an artifact without touching the target.
- Test a real restore periodically; a backup that has never been restored is
  not a proven backup.

## API key rotation

1. Create a new key for the user (`POST /users/{id}/keys` or `POST /auth/keys`).
2. Update the client to use the new key.
3. Revoke the old key.
4. Deactivate users that no longer need access.

`ADMIN_KEY` bootstraps `admin@localhost` on first start. It can be revoked like
any other key. For lockout recovery:

```sh
backup-manager user create --config /etc/backup-manager/config.yaml \
  --email admin2@example.com --name "Admin 2"
backup-manager key create --config /etc/backup-manager/config.yaml \
  --email admin2@example.com --name recovery
```

## Upgrade

1. Back up the metadata database (or rely on the embedded metadata snapshots).
2. Pull the new image/tag.
3. Restart the service; migrations run automatically with a 60s wait.
4. Run `backup-manager config check --connect` before or after rollout.
5. Verify `/readyz`, `/version` and a fresh execution.

Rollback to a previous image is safe while the schema remains compatible; newer
tables are ignored by older binaries. Restoring a metadata snapshot requires
`metadata-restore` before boot.

## Incident checklist

1. Check `/readyz` and `/api/v1/health/summary`.
2. Inspect failed executions and restore jobs.
3. Check `/api/v1/metrics` for failures, overdue and queue depth.
4. Verify storage providers and target database connectivity.
5. For a failed restore, use `verify_only` first and inspect `GET /restores/{id}`.
6. Use the audit log for who triggered the change.

## Limits

- The scheduler is in-memory and assumes one service instance. Multiple replicas
  would schedule the same profiles twice.
- Restores and backups share a target database gate and never overlap.
- The target database must be empty for a restore.
