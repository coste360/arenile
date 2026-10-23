-- M6/M7: paired devices (mobile app, desktop app in remote mode), one-time
-- pairing codes and the fiscal bridge towards the FiscalDriver of a desktop
-- app in remote mode (spec 29.1, 29.2, 14.2).
-- IDs are UUIDv7 stored as TEXT; JSON as TEXT; booleans INTEGER 0/1;
-- instants UTC timestamps.
--
-- Tokens are never stored in clear:
--   * devices.token_hash is the SHA-256 (hex) of the device token
--     ar_dev_<prefix>_<secret>; token_prefix is the visible part;
--   * device_pairing_codes.code_hash is the SHA-256 (hex) of the normalised
--     8-character code; the code exists only in the panel and in the QR.
-- A device belongs to one user and one establishment: its principal has the
-- user's role narrowed to that establishment.

-- +goose Up
CREATE TABLE devices (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    platform         TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'windows', 'macos', 'linux', 'other')),
    kind             TEXT NOT NULL CHECK (kind IN ('mobile', 'desktop')),
    app_version      TEXT,
    token_prefix     TEXT NOT NULL,
    token_hash       TEXT NOT NULL UNIQUE,
    -- Pairing code that created the device (audit; NULL after code cleanup).
    pairing_code_id  TEXT,
    created_at       TIMESTAMP NOT NULL,
    last_used_at     TIMESTAMP,
    revoked_at       TIMESTAMP,
    updated_at       TIMESTAMP NOT NULL
);
CREATE INDEX devices_establishment_id_idx ON devices (establishment_id);
CREATE INDEX devices_user_id_idx ON devices (user_id);

-- Short-lived, single-use pairing codes. Consumed with a conditional UPDATE
-- (used_at IS NULL AND expires_at > now), so concurrent exchanges of the
-- same code produce one device at most.
CREATE TABLE device_pairing_codes (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    user_id            TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    kind               TEXT CHECK (kind IN ('mobile', 'desktop')),
    code_hash          TEXT NOT NULL UNIQUE,
    expires_at         TIMESTAMP NOT NULL,
    used_at            TIMESTAMP,
    device_id          TEXT REFERENCES devices (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL
);
CREATE INDEX device_pairing_codes_establishment_id_idx ON device_pairing_codes (establishment_id);
CREATE INDEX device_pairing_codes_expires_at_idx ON device_pairing_codes (expires_at);

-- Fiscal bridge registrations: the device that executes fiscal jobs on its
-- local FiscalDriver. At most one active registration per establishment.
CREATE TABLE fiscal_bridges (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    device_id        TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    driver           TEXT NOT NULL,
    active           INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    -- Last FiscalDeviceStatus reported by the device: {"online": bool, "message": "...", "checked_at": "..."}.
    device_status    TEXT NOT NULL DEFAULT '{}',
    last_seen_at     TIMESTAMP,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX fiscal_bridges_active_uidx ON fiscal_bridges (establishment_id) WHERE active = 1;
CREATE INDEX fiscal_bridges_device_id_idx ON fiscal_bridges (device_id);

-- Fiscal jobs dispatched to the bridge (at-least-once with leases).
-- (establishment_id, kind, idempotency_key) is unique: the bridge driver enqueues
-- idempotently every time the fiscal job retries a pending document.
CREATE TABLE fiscal_bridge_jobs (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    fiscal_document_id TEXT REFERENCES fiscal_documents (id) ON DELETE CASCADE,
    kind               TEXT NOT NULL CHECK (kind IN ('issue', 'void')),
    -- Idempotency key of the ext.FiscalDocumentRequest (or of the void).
    idempotency_key    TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (status IN ('queued', 'leased', 'succeeded', 'failed')),
    -- FiscalBridgeDocument (issue) or {"ref": "..."} (void); amounts in cents.
    payload            TEXT NOT NULL DEFAULT '{}',
    -- FiscalBridgeJobResult as received from the device.
    result             TEXT,
    attempts           INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    leased_device_id   TEXT REFERENCES devices (id) ON DELETE SET NULL,
    lease_expires_at   TIMESTAMP,
    next_attempt_at    TIMESTAMP NOT NULL,
    completed_at       TIMESTAMP,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX fiscal_bridge_jobs_establishment_key_uidx ON fiscal_bridge_jobs (establishment_id, kind, idempotency_key);
CREATE INDEX fiscal_bridge_jobs_establishment_status_idx ON fiscal_bridge_jobs (establishment_id, status, next_attempt_at);
CREATE INDEX fiscal_bridge_jobs_fiscal_document_id_idx ON fiscal_bridge_jobs (fiscal_document_id);

-- +goose Down
DROP TABLE fiscal_bridge_jobs;
DROP TABLE fiscal_bridges;
DROP TABLE device_pairing_codes;
DROP TABLE devices;
