-- +goose Up
CREATE TABLE IF NOT EXISTS data_config (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    working_dir VARCHAR(512) NOT NULL,
    config LONGTEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    updated_at BIGINT NOT NULL DEFAULT (UNIX_TIMESTAMP()),
    UNIQUE KEY uq_data_config_working_dir (working_dir),
    KEY idx_data_config_working_dir (working_dir)
);

CREATE TRIGGER update_data_config_updated_at
BEFORE UPDATE ON data_config
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

-- +goose Down
DROP TRIGGER IF EXISTS update_data_config_updated_at;
DROP TABLE IF EXISTS data_config;
