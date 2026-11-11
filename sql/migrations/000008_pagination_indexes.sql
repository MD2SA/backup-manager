-- +goose Up

-- Keyset pagination indexes. The trailing id column is the stable tie-breaker.
CREATE INDEX executions_created_id_idx
    ON executions (created_at DESC, id DESC);
CREATE INDEX executions_profile_created_id_idx
    ON executions (profile_id, created_at DESC, id DESC);

CREATE INDEX restores_created_id_idx
    ON restores (created_at DESC, id DESC);
CREATE INDEX restores_execution_created_id_idx
    ON restores (execution_id, created_at DESC, id DESC);

CREATE INDEX audit_log_created_id_idx
    ON audit_log (created_at DESC, id DESC);

CREATE INDEX users_created_id_idx
    ON users (created_at ASC, id ASC);

CREATE INDEX profiles_name_id_idx
    ON profiles (name ASC, id ASC);
CREATE INDEX storage_providers_name_id_idx
    ON storage_providers (name ASC, id ASC);
CREATE INDEX notification_providers_name_id_idx
    ON notification_providers (name ASC, id ASC);
CREATE INDEX retention_policies_name_id_idx
    ON retention_policies (name ASC, id ASC);

CREATE INDEX api_keys_user_created_id_idx
    ON api_keys (user_id, created_at ASC, id ASC);

-- +goose Down
DROP INDEX api_keys_user_created_id_idx;
DROP INDEX retention_policies_name_id_idx;
DROP INDEX notification_providers_name_id_idx;
DROP INDEX storage_providers_name_id_idx;
DROP INDEX profiles_name_id_idx;
DROP INDEX users_created_id_idx;
DROP INDEX audit_log_created_id_idx;
DROP INDEX restores_execution_created_id_idx;
DROP INDEX restores_created_id_idx;
DROP INDEX executions_profile_created_id_idx;
DROP INDEX executions_created_id_idx;
