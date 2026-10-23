-- M3: customers, bookings, availability (allocations, holds, blocks, pools),
-- season releases, customer credits, payments, cash and fiscal documents
-- (spec 6.5-6.7, 7, 10, 13, 14). Design notes: docs/adr/0010-scelte-m3.md.
-- IDs are UUIDv7 stored as TEXT; dates are TEXT 'YYYY-MM-DD'; JSON as TEXT;
-- booleans are INTEGER 0/1; money is INTEGER cents.
--
-- Availability is enforced here, never by application checks (ADR 0003):
--   * spot_allocations has a plain unique (spot_id, date, slot). Releasing
--     resources (cancel, expiry, move, season release, unblock) DELETES the
--     rows in the same transaction, so cancelled bookings never block a spot
--     and the constraint stays identical on both dialects (no partial index).
--   * pool_usage is a counter per (pool_id, date, slot) updated only with
--     UPDATE ... SET held = held + :n WHERE used + held + :n <= :capacity;
--     CHECK constraints keep used/held non-negative.

-- +goose Up
CREATE TABLE customers (
    id                TEXT PRIMARY KEY,
    establishment_id  TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    first_name        TEXT NOT NULL DEFAULT '',
    last_name         TEXT NOT NULL,
    email             TEXT,
    phone             TEXT,
    locale            TEXT NOT NULL DEFAULT 'it',
    notes             TEXT NOT NULL DEFAULT '',
    tags              TEXT NOT NULL DEFAULT '[]',
    agreement_id      TEXT REFERENCES agreements (id) ON DELETE SET NULL,
    marketing_consent TEXT,
    -- Lowercase, accent-free "last first email phone" maintained by the
    -- customer service for LIKE search on both dialects.
    search_text       TEXT NOT NULL DEFAULT '',
    anonymized_at     TIMESTAMP,
    version           INTEGER NOT NULL DEFAULT 1,
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL,
    CHECK (email IS NULL OR email = lower(email))
);
CREATE INDEX customers_establishment_id_idx ON customers (establishment_id);
CREATE INDEX customers_establishment_email_idx ON customers (establishment_id, email);
CREATE INDEX customers_establishment_phone_idx ON customers (establishment_id, phone);
CREATE INDEX customers_establishment_last_name_idx ON customers (establishment_id, last_name, first_name);

CREATE TABLE holds (
    id                   TEXT PRIMARY KEY,
    establishment_id     TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    channel              TEXT NOT NULL DEFAULT 'desk' CHECK (channel IN ('desk', 'online', 'api')),
    starts_on            TEXT NOT NULL,
    ends_on              TEXT NOT NULL,
    slots                TEXT NOT NULL CHECK (slots IN ('AM', 'PM', 'FULL')),
    items                TEXT NOT NULL DEFAULT '[]',
    note                 TEXT NOT NULL DEFAULT '',
    expires_at           TIMESTAMP NOT NULL,
    released_at          TIMESTAMP,
    converted_booking_id TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    created_by_user_id   TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMP NOT NULL,
    updated_at           TIMESTAMP NOT NULL,
    CHECK (ends_on >= starts_on)
);
CREATE INDEX holds_establishment_id_idx ON holds (establishment_id);
CREATE INDEX holds_active_expires_idx ON holds (expires_at) WHERE released_at IS NULL AND converted_booking_id IS NULL;

