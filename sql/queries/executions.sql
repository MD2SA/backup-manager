-- name: GetExecution :one
SELECT *
FROM executions
WHERE id = $1
LIMIT 1;

-- name: GetLatestExecution :one
SELECT *
FROM executions
WHERE profile_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListExecutionsByProfile :many
SELECT *
FROM executions
WHERE profile_id = $1
ORDER BY created_at DESC;

-- name: ListAllExecutions :many
SELECT *
FROM executions
ORDER BY created_at DESC;

-- name: CreateExecution :one
INSERT INTO executions (
    profile_id,
    status,
    created_at
) VALUES (
    $1,
    $2,
    now()
)
RETURNING *;

-- name: UpdateExecution :one
UPDATE executions
SET
    status = $2,
    start_time = $3,
    end_time = $4,
    duration = $5,
    size = $6,
    checksum = $7,
    storage_path = $8,
    logs = $9,
    error_message = $10,
    is_pinned = $11,
    is_encrypted = $12
WHERE id = $1
RETURNING *;

-- name: DeleteExecution :exec
DELETE FROM executions
WHERE id = $1;

-- name: SetExecutionPinned :exec
UPDATE executions
SET is_pinned = $2
WHERE id = $1;

-- name: ListExecutionsPage :many
SELECT *
FROM executions
WHERE (sqlc.arg(cursor_time)::timestamptz IS NULL OR (created_at, id) < (sqlc.arg(cursor_time), sqlc.arg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListExecutionsByProfilePage :many
SELECT *
FROM executions
WHERE profile_id = sqlc.arg(profile_id)
  AND (sqlc.arg(cursor_time)::timestamptz IS NULL OR (created_at, id) < (sqlc.arg(cursor_time), sqlc.arg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ReconcileInterruptedBackups :execrows
UPDATE executions
SET
    status = 'failed',
    end_time = now(),
    duration = CASE WHEN start_time IS NULL THEN NULL ELSE now() - start_time END,
    error_message = COALESCE(error_message, 'interrupted by restart')
WHERE status IN ('pending', 'running');
