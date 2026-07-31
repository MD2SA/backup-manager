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

-- name: CreateExecution :one
INSERT INTO executions (
    id,
    profile_id,
    status,
    created_at
) VALUES (
    $1,
    $2,
    $3,
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
    is_pinned = $11
WHERE id = $1
RETURNING *;

-- name: DeleteExecution :exec
DELETE FROM executions
WHERE id = $1;

-- name: SetExecutionPinned :exec
UPDATE executions
SET is_pinned = $2
WHERE id = $1;
