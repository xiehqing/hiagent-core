-- +goose Up
CREATE TABLE IF NOT EXISTS providers (
    id VARCHAR(128) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL,
    api_endpoint TEXT NULL,
    api_key TEXT NULL,
    default_large_model_id VARCHAR(255) NULL,
    default_small_model_id VARCHAR(255) NULL,
    default_headers LONGTEXT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    updated_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    KEY idx_providers_sort_order (sort_order, id)
);

CREATE TABLE IF NOT EXISTS big_models (
    provider_id VARCHAR(128) NOT NULL,
    id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    cost_per_1m_in DOUBLE NOT NULL DEFAULT 0,
    cost_per_1m_out DOUBLE NOT NULL DEFAULT 0,
    cost_per_1m_in_cached DOUBLE NOT NULL DEFAULT 0,
    cost_per_1m_out_cached DOUBLE NOT NULL DEFAULT 0,
    context_window BIGINT NOT NULL DEFAULT 0,
    default_max_tokens BIGINT NOT NULL DEFAULT 0,
    can_reason BOOLEAN NOT NULL DEFAULT FALSE,
    reasoning_levels LONGTEXT NULL,
    default_reasoning_effort VARCHAR(64) NULL,
    supports_images BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    updated_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    PRIMARY KEY (provider_id, id),
    CONSTRAINT fk_big_models_provider
        FOREIGN KEY (provider_id) REFERENCES providers (id) ON DELETE CASCADE,
    KEY idx_big_models_provider_sort_order (provider_id, sort_order, id)
);

CREATE TRIGGER update_providers_updated_at
BEFORE UPDATE ON providers
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

CREATE TRIGGER update_big_models_updated_at
BEFORE UPDATE ON big_models
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

-- +goose Down
DROP TRIGGER IF EXISTS update_big_models_updated_at;
DROP TRIGGER IF EXISTS update_providers_updated_at;
DROP TABLE IF EXISTS big_models;
DROP TABLE IF EXISTS providers;
