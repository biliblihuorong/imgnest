CREATE TABLE plugin_user_settings (
    plugin TEXT NOT NULL CHECK (length(plugin) BETWEEN 1 AND 32),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value TEXT NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(value::jsonb) = 'object' AND octet_length(value) <= 65536),
    asset BYTEA,
    asset_mime TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin, user_id),
    CHECK (asset IS NULL OR octet_length(asset) <= 4194304),
    CHECK ((asset IS NULL AND asset_mime = '') OR (asset IS NOT NULL AND asset_mime IN ('image/png', 'image/jpeg', 'image/webp')))
);
CREATE INDEX plugin_user_settings_user ON plugin_user_settings (user_id);
