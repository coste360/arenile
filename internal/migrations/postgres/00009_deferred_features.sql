-- Deferred features (PostgreSQL dialect of sqlite/00009; ADR 0019).
--   * bookings.test / payments.test: test mode (spec 15.4). Rows created while
--     test mode is on are marked and excluded from reports, KPIs,
--     corrispettivi and exports; purgeTestBookings deletes them in bulk.
--     test_mode_settings holds the switch per establishment (versioned).
--   * bookings.price_override_*: manual price of a season pass (audited).
--     price_snapshot stays immutable (INV-21); total_amount follows the
--     override, price_current carries the override line.
--   * refund_requests: guaranteed provider refund of a cancellation, inserted
--     in the cancellation transaction and processed by a job with retries
--     until the provider confirms. One open or succeeded request per booking
--     and source payment intent (unique key): never lost, never doubled.
--   * cash_registers + cash_sessions.register_id: several cash registers per
--     establishment. Backfill: one default register "Cassa principale"
--     (code main) per establishment plus one register per other register
--     code already used by sessions. At most one open session per register
--     (unique partial index), next to the 00003 index on (establishment, code).
--   * webhook_delivery_attempts: one row per delivery attempt (retention
--     job), webhook_deliveries keeps the summary of the last attempt.
--   * import_jobs: CSV imports (customers, season passes) with preview and
--     apply; the same file (sha256) is applied at most once per kind.
--   * admin_notifications: in-panel notifications (webhook endpoint
--     disabled, refund failed, import completed). dedupe_key makes the
--     producer idempotent. params never contain personal data.
-- spot_blocks exists since 00003 (map "block spot" reuses it).

-- +goose Up
ALTER TABLE bookings ADD COLUMN test boolean NOT NULL DEFAULT false;
ALTER TABLE bookings ADD COLUMN price_override_amount bigint CHECK (price_override_amount IS NULL OR price_override_amount >= 0);
ALTER TABLE bookings ADD COLUMN price_override_engine_amount bigint;
ALTER TABLE bookings ADD COLUMN price_override_reason text;
ALTER TABLE bookings ADD COLUMN price_override_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN price_override_at timestamptz;
CREATE INDEX bookings_establishment_test_idx ON bookings (establishment_id) WHERE test;

ALTER TABLE payments ADD COLUMN test boolean NOT NULL DEFAULT false;
CREATE INDEX payments_establishment_test_idx ON payments (establishment_id) WHERE test;

CREATE TABLE test_mode_settings (
    establishment_id   uuid PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    enabled            boolean NOT NULL DEFAULT false,
    public_enabled     boolean NOT NULL DEFAULT false,
    enabled_at         timestamptz,
    enabled_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CHECK (enabled OR NOT public_enabled)
);

CREATE TABLE refund_requests (
    id                uuid PRIMARY KEY,
    establishment_id  uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id        uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    -- Provider payment being refunded (one request per source intent).
    payment_intent_id uuid NOT NULL REFERENCES payment_intents (id) ON DELETE RESTRICT,
    -- Provider refund created by the first successful send.
    refund_intent_id  uuid REFERENCES payment_intents (id) ON DELETE SET NULL,
    -- Ledger row (direction refund) written when the provider confirms.
    payment_id        uuid REFERENCES payments (id) ON DELETE SET NULL,
    provider          text NOT NULL,
    amount            bigint NOT NULL CHECK (amount > 0),
    currency          text NOT NULL DEFAULT 'EUR',
    reason            text NOT NULL CHECK (reason IN ('customer_cancellation', 'panel_cancellation', 'expired_hold')),
    status            text NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'cancelled')),
    -- Sent to the provider on every attempt: the provider deduplicates.
    idempotency_key   text NOT NULL,
    attempts          integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at   timestamptz,
    last_error        text,
    test              boolean NOT NULL DEFAULT false,
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    version           integer NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    completed_at      timestamptz,
    CHECK ((status IN ('succeeded', 'cancelled')) = (completed_at IS NOT NULL)),
    CHECK (status <> 'succeeded' OR payment_id IS NOT NULL)
);
CREATE UNIQUE INDEX refund_requests_idempotency_uidx ON refund_requests (establishment_id, idempotency_key);
-- No double refund: one live (not cancelled) request per booking and source intent.
CREATE UNIQUE INDEX refund_requests_booking_intent_uidx ON refund_requests (booking_id, payment_intent_id) WHERE status <> 'cancelled';
CREATE INDEX refund_requests_establishment_created_idx ON refund_requests (establishment_id, created_at);
CREATE INDEX refund_requests_due_idx ON refund_requests (next_attempt_at) WHERE status IN ('pending', 'processing');

CREATE TABLE cash_registers (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             text NOT NULL,
    name             text NOT NULL,
    active           boolean NOT NULL DEFAULT true,
    is_default       boolean NOT NULL DEFAULT false,
    sort_order       integer NOT NULL DEFAULT 0,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CHECK (NOT is_default OR active)
);
CREATE UNIQUE INDEX cash_registers_establishment_code_uidx ON cash_registers (establishment_id, code);
CREATE UNIQUE INDEX cash_registers_default_uidx ON cash_registers (establishment_id) WHERE is_default;

