-- name: CreateAuditEntry :one
INSERT INTO audit_log (
    user_id,
    api_key_id,
    action,
    resource_type,
    resource_id,
    request_id,
    ip,
    metadata
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8
)
RETURNING *;

-- name: ListAuditEntries :many
SELECT *
FROM audit_log
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
