-- M0 foundations: tenancy, users, auth, audit, outbox, webhooks, idempotency.
-- IDs are UUIDv7 stored as TEXT; instants are UTC timestamps; JSON as TEXT.

-- +goose Up
CREATE TABLE organizations (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    vat_number      TEXT,
    fiscal_code     TEXT,
    billing_address TEXT,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);
CREATE INDEX organizations_name_idx ON organizations (name);

CREATE TABLE establishments (
    id                    TEXT PRIMARY KEY,
    organization_id       TEXT NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    slug                  TEXT NOT NULL UNIQUE,
    name                  TEXT NOT NULL,
    timezone              TEXT NOT NULL DEFAULT 'Europe/Rome',
    currency              TEXT NOT NULL DEFAULT 'EUR',
    default_locale        TEXT NOT NULL DEFAULT 'it',
    enabled_locales       TEXT NOT NULL DEFAULT '["it"]',
    contact_email         TEXT NOT NULL DEFAULT '',
    contact_phone         TEXT NOT NULL DEFAULT '',
    address               TEXT NOT NULL DEFAULT '{}',
    geo                   TEXT,
    concession_ref        TEXT,
    booking_enabled       INTEGER NOT NULL DEFAULT 0 CHECK (booking_enabled IN (0, 1)),
    allowed_embed_origins TEXT NOT NULL DEFAULT '[]',
    theme                 TEXT NOT NULL DEFAULT '{}',
    settings              TEXT NOT NULL DEFAULT '{}',
    archived_at           TIMESTAMP,
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL
);
CREATE INDEX establishments_organization_id_idx ON establishments (organization_id);

CREATE TABLE users (
    id                TEXT PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    name              TEXT NOT NULL,
    locale            TEXT NOT NULL DEFAULT 'it',
    totp_secret       TEXT,
    totp_confirmed_at TIMESTAMP,
    totp_last_step    INTEGER NOT NULL DEFAULT 0,
    active            INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    last_login_at     TIMESTAMP,
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL
);

CREATE TABLE memberships (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id  TEXT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    establishment_id TEXT REFERENCES establishments (id) ON DELETE CASCADE,
    role             TEXT NOT NULL CHECK (role IN ('owner', 'manager', 'cashier', 'beach_staff', 'viewer')),
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX memberships_user_scope_uidx
    ON memberships (user_id, organization_id, COALESCE(establishment_id, ''));
CREATE INDEX memberships_establishment_id_idx ON memberships (establishment_id);
CREATE INDEX memberships_organization_id_idx ON memberships (organization_id);

CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at   TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    ip           TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE api_keys (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    prefix             TEXT NOT NULL UNIQUE,
    key_hash           TEXT NOT NULL UNIQUE,
    scopes             TEXT NOT NULL DEFAULT '[]',
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    last_used_at       TIMESTAMP,
    revoked_at         TIMESTAMP,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE INDEX api_keys_establishment_id_idx ON api_keys (establishment_id);

CREATE TABLE audit_log (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT,
    actor_type       TEXT NOT NULL CHECK (actor_type IN ('user', 'customer', 'system', 'api_key')),
    actor_id         TEXT,
    action           TEXT NOT NULL,
    entity_type      TEXT NOT NULL,
    entity_id        TEXT,
    diff             TEXT,
    ip               TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMP NOT NULL
);
CREATE INDEX audit_log_establishment_created_idx ON audit_log (establishment_id, created_at);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id);

-- +goose StatementBegin
CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is insert-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is insert-only');
END;
-- +goose StatementEnd

CREATE TABLE outbox_events (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT,
    type             TEXT NOT NULL,
    payload          TEXT NOT NULL,
    version          INTEGER NOT NULL DEFAULT 1,
    occurred_at      TIMESTAMP NOT NULL,
    created_at       TIMESTAMP NOT NULL,
    dispatched_at    TIMESTAMP
);
CREATE INDEX outbox_events_pending_idx ON outbox_events (created_at) WHERE dispatched_at IS NULL;
CREATE INDEX outbox_events_establishment_idx ON outbox_events (establishment_id, created_at);

CREATE TABLE webhook_endpoints (
    id                   TEXT PRIMARY KEY,
    establishment_id     TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    url                  TEXT NOT NULL,
    secret               TEXT NOT NULL,
    events               TEXT NOT NULL DEFAULT '[]',
    active               INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    disabled_at          TIMESTAMP,
    disabled_reason      TEXT,
    created_at           TIMESTAMP NOT NULL,
    updated_at           TIMESTAMP NOT NULL
);
CREATE INDEX webhook_endpoints_establishment_id_idx ON webhook_endpoints (establishment_id);

CREATE TABLE webhook_deliveries (
    id               TEXT PRIMARY KEY,
    endpoint_id      TEXT NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    event_id         TEXT NOT NULL REFERENCES outbox_events (id) ON DELETE CASCADE,
    attempt          INTEGER NOT NULL DEFAULT 0,
    status_code      INTEGER,
    response_excerpt TEXT,
    last_error       TEXT,
    next_attempt_at  TIMESTAMP,
    delivered_at     TIMESTAMP,
    failed_at        TIMESTAMP,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX webhook_deliveries_endpoint_event_uidx ON webhook_deliveries (endpoint_id, event_id);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at)
    WHERE delivered_at IS NULL AND failed_at IS NULL;

CREATE TABLE idempotency_keys (
    scope                 TEXT NOT NULL,
    key                   TEXT NOT NULL,
    request_hash          TEXT NOT NULL,
    response_status       INTEGER,
    response_content_type TEXT,
    response_body         BLOB,
    created_at            TIMESTAMP NOT NULL,
    expires_at            TIMESTAMP NOT NULL,
    PRIMARY KEY (scope, key)
);
CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
DROP TABLE outbox_events;
DROP TRIGGER audit_log_no_delete;
DROP TRIGGER audit_log_no_update;
DROP TABLE audit_log;
DROP TABLE api_keys;
DROP TABLE sessions;
DROP TABLE memberships;
DROP TABLE users;
DROP TABLE establishments;
DROP TABLE organizations;
