-- M5 part A: API keys with expiry, configurable outgoing webhooks and
-- report indexes (spec 15.3, 16, 17, 18.2, 22).
-- 00006 is reserved for M6 (desktop edition, another branch).
--
-- The tables api_keys, webhook_endpoints and webhook_deliveries exist since
-- 00001 together with the delivery machinery (internal/domain/event): this
-- migration adds only what the panel management needs.
--   * api_keys.expires_at: optional expiry (NULL = never). Keys are still
--     stored as SHA-256 hashes (key_hash); prefix is the visible part.
--   * webhook_endpoints.description: panel label (the creator is in the audit log).
--   * webhook_endpoints.previous_secret / previous_secret_expires_at: secret
--     rotation with a grace period; previous_secret is a key ring ciphertext
--     like secret (AES-256-GCM, AAD = "<endpoint id>:previous"), never in clear.
--   * webhook_endpoints.last_delivery_at: last attempt of any delivery.
--   * webhook_deliveries_endpoint_created_idx: delivery log, newest first.
--   * bookings_establishment_created_idx: reports by sale date.
-- Reports are plain queries (no materialisation); see internal/domain/report.
-- maps.sea_side keeps its column DEFAULT 'bottom' in both dialects (SQLite
-- cannot change a default without a table rebuild): the application always writes the
-- value and defaults new maps to 'top' (ADR 0016).

-- +goose Up
ALTER TABLE api_keys ADD COLUMN expires_at timestamptz;

ALTER TABLE webhook_endpoints ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE webhook_endpoints ADD COLUMN previous_secret text;
ALTER TABLE webhook_endpoints ADD COLUMN previous_secret_expires_at timestamptz;
ALTER TABLE webhook_endpoints ADD COLUMN last_delivery_at timestamptz;

CREATE INDEX webhook_deliveries_endpoint_created_idx ON webhook_deliveries (endpoint_id, created_at);
CREATE INDEX bookings_establishment_created_idx ON bookings (establishment_id, created_at);

-- +goose Down
DROP INDEX bookings_establishment_created_idx;
DROP INDEX webhook_deliveries_endpoint_created_idx;
ALTER TABLE webhook_endpoints DROP COLUMN last_delivery_at;
ALTER TABLE webhook_endpoints DROP COLUMN previous_secret_expires_at;
ALTER TABLE webhook_endpoints DROP COLUMN previous_secret;
ALTER TABLE webhook_endpoints DROP COLUMN description;
ALTER TABLE api_keys DROP COLUMN expires_at;
