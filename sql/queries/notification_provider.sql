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
    name,
    type,
    config
) VALUES (
    $1,$2,$3
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

-- name: ListNotificationProvidersPage :many
SELECT *
FROM notification_providers
WHERE (sqlc.narg(cursor_name)::text IS NULL OR (name, id) > (sqlc.arg(cursor_name), sqlc.arg(cursor_id)::uuid))
ORDER BY name ASC, id ASC
LIMIT sqlc.arg(page_limit);
