CREATE TABLE plugin_user_settings (
    plugin TEXT NOT NULL CHECK (length(plugin) BETWEEN 1 AND 32),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(value) AND json_type(value) = 'object' AND length(value) <= 65536),
    asset BLOB,
    asset_mime TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin, user_id),
    CHECK (asset IS NULL OR length(asset) <= 4194304),
    CHECK ((asset IS NULL AND asset_mime = '') OR (asset IS NOT NULL AND asset_mime IN ('image/png', 'image/jpeg', 'image/webp')))
);
CREATE INDEX plugin_user_settings_user ON plugin_user_settings (user_id);
