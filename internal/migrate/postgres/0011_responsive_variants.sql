-- Responsive image variants: an opt-in per-rule switch, the stored variants
-- of each image, a write journal for background generation and the single
-- resumable progress row of the historic backfill.
ALTER TABLE policies ADD COLUMN responsive_variants BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE images ADD COLUMN variants JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(variants) = 'array');

CREATE TABLE image_variant_jobs (
    image_id BIGINT PRIMARY KEY REFERENCES images(id) ON DELETE CASCADE,
    image_key TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    objects JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(objects) = 'array'),
    variants JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(variants) = 'array'),
    bytes BIGINT NOT NULL DEFAULT 0 CHECK (bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE variant_backfill (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'running', 'paused', 'done')),
    last_id BIGINT NOT NULL DEFAULT 0 CHECK (last_id >= 0),
    processed BIGINT NOT NULL DEFAULT 0,
    generated BIGINT NOT NULL DEFAULT 0,
    skipped BIGINT NOT NULL DEFAULT 0,
    failed BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO variant_backfill (id) VALUES (1);
