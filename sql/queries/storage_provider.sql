-- name: GetStorageProvider :one
SELECT *
FROM storage_providers
WHERE id = $1
LIMIT 1;

-- name: ListStorageProviders :many
SELECT *
FROM storage_providers
ORDER BY name;

-- name: CreateStorageProvider :one
INSERT INTO storage_providers (
    id,
    name,
    type,
    config
) VALUES (
    $1,$2,$3,$4
)
RETURNING *;

-- name: UpdateStorageProvider :one
UPDATE storage_providers
SET
    name = $2,
    type = $3,
    config = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteStorageProvider :exec
DELETE FROM storage_providers
WHERE id = $1;
