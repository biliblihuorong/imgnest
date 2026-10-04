-- 0003: Lsky v1 compatibility and admin groundwork. Additive only.
ALTER TABLE users ADD COLUMN registered_ip TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('api_enabled', 'true'),
    ('site_name', '"ImgNest"'),
    ('guest_group_id', '0');

-- Guest anchor row: guest uploads store images.user_id = 0, which the users
-- foreign key requires to exist. The account can never authenticate (disabled
-- status, unusable password hash) and must never be shown or managed as one.
INSERT OR IGNORE INTO users (id, username, email, password_hash, role, group_id, status, used_bytes, registered_ip)
SELECT 0, 'guest', 'guest@imgnest.invalid', '!guest-anchor-disabled', 'user', g.id, 'disabled', 0, ''
FROM groups g
WHERE g.is_default = true;
