-- Deferred features (SQLite dialect of postgres/00009; ADR 0019).
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
ALTER TABLE bookings ADD COLUMN test INTEGER NOT NULL DEFAULT 0 CHECK (test IN (0, 1));
ALTER TABLE bookings ADD COLUMN price_override_amount INTEGER CHECK (price_override_amount IS NULL OR price_override_amount >= 0);
ALTER TABLE bookings ADD COLUMN price_override_engine_amount INTEGER;
ALTER TABLE bookings ADD COLUMN price_override_reason TEXT;
ALTER TABLE bookings ADD COLUMN price_override_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN price_override_at TIMESTAMP;
CREATE INDEX bookings_establishment_test_idx ON bookings (establishment_id) WHERE test = 1;

ALTER TABLE payments ADD COLUMN test INTEGER NOT NULL DEFAULT 0 CHECK (test IN (0, 1));
-- Set when the event is written (same transaction as the test row it is
-- about): the dispatcher keeps flagged events out of the outgoing webhooks
-- even after the purge deleted the row.
ALTER TABLE outbox_events ADD COLUMN test INTEGER NOT NULL DEFAULT 0 CHECK (test IN (0, 1));
CREATE INDEX payments_establishment_test_idx ON payments (establishment_id) WHERE test = 1;

CREATE TABLE test_mode_settings (
    establishment_id   TEXT PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    enabled            INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    public_enabled     INTEGER NOT NULL DEFAULT 0 CHECK (public_enabled IN (0, 1)),
    enabled_at         TIMESTAMP,
    enabled_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK (enabled = 1 OR public_enabled = 0)
);

CREATE TABLE refund_requests (
    id                TEXT PRIMARY KEY,
    establishment_id  TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id        TEXT NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    -- Provider payment being refunded (one request per source intent).
    payment_intent_id TEXT NOT NULL REFERENCES payment_intents (id) ON DELETE RESTRICT,
    -- Provider refund created by the first successful send.
    refund_intent_id  TEXT REFERENCES payment_intents (id) ON DELETE SET NULL,
    -- Ledger row (direction refund) written when the provider confirms.
    payment_id        TEXT REFERENCES payments (id) ON DELETE SET NULL,
    provider          TEXT NOT NULL,
    amount            INTEGER NOT NULL CHECK (amount > 0),
    currency          TEXT NOT NULL DEFAULT 'EUR',
    reason            TEXT NOT NULL CHECK (reason IN ('customer_cancellation', 'panel_cancellation', 'expired_hold')),
    status            TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'cancelled')),
    -- Sent to the provider on every attempt: the provider deduplicates.
    idempotency_key   TEXT NOT NULL,
    attempts          INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at   TIMESTAMP,
    last_error        TEXT,
    test              INTEGER NOT NULL DEFAULT 0 CHECK (test IN (0, 1)),
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    version           INTEGER NOT NULL DEFAULT 1,
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL,
    completed_at      TIMESTAMP,
    CHECK ((status IN ('succeeded', 'cancelled')) = (completed_at IS NOT NULL)),
    CHECK (status <> 'succeeded' OR payment_id IS NOT NULL)
);
CREATE UNIQUE INDEX refund_requests_idempotency_uidx ON refund_requests (establishment_id, idempotency_key);
-- No double refund: one live (not cancelled) request per booking and source intent.
CREATE UNIQUE INDEX refund_requests_booking_intent_uidx ON refund_requests (booking_id, payment_intent_id) WHERE status <> 'cancelled';
CREATE INDEX refund_requests_establishment_created_idx ON refund_requests (establishment_id, created_at);
CREATE INDEX refund_requests_due_idx ON refund_requests (next_attempt_at) WHERE status IN ('pending', 'processing');

CREATE TABLE cash_registers (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL,
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    is_default       INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    CHECK (is_default = 0 OR active = 1)
);
CREATE UNIQUE INDEX cash_registers_establishment_code_uidx ON cash_registers (establishment_id, code);
CREATE UNIQUE INDEX cash_registers_default_uidx ON cash_registers (establishment_id) WHERE is_default = 1;

