-- name: CreateUser :one
INSERT INTO users (email, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetUser :one
SELECT *
FROM users
WHERE id = $1
LIMIT 1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1
LIMIT 1;

-- name: ListUsers :many
SELECT *
FROM users
ORDER BY created_at ASC;

-- name: UpdateUser :one
UPDATE users
SET
    email = $2,
    name = $3,
    active = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CountActiveUsers :one
SELECT count(*) FROM users WHERE active;

-- name: DeactivateUser :one
UPDATE users
SET active = false, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListUsersPage :many
SELECT *
FROM users
WHERE (sqlc.arg(cursor_time)::timestamptz IS NULL OR (created_at, id) > (sqlc.arg(cursor_time), sqlc.arg(cursor_id)::uuid))
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg(page_limit);
