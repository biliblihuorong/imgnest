CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at DATETIME NOT NULL
);

CREATE TABLE groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_guest BOOLEAN NOT NULL DEFAULT false,
    capacity_bytes BIGINT NOT NULL DEFAULT 0 CHECK (capacity_bytes >= 0),
    max_file_bytes BIGINT NOT NULL DEFAULT 0 CHECK (max_file_bytes >= 0),
    allowed_exts TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(allowed_exts)),
    upload_per_min INTEGER NOT NULL DEFAULT 0 CHECK (upload_per_min >= 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX groups_one_default ON groups (is_default) WHERE is_default = true;

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    group_id BIGINT NOT NULL REFERENCES groups (id),
    status TEXT NOT NULL CHECK (status IN ('enabled', 'disabled')),
    used_bytes BIGINT NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX users_group_id ON users (group_id);

CREATE TABLE tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('web', 'api')),
    abilities TEXT NOT NULL CHECK (json_valid(abilities)),
    last_used_at DATETIME,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX tokens_user_id ON tokens (user_id);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL CHECK (json_valid(value)),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO groups (name, is_default, is_guest, capacity_bytes)
VALUES ('Default', true, false, 0);
INSERT INTO settings (key, value) VALUES
    ('registration_enabled', 'false'),
    ('guest_upload_enabled', 'false'),
    ('gallery_enabled', 'false'),
    ('trash_days', '7');
INSERT INTO settings (key, value)
SELECT 'default_group_id', CAST(id AS TEXT) FROM groups WHERE is_default = true;
