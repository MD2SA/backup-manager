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
    retention_policy_id,
    compression_type,
    compression_level,
    encryption_enabled,
    scheduled_since
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,
    CASE WHEN $3 THEN now() ELSE NULL END
)
RETURNING *;

-- name: UpdateProfile :one
UPDATE profiles
SET
    name = $2,
    description = $3,
    enabled = $4,
    schedule = $5,
    retention_policy_id = $6,
    compression_type = $7,
    compression_level = $8,
    encryption_enabled = $9,
    scheduled_since = CASE
        WHEN $4 AND (NOT enabled OR schedule <> $5) THEN now()
        WHEN $4 THEN scheduled_since
        ELSE NULL
    END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteProfile :exec
DELETE FROM profiles
WHERE id = $1;

-- name: SetProfileEnabled :one
UPDATE profiles
SET
    enabled = $2,
    scheduled_since = CASE WHEN $2 THEN now() ELSE NULL END,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetProfileStorageProviderIDs :many
SELECT storage_provider_id FROM profile_storage_providers
WHERE profile_id = $1;

-- name: GetProfileNotificationProviderIDs :many
SELECT notification_provider_id FROM profile_notification_providers
WHERE profile_id = $1;

-- name: AddProfileStorageProvider :exec
INSERT INTO profile_storage_providers (profile_id, storage_provider_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: AddProfileNotificationProvider :exec
INSERT INTO profile_notification_providers (profile_id, notification_provider_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ClearProfileStorageProviders :exec
DELETE FROM profile_storage_providers
WHERE profile_id = $1;

-- name: ClearProfileNotificationProviders :exec
DELETE FROM profile_notification_providers
WHERE profile_id = $1;

-- name: ListProfilesPage :many
SELECT *
FROM profiles
WHERE (sqlc.narg(cursor_name)::text IS NULL OR (name, id) > (sqlc.arg(cursor_name), sqlc.arg(cursor_id)::uuid))
ORDER BY name ASC, id ASC
LIMIT sqlc.arg(page_limit);
