-- M8: advanced cash desk and agreements (spec 30, ADR 0028). SQLite dialect
-- of postgres/00013. IDs are UUIDv7 stored as TEXT; dates are TEXT
-- 'YYYY-MM-DD'; JSON as TEXT; booleans are INTEGER 0/1; money is INTEGER
-- cents.
--
-- New columns on existing tables are nullable or default to the Go zero
-- value of their model field, so code that does not know them yet keeps
-- inserting valid rows (spots.online_excluded is the negation of the API
-- flag online_bookable for that reason).
--
--   * partners: the counterparts of agreements (hotels, B&B, municipality,
--     companies, associations), separate from customers (spec 30.2.1).
--   * customer_categories / customer_category_members: residents, over 65...
--     assignable to customers and targets of agreements (spec 30.2.2).
--   * agreements: target (partner | customer_category | individual_customers;
--     NULL = individual_customers), fixed prices per pack, scope, seasons,
--     quota configuration, requirement, channels, register category.
--   * agreement_versions: one immutable snapshot per saved version (history,
--     the applied condition stays in the price snapshot, INV-21).
--   * agreement_quota_usage: per (agreement, date, slot) counter of the
--     agreement quota, updated only with conditional UPDATEs like pool_usage
--     (INV-103): used <= max_units is also a CHECK. reserved is the capacity
--     held back from general sale until release_on (release_days_before
--     days before the date, ADR 0028); agreement_quota_allocations is the
--     detail per booking line (rebuilds the counter, released on cancel).
--   * bookings: partner, group and party, guest names, the partner share
--     (type and value) and the resolved partner_amount kept by the booking
--     service in the same transaction as total_amount (statement, INV-104).
--   * partner_payments: payments received from a partner (statement balance =
--     charged - paid, INV-104); voided rows stay with reason and operator.
--   * requirement_verifications: date and operator of a requirement check
--     (never a copy of the document, spec 23).
--   * move_undo_log: what a move, swap or group move changed, to undo it
--     within 5 minutes if the original spots are still free (spec 30.7).
--   * spot_allocations: a booking line holds at most one spot per half-day
--     (split stays and swaps never give a line two spots, INV-102).

-- +goose Up
CREATE TABLE partners (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name               TEXT NOT NULL CHECK (length(name) > 0),
    kind               TEXT NOT NULL CHECK (kind IN ('struttura_ricettiva', 'ente', 'azienda', 'associazione', 'altro')),
    accommodation_type TEXT CHECK (accommodation_type IS NULL OR accommodation_type IN ('hotel', 'bnb', 'affittacamere', 'campeggio', 'casa_vacanze', 'altro')),
    municipality       TEXT NOT NULL DEFAULT '',
    address            TEXT NOT NULL DEFAULT '',
    contact_name       TEXT NOT NULL DEFAULT '',
    email              TEXT,
    phone              TEXT,
    vat_number         TEXT NOT NULL DEFAULT '',
    tax_code           TEXT NOT NULL DEFAULT '',
    notes              TEXT NOT NULL DEFAULT '',
    active             INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    -- Lowercase, accent-free "name municipality contact" for LIKE search.
    search_text        TEXT NOT NULL DEFAULT '',
    version            INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK (email IS NULL OR email = lower(email))
);
CREATE INDEX partners_establishment_id_idx ON partners (establishment_id);
CREATE INDEX partners_establishment_name_idx ON partners (establishment_id, name);

