-- Durable counters extension plugins keep, such as uploads per user per month.
-- Rows untouched for 400 days are pruned at startup.
CREATE TABLE plugin_counters (
    plugin TEXT NOT NULL CHECK (length(plugin) BETWEEN 1 AND 32),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 80),
    value BIGINT NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin, name, subject)
);
CREATE INDEX plugin_counters_updated_at ON plugin_counters (updated_at);
