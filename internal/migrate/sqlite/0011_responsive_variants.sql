-- Responsive image variants: an opt-in per-rule switch, the stored variants
-- of each image, a write journal for background generation and the single
-- resumable progress row of the historic backfill.
ALTER TABLE policies ADD COLUMN responsive_variants BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE images ADD COLUMN variants TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(variants) AND json_type(variants) = 'array');

CREATE TABLE image_variant_jobs (
    image_id BIGINT PRIMARY KEY REFERENCES images(id) ON DELETE CASCADE,
    image_key TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    objects TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(objects) AND json_type(objects) = 'array'),
    variants TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(variants) AND json_type(variants) = 'array'),
    bytes BIGINT NOT NULL DEFAULT 0 CHECK (bytes >= 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE variant_backfill (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'running', 'paused', 'done')),
    last_id BIGINT NOT NULL DEFAULT 0 CHECK (last_id >= 0),
    processed BIGINT NOT NULL DEFAULT 0,
    generated BIGINT NOT NULL DEFAULT 0,
    skipped BIGINT NOT NULL DEFAULT 0,
    failed BIGINT NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO variant_backfill (id) VALUES (1);
