-- +goose Up

-- Every authenticated user has the single admin role. The users table exists
-- for identity, key ownership, revocation and audit, not for privilege levels.
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX api_keys_user_idx ON api_keys (user_id);
CREATE INDEX api_keys_hash_idx ON api_keys (key_hash);

CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    api_key_id UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    request_id TEXT,
    ip TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_user_idx ON audit_log (user_id, created_at DESC);
CREATE INDEX audit_log_action_idx ON audit_log (action, created_at DESC);

-- Attribute restore jobs to the user/key that requested them.
ALTER TABLE restores
    ADD COLUMN requested_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN requested_by_key_id UUID REFERENCES api_keys(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE restores
    DROP COLUMN requested_by_key_id,
    DROP COLUMN requested_by_user_id;

DROP TABLE audit_log;
DROP TABLE api_keys;
DROP TABLE users;
