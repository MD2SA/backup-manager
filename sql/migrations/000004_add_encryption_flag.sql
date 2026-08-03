-- +goose Up
ALTER TABLE executions ADD COLUMN is_encrypted BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE executions DROP COLUMN is_encrypted;
