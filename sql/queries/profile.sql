-- name: GetProfile :one
SELECT * FROM profiles
WHERE id = $1
LIMIT 1;

-- name: ListProfiles :many
SELECT * FROM profiles
ORDER BY name;

-- name: CreateProfile :one
INSERT INTO profiles (
    name,
    description,
    enabled,
    schedule,
    storage_provider_id,
    notification_provider_id,
    retention_policy_id,
    compression_type,
    compression_level
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9
)
RETURNING *;

-- name: UpdateProfile :one
UPDATE profiles
SET
    name = $2,
    description = $3,
    enabled = $4,
    schedule = $5,
    storage_provider_id = $6,
    notification_provider_id = $7,
    retention_policy_id = $8,
    compression_type = $9,
    compression_level = $10,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProfile :exec
DELETE FROM profiles
WHERE id = $1;

-- name: ActivateProfile :exec
UPDATE profiles
SET enabled = CASE
    WHEN id = $1 THEN true
    ELSE false
END;
