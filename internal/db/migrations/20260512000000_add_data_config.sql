-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS data_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    working_dir TEXT NOT NULL,
    config TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    UNIQUE(working_dir)
);

CREATE INDEX IF NOT EXISTS idx_data_config_working_dir
ON data_config (working_dir);

CREATE TRIGGER IF NOT EXISTS update_data_config_updated_at
AFTER UPDATE ON data_config
BEGIN
UPDATE data_config SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_data_config_updated_at;
DROP TABLE IF EXISTS data_config;
-- +goose StatementEnd
