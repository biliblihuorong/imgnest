-- Optional self-chosen profile name; empty means "fall back to the username".
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
