-- Daily sale, tickets and entry tickets (SQLite dialect of postgres/00010).
--   * daily_ticket_counters: one row per (establishment, ticket date). The
--     next number is taken with a single atomic upsert
--       INSERT ... ON CONFLICT (establishment_id, ticket_date)
--       DO UPDATE SET last_number = last_number + 1 RETURNING last_number
--     inside the sale transaction (SQLite serialises writers; PostgreSQL
--     locks the conflicting row), so numbers restart at 1 every day in the
--     establishment timezone and are never handed out twice.
--   * tickets: one ticket per walk-in sale or entry sale, owning exactly one
--     booking (confirmed, channel desk, created by internal/domain/booking).
--     UNIQUE (establishment_id, ticket_date, ticket_number) is the final
--     guard against duplicates; a voided ticket keeps its row and number
--     (status = 'voided'), so the only gaps are voids.
--   * needs_assignment marks a zone-first sale ("da assegnare"). Capacity is
--     consumed at sale time by the existing DB constraints: in zone-mode
--     zones the booking line is pending and consumes the implicit zone pool
--     (pool_usage conditional update, spec 7.5); in seat-mode zones the line
--     holds a provisional spot allocation (spot_allocations unique key) on
--     the last free spot of the zone, recorded in provisional_spot_id, until
--     the ticket is assigned (booking Move, atomic, 409 if taken).
--   * entry_ticket_types: entry-only tickets (Adulto, Ridotto, ...) for a
--     quantity zone; each type is one pack priced by the pricing engine per
--     rate period, and the zone pool is the daily capacity shared with the
--     online channel.
--   * daily_sale_settings: per-establishment options of the quick sale and
--     the walk-in customer used when no name is given.

-- +goose Up
CREATE TABLE daily_ticket_counters (
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    ticket_date      TEXT NOT NULL,
    last_number      INTEGER NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    updated_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (establishment_id, ticket_date)
);

CREATE TABLE tickets (
    id                  TEXT PRIMARY KEY,
    establishment_id    TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id          TEXT NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    ticket_date         TEXT NOT NULL,
    ticket_number       INTEGER NOT NULL CHECK (ticket_number >= 1),
    kind                TEXT NOT NULL CHECK (kind IN ('sale', 'entry')),
    status              TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid', 'voided')),
    zone_id             TEXT NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    needs_assignment    INTEGER NOT NULL DEFAULT 0 CHECK (needs_assignment IN (0, 1)),
    provisional_spot_id TEXT REFERENCES spots (id) ON DELETE SET NULL,
    -- Optional label for the map ("Rossi"); never an email or phone.
    holder_name         TEXT NOT NULL DEFAULT '',
    total_amount        INTEGER NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    payment_method      TEXT NOT NULL CHECK (payment_method IN ('cash', 'card')),
    payment_id          TEXT REFERENCES payments (id) ON DELETE SET NULL,
    cash_session_id     TEXT REFERENCES cash_sessions (id) ON DELETE SET NULL,
    print_count         INTEGER NOT NULL DEFAULT 0 CHECK (print_count >= 0),
    created_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    voided_at           TIMESTAMP,
    voided_by_user_id   TEXT REFERENCES users (id) ON DELETE SET NULL,
    void_reason         TEXT NOT NULL DEFAULT '',
    refund_payment_id   TEXT REFERENCES payments (id) ON DELETE SET NULL,
    entered_at          TIMESTAMP,
    entered_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    -- Random, unguessable QR payload (128 bit, base32): the gate admits by
    -- this token, never by the sequential booking code alone.
    gate_token          TEXT NOT NULL,
    version             INTEGER NOT NULL DEFAULT 1,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL,
    CHECK ((status = 'voided') = (voided_at IS NOT NULL)),
    CHECK (provisional_spot_id IS NULL OR needs_assignment = 1),
    CHECK (kind = 'sale' OR needs_assignment = 0),
    CHECK (length(gate_token) >= 20)
);
-- No duplicate numbers per establishment and day, across registers and devices.
CREATE UNIQUE INDEX tickets_establishment_date_number_uidx ON tickets (establishment_id, ticket_date, ticket_number);
CREATE UNIQUE INDEX tickets_booking_id_uidx ON tickets (booking_id);
CREATE UNIQUE INDEX tickets_gate_token_uidx ON tickets (gate_token);
CREATE INDEX tickets_establishment_date_status_idx ON tickets (establishment_id, ticket_date, status);
CREATE INDEX tickets_unassigned_idx ON tickets (establishment_id, ticket_date, zone_id) WHERE needs_assignment = 1 AND status = 'valid';
CREATE INDEX tickets_cash_session_id_idx ON tickets (cash_session_id);

CREATE TABLE entry_ticket_types (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL DEFAULT '{}',
    description      TEXT NOT NULL DEFAULT '{}',
    -- The pack priced by the pricing engine (rate periods, price category).
    pack_id          TEXT NOT NULL REFERENCES packs (id) ON DELETE RESTRICT,
    -- A quantity-mode zone: its pool is the daily capacity (shared with online).
    zone_id          TEXT NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    sort_order       INTEGER NOT NULL DEFAULT 0,
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX entry_ticket_types_establishment_code_uidx ON entry_ticket_types (establishment_id, code);
CREATE UNIQUE INDEX entry_ticket_types_establishment_pack_uidx ON entry_ticket_types (establishment_id, pack_id);
CREATE INDEX entry_ticket_types_zone_id_idx ON entry_ticket_types (zone_id);

CREATE TABLE daily_sale_settings (
    establishment_id    TEXT PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    walk_in_customer_id TEXT REFERENCES customers (id) ON DELETE SET NULL,
    print_format        TEXT NOT NULL DEFAULT 'thermal_80' CHECK (print_format IN ('thermal_80', 'thermal_58', 'a4', 'a5')),
    auto_print          INTEGER NOT NULL DEFAULT 1 CHECK (auto_print IN (0, 1)),
    zone_first_enabled  INTEGER NOT NULL DEFAULT 1 CHECK (zone_first_enabled IN (0, 1)),
    footer_text         TEXT NOT NULL DEFAULT '{}',
    version             INTEGER NOT NULL DEFAULT 1,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE daily_sale_settings;
DROP TABLE entry_ticket_types;
DROP TABLE tickets;
DROP TABLE daily_ticket_counters;
