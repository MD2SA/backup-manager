-- name: CreateAPIKey :one
INSERT INTO api_keys (
    user_id,
    name,
    key_prefix,
    key_hash,
    expires_at
) VALUES (
    $1,$2,$3,$4,$5
)
RETURNING *;

-- name: GetAPIKey :one
SELECT *
FROM api_keys
WHERE id = $1
LIMIT 1;

-- name: GetAPIKeyByHash :one
SELECT *
FROM api_keys
WHERE key_hash = $1
LIMIT 1;

-- name: ListAPIKeysByUser :many
SELECT *
FROM api_keys
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: ListAPIKeys :many
SELECT *
FROM api_keys
ORDER BY created_at DESC;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = now()
WHERE id = $1
  AND revoked_at IS NULL
RETURNING *;

-- name: RevokeAllUserAPIKeys :exec
UPDATE api_keys
SET revoked_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys
SET last_used_at = now()
WHERE id = $1
  AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');