CREATE TABLE bookings (
    id                    TEXT PRIMARY KEY,
    establishment_id      TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code                  TEXT NOT NULL,
    customer_id           TEXT NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,
    channel               TEXT NOT NULL CHECK (channel IN ('desk', 'online', 'api')),
    status                TEXT NOT NULL CHECK (status IN ('draft', 'held', 'pending_payment', 'awaiting_approval', 'confirmed',
                                                        'checked_in', 'completed', 'cancelled', 'no_show', 'expired', 'rejected')),
    starts_on             TEXT NOT NULL,
    ends_on               TEXT NOT NULL,
    slots                 TEXT NOT NULL CHECK (slots IN ('AM', 'PM', 'FULL')),
    formula               TEXT NOT NULL CHECK (formula IN ('half_day', 'day', 'multi_day', 'week', 'fortnight', 'month', 'season')),
    season_id             TEXT REFERENCES seasons (id) ON DELETE SET NULL,
    is_season_pass        INTEGER NOT NULL DEFAULT 0 CHECK (is_season_pass IN (0, 1)),
    renewal_of_booking_id TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    price_snapshot        TEXT NOT NULL DEFAULT '{}',
    -- Breakdown of the latest re-quote after confirmation (ADR 0011); NULL = price_snapshot is current.
    price_current         TEXT,
    total_amount          INTEGER NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    paid_amount           INTEGER NOT NULL DEFAULT 0,
    deposit_due           INTEGER NOT NULL DEFAULT 0 CHECK (deposit_due >= 0),
    policy_snapshot       TEXT NOT NULL DEFAULT '{}',
    promo_code_id         TEXT REFERENCES promo_codes (id) ON DELETE SET NULL,
    agreement_id          TEXT REFERENCES agreements (id) ON DELETE SET NULL,
    hold_expires_at       TIMESTAMP,
    hold_id               TEXT REFERENCES holds (id) ON DELETE SET NULL,
    manage_token_hash     TEXT,
    terms_version         TEXT,
    notes_customer        TEXT NOT NULL DEFAULT '',
    notes_internal        TEXT NOT NULL DEFAULT '',
    created_by_user_id    TEXT REFERENCES users (id) ON DELETE SET NULL,
    confirmed_at          TIMESTAMP,
    checked_in_at         TIMESTAMP,
    cancelled_at          TIMESTAMP,
    cancel_reason         TEXT,
    version               INTEGER NOT NULL DEFAULT 1,
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL,
    CHECK (ends_on >= starts_on),
    -- INV-06: hold_expires_at only while resources are held temporarily.
    CHECK (hold_expires_at IS NULL OR status IN ('held', 'pending_payment', 'awaiting_approval'))
);
CREATE UNIQUE INDEX bookings_establishment_code_uidx ON bookings (establishment_id, code);
CREATE UNIQUE INDEX bookings_manage_token_hash_uidx ON bookings (manage_token_hash) WHERE manage_token_hash IS NOT NULL;
CREATE INDEX bookings_establishment_dates_idx ON bookings (establishment_id, starts_on, ends_on);
CREATE INDEX bookings_establishment_status_idx ON bookings (establishment_id, status);
CREATE INDEX bookings_customer_id_idx ON bookings (customer_id);
CREATE INDEX bookings_hold_expires_idx ON bookings (hold_expires_at) WHERE hold_expires_at IS NOT NULL;

CREATE TABLE booking_lines (
    id                TEXT PRIMARY KEY,
    establishment_id  TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id        TEXT NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    pack_id           TEXT NOT NULL REFERENCES packs (id) ON DELETE RESTRICT,
    zone_id           TEXT NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    spot_id           TEXT REFERENCES spots (id) ON DELETE SET NULL,
    assignment_status TEXT NOT NULL CHECK (assignment_status IN ('assigned', 'pending', 'not_applicable')),
    quantity          INTEGER NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    price_category    TEXT NOT NULL DEFAULT '',
    line_total        INTEGER NOT NULL DEFAULT 0 CHECK (line_total >= 0),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL,
    CHECK (assignment_status <> 'assigned' OR spot_id IS NOT NULL)
);
CREATE INDEX booking_lines_establishment_id_idx ON booking_lines (establishment_id);
CREATE INDEX booking_lines_booking_id_idx ON booking_lines (booking_id);
CREATE INDEX booking_lines_spot_id_idx ON booking_lines (spot_id);
CREATE INDEX booking_lines_pending_idx ON booking_lines (establishment_id, zone_id) WHERE assignment_status = 'pending';

