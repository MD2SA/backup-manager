-- +goose Up
ALTER TABLE profiles ADD COLUMN encryption_enabled BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE profiles DROP COLUMN encryption_enabled;
