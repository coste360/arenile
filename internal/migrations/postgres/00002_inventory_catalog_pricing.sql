-- M1/M2: seasons, rate periods, closures, maps and inventory, catalog, pricing
-- configuration and booking policies (spec 6.1-6.4).
-- IDs are uuid; dates are date; JSON as jsonb; booleans are boolean;
-- money is bigint cents.
-- Configuration rows carry `version` for ETag / If-Match (spec 17.1).

-- +goose Up
ALTER TABLE establishments ADD COLUMN version integer NOT NULL DEFAULT 1;

CREATE TABLE seasons (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    year             integer NOT NULL,
    opens_on         date NOT NULL,
    closes_on        date NOT NULL,
    status           text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'closed')),
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CHECK (closes_on >= opens_on)
);
CREATE UNIQUE INDEX seasons_establishment_year_uidx ON seasons (establishment_id, year);

CREATE TABLE closure_days (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    date             date NOT NULL,
    slots            jsonb NOT NULL DEFAULT '["AM","PM"]',
    reason           text NOT NULL DEFAULT '',
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX closure_days_establishment_date_uidx ON closure_days (establishment_id, date);

CREATE TABLE rate_periods (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        uuid NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    code             text NOT NULL,
    name             jsonb NOT NULL DEFAULT '{}',
    color            text NOT NULL DEFAULT '',
    ranges           jsonb NOT NULL DEFAULT '[]',
    sort_order       integer NOT NULL DEFAULT 0,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX rate_periods_establishment_id_idx ON rate_periods (establishment_id);
CREATE UNIQUE INDEX rate_periods_season_code_uidx ON rate_periods (season_id, code);

CREATE TABLE maps (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name             jsonb NOT NULL DEFAULT '{}',
    width            integer NOT NULL CHECK (width > 0),
    height           integer NOT NULL CHECK (height > 0),
    grid_size        integer NOT NULL DEFAULT 1 CHECK (grid_size > 0),
    background       jsonb,
    sea_side         text NOT NULL DEFAULT 'bottom' CHECK (sea_side IN ('top', 'bottom', 'left', 'right')),
    sort_order       integer NOT NULL DEFAULT 0,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX maps_establishment_id_idx ON maps (establishment_id);

CREATE TABLE zones (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id           uuid REFERENCES maps (id) ON DELETE SET NULL,
    code             text NOT NULL,
    name             jsonb NOT NULL DEFAULT '{}',
    assignment_mode  text NOT NULL CHECK (assignment_mode IN ('seat', 'zone', 'quantity')),
    online_visible   boolean NOT NULL DEFAULT true,
    online_quota     jsonb,
    price_category   text NOT NULL DEFAULT '',
    auto_assign      boolean NOT NULL DEFAULT false,
    sort_order       integer NOT NULL DEFAULT 0,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX zones_establishment_code_uidx ON zones (establishment_id, code);
CREATE INDEX zones_map_id_idx ON zones (map_id);

CREATE TABLE spots (
    id                      uuid PRIMARY KEY,
    establishment_id        uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id                  uuid NOT NULL REFERENCES maps (id) ON DELETE CASCADE,
    zone_id                 uuid NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    label                   text NOT NULL,
    row_label               text NOT NULL DEFAULT '',
    number                  integer NOT NULL DEFAULT 0,
    kind                    text NOT NULL DEFAULT 'umbrella' CHECK (kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom')),
    x                       double precision NOT NULL DEFAULT 0,
    y                       double precision NOT NULL DEFAULT 0,
    rotation                double precision NOT NULL DEFAULT 0,
    shape                   jsonb NOT NULL DEFAULT '{"type":"circle","w":1,"h":1}',
    capacity                jsonb NOT NULL DEFAULT '{}',
    max_extra               jsonb NOT NULL DEFAULT '{}',
    price_category_override text,
    attributes              jsonb NOT NULL DEFAULT '[]',
    active                  boolean NOT NULL DEFAULT true,
    created_at              timestamptz NOT NULL,
    updated_at              timestamptz NOT NULL
);
CREATE UNIQUE INDEX spots_map_label_uidx ON spots (map_id, label);
CREATE INDEX spots_establishment_id_idx ON spots (establishment_id);
CREATE INDEX spots_zone_id_idx ON spots (zone_id);

CREATE TABLE map_decorations (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id           uuid NOT NULL REFERENCES maps (id) ON DELETE CASCADE,
    kind             text NOT NULL CHECK (kind IN ('walkway', 'shower', 'lifeguard', 'bar', 'entrance', 'cabin_block', 'text', 'custom')),
    x                double precision NOT NULL DEFAULT 0,
    y                double precision NOT NULL DEFAULT 0,
    rotation         double precision NOT NULL DEFAULT 0,
    shape            jsonb NOT NULL DEFAULT '{"type":"rect","w":1,"h":1}',
    label            jsonb,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX map_decorations_establishment_id_idx ON map_decorations (establishment_id);
CREATE INDEX map_decorations_map_id_idx ON map_decorations (map_id);

CREATE TABLE pools (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    zone_id            uuid REFERENCES zones (id) ON DELETE CASCADE,
    component_code     text NOT NULL,
    name               jsonb NOT NULL DEFAULT '{}',
    capacity           integer NOT NULL DEFAULT 0 CHECK (capacity >= 0),
    capacity_overrides jsonb NOT NULL DEFAULT '[]',
    system_managed     boolean NOT NULL DEFAULT false,
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);
CREATE INDEX pools_establishment_id_idx ON pools (establishment_id);
CREATE INDEX pools_zone_id_idx ON pools (zone_id);
-- At most one implicit (system managed) pool per zone (spec 7.5).
CREATE UNIQUE INDEX pools_zone_system_uidx ON pools (zone_id) WHERE system_managed;

CREATE TABLE components (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             text NOT NULL,
    name             jsonb NOT NULL DEFAULT '{}',
    tracked          boolean NOT NULL DEFAULT false,
    pool_id          uuid REFERENCES pools (id) ON DELETE SET NULL,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX components_establishment_code_uidx ON components (establishment_id, code);

CREATE TABLE packs (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code               text NOT NULL,
    name               jsonb NOT NULL DEFAULT '{}',
    description        jsonb NOT NULL DEFAULT '{}',
    image              text,
    composition        jsonb NOT NULL DEFAULT '{}',
    requires_spot_kind jsonb,
    allowed_zone_ids   jsonb,
    allowed_formulas   jsonb NOT NULL DEFAULT '[]',
    channels           jsonb NOT NULL DEFAULT '["desk","online"]',
    max_per_booking    integer NOT NULL DEFAULT 1 CHECK (max_per_booking >= 1),
    requires_approval  boolean NOT NULL DEFAULT false,
    sort_order         integer NOT NULL DEFAULT 0,
    active             boolean NOT NULL DEFAULT true,
    vat_rate_bp        bigint CHECK (vat_rate_bp IS NULL OR (vat_rate_bp >= 0 AND vat_rate_bp <= 10000)),
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX packs_establishment_code_uidx ON packs (establishment_id, code);

CREATE TABLE extras (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             text NOT NULL,
    name             jsonb NOT NULL DEFAULT '{}',
    description      jsonb NOT NULL DEFAULT '{}',
    component_code   text,
    pricing_unit     text NOT NULL CHECK (pricing_unit IN ('per_booking', 'per_day', 'per_slot')),
    max_quantity     integer NOT NULL DEFAULT 1 CHECK (max_quantity >= 1),
    attach_to        text NOT NULL DEFAULT 'line' CHECK (attach_to IN ('line', 'booking')),
    allowed_pack_ids jsonb,
    channels         jsonb NOT NULL DEFAULT '["desk","online"]',
    sort_order       integer NOT NULL DEFAULT 0,
    active           boolean NOT NULL DEFAULT true,
    vat_rate_bp      bigint CHECK (vat_rate_bp IS NULL OR (vat_rate_bp >= 0 AND vat_rate_bp <= 10000)),
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX extras_establishment_code_uidx ON extras (establishment_id, code);

CREATE TABLE price_rules (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        uuid NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    name             text NOT NULL,
    target_type      text NOT NULL CHECK (target_type IN ('pack', 'extra')),
    target_ids       jsonb,
    conditions       jsonb NOT NULL DEFAULT '{}',
    action           jsonb NOT NULL,
    priority         integer NOT NULL DEFAULT 0,
    stage            text NOT NULL CHECK (stage IN ('base', 'modifier', 'discount')),
    stackable        boolean NOT NULL DEFAULT false,
    active           boolean NOT NULL DEFAULT true,
    -- Recency tie-break of the pricing engine (spec 8.4): higher = more recent.
    seq              bigint NOT NULL,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX price_rules_establishment_id_idx ON price_rules (establishment_id);
CREATE INDEX price_rules_season_id_idx ON price_rules (season_id);

CREATE TABLE fixed_duration_prices (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        uuid NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    pack_id          uuid NOT NULL REFERENCES packs (id) ON DELETE CASCADE,
    formula          text NOT NULL CHECK (formula IN ('week', 'fortnight', 'month', 'season')),
    price_category   text NOT NULL,
    rate_period_code text,
    amount           bigint NOT NULL CHECK (amount >= 0),
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX fixed_duration_prices_establishment_id_idx ON fixed_duration_prices (establishment_id);
CREATE UNIQUE INDEX fixed_duration_prices_key_uidx
    ON fixed_duration_prices (season_id, pack_id, formula, price_category, COALESCE(rate_period_code, ''));

CREATE TABLE promo_codes (
    id                    uuid PRIMARY KEY,
    establishment_id      uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code                  text NOT NULL,
    description           text NOT NULL DEFAULT '',
    action                jsonb NOT NULL,
    conditions            jsonb NOT NULL DEFAULT '{}',
    valid_from            date,
    valid_to              date,
    max_uses              integer CHECK (max_uses IS NULL OR max_uses >= 1),
    max_uses_per_customer integer CHECK (max_uses_per_customer IS NULL OR max_uses_per_customer >= 1),
    uses                  integer NOT NULL DEFAULT 0 CHECK (uses >= 0),
    stackable             boolean NOT NULL DEFAULT false,
    active                boolean NOT NULL DEFAULT true,
    version               integer NOT NULL DEFAULT 1,
    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL
);
-- Case-insensitive uniqueness per establishment (spec 6.4).
CREATE UNIQUE INDEX promo_codes_establishment_code_uidx ON promo_codes (establishment_id, lower(code));

CREATE TABLE agreements (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    code             text,
    name             text NOT NULL,
    partner_name     text NOT NULL DEFAULT '',
    notes            text NOT NULL DEFAULT '',
    action           jsonb NOT NULL,
    conditions       jsonb NOT NULL DEFAULT '{}',
    valid_from       date,
    valid_to         date,
    stackable        boolean NOT NULL DEFAULT false,
    active           boolean NOT NULL DEFAULT true,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX agreements_establishment_id_idx ON agreements (establishment_id);
CREATE UNIQUE INDEX agreements_establishment_code_uidx ON agreements (establishment_id, lower(code))
    WHERE code IS NOT NULL;

CREATE TABLE booking_policies (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    season_id        uuid NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    name             text NOT NULL,
    scope            jsonb NOT NULL DEFAULT '{}',
    rules            jsonb NOT NULL DEFAULT '{}',
    priority         integer NOT NULL DEFAULT 0,
    active           boolean NOT NULL DEFAULT true,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
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