CREATE TABLE booking_line_extras (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id       TEXT NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    booking_line_id  TEXT REFERENCES booking_lines (id) ON DELETE CASCADE,
    extra_id         TEXT NOT NULL REFERENCES extras (id) ON DELETE RESTRICT,
    quantity         INTEGER NOT NULL CHECK (quantity >= 1),
    total            INTEGER NOT NULL DEFAULT 0 CHECK (total >= 0),
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX booking_line_extras_establishment_id_idx ON booking_line_extras (establishment_id);
CREATE INDEX booking_line_extras_booking_id_idx ON booking_line_extras (booking_id);

CREATE TABLE spot_blocks (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    spot_id            TEXT NOT NULL REFERENCES spots (id) ON DELETE CASCADE,
    starts_on          TEXT NOT NULL,
    ends_on            TEXT NOT NULL,
    slots              TEXT NOT NULL CHECK (slots IN ('AM', 'PM', 'FULL')),
    reason             TEXT NOT NULL DEFAULT '',
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK (ends_on >= starts_on)
);
CREATE INDEX spot_blocks_establishment_dates_idx ON spot_blocks (establishment_id, starts_on, ends_on);
CREATE INDEX spot_blocks_spot_id_idx ON spot_blocks (spot_id);

-- One row per occupied (spot, date, slot): the key of availability (spec 7.2).
CREATE TABLE spot_allocations (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    spot_id          TEXT NOT NULL REFERENCES spots (id) ON DELETE RESTRICT,
    date             TEXT NOT NULL,
    slot             TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    kind             TEXT NOT NULL CHECK (kind IN ('booking', 'hold', 'block', 'season')),
    booking_id       TEXT REFERENCES bookings (id) ON DELETE CASCADE,
    booking_line_id  TEXT REFERENCES booking_lines (id) ON DELETE CASCADE,
    hold_id          TEXT REFERENCES holds (id) ON DELETE CASCADE,
    block_id         TEXT REFERENCES spot_blocks (id) ON DELETE CASCADE,
    expires_at       TIMESTAMP,
    created_at       TIMESTAMP NOT NULL,
    -- INV-06: expires_at iff kind = hold.
    CHECK ((kind = 'hold') = (expires_at IS NOT NULL)),
    -- Exactly one owner: a booking line (booking, season, booking hold), a
    -- panel hold, or a manual block.
    CHECK (
        (kind IN ('booking', 'season') AND booking_line_id IS NOT NULL AND hold_id IS NULL AND block_id IS NULL)
        OR (kind = 'hold' AND block_id IS NULL AND ((booking_line_id IS NOT NULL) <> (hold_id IS NOT NULL)))
        OR (kind = 'block' AND block_id IS NOT NULL AND booking_line_id IS NULL AND hold_id IS NULL)
    ),
    CHECK ((booking_line_id IS NULL) = (booking_id IS NULL))
);
-- INV-01: never two allocations of the same spot on the same half-day.
CREATE UNIQUE INDEX spot_allocations_spot_date_slot_uidx ON spot_allocations (spot_id, date, slot);
CREATE INDEX spot_allocations_establishment_date_idx ON spot_allocations (establishment_id, date);
CREATE INDEX spot_allocations_booking_id_idx ON spot_allocations (booking_id);
CREATE INDEX spot_allocations_booking_line_id_idx ON spot_allocations (booking_line_id);
CREATE INDEX spot_allocations_hold_id_idx ON spot_allocations (hold_id);
CREATE INDEX spot_allocations_block_id_idx ON spot_allocations (block_id);
CREATE INDEX spot_allocations_expires_idx ON spot_allocations (expires_at) WHERE expires_at IS NOT NULL;

-- Counter per (pool, date, slot); capacity comes from pools (+ overrides) and
-- is enforced by the conditional UPDATE (spec 7.3, INV-02).
CREATE TABLE pool_usage (
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    pool_id          TEXT NOT NULL REFERENCES pools (id) ON DELETE CASCADE,
    date             TEXT NOT NULL,
    slot             TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    used             INTEGER NOT NULL DEFAULT 0 CHECK (used >= 0),
    held             INTEGER NOT NULL DEFAULT 0 CHECK (held >= 0),
    -- Units sold through the online channel (online quota, spec 7.6, INV-09).
    online_used      INTEGER NOT NULL DEFAULT 0 CHECK (online_used >= 0),
    updated_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (pool_id, date, slot)
);
CREATE INDEX pool_usage_establishment_date_idx ON pool_usage (establishment_id, date);

-- Detail of pool consumption per booking line / hold (rebuilds pool_usage, INV-03).
CREATE TABLE pool_allocations (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    pool_id          TEXT NOT NULL REFERENCES pools (id) ON DELETE RESTRICT,
    date             TEXT NOT NULL,
    slot             TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    kind             TEXT NOT NULL CHECK (kind IN ('booking', 'hold')),
    channel          TEXT NOT NULL DEFAULT 'desk' CHECK (channel IN ('desk', 'online', 'api')),
    quantity         INTEGER NOT NULL CHECK (quantity >= 1),
    booking_id       TEXT REFERENCES bookings (id) ON DELETE CASCADE,
    booking_line_id  TEXT REFERENCES booking_lines (id) ON DELETE CASCADE,
    hold_id          TEXT REFERENCES holds (id) ON DELETE CASCADE,
    expires_at       TIMESTAMP,
    created_at       TIMESTAMP NOT NULL,
    CHECK ((kind = 'hold') = (expires_at IS NOT NULL)),
    CHECK ((booking_line_id IS NOT NULL) <> (hold_id IS NOT NULL)),
    CHECK ((booking_line_id IS NULL) = (booking_id IS NULL))
);
CREATE INDEX pool_allocations_establishment_date_idx ON pool_allocations (establishment_id, date);
CREATE INDEX pool_allocations_pool_date_slot_idx ON pool_allocations (pool_id, date, slot);
CREATE INDEX pool_allocations_booking_id_idx ON pool_allocations (booking_id);
CREATE INDEX pool_allocations_hold_id_idx ON pool_allocations (hold_id);
CREATE INDEX pool_allocations_expires_idx ON pool_allocations (expires_at) WHERE expires_at IS NOT NULL;

-- Half-days released by a season pass holder (spec 7.7, INV-10). One row per
-- (line, date, slot); a revoked release keeps its row (revoked_at).
CREATE TABLE season_releases (
    id                     TEXT PRIMARY KEY,
    establishment_id       TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id             TEXT NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    booking_line_id        TEXT NOT NULL REFERENCES booking_lines (id) ON DELETE CASCADE,
    spot_id                TEXT NOT NULL REFERENCES spots (id) ON DELETE RESTRICT,
    date                   TEXT NOT NULL,
    slot                   TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    resold_booking_line_id TEXT REFERENCES booking_lines (id) ON DELETE SET NULL,
    credit_amount          INTEGER NOT NULL DEFAULT 0 CHECK (credit_amount >= 0),
    note                   TEXT NOT NULL DEFAULT '',
    created_by_user_id     TEXT REFERENCES users (id) ON DELETE SET NULL,
    revoked_at             TIMESTAMP,
    created_at             TIMESTAMP NOT NULL,
    updated_at             TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX season_releases_active_uidx ON season_releases (booking_line_id, date, slot) WHERE revoked_at IS NULL;
CREATE INDEX season_releases_establishment_date_idx ON season_releases (establishment_id, date);
CREATE INDEX season_releases_booking_id_idx ON season_releases (booking_id);
CREATE INDEX season_releases_spot_date_idx ON season_releases (spot_id, date);

-- Customer credit ledger (spec 13.4): positive = credit granted, negative = used.
CREATE TABLE customer_credits (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id        TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    amount             INTEGER NOT NULL CHECK (amount <> 0),
    source             TEXT NOT NULL CHECK (source IN ('cancellation', 'season_release', 'weather', 'manual', 'redemption')),
    booking_id         TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    season_release_id  TEXT REFERENCES season_releases (id) ON DELETE SET NULL,
    payment_id         TEXT REFERENCES payments (id) ON DELETE SET NULL,
    note               TEXT NOT NULL DEFAULT '',
    expires_on         TEXT,
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL
);
CREATE INDEX customer_credits_establishment_customer_idx ON customer_credits (establishment_id, customer_id);

CREATE TABLE cash_sessions (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    register         TEXT NOT NULL DEFAULT 'main',
    opened_by        TEXT REFERENCES users (id) ON DELETE SET NULL,
    closed_by        TEXT REFERENCES users (id) ON DELETE SET NULL,
    opened_at        TIMESTAMP NOT NULL,
    closed_at        TIMESTAMP,
    opening_float    INTEGER NOT NULL DEFAULT 0 CHECK (opening_float >= 0),
    expected_totals  TEXT NOT NULL DEFAULT '{}',
    counted_totals   TEXT,
    difference       INTEGER,
    notes            TEXT NOT NULL DEFAULT '',
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    CHECK ((closed_at IS NULL) = (counted_totals IS NULL)),
    CHECK ((closed_at IS NULL) = (difference IS NULL))
);
-- At most one open session per establishment and register.
CREATE UNIQUE INDEX cash_sessions_open_uidx ON cash_sessions (establishment_id, register) WHERE closed_at IS NULL;
CREATE INDEX cash_sessions_establishment_opened_idx ON cash_sessions (establishment_id, opened_at);

CREATE TABLE cash_movements (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    cash_session_id    TEXT NOT NULL REFERENCES cash_sessions (id) ON DELETE CASCADE,
    kind               TEXT NOT NULL CHECK (kind IN ('in', 'out')),
    amount             INTEGER NOT NULL CHECK (amount > 0),
    reason             TEXT NOT NULL DEFAULT '',
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL
);
CREATE INDEX cash_movements_establishment_id_idx ON cash_movements (establishment_id);
CREATE INDEX cash_movements_cash_session_id_idx ON cash_movements (cash_session_id);

-- Payments and refunds share one ledger (spec 6.7): direction in | refund,
-- amount always positive (INV-22, INV-24).
CREATE TABLE payments (
    id                   TEXT PRIMARY KEY,
    establishment_id     TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id           TEXT REFERENCES bookings (id) ON DELETE RESTRICT,
    customer_id          TEXT REFERENCES customers (id) ON DELETE SET NULL,
    provider             TEXT NOT NULL,
    method               TEXT NOT NULL CHECK (method IN ('card', 'cash', 'bank_transfer', 'wallet', 'credit', 'other')),
    direction            TEXT NOT NULL CHECK (direction IN ('in', 'refund')),
    amount               INTEGER NOT NULL CHECK (amount > 0),
    status               TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled')),
    provider_ref         TEXT,
    idempotency_key      TEXT,
    cash_session_id      TEXT REFERENCES cash_sessions (id) ON DELETE RESTRICT,
    refund_of_payment_id TEXT REFERENCES payments (id) ON DELETE SET NULL,
    received_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    description          TEXT NOT NULL DEFAULT '',
    note                 TEXT NOT NULL DEFAULT '',
    -- VAT split of this amount, [{rate_bp, gross, taxable, tax}], frozen at
    -- creation from the booking price snapshot (corrispettivi export, spec 14.2).
    vat_breakdown        TEXT NOT NULL DEFAULT '[]',
    business_date        TEXT NOT NULL,
    metadata             TEXT NOT NULL DEFAULT '{}',
    created_at           TIMESTAMP NOT NULL,
    updated_at           TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX payments_establishment_idempotency_uidx ON payments (establishment_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX payments_provider_ref_uidx ON payments (provider, provider_ref) WHERE provider_ref IS NOT NULL;
CREATE INDEX payments_establishment_date_idx ON payments (establishment_id, business_date);
CREATE INDEX payments_booking_id_idx ON payments (booking_id);
CREATE INDEX payments_cash_session_id_idx ON payments (cash_session_id);

CREATE TABLE fiscal_documents (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    payment_id       TEXT NOT NULL REFERENCES payments (id) ON DELETE RESTRICT,
    driver           TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('pending', 'issued', 'failed', 'void')),
    document_number  TEXT,
    business_date    TEXT NOT NULL,
    payload          TEXT NOT NULL DEFAULT '{}',
    response         TEXT,
    error            TEXT,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX fiscal_documents_establishment_date_idx ON fiscal_documents (establishment_id, business_date);
CREATE INDEX fiscal_documents_payment_id_idx ON fiscal_documents (payment_id);

-- +goose Down
DROP TABLE fiscal_documents;
DROP TABLE payments;
DROP TABLE cash_movements;
DROP TABLE cash_sessions;
DROP TABLE customer_credits;
DROP TABLE season_releases;
DROP TABLE pool_allocations;
DROP TABLE pool_usage;
DROP TABLE spot_allocations;
DROP TABLE spot_blocks;
DROP TABLE booking_line_extras;
DROP TABLE booking_lines;
DROP TABLE bookings;
DROP TABLE holds;
DROP TABLE customers;
