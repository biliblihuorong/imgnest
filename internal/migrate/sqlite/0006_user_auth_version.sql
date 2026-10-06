-- Persist a monotonic revocation boundary even if account fields change back.
ALTER TABLE users ADD COLUMN auth_version INTEGER NOT NULL DEFAULT 0 CHECK (auth_version >= 0);