INSERT INTO cash_registers (id, establishment_id, code, name, active, is_default, sort_order, version, created_at, updated_at)
SELECT gen_random_uuid(),
       e.id, 'main', 'Cassa principale', true, true, 0, 1, e.created_at, e.created_at
FROM establishments e;

INSERT INTO cash_registers (id, establishment_id, code, name, active, is_default, sort_order, version, created_at, updated_at)
SELECT gen_random_uuid(),
       s.establishment_id, s.register, s.register, true, false, 10, 1, s.first_opened_at, s.first_opened_at
FROM (SELECT establishment_id, register, MIN(opened_at) AS first_opened_at
      FROM cash_sessions WHERE register <> 'main' GROUP BY establishment_id, register) s;

ALTER TABLE cash_sessions ADD COLUMN register_id uuid REFERENCES cash_registers (id) ON DELETE RESTRICT;
UPDATE cash_sessions SET register_id = (
    SELECT r.id FROM cash_registers r
    WHERE r.establishment_id = cash_sessions.establishment_id AND r.code = cash_sessions.register
);
CREATE UNIQUE INDEX cash_sessions_register_open_uidx ON cash_sessions (register_id) WHERE closed_at IS NULL;
CREATE INDEX cash_sessions_register_opened_idx ON cash_sessions (register_id, opened_at);

CREATE TABLE webhook_delivery_attempts (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    endpoint_id      uuid NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    delivery_id      uuid NOT NULL REFERENCES webhook_deliveries (id) ON DELETE CASCADE,
    attempt          integer NOT NULL CHECK (attempt >= 1),
    status_code      integer,
    error            text,
    response_excerpt text,
    duration_ms      bigint NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    attempted_at     timestamptz NOT NULL
);
CREATE UNIQUE INDEX webhook_delivery_attempts_delivery_attempt_uidx ON webhook_delivery_attempts (delivery_id, attempt);
CREATE INDEX webhook_delivery_attempts_establishment_idx ON webhook_delivery_attempts (establishment_id, attempted_at);

CREATE TABLE import_jobs (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind               text NOT NULL CHECK (kind IN ('customers', 'season_passes')),
    status             text NOT NULL DEFAULT 'uploaded'
                           CHECK (status IN ('uploaded', 'validated', 'applying', 'applied', 'failed', 'discarded')),
    filename           text NOT NULL DEFAULT '',
    size_bytes         bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256             text NOT NULL,
    -- Raw CSV (personal data): cleared on apply/discard and by the retention job (7 days).
    content            text,
    options            jsonb NOT NULL DEFAULT '{}',
    -- ImportReport JSON: row numbers, columns and codes only.
    report             jsonb,
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    applied_at         timestamptz,
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CHECK ((status = 'applied') = (applied_at IS NOT NULL))
);
CREATE INDEX import_jobs_establishment_created_idx ON import_jobs (establishment_id, created_at);
-- The same file is applied at most once per kind (idempotent import).
CREATE UNIQUE INDEX import_jobs_applied_file_uidx ON import_jobs (establishment_id, kind, sha256) WHERE status = 'applied';

CREATE TABLE admin_notifications (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind             text NOT NULL,
    severity         text NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'error')),
    -- owner = only users with users.manage; staff = everyone with bookings.read.
    audience         text NOT NULL DEFAULT 'staff' CHECK (audience IN ('staff', 'owner')),
    params           jsonb NOT NULL DEFAULT '{}',
    resource_type    text,
    resource_id      text,
    dedupe_key       text NOT NULL,
    read_at          timestamptz,
    read_by_user_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX admin_notifications_dedupe_uidx ON admin_notifications (establishment_id, dedupe_key);
CREATE INDEX admin_notifications_establishment_created_idx ON admin_notifications (establishment_id, created_at);
CREATE INDEX admin_notifications_unread_idx ON admin_notifications (establishment_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE admin_notifications;
DROP TABLE import_jobs;
DROP TABLE webhook_delivery_attempts;
DROP INDEX cash_sessions_register_opened_idx;
DROP INDEX cash_sessions_register_open_uidx;
ALTER TABLE cash_sessions DROP COLUMN register_id;
DROP TABLE cash_registers;
DROP TABLE refund_requests;
DROP TABLE test_mode_settings;
DROP INDEX payments_establishment_test_idx;
ALTER TABLE payments DROP COLUMN test;
DROP INDEX bookings_establishment_test_idx;
ALTER TABLE bookings DROP COLUMN price_override_at;
ALTER TABLE bookings DROP COLUMN price_override_by_user_id;
ALTER TABLE bookings DROP COLUMN price_override_reason;
ALTER TABLE bookings DROP COLUMN price_override_engine_amount;
ALTER TABLE bookings DROP COLUMN price_override_amount;
ALTER TABLE bookings DROP COLUMN test;
