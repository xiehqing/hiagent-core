-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    api_endpoint TEXT,
    api_key TEXT,
    default_large_model_id TEXT,
    default_small_model_id TEXT,
    default_headers TEXT,
    disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE TABLE IF NOT EXISTS big_models (
    provider_id TEXT NOT NULL,
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    cost_per_1m_in REAL NOT NULL DEFAULT 0,
    cost_per_1m_out REAL NOT NULL DEFAULT 0,
    cost_per_1m_in_cached REAL NOT NULL DEFAULT 0,
    cost_per_1m_out_cached REAL NOT NULL DEFAULT 0,
    context_window INTEGER NOT NULL DEFAULT 0,
    default_max_tokens INTEGER NOT NULL DEFAULT 0,
    can_reason INTEGER NOT NULL DEFAULT 0 CHECK (can_reason IN (0, 1)),
    reasoning_levels TEXT,
    default_reasoning_effort TEXT,
    supports_images INTEGER NOT NULL DEFAULT 0 CHECK (supports_images IN (0, 1)),
    disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (provider_id, id),
    FOREIGN KEY (provider_id) REFERENCES providers (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_providers_sort_order
ON providers (sort_order, id);

CREATE INDEX IF NOT EXISTS idx_big_models_provider_sort_order
ON big_models (provider_id, sort_order, id);

CREATE TRIGGER IF NOT EXISTS update_providers_updated_at
AFTER UPDATE ON providers
BEGIN
UPDATE providers SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS update_big_models_updated_at
AFTER UPDATE ON big_models
BEGIN
UPDATE big_models SET updated_at = strftime('%s', 'now')
WHERE provider_id = new.provider_id AND id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_big_models_updated_at;
DROP TRIGGER IF EXISTS update_providers_updated_at;
DROP TABLE IF EXISTS big_models;
DROP TABLE IF EXISTS providers;
-- +goose StatementEnd