CREATE TABLE customer_categories (
    id                TEXT PRIMARY KEY,
    establishment_id  TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code              TEXT NOT NULL,
    name              TEXT NOT NULL CHECK (length(name) > 0),
    description       TEXT NOT NULL DEFAULT '',
    requirement       TEXT NOT NULL DEFAULT '',
    register_category TEXT NOT NULL DEFAULT 'other'
        CHECK (register_category IN ('residents', 'accommodation', 'protected', 'companies', 'associations', 'other')),
    active            INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version           INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX customer_categories_establishment_code_uidx ON customer_categories (establishment_id, code);

CREATE TABLE customer_category_members (
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id      TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    category_id      TEXT NOT NULL REFERENCES customer_categories (id) ON DELETE CASCADE,
    created_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (customer_id, category_id)
);
CREATE INDEX customer_category_members_establishment_id_idx ON customer_category_members (establishment_id);
CREATE INDEX customer_category_members_category_id_idx ON customer_category_members (category_id);

ALTER TABLE agreements ADD COLUMN target TEXT CHECK (target IS NULL OR target IN ('partner', 'customer_category', 'individual_customers'));
ALTER TABLE agreements ADD COLUMN partner_id TEXT REFERENCES partners (id) ON DELETE RESTRICT;
ALTER TABLE agreements ADD COLUMN customer_category_id TEXT REFERENCES customer_categories (id) ON DELETE RESTRICT;
ALTER TABLE agreements ADD COLUMN pack_prices TEXT;
ALTER TABLE agreements ADD COLUMN scope TEXT;
ALTER TABLE agreements ADD COLUMN season_ids TEXT;
ALTER TABLE agreements ADD COLUMN quota TEXT;
ALTER TABLE agreements ADD COLUMN requirement TEXT NOT NULL DEFAULT '';
ALTER TABLE agreements ADD COLUMN channels TEXT;
ALTER TABLE agreements ADD COLUMN register_category TEXT
    CHECK (register_category IS NULL OR register_category IN ('residents', 'accommodation', 'protected', 'companies', 'associations', 'other'));
CREATE INDEX agreements_partner_id_idx ON agreements (partner_id) WHERE partner_id IS NOT NULL;
CREATE INDEX agreements_customer_category_id_idx ON agreements (customer_category_id) WHERE customer_category_id IS NOT NULL;

CREATE TABLE agreement_versions (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    agreement_id       TEXT NOT NULL REFERENCES agreements (id) ON DELETE CASCADE,
    version            INTEGER NOT NULL CHECK (version >= 1),
    snapshot           TEXT NOT NULL,
    changed_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    changed_at         TIMESTAMP NOT NULL
);
CREATE INDEX agreement_versions_establishment_id_idx ON agreement_versions (establishment_id);
CREATE UNIQUE INDEX agreement_versions_agreement_version_uidx ON agreement_versions (agreement_id, version);

CREATE TABLE agreement_quota_usage (
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    agreement_id     TEXT NOT NULL REFERENCES agreements (id) ON DELETE CASCADE,
    date             TEXT NOT NULL,
    slot             TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    -- Capacity held back for the agreement until release_on (0 = none).
    reserved         INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    -- Units sold in the agreement on this half-day.
    used             INTEGER NOT NULL DEFAULT 0 CHECK (used >= 0),
    -- Cap of agreement sales (max_per_day); NULL = no cap.
    max_units        INTEGER CHECK (max_units IS NULL OR max_units >= 0),
    release_on       TEXT NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (agreement_id, date, slot),
    CHECK (max_units IS NULL OR used <= max_units)
);
CREATE INDEX agreement_quota_usage_establishment_date_idx ON agreement_quota_usage (establishment_id, date);

CREATE TABLE agreement_quota_allocations (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    agreement_id     TEXT NOT NULL REFERENCES agreements (id) ON DELETE CASCADE,
    booking_id       TEXT NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    booking_line_id  TEXT NOT NULL REFERENCES booking_lines (id) ON DELETE CASCADE,
    date             TEXT NOT NULL,
    slot             TEXT NOT NULL CHECK (slot IN ('AM', 'PM')),
    quantity         INTEGER NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    created_at       TIMESTAMP NOT NULL
);
CREATE INDEX agreement_quota_allocations_establishment_date_idx ON agreement_quota_allocations (establishment_id, date);
CREATE INDEX agreement_quota_allocations_booking_id_idx ON agreement_quota_allocations (booking_id);
CREATE UNIQUE INDEX agreement_quota_allocations_line_date_slot_uidx ON agreement_quota_allocations (booking_line_id, date, slot);

ALTER TABLE bookings ADD COLUMN partner_id TEXT REFERENCES partners (id) ON DELETE RESTRICT;
ALTER TABLE bookings ADD COLUMN group_name TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN group_origin TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN party_adults INTEGER CHECK (party_adults IS NULL OR party_adults >= 0);
ALTER TABLE bookings ADD COLUMN party_children INTEGER CHECK (party_children IS NULL OR party_children >= 0);
ALTER TABLE bookings ADD COLUMN guest_names TEXT;
ALTER TABLE bookings ADD COLUMN partner_share_type TEXT CHECK (partner_share_type IS NULL OR partner_share_type IN ('full', 'percent', 'amount'));
-- percent: basis points (0..10000); amount: cents.
ALTER TABLE bookings ADD COLUMN partner_share_value INTEGER CHECK (partner_share_value IS NULL OR partner_share_value >= 0);
-- Resolved charge on the partner account, kept with total_amount.
ALTER TABLE bookings ADD COLUMN partner_amount INTEGER NOT NULL DEFAULT 0 CHECK (partner_amount >= 0);
ALTER TABLE bookings ADD COLUMN partner_stay_from TEXT;
ALTER TABLE bookings ADD COLUMN partner_stay_to TEXT;
CREATE INDEX bookings_establishment_partner_idx ON bookings (establishment_id, partner_id) WHERE partner_id IS NOT NULL;
CREATE INDEX bookings_establishment_agreement_idx ON bookings (establishment_id, agreement_id) WHERE agreement_id IS NOT NULL;

CREATE TABLE partner_payments (
    id                  TEXT PRIMARY KEY,
    establishment_id    TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    partner_id          TEXT NOT NULL REFERENCES partners (id) ON DELETE RESTRICT,
    amount              INTEGER NOT NULL CHECK (amount > 0),
    received_on         TEXT NOT NULL,
    method              TEXT NOT NULL CHECK (method IN ('bank_transfer', 'cash', 'card', 'other')),
    reference           TEXT NOT NULL DEFAULT '',
    note                TEXT NOT NULL DEFAULT '',
    recorded_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    voided_at           TIMESTAMP,
    void_reason         TEXT,
    voided_by_user_id   TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL,
    CHECK ((voided_at IS NULL) = (void_reason IS NULL))
);
CREATE INDEX partner_payments_establishment_partner_idx ON partner_payments (establishment_id, partner_id, received_on);

CREATE TABLE requirement_verifications (
    id                   TEXT PRIMARY KEY,
    establishment_id     TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    subject              TEXT NOT NULL CHECK (subject IN ('booking', 'customer')),
    booking_id           TEXT REFERENCES bookings (id) ON DELETE CASCADE,
    customer_id          TEXT REFERENCES customers (id) ON DELETE CASCADE,
    agreement_id         TEXT REFERENCES agreements (id) ON DELETE SET NULL,
    customer_category_id TEXT REFERENCES customer_categories (id) ON DELETE SET NULL,
    requirement          TEXT NOT NULL DEFAULT '',
    note                 TEXT NOT NULL DEFAULT '',
    verified_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    verified_at          TIMESTAMP NOT NULL,
    CHECK ((subject = 'booking') = (booking_id IS NOT NULL)),
    CHECK (subject <> 'customer' OR customer_id IS NOT NULL)
);
CREATE INDEX requirement_verifications_establishment_id_idx ON requirement_verifications (establishment_id);
CREATE INDEX requirement_verifications_booking_id_idx ON requirement_verifications (booking_id);
CREATE INDEX requirement_verifications_customer_id_idx ON requirement_verifications (customer_id);

CREATE TABLE move_undo_log (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    user_id          TEXT REFERENCES users (id) ON DELETE CASCADE,
    kind             TEXT NOT NULL CHECK (kind IN ('move', 'swap', 'group')),
    -- Lines moved: booking, line, spot before/after, dates, booking version after.
    payload          TEXT NOT NULL,
    booking_ids      TEXT NOT NULL DEFAULT '[]',
    created_at       TIMESTAMP NOT NULL,
    expires_at       TIMESTAMP NOT NULL,
    undone_at        TIMESTAMP,
    CHECK (expires_at > created_at)
);
CREATE INDEX move_undo_log_establishment_user_idx ON move_undo_log (establishment_id, user_id, created_at);

ALTER TABLE spots ADD COLUMN online_excluded INTEGER NOT NULL DEFAULT 0 CHECK (online_excluded IN (0, 1));

ALTER TABLE extras ADD COLUMN show_on_map INTEGER NOT NULL DEFAULT 0 CHECK (show_on_map IN (0, 1));
ALTER TABLE extras ADD COLUMN map_label TEXT NOT NULL DEFAULT '';

-- INV-102: one spot per booking line and half-day.
CREATE UNIQUE INDEX spot_allocations_line_date_slot_uidx ON spot_allocations (booking_line_id, date, slot)
    WHERE booking_line_id IS NOT NULL;

-- +goose Down
DROP INDEX spot_allocations_line_date_slot_uidx;
ALTER TABLE extras DROP COLUMN map_label;
ALTER TABLE extras DROP COLUMN show_on_map;
ALTER TABLE spots DROP COLUMN online_excluded;
DROP TABLE move_undo_log;
DROP TABLE requirement_verifications;
DROP TABLE partner_payments;
DROP INDEX bookings_establishment_agreement_idx;
DROP INDEX bookings_establishment_partner_idx;
ALTER TABLE bookings DROP COLUMN partner_stay_to;
ALTER TABLE bookings DROP COLUMN partner_stay_from;
ALTER TABLE bookings DROP COLUMN partner_amount;
ALTER TABLE bookings DROP COLUMN partner_share_value;
ALTER TABLE bookings DROP COLUMN partner_share_type;
ALTER TABLE bookings DROP COLUMN guest_names;
ALTER TABLE bookings DROP COLUMN party_children;
ALTER TABLE bookings DROP COLUMN party_adults;
ALTER TABLE bookings DROP COLUMN group_origin;
ALTER TABLE bookings DROP COLUMN group_name;
ALTER TABLE bookings DROP COLUMN partner_id;
DROP TABLE agreement_quota_allocations;
DROP TABLE agreement_quota_usage;
DROP TABLE agreement_versions;
DROP INDEX agreements_customer_category_id_idx;
DROP INDEX agreements_partner_id_idx;
ALTER TABLE agreements DROP COLUMN register_category;
ALTER TABLE agreements DROP COLUMN channels;
ALTER TABLE agreements DROP COLUMN requirement;
ALTER TABLE agreements DROP COLUMN quota;
ALTER TABLE agreements DROP COLUMN season_ids;
ALTER TABLE agreements DROP COLUMN scope;
ALTER TABLE agreements DROP COLUMN pack_prices;
ALTER TABLE agreements DROP COLUMN customer_category_id;
ALTER TABLE agreements DROP COLUMN partner_id;
ALTER TABLE agreements DROP COLUMN target;
DROP TABLE customer_category_members;
DROP TABLE customer_categories;
DROP TABLE partners;