INSERT INTO cash_registers (id, establishment_id, code, name, active, is_default, sort_order, version, created_at, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-'
           || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
       e.id, 'main', 'Cassa principale', 1, 1, 0, 1, e.created_at, e.created_at
FROM establishments e;

INSERT INTO cash_registers (id, establishment_id, code, name, active, is_default, sort_order, version, created_at, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-'
           || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
       s.establishment_id, s.register, s.register, 1, 0, 10, 1, s.first_opened_at, s.first_opened_at
FROM (SELECT establishment_id, register, MIN(opened_at) AS first_opened_at
      FROM cash_sessions WHERE register <> 'main' GROUP BY establishment_id, register) s;

ALTER TABLE cash_sessions ADD COLUMN register_id TEXT REFERENCES cash_registers (id) ON DELETE RESTRICT;
UPDATE cash_sessions SET register_id = (
    SELECT r.id FROM cash_registers r
    WHERE r.establishment_id = cash_sessions.establishment_id AND r.code = cash_sessions.register
);
CREATE UNIQUE INDEX cash_sessions_register_open_uidx ON cash_sessions (register_id) WHERE closed_at IS NULL;
CREATE INDEX cash_sessions_register_opened_idx ON cash_sessions (register_id, opened_at);
-- INV-73: every session belongs to a register of its own establishment.
-- SQLite cannot add NOT NULL or a composite foreign key to an existing
-- column, so triggers enforce both (PostgreSQL uses constraints).
-- +goose StatementBegin
CREATE TRIGGER cash_sessions_register_required_ins BEFORE INSERT ON cash_sessions
WHEN NEW.register_id IS NULL
    OR (SELECT r.establishment_id FROM cash_registers r WHERE r.id = NEW.register_id) IS NOT NEW.establishment_id
BEGIN
    SELECT RAISE(ABORT, 'cash_sessions.register_id must name a register of the same establishment');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER cash_sessions_register_required_upd BEFORE UPDATE OF register_id, establishment_id ON cash_sessions
WHEN NEW.register_id IS NULL
    OR (SELECT r.establishment_id FROM cash_registers r WHERE r.id = NEW.register_id) IS NOT NEW.establishment_id
BEGIN
    SELECT RAISE(ABORT, 'cash_sessions.register_id must name a register of the same establishment');
END;
-- +goose StatementEnd

CREATE TABLE webhook_delivery_attempts (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    endpoint_id      TEXT NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    delivery_id      TEXT NOT NULL REFERENCES webhook_deliveries (id) ON DELETE CASCADE,
    attempt          INTEGER NOT NULL CHECK (attempt >= 1),
    status_code      INTEGER,
    error            TEXT,
    response_excerpt TEXT,
    duration_ms      INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    attempted_at     TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX webhook_delivery_attempts_delivery_attempt_uidx ON webhook_delivery_attempts (delivery_id, attempt);
CREATE INDEX webhook_delivery_attempts_establishment_idx ON webhook_delivery_attempts (establishment_id, attempted_at);

CREATE TABLE import_jobs (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind               TEXT NOT NULL CHECK (kind IN ('customers', 'season_passes')),
    status             TEXT NOT NULL DEFAULT 'uploaded'
                           CHECK (status IN ('uploaded', 'validated', 'applying', 'applied', 'failed', 'discarded')),
    filename           TEXT NOT NULL DEFAULT '',
    size_bytes         INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256             TEXT NOT NULL,
    -- Raw CSV (personal data): cleared on apply/discard and by the retention job (7 days).
    content            TEXT,
    options            TEXT NOT NULL DEFAULT '{}',
    -- ImportReport JSON: row numbers, columns and codes only.
    report             TEXT,
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    applied_at         TIMESTAMP,
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK ((status = 'applied') = (applied_at IS NOT NULL))
);
CREATE INDEX import_jobs_establishment_created_idx ON import_jobs (establishment_id, created_at);
-- The same file is applied at most once per kind (idempotent import).
CREATE UNIQUE INDEX import_jobs_applied_file_uidx ON import_jobs (establishment_id, kind, sha256) WHERE status = 'applied';

CREATE TABLE admin_notifications (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind             TEXT NOT NULL,
    severity         TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info', 'warning', 'error')),
    -- owner = only users with users.manage; staff = everyone with bookings.read.
    audience         TEXT NOT NULL DEFAULT 'staff' CHECK (audience IN ('staff', 'owner', 'manager')),
    params           TEXT NOT NULL DEFAULT '{}',
    resource_type    TEXT,
    resource_id      TEXT,
    dedupe_key       TEXT NOT NULL,
    read_at          TIMESTAMP,
    read_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX admin_notifications_dedupe_uidx ON admin_notifications (establishment_id, dedupe_key);
CREATE INDEX admin_notifications_establishment_created_idx ON admin_notifications (establishment_id, created_at);
CREATE INDEX admin_notifications_unread_idx ON admin_notifications (establishment_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE admin_notifications;
DROP TABLE import_jobs;
DROP TABLE webhook_delivery_attempts;
DROP TRIGGER cash_sessions_register_required_upd;
DROP TRIGGER cash_sessions_register_required_ins;
DROP INDEX cash_sessions_register_opened_idx;
DROP INDEX cash_sessions_register_open_uidx;
ALTER TABLE cash_sessions DROP COLUMN register_id;
DROP TABLE cash_registers;
DROP TABLE refund_requests;
DROP TABLE test_mode_settings;
DROP INDEX payments_establishment_test_idx;
ALTER TABLE outbox_events DROP COLUMN test;
ALTER TABLE payments DROP COLUMN test;
DROP INDEX bookings_establishment_test_idx;
ALTER TABLE bookings DROP COLUMN price_override_at;
ALTER TABLE bookings DROP COLUMN price_override_by_user_id;
ALTER TABLE bookings DROP COLUMN price_override_reason;
ALTER TABLE bookings DROP COLUMN price_override_engine_amount;
ALTER TABLE bookings DROP COLUMN price_override_amount;
ALTER TABLE bookings DROP COLUMN test;
