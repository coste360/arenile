-- M6/M7: paired devices (mobile app, desktop app in remote mode), one-time
-- pairing codes and the fiscal bridge towards the FiscalDriver of a desktop
-- app in remote mode (spec 29.1, 29.2, 14.2).
-- IDs are uuid; JSON as jsonb; booleans boolean;
-- instants timestamptz.
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
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             text NOT NULL,
    platform         text NOT NULL CHECK (platform IN ('ios', 'android', 'windows', 'macos', 'linux', 'other')),
    kind             text NOT NULL CHECK (kind IN ('mobile', 'desktop')),
    app_version      text,
    token_prefix     text NOT NULL,
    token_hash       text NOT NULL UNIQUE,
    -- Pairing code that created the device (audit; NULL after code cleanup).
    pairing_code_id  uuid,
    created_at       timestamptz NOT NULL,
    last_used_at     timestamptz,
    revoked_at       timestamptz,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX devices_establishment_id_idx ON devices (establishment_id);
CREATE INDEX devices_user_id_idx ON devices (user_id);

-- Short-lived, single-use pairing codes. Consumed with a conditional UPDATE
-- (used_at IS NULL AND expires_at > now), so concurrent exchanges of the
-- same code produce one device at most.
CREATE TABLE device_pairing_codes (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    user_id            uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    kind               text CHECK (kind IN ('mobile', 'desktop')),
    code_hash          text NOT NULL UNIQUE,
    expires_at         timestamptz NOT NULL,
    used_at            timestamptz,
    device_id          uuid REFERENCES devices (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL
);
CREATE INDEX device_pairing_codes_establishment_id_idx ON device_pairing_codes (establishment_id);
CREATE INDEX device_pairing_codes_expires_at_idx ON device_pairing_codes (expires_at);

-- Fiscal bridge registrations: the device that executes fiscal jobs on its
-- local FiscalDriver. At most one active registration per establishment.
CREATE TABLE fiscal_bridges (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    device_id        uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    driver           text NOT NULL,
    active           boolean NOT NULL DEFAULT true,
    -- Last FiscalDeviceStatus reported by the device: {"online": bool, "message": "...", "checked_at": "..."}.
    device_status    jsonb NOT NULL DEFAULT '{}',
    last_seen_at     timestamptz,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX fiscal_bridges_active_uidx ON fiscal_bridges (establishment_id) WHERE active;
CREATE INDEX fiscal_bridges_device_id_idx ON fiscal_bridges (device_id);

-- Fiscal jobs dispatched to the bridge (at-least-once with leases).
-- (establishment_id, kind, idempotency_key) is unique: the bridge driver enqueues
-- idempotently every time the fiscal job retries a pending document.
CREATE TABLE fiscal_bridge_jobs (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    fiscal_document_id uuid REFERENCES fiscal_documents (id) ON DELETE CASCADE,
    kind               text NOT NULL CHECK (kind IN ('issue', 'void')),
    -- Idempotency key of the ext.FiscalDocumentRequest (or of the void).
    idempotency_key    text NOT NULL,
    status             text NOT NULL CHECK (status IN ('queued', 'leased', 'succeeded', 'failed')),
    -- FiscalBridgeDocument (issue) or {"ref": "..."} (void); amounts in cents.
    payload            jsonb NOT NULL DEFAULT '{}',
    -- FiscalBridgeJobResult as received from the device.
    result             jsonb,
    attempts           integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    leased_device_id   uuid REFERENCES devices (id) ON DELETE SET NULL,
    lease_expires_at   timestamptz,
    next_attempt_at    timestamptz NOT NULL,
    completed_at       timestamptz,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX fiscal_bridge_jobs_establishment_key_uidx ON fiscal_bridge_jobs (establishment_id, kind, idempotency_key);
CREATE INDEX fiscal_bridge_jobs_establishment_status_idx ON fiscal_bridge_jobs (establishment_id, status, next_attempt_at);
CREATE INDEX fiscal_bridge_jobs_fiscal_document_id_idx ON fiscal_bridge_jobs (fiscal_document_id);

-- +goose Down
DROP TABLE fiscal_bridge_jobs;
DROP TABLE fiscal_bridges;
DROP TABLE device_pairing_codes;
DROP TABLE devices;
