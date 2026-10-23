CREATE TABLE billing_wallets (
 user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
 username TEXT NOT NULL,
 balance_cents BIGINT NOT NULL DEFAULT 0 CHECK(balance_cents BETWEEN 0 AND 1000000000000),
 version BIGINT NOT NULL DEFAULT 0 CHECK(version >= 0),
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE billing_plans (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 price_cents BIGINT NOT NULL CHECK(price_cents BETWEEN 1 AND 1000000000000),
 duration_days INTEGER NOT NULL CHECK(duration_days BETWEEN 1 AND 3650),
 group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
 published BOOLEAN NOT NULL DEFAULT FALSE,
 archived BOOLEAN NOT NULL DEFAULT FALSE,
 version BIGINT NOT NULL CHECK(version > 0),
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 CHECK(NOT (published AND archived))
);
CREATE TABLE billing_orders (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 username TEXT NOT NULL,
 plan_id BIGINT NOT NULL REFERENCES billing_plans(id) ON DELETE RESTRICT,
 plan_name TEXT NOT NULL, plan_version BIGINT NOT NULL,
 price_cents BIGINT NOT NULL CHECK(price_cents BETWEEN 1 AND 1000000000000),
 duration_days INTEGER NOT NULL CHECK(duration_days BETWEEN 1 AND 3650),
 group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
 previous_expires_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ NOT NULL,
 requires_login BOOLEAN NOT NULL,
 request_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(user_id,request_id)
);
CREATE INDEX billing_orders_user_time ON billing_orders(user_id,created_at,id);
CREATE TABLE billing_entries (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 username TEXT NOT NULL,
 sequence BIGINT NOT NULL CHECK(sequence > 0),
 amount_cents BIGINT NOT NULL CHECK(amount_cents <> 0 AND amount_cents BETWEEN -1000000000000 AND 1000000000000),
 before_cents BIGINT NOT NULL CHECK(before_cents BETWEEN 0 AND 1000000000000),
 after_cents BIGINT NOT NULL CHECK(after_cents BETWEEN 0 AND 1000000000000),
 kind TEXT NOT NULL CHECK(kind IN ('admin_credit','admin_debit','reversal','purchase')),
 reason TEXT NOT NULL,
 actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 actor_name TEXT NOT NULL,
 order_id BIGINT UNIQUE REFERENCES billing_orders(id) ON DELETE RESTRICT,
 reverses_id BIGINT UNIQUE REFERENCES billing_entries(id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(user_id,sequence),
 CHECK(after_cents = before_cents + amount_cents),
 CHECK((kind = 'purchase' AND order_id IS NOT NULL AND amount_cents < 0 AND reverses_id IS NULL) OR (kind <> 'purchase' AND order_id IS NULL)),
 CHECK((kind = 'reversal' AND reverses_id IS NOT NULL) OR (kind <> 'reversal' AND reverses_id IS NULL))
);
CREATE INDEX billing_entries_user_time ON billing_entries(user_id,created_at,id);
CREATE TABLE billing_memberships (
 user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
 plan_id BIGINT NOT NULL REFERENCES billing_plans(id) ON DELETE RESTRICT,
 group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
 base_group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
 starts_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL CHECK(expires_at > starts_at),
 status TEXT NOT NULL CHECK(status IN ('active','expired','overridden')),
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX billing_memberships_due ON billing_memberships(status,expires_at,user_id);
CREATE TABLE billing_requests (
 actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 operation TEXT NOT NULL CHECK(operation IN ('adjust','reverse','purchase')),
 request_id TEXT NOT NULL CHECK(length(request_id) BETWEEN 1 AND 100),
 fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64),
 result TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(actor_id,operation,request_id)
);
