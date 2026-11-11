-- name: CreateRestore :one
INSERT INTO restores (
    execution_id,
    status,
    request_id,
    options,
    target_db,
    provider_id,
    storage_path,
    requested_by_user_id,
    requested_by_key_id
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9
)
RETURNING *;

-- name: UpdateRestore :one
UPDATE restores
SET
    status = $2,
    provider_id = $3,
    storage_path = $4,
    checksum_verified = $5,
    started_at = $6,
    finished_at = $7,
    duration = $8,
    error_message = $9,
    logs = $10
WHERE id = $1
RETURNING *;

-- name: GetRestore :one
SELECT *
FROM restores
WHERE id = $1
LIMIT 1;

-- name: ListRestoresByExecution :many
SELECT *
FROM restores
WHERE execution_id = $1
ORDER BY created_at DESC;

-- name: ListRestores :many
SELECT *
FROM restores
ORDER BY created_at DESC;

-- name: GetActiveRestoreByExecution :one
SELECT *
FROM restores
WHERE execution_id = $1
  AND status IN ('pending', 'running')
ORDER BY created_at DESC
LIMIT 1;

-- name: ReconcileInterruptedRestores :execrows
UPDATE restores
SET
    status = 'failed',
    finished_at = now(),
    error_message = COALESCE(error_message, 'interrupted by restart')
WHERE status IN ('pending', 'running');
