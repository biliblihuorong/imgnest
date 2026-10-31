-- Operation audit trail: one row per state-changing API request. Rows keep
-- the account id and name after the account is deleted, so there is no
-- foreign key; entries older than log.audit_days are pruned by the server.
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id BIGINT NOT NULL DEFAULT 0,
    user_name TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL,
    route TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    status INTEGER NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX audit_logs_created_at ON audit_logs (created_at);
CREATE INDEX audit_logs_user_id ON audit_logs (user_id, id);
