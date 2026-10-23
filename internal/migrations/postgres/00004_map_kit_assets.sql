-- Beach map kit (docs/adr/0012-kit-mappa.md): the illustration chosen for a
-- spot or decoration (`asset`, an id of assets/beach-map-kit/manifest.json,
-- validated by the application against the generated list) and the colour
-- variant of a decoration (`variant`, e.g. the sea flag). Spot kinds gain the
-- bookable families of the kit: sunbed, daybed, cabana, hammock.

-- +goose Up
ALTER TABLE spots DROP CONSTRAINT spots_kind_check;
ALTER TABLE spots ADD CONSTRAINT spots_kind_check
    CHECK (kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom', 'sunbed', 'daybed', 'cabana', 'hammock'));
ALTER TABLE spots ADD COLUMN asset text;
ALTER TABLE map_decorations ADD COLUMN asset text;
ALTER TABLE map_decorations ADD COLUMN variant text;

-- +goose Down
ALTER TABLE map_decorations DROP COLUMN variant;
ALTER TABLE map_decorations DROP COLUMN asset;
ALTER TABLE spots DROP COLUMN asset;
UPDATE spots SET kind = 'custom' WHERE kind NOT IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom');
ALTER TABLE spots DROP CONSTRAINT spots_kind_check;
ALTER TABLE spots ADD CONSTRAINT spots_kind_check
    CHECK (kind IN ('umbrella', 'gazebo', 'tent', 'palm', 'custom'));
