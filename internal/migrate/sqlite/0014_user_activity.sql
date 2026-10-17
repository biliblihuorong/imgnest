-- Account activity: when a web session was last active, the most recent
-- sign-ins, and how much each API token is used. Request counts are buffered
-- in memory and written at most once per minute per token.
ALTER TABLE users ADD COLUMN last_seen_at DATETIME;
ALTER TABLE tokens ADD COLUMN last_used_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE tokens ADD COLUMN use_count BIGINT NOT NULL DEFAULT 0 CHECK (use_count >= 0);

-- Only the newest sign-ins per account are kept; older rows are pruned on insert.
CREATE TABLE login_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX login_events_user_id ON login_events (user_id, id);

-- Per-account daily totals of API-token requests (UTC days, 'YYYY-MM-DD'),
-- kept when a token is revoked so the history stays complete.
CREATE TABLE api_usage_daily (
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day TEXT NOT NULL CHECK (length(day) = 10),
    requests BIGINT NOT NULL DEFAULT 0 CHECK (requests >= 0),
    PRIMARY KEY (user_id, day)
);
