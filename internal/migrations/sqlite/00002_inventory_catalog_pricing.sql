-- M1/M2: seasons, rate periods, closures, maps and inventory, catalog, pricing
-- configuration and booking policies (spec 6.1-6.4).
-- IDs are UUIDv7 stored as TEXT; dates are TEXT 'YYYY-MM-DD'; JSON as TEXT;
-- booleans are INTEGER 0/1; money is INTEGER cents.
-- Configuration rows carry `version` for ETag / If-Match (spec 17.1).

-- +goose Up
ALTER TABLE establishments ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

CREATE TABLE seasons (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    year             INTEGER NOT NULL,
    opens_on         TEXT NOT NULL,
    closes_on        TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'closed')),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    CHECK (closes_on >= opens_on)
);
CREATE UNIQUE INDEX seasons_establishment_year_uidx ON seasons (establishment_id, year);

CREATE TABLE closure_days (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    date             TEXT NOT NULL,
    slots            TEXT NOT NULL DEFAULT '["AM","PM"]',
    reason           TEXT NOT NULL DEFAULT '',
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX closure_days_establishment_date_uidx ON closure_days (establishment_id, date);

CREATE TABLE rate_periods (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL DEFAULT '{}',
    color            TEXT NOT NULL DEFAULT '',
    ranges           TEXT NOT NULL DEFAULT '[]',
    sort_order       INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX rate_periods_establishment_id_idx ON rate_periods (establishment_id);
CREATE UNIQUE INDEX rate_periods_season_code_uidx ON rate_periods (season_id, code);

CREATE TABLE maps (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name             TEXT NOT NULL DEFAULT '{}',
    width            INTEGER NOT NULL CHECK (width > 0),
    height           INTEGER NOT NULL CHECK (height > 0),
    grid_size        INTEGER NOT NULL DEFAULT 1 CHECK (grid_size > 0),
    background       TEXT,
    sea_side         TEXT NOT NULL DEFAULT 'bottom' CHECK (sea_side IN ('top', 'bottom', 'left', 'right')),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX maps_establishment_id_idx ON maps (establishment_id);

CREATE TABLE zones (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id           TEXT REFERENCES maps (id) ON DELETE SET NULL,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL DEFAULT '{}',
    assignment_mode  TEXT NOT NULL CHECK (assignment_mode IN ('seat', 'zone', 'quantity')),
    online_visible   INTEGER NOT NULL DEFAULT 1 CHECK (online_visible IN (0, 1)),
    online_quota     TEXT,
    price_category   TEXT NOT NULL DEFAULT '',
    auto_assign      INTEGER NOT NULL DEFAULT 0 CHECK (auto_assign IN (0, 1)),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX zones_establishment_code_uidx ON zones (establishment_id, code);
CREATE INDEX zones_map_id_idx ON zones (map_id);

CREATE TABLE spots (
    id                      TEXT PRIMARY KEY,
    establishment_id        TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id                  TEXT NOT NULL REFERENCES maps (id) ON DELETE CASCADE,
    zone_id                 TEXT NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    label                   TEXT NOT NULL,
    row_label               TEXT NOT NULL DEFAULT '',
    number                  INTEGER NOT NULL DEFAULT 0,
    kind                    TEXT NOT NULL DEFAULT 'umbrella' CHECK (kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom')),
    x                       REAL NOT NULL DEFAULT 0,
    y                       REAL NOT NULL DEFAULT 0,
    rotation                REAL NOT NULL DEFAULT 0,
    shape                   TEXT NOT NULL DEFAULT '{"type":"circle","w":1,"h":1}',
    capacity                TEXT NOT NULL DEFAULT '{}',
    max_extra               TEXT NOT NULL DEFAULT '{}',
    price_category_override TEXT,
    attributes              TEXT NOT NULL DEFAULT '[]',
    active                  INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at              TIMESTAMP NOT NULL,
    updated_at              TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX spots_map_label_uidx ON spots (map_id, label);
CREATE INDEX spots_establishment_id_idx ON spots (establishment_id);
CREATE INDEX spots_zone_id_idx ON spots (zone_id);

CREATE TABLE map_decorations (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id           TEXT NOT NULL REFERENCES maps (id) ON DELETE CASCADE,
    kind             TEXT NOT NULL CHECK (kind IN ('walkway', 'shower', 'lifeguard', 'bar', 'entrance', 'cabin_block', 'text', 'custom')),
    x                REAL NOT NULL DEFAULT 0,
    y                REAL NOT NULL DEFAULT 0,
    rotation         REAL NOT NULL DEFAULT 0,
    shape            TEXT NOT NULL DEFAULT '{"type":"rect","w":1,"h":1}',
    label            TEXT,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX map_decorations_establishment_id_idx ON map_decorations (establishment_id);
CREATE INDEX map_decorations_map_id_idx ON map_decorations (map_id);

CREATE TABLE pools (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    zone_id            TEXT REFERENCES zones (id) ON DELETE CASCADE,
    component_code     TEXT NOT NULL,
    name               TEXT NOT NULL DEFAULT '{}',
    capacity           INTEGER NOT NULL DEFAULT 0 CHECK (capacity >= 0),
    capacity_overrides TEXT NOT NULL DEFAULT '[]',
    system_managed     INTEGER NOT NULL DEFAULT 0 CHECK (system_managed IN (0, 1)),
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE INDEX pools_establishment_id_idx ON pools (establishment_id);
CREATE INDEX pools_zone_id_idx ON pools (zone_id);
-- At most one implicit (system managed) pool per zone (spec 7.5).
CREATE UNIQUE INDEX pools_zone_system_uidx ON pools (zone_id) WHERE system_managed = 1;

CREATE TABLE components (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL DEFAULT '{}',
    tracked          INTEGER NOT NULL DEFAULT 0 CHECK (tracked IN (0, 1)),
    pool_id          TEXT REFERENCES pools (id) ON DELETE SET NULL,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX components_establishment_code_uidx ON components (establishment_id, code);

CREATE TABLE packs (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code               TEXT NOT NULL,
    name               TEXT NOT NULL DEFAULT '{}',
    description        TEXT NOT NULL DEFAULT '{}',
    image              TEXT,
    composition        TEXT NOT NULL DEFAULT '{}',
    requires_spot_kind TEXT,
    allowed_zone_ids   TEXT,
    allowed_formulas   TEXT NOT NULL DEFAULT '[]',
    channels           TEXT NOT NULL DEFAULT '["desk","online"]',
    max_per_booking    INTEGER NOT NULL DEFAULT 1 CHECK (max_per_booking >= 1),
    requires_approval  INTEGER NOT NULL DEFAULT 0 CHECK (requires_approval IN (0, 1)),
    sort_order         INTEGER NOT NULL DEFAULT 0,
    active             INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    vat_rate_bp        INTEGER CHECK (vat_rate_bp IS NULL OR (vat_rate_bp >= 0 AND vat_rate_bp <= 10000)),
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX packs_establishment_code_uidx ON packs (establishment_id, code);

CREATE TABLE extras (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL DEFAULT '{}',
    description      TEXT NOT NULL DEFAULT '{}',
    component_code   TEXT,
    pricing_unit     TEXT NOT NULL CHECK (pricing_unit IN ('per_booking', 'per_day', 'per_slot')),
    max_quantity     INTEGER NOT NULL DEFAULT 1 CHECK (max_quantity >= 1),
    attach_to        TEXT NOT NULL DEFAULT 'line' CHECK (attach_to IN ('line', 'booking')),
    allowed_pack_ids TEXT,
    channels         TEXT NOT NULL DEFAULT '["desk","online"]',
    sort_order       INTEGER NOT NULL DEFAULT 0,
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    vat_rate_bp      INTEGER CHECK (vat_rate_bp IS NULL OR (vat_rate_bp >= 0 AND vat_rate_bp <= 10000)),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX extras_establishment_code_uidx ON extras (establishment_id, code);

CREATE TABLE price_rules (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    target_type      TEXT NOT NULL CHECK (target_type IN ('pack', 'extra')),
    target_ids       TEXT,
    conditions       TEXT NOT NULL DEFAULT '{}',
    action           TEXT NOT NULL,
    priority         INTEGER NOT NULL DEFAULT 0,
    stage            TEXT NOT NULL CHECK (stage IN ('base', 'modifier', 'discount')),
    stackable        INTEGER NOT NULL DEFAULT 0 CHECK (stackable IN (0, 1)),
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    -- Recency tie-break of the pricing engine (spec 8.4): higher = more recent.
    seq              INTEGER NOT NULL,
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX price_rules_establishment_id_idx ON price_rules (establishment_id);
CREATE INDEX price_rules_season_id_idx ON price_rules (season_id);

CREATE TABLE fixed_duration_prices (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    pack_id          TEXT NOT NULL REFERENCES packs (id) ON DELETE CASCADE,
    formula          TEXT NOT NULL CHECK (formula IN ('week', 'fortnight', 'month', 'season')),
    price_category   TEXT NOT NULL,
    rate_period_code TEXT,
    amount           INTEGER NOT NULL CHECK (amount >= 0),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX fixed_duration_prices_establishment_id_idx ON fixed_duration_prices (establishment_id);
CREATE UNIQUE INDEX fixed_duration_prices_key_uidx
    ON fixed_duration_prices (season_id, pack_id, formula, price_category, COALESCE(rate_period_code, ''));

CREATE TABLE promo_codes (
    id                    TEXT PRIMARY KEY,
    establishment_id      TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code                  TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL,
    conditions            TEXT NOT NULL DEFAULT '{}',
    valid_from            TEXT,
    valid_to              TEXT,
    max_uses              INTEGER CHECK (max_uses IS NULL OR max_uses >= 1),
    max_uses_per_customer INTEGER CHECK (max_uses_per_customer IS NULL OR max_uses_per_customer >= 1),
    uses                  INTEGER NOT NULL DEFAULT 0 CHECK (uses >= 0),
    stackable             INTEGER NOT NULL DEFAULT 0 CHECK (stackable IN (0, 1)),
    active                INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version               INTEGER NOT NULL DEFAULT 1,
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL
);
-- Case-insensitive uniqueness per establishment (spec 6.4).
CREATE UNIQUE INDEX promo_codes_establishment_code_uidx ON promo_codes (establishment_id, lower(code));

CREATE TABLE agreements (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             TEXT,
    name             TEXT NOT NULL,
    partner_name     TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    action           TEXT NOT NULL,
    conditions       TEXT NOT NULL DEFAULT '{}',
    valid_from       TEXT,
    valid_to         TEXT,
    stackable        INTEGER NOT NULL DEFAULT 0 CHECK (stackable IN (0, 1)),
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX agreements_establishment_id_idx ON agreements (establishment_id);
CREATE UNIQUE INDEX agreements_establishment_code_uidx ON agreements (establishment_id, lower(code))
    WHERE code IS NOT NULL;

CREATE TABLE booking_policies (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    scope            TEXT NOT NULL DEFAULT '{}',
    rules            TEXT NOT NULL DEFAULT '{}',
    priority         INTEGER NOT NULL DEFAULT 0,
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version          INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX booking_policies_establishment_id_idx ON booking_policies (establishment_id);
CREATE INDEX booking_policies_season_id_idx ON booking_policies (season_id);

-- +goose Down
DROP TABLE booking_policies;
DROP TABLE agreements;
DROP TABLE promo_codes;
DROP TABLE fixed_duration_prices;
DROP TABLE price_rules;
DROP TABLE extras;
DROP TABLE packs;
DROP TABLE components;
DROP TABLE pools;
DROP TABLE map_decorations;
DROP TABLE spots;
DROP TABLE zones;
DROP TABLE maps;
DROP TABLE rate_periods;
DROP TABLE closure_days;
DROP TABLE seasons;
ALTER TABLE establishments DROP COLUMN version;
