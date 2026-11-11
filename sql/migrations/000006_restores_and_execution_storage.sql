-- +goose Up

-- Per-execution storage copies. An execution may be uploaded to several
-- providers, each with its own object key.
CREATE TABLE execution_storage (
    execution_id UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    provider_id  UUID NOT NULL REFERENCES storage_providers(id) ON DELETE CASCADE,
    storage_path TEXT NOT NULL,
    size         BIGINT NOT NULL DEFAULT 0,
    uploaded_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (execution_id, provider_id)
);

CREATE INDEX execution_storage_provider_idx
    ON execution_storage (provider_id);

-- Asynchronous restore jobs. Restores are deliberately separate from
-- executions: a backup can be restored many times.
CREATE TABLE restores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    status TEXT NOT NULL
        CHECK (status IN ('pending', 'running', 'success', 'failed')),
    request_id TEXT,
    provider_id UUID REFERENCES storage_providers(id) ON DELETE SET NULL,
    storage_path TEXT,
    target_db TEXT,
    options JSONB NOT NULL DEFAULT '{}',
    checksum_verified BOOLEAN NOT NULL DEFAULT false,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration INTERVAL,
    error_message TEXT,
    logs JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX restores_execution_idx
    ON restores (execution_id, created_at DESC);

CREATE INDEX restores_status_idx
    ON restores (status);

-- At most one active restore per execution, even with concurrent requests.
CREATE UNIQUE INDEX restores_one_active_per_execution
    ON restores (execution_id)
    WHERE status IN ('pending', 'running');

-- +goose Down
DROP TABLE restores;
DROP TABLE execution_storage;
