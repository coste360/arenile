-- Push notifications through the Coste360 relay (SQLite dialect of postgres/00013; ADR 0027).
--   * push_settings: per-establishment opt-in of the manager (enabled, off by
--     default) and the events sent (all on once enabled). One row per
--     establishment, created on the first update; no row = defaults. version
--     is the ETag of GET/PUT .../push-settings.
--   * push_registrations: the native APNs/FCM token of a paired mobile device
--     (one per device, removed with the device and by its revocation).
--     token_ciphertext is the token encrypted with the ARENILE_SECRET_KEY key
--     ring (AAD = device id); token_hash is its SHA-256, unique per
--     establishment so the same phone is never registered twice. topics is
--     the JSON array of message keys the user opted into.
--   * push_deliveries: one notification queued by the outbox sink for a
--     (registration, event) pair, unique so a redelivered event queues
--     nothing new; sent to the relay by the job push.deliver with a lease
--     (status sending, locked_until). last_error holds the relay status or a
--     generic network error, never a token or personal data. Closed rows are
--     deleted after 7 days.

-- +goose Up
CREATE TABLE push_settings (
    establishment_id          TEXT PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    enabled                   INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    notify_new_online_booking INTEGER NOT NULL DEFAULT 1 CHECK (notify_new_online_booking IN (0, 1)),
    notify_approval_requested INTEGER NOT NULL DEFAULT 1 CHECK (notify_approval_requested IN (0, 1)),
    notify_payment_failed     INTEGER NOT NULL DEFAULT 1 CHECK (notify_payment_failed IN (0, 1)),
    version                   INTEGER NOT NULL DEFAULT 1,
    created_at                TIMESTAMP NOT NULL,
    updated_at                TIMESTAMP NOT NULL
);

CREATE TABLE push_registrations (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    device_id        TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform         TEXT NOT NULL CHECK (platform IN ('ios', 'android')),
    environment      TEXT NOT NULL DEFAULT 'production' CHECK (environment IN ('production', 'sandbox')),
    locale           TEXT NOT NULL DEFAULT 'it' CHECK (locale IN ('it', 'en', 'de', 'fr')),
    topics           TEXT NOT NULL DEFAULT '[]',
    token_ciphertext TEXT NOT NULL,
    token_hash       TEXT NOT NULL,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX push_registrations_device_uidx ON push_registrations (device_id);
CREATE UNIQUE INDEX push_registrations_token_uidx ON push_registrations (establishment_id, token_hash);

CREATE TABLE push_deliveries (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    registration_id  TEXT NOT NULL REFERENCES push_registrations (id) ON DELETE CASCADE,
    event_id         TEXT NOT NULL,
    message_key      TEXT NOT NULL
                         CHECK (message_key IN ('booking_new_online', 'booking_approval_requested', 'payment_failed')),
    status           TEXT NOT NULL DEFAULT 'queued'
                         CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'expired')),
    attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at  TIMESTAMP,
    locked_until     TIMESTAMP,
    last_error       TEXT,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    sent_at          TIMESTAMP,
    CHECK ((status = 'sent') = (sent_at IS NOT NULL))
);
CREATE UNIQUE INDEX push_deliveries_event_uidx ON push_deliveries (registration_id, event_id);
CREATE INDEX push_deliveries_establishment_created_idx ON push_deliveries (establishment_id, created_at);
CREATE INDEX push_deliveries_due_idx ON push_deliveries (next_attempt_at) WHERE status IN ('queued', 'sending');

-- +goose Down
DROP TABLE push_deliveries;
DROP TABLE push_registrations;
DROP TABLE push_settings;
