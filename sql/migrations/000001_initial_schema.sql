-- +goose Up

CREATE TABLE storage_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- local, s3, gdrive
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- discord, email, etc.
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE retention_policies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    keep_hourly INT NOT NULL DEFAULT 0,
    keep_daily INT NOT NULL DEFAULT 0,
    keep_weekly INT NOT NULL DEFAULT 0,
    keep_monthly INT NOT NULL DEFAULT 0,
    keep_yearly INT NOT NULL DEFAULT 0,
    yearly_month INT NOT NULL DEFAULT 1, -- 1-12
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    schedule TEXT NOT NULL, -- cron expression or interval

    storage_provider_id UUID REFERENCES storage_providers(id) ON DELETE SET NULL,
    notification_provider_id UUID REFERENCES notification_providers(id) ON DELETE SET NULL,
    retention_policy_id UUID REFERENCES retention_policies(id) ON DELETE SET NULL,

    compression_type TEXT NOT NULL,
    compression_level INT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE executions (
    id UUID PRIMARY KEY,
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    status TEXT NOT NULL, -- pending, running, success, failed

    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    duration INTERVAL,

    size BIGINT,
    checksum TEXT,
    storage_path TEXT,

    logs JSONB, -- store structured logs or metadata about stages
    error_message TEXT,

    is_pinned BOOLEAN NOT NULL DEFAULT false,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE executions;
DROP TABLE profiles;
DROP TABLE retention_policies;
DROP TABLE notification_providers;
DROP TABLE storage_providers;
