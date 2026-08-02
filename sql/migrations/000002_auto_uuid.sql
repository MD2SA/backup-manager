-- +goose Up
ALTER TABLE storage_providers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE notification_providers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE retention_policies ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE profiles ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE executions ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- +goose Down
ALTER TABLE executions ALTER COLUMN id DROP DEFAULT;
ALTER TABLE profiles ALTER COLUMN id DROP DEFAULT;
ALTER TABLE retention_policies ALTER COLUMN id DROP DEFAULT;
ALTER TABLE notification_providers ALTER COLUMN id DROP DEFAULT;
ALTER TABLE storage_providers ALTER COLUMN id DROP DEFAULT;
