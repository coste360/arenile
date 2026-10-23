-- Daily sale, tickets and entry tickets (PostgreSQL dialect of sqlite/00010).
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
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    ticket_date      date NOT NULL,
    last_number      integer NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    updated_at       timestamptz NOT NULL,
    PRIMARY KEY (establishment_id, ticket_date)
);

CREATE TABLE tickets (
    id                  uuid PRIMARY KEY,
    establishment_id    uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id          uuid NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    ticket_date         date NOT NULL,
    ticket_number       integer NOT NULL CHECK (ticket_number >= 1),
    kind                text NOT NULL CHECK (kind IN ('sale', 'entry')),
    status              text NOT NULL DEFAULT 'valid' CHECK (status IN ('valid', 'voided')),
    zone_id             uuid NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    needs_assignment    boolean NOT NULL DEFAULT false,
    provisional_spot_id uuid REFERENCES spots (id) ON DELETE SET NULL,
    -- Optional label for the map ("Rossi"); never an email or phone.
    holder_name         text NOT NULL DEFAULT '',
    total_amount        bigint NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    payment_method      text NOT NULL CHECK (payment_method IN ('cash', 'card')),
    payment_id          uuid REFERENCES payments (id) ON DELETE SET NULL,
    cash_session_id     uuid REFERENCES cash_sessions (id) ON DELETE SET NULL,
    print_count         integer NOT NULL DEFAULT 0 CHECK (print_count >= 0),
    created_by_user_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    voided_at           timestamptz,
    voided_by_user_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    void_reason         text NOT NULL DEFAULT '',
    refund_payment_id   uuid REFERENCES payments (id) ON DELETE SET NULL,
    entered_at          timestamptz,
    entered_by_user_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    version             integer NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    CHECK ((status = 'voided') = (voided_at IS NOT NULL)),
    CHECK (provisional_spot_id IS NULL OR needs_assignment),
    CHECK (kind = 'sale' OR NOT needs_assignment)
);
-- No duplicate numbers per establishment and day, across registers and devices.
CREATE UNIQUE INDEX tickets_establishment_date_number_uidx ON tickets (establishment_id, ticket_date, ticket_number);
CREATE UNIQUE INDEX tickets_booking_id_uidx ON tickets (booking_id);
CREATE INDEX tickets_establishment_date_status_idx ON tickets (establishment_id, ticket_date, status);
CREATE INDEX tickets_unassigned_idx ON tickets (establishment_id, ticket_date, zone_id) WHERE needs_assignment AND status = 'valid';
CREATE INDEX tickets_cash_session_id_idx ON tickets (cash_session_id);

CREATE TABLE entry_ticket_types (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             text NOT NULL,
    name             jsonb NOT NULL DEFAULT '{}',
    description      jsonb NOT NULL DEFAULT '{}',
    -- The pack priced by the pricing engine (rate periods, price category).
    pack_id          uuid NOT NULL REFERENCES packs (id) ON DELETE RESTRICT,
    -- A quantity-mode zone: its pool is the daily capacity (shared with online).
    zone_id          uuid NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    sort_order       integer NOT NULL DEFAULT 0,
    active           boolean NOT NULL DEFAULT true,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX entry_ticket_types_establishment_code_uidx ON entry_ticket_types (establishment_id, code);
CREATE UNIQUE INDEX entry_ticket_types_establishment_pack_uidx ON entry_ticket_types (establishment_id, pack_id);
CREATE INDEX entry_ticket_types_zone_id_idx ON entry_ticket_types (zone_id);

CREATE TABLE daily_sale_settings (
    establishment_id    uuid PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    walk_in_customer_id uuid REFERENCES customers (id) ON DELETE SET NULL,
    print_format        text NOT NULL DEFAULT 'thermal_80' CHECK (print_format IN ('thermal_80', 'thermal_58', 'a4', 'a5')),
    auto_print          boolean NOT NULL DEFAULT true,
    zone_first_enabled  boolean NOT NULL DEFAULT true,
    footer_text         jsonb NOT NULL DEFAULT '{}',
    version             integer NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);

-- +goose Down
DROP TABLE daily_sale_settings;
DROP TABLE entry_ticket_types;
DROP TABLE tickets;
DROP TABLE daily_ticket_counters;
