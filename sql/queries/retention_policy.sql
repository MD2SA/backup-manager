-- name: GetRetentionPolicy :one
SELECT *
FROM retention_policies
WHERE id = $1
LIMIT 1;

-- name: ListRetentionPolicies :many
SELECT *
FROM retention_policies
ORDER BY name;

-- name: CreateRetentionPolicy :one
INSERT INTO retention_policies (
    id,
    name,
    keep_hourly,
    keep_daily,
    keep_weekly,
    keep_monthly,
    keep_yearly,
    yearly_month
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8
)
RETURNING *;

-- name: UpdateRetentionPolicy :one
UPDATE retention_policies
SET
    name = $2,
    keep_hourly = $3,
    keep_daily = $4,
    keep_weekly = $5,
    keep_monthly = $6,
    keep_yearly = $7,
    yearly_month = $8,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteRetentionPolicy :exec
DELETE FROM retention_policies
WHERE id = $1;
