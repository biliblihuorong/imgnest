-- One-time email codes for registration and password reset. A newer request
-- for the same purpose and address replaces the older code; only a hash of the
-- code is stored.
CREATE TABLE email_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    purpose TEXT NOT NULL CHECK (purpose IN ('register', 'reset_password')),
    email TEXT NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    code_hash TEXT NOT NULL CHECK (length(code_hash) = 64),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE (purpose, email)
);
CREATE INDEX email_codes_expires_at ON email_codes (expires_at);
