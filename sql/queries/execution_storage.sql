-- name: AddExecutionStorage :exec
INSERT INTO execution_storage (
    execution_id,
    provider_id,
    storage_path,
    size
) VALUES (
    $1,$2,$3,$4
)
ON CONFLICT (execution_id, provider_id) DO UPDATE
SET
    storage_path = EXCLUDED.storage_path,
    size = EXCLUDED.size,
    uploaded_at = now();

-- name: ListExecutionStorage :many
SELECT *
FROM execution_storage
WHERE execution_id = $1
ORDER BY uploaded_at ASC, provider_id ASC;

-- name: GetExecutionStorageForProvider :one
SELECT *
FROM execution_storage
WHERE execution_id = $1
  AND provider_id = $2
LIMIT 1;

-- name: DeleteExecutionStorage :exec
DELETE FROM execution_storage
WHERE execution_id = $1
  AND provider_id = $2;

-- name: CountExecutionStorage :one
SELECT count(*)
FROM execution_storage
WHERE execution_id = $1;

-- name: ListExecutionsByProfileForRetention :many
SELECT e.*
FROM executions e
WHERE e.profile_id = $1
  AND NOT EXISTS (
      SELECT 1
      FROM restores r
      WHERE r.execution_id = e.id
        AND r.status IN ('pending', 'running')
  )
ORDER BY e.created_at DESC;

-- name: DeleteExecutionIfNoStorage :execrows
DELETE FROM executions e
WHERE e.id = $1
  AND NOT EXISTS (
      SELECT 1 FROM execution_storage es WHERE es.execution_id = e.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM restores r
      WHERE r.execution_id = e.id
        AND r.status IN ('pending', 'running')
  );
