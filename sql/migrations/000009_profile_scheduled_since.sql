-- +goose Up

ALTER TABLE profiles ADD COLUMN scheduled_since TIMESTAMPTZ;

UPDATE profiles
SET scheduled_since = COALESCE(updated_at, created_at, now())
WHERE enabled;

UPDATE profiles
SET scheduled_since = NULL
WHERE NOT enabled;

-- +goose Down
ALTER TABLE profiles DROP COLUMN scheduled_since;
