-- name: GetNotificationProvider :one
SELECT *
FROM notification_providers
WHERE id = $1
LIMIT 1;

-- name: ListNotificationProviders :many
SELECT *
FROM notification_providers
ORDER BY name;

-- name: CreateNotificationProvider :one
INSERT INTO notification_providers (
    id,
    name,
    type,
    config
) VALUES (
    $1,$2,$3,$4
)
RETURNING *;

-- name: UpdateNotificationProvider :one
UPDATE notification_providers
SET
    name = $2,
    type = $3,
    config = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteNotificationProvider :exec
DELETE FROM notification_providers
WHERE id = $1;
