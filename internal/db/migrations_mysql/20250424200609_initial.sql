-- +goose Up
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(36) PRIMARY KEY,
    parent_session_id VARCHAR(36) NULL,
    title TEXT NOT NULL,
    message_count BIGINT NOT NULL DEFAULT 0,
    prompt_tokens BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cost DOUBLE NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    summary_message_id VARCHAR(36) NULL,
    todos LONGTEXT NULL
);

CREATE TABLE IF NOT EXISTS files (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    path VARCHAR(512) NOT NULL,
    content LONGTEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT fk_files_session FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    UNIQUE KEY uq_files_path_session_version (path, session_id, version),
    KEY idx_files_session_id (session_id),
    KEY idx_files_path (path),
    KEY idx_files_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS messages (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    role VARCHAR(64) NOT NULL,
    parts LONGTEXT NOT NULL,
    model TEXT NULL,
    provider VARCHAR(255) NULL,
    is_summary_message BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    finished_at BIGINT NULL,
    CONSTRAINT fk_messages_session FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    KEY idx_messages_session_id (session_id),
    KEY idx_messages_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS read_files (
    session_id VARCHAR(36) NOT NULL,
    path VARCHAR(512) NOT NULL,
    read_at BIGINT NOT NULL,
    CONSTRAINT fk_read_files_session FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    PRIMARY KEY (path, session_id),
    KEY idx_read_files_session_id (session_id),
    KEY idx_read_files_path (path)
);

CREATE INDEX idx_sessions_created_at ON sessions (created_at);

CREATE TRIGGER update_sessions_updated_at
BEFORE UPDATE ON sessions
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

CREATE TRIGGER update_files_updated_at
BEFORE UPDATE ON files
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

CREATE TRIGGER update_messages_updated_at
BEFORE UPDATE ON messages
FOR EACH ROW
SET NEW.updated_at = UNIX_TIMESTAMP();

CREATE TRIGGER update_session_message_count_on_insert
AFTER INSERT ON messages
FOR EACH ROW
UPDATE sessions
SET message_count = message_count + 1
WHERE id = NEW.session_id;

CREATE TRIGGER update_session_message_count_on_delete
AFTER DELETE ON messages
FOR EACH ROW
UPDATE sessions
SET message_count = GREATEST(message_count - 1, 0)
WHERE id = OLD.session_id;

-- +goose Down
DROP TRIGGER IF EXISTS update_session_message_count_on_delete;
DROP TRIGGER IF EXISTS update_session_message_count_on_insert;
DROP TRIGGER IF EXISTS update_messages_updated_at;
DROP TRIGGER IF EXISTS update_files_updated_at;
DROP TRIGGER IF EXISTS update_sessions_updated_at;
DROP TABLE IF EXISTS read_files;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS sessions;
