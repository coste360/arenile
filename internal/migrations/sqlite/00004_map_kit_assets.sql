-- Beach map kit (docs/adr/0012-kit-mappa.md): the illustration chosen for a
-- spot or decoration (`asset`, an id of assets/beach-map-kit/manifest.json,
-- validated by the application against the generated list) and the colour
-- variant of a decoration (`variant`, e.g. the sea flag). Spot kinds gain the
-- bookable families of the kit: sunbed, daybed, cabana, hammock.
--
-- SQLite cannot alter a CHECK constraint, so `spots` is rebuilt with the
-- documented procedure (https://www.sqlite.org/lang_altertable.html#otheralter):
-- foreign keys off OUTSIDE the transaction (other tables reference spots with
-- ON DELETE CASCADE/RESTRICT: dropping the old table with foreign keys on
-- would cascade), copy, drop, rename, indexes, foreign keys on (the rows are
-- copied verbatim, so every reference stays valid). goose runs a NO TRANSACTION migration on a single connection, so the
-- pragma and the explicit transaction apply to the same session.

-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;

BEGIN IMMEDIATE;

CREATE TABLE spots_new (
    id                      TEXT PRIMARY KEY,
    establishment_id        TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    map_id                  TEXT NOT NULL REFERENCES maps (id) ON DELETE CASCADE,
    zone_id                 TEXT NOT NULL REFERENCES zones (id) ON DELETE RESTRICT,
    label                   TEXT NOT NULL,
    row_label               TEXT NOT NULL DEFAULT '',
    number                  INTEGER NOT NULL DEFAULT 0,
    kind                    TEXT NOT NULL DEFAULT 'umbrella' CHECK (kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom', 'sunbed', 'daybed', 'cabana', 'hammock')),
    asset                   TEXT,
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

INSERT INTO spots_new (id, establishment_id, map_id, zone_id, label, row_label, number, kind, x, y,
    rotation, shape, capacity, max_extra, price_category_override, attributes, active, created_at, updated_at)
SELECT id, establishment_id, map_id, zone_id, label, row_label, number, kind, x, y,
    rotation, shape, capacity, max_extra, price_category_override, attributes, active, created_at, updated_at
FROM spots;

DROP TABLE spots;

ALTER TABLE spots_new RENAME TO spots;

CREATE UNIQUE INDEX spots_map_label_uidx ON spots (map_id, label);
CREATE INDEX spots_establishment_id_idx ON spots (establishment_id);
CREATE INDEX spots_zone_id_idx ON spots (zone_id);

ALTER TABLE map_decorations ADD COLUMN asset TEXT;
ALTER TABLE map_decorations ADD COLUMN variant TEXT;

COMMIT;

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;

BEGIN IMMEDIATE;

ALTER TABLE map_decorations DROP COLUMN variant;
ALTER TABLE map_decorations DROP COLUMN asset;

CREATE TABLE spots_old (
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

INSERT INTO spots_old (id, establishment_id, map_id, zone_id, label, row_label, number, kind, x, y,
    rotation, shape, capacity, max_extra, price_category_override, attributes, active, created_at, updated_at)
SELECT id, establishment_id, map_id, zone_id, label, row_label, number,
    CASE WHEN kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom') THEN kind ELSE 'custom' END,
    x, y, rotation, shape, capacity, max_extra, price_category_override, attributes, active, created_at, updated_at
FROM spots;

DROP TABLE spots;

ALTER TABLE spots_old RENAME TO spots;

CREATE UNIQUE INDEX spots_map_label_uidx ON spots (map_id, label);
CREATE INDEX spots_establishment_id_idx ON spots (establishment_id);
CREATE INDEX spots_zone_id_idx ON spots (zone_id);

COMMIT;

PRAGMA foreign_keys = ON;
