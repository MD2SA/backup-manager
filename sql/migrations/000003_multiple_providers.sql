-- +goose Up

CREATE TABLE profile_storage_providers (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    storage_provider_id UUID NOT NULL REFERENCES storage_providers(id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, storage_provider_id)
);

CREATE TABLE profile_notification_providers (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    notification_provider_id UUID NOT NULL REFERENCES notification_providers(id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, notification_provider_id)
);

-- Migrate existing data
INSERT INTO profile_storage_providers (profile_id, storage_provider_id)
SELECT id, storage_provider_id FROM profiles WHERE storage_provider_id IS NOT NULL;

INSERT INTO profile_notification_providers (profile_id, notification_provider_id)
SELECT id, notification_provider_id FROM profiles WHERE notification_provider_id IS NOT NULL;

-- Drop old columns
ALTER TABLE profiles DROP COLUMN storage_provider_id;
ALTER TABLE profiles DROP COLUMN notification_provider_id;

-- +goose Down
ALTER TABLE profiles ADD COLUMN storage_provider_id UUID REFERENCES storage_providers(id) ON DELETE SET NULL;
ALTER TABLE profiles ADD COLUMN notification_provider_id UUID REFERENCES notification_providers(id) ON DELETE SET NULL;

-- Restore data (optional/best effort if possible, but many-to-many to one-to-one is lossy)
UPDATE profiles p
SET storage_provider_id = (SELECT storage_provider_id FROM profile_storage_providers WHERE profile_id = p.id LIMIT 1),
    notification_provider_id = (SELECT notification_provider_id FROM profile_notification_providers WHERE profile_id = p.id LIMIT 1);

DROP TABLE profile_notification_providers;
DROP TABLE profile_storage_providers;
