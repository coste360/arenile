-- M0 foundations: tenancy, users, auth, audit, outbox, webhooks, idempotency.
-- IDs are UUIDv7 (uuid); instants are timestamptz (UTC); JSON as jsonb.

-- +goose Up
CREATE TABLE organizations (
    id              uuid PRIMARY KEY,
    name            text NOT NULL,
    vat_number      text,
    fiscal_code     text,
    billing_address jsonb,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL
);
CREATE INDEX organizations_name_idx ON organizations (name);

CREATE TABLE establishments (
    id                    uuid PRIMARY KEY,
    organization_id       uuid NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    slug                  text NOT NULL UNIQUE,
    name                  text NOT NULL,
    timezone              text NOT NULL DEFAULT 'Europe/Rome',
    currency              text NOT NULL DEFAULT 'EUR',
    default_locale        text NOT NULL DEFAULT 'it',
    enabled_locales       jsonb NOT NULL DEFAULT '["it"]',
    contact_email         text NOT NULL DEFAULT '',
    contact_phone         text NOT NULL DEFAULT '',
    address               jsonb NOT NULL DEFAULT '{}',
    geo                   jsonb,
    concession_ref        text,
    booking_enabled       boolean NOT NULL DEFAULT false,
    allowed_embed_origins jsonb NOT NULL DEFAULT '[]',
    theme                 jsonb NOT NULL DEFAULT '{}',
    settings              jsonb NOT NULL DEFAULT '{}',
    archived_at           timestamptz,
    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL
);
CREATE INDEX establishments_organization_id_idx ON establishments (organization_id);

CREATE TABLE users (
    id                uuid PRIMARY KEY,
    email             text NOT NULL UNIQUE,
    password_hash     text NOT NULL,
    name              text NOT NULL,
    locale            text NOT NULL DEFAULT 'it',
    totp_secret       text,
    totp_confirmed_at timestamptz,
    totp_last_step    bigint NOT NULL DEFAULT 0,
    active            boolean NOT NULL DEFAULT true,
    last_login_at     timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);

CREATE TABLE memberships (
    id               uuid PRIMARY KEY,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    establishment_id uuid REFERENCES establishments (id) ON DELETE CASCADE,
    role             text NOT NULL CHECK (role IN ('owner', 'manager', 'cashier', 'beach_staff', 'viewer')),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX memberships_user_scope_uidx
    ON memberships (user_id, organization_id, COALESCE(establishment_id::text, ''));
CREATE INDEX memberships_establishment_id_idx ON memberships (establishment_id);
CREATE INDEX memberships_organization_id_idx ON memberships (organization_id);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    token_hash   text NOT NULL UNIQUE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    ip           text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE api_keys (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name               text NOT NULL,
    prefix             text NOT NULL UNIQUE,
    key_hash           text NOT NULL UNIQUE,
    scopes             jsonb NOT NULL DEFAULT '[]',
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    last_used_at       timestamptz,
    revoked_at         timestamptz,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);
CREATE INDEX api_keys_establishment_id_idx ON api_keys (establishment_id);

CREATE TABLE audit_log (
    id               uuid PRIMARY KEY,
    establishment_id uuid,
    actor_type       text NOT NULL CHECK (actor_type IN ('user', 'customer', 'system', 'api_key')),
    actor_id         text,
    action           text NOT NULL,
    entity_type      text NOT NULL,
    entity_id        text,
    diff             jsonb,
    ip               text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL
);
CREATE INDEX audit_log_establishment_created_idx ON audit_log (establishment_id, created_at);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id);

-- +goose StatementBegin
CREATE FUNCTION audit_log_insert_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is insert-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_update_delete BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_insert_only();

CREATE TABLE outbox_events (
    id               uuid PRIMARY KEY,
    establishment_id uuid,
    type             text NOT NULL,
    payload          jsonb NOT NULL,
    version          integer NOT NULL DEFAULT 1,
    occurred_at      timestamptz NOT NULL,
    created_at       timestamptz NOT NULL,
    dispatched_at    timestamptz
);
CREATE INDEX outbox_events_pending_idx ON outbox_events (created_at) WHERE dispatched_at IS NULL;
CREATE INDEX outbox_events_establishment_idx ON outbox_events (establishment_id, created_at);

CREATE TABLE webhook_endpoints (
    id                   uuid PRIMARY KEY,
    establishment_id     uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    url                  text NOT NULL,
    secret               text NOT NULL,
    events               jsonb NOT NULL DEFAULT '[]',
    active               boolean NOT NULL DEFAULT true,
    consecutive_failures integer NOT NULL DEFAULT 0,
    disabled_at          timestamptz,
    disabled_reason      text,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX webhook_endpoints_establishment_id_idx ON webhook_endpoints (establishment_id);

CREATE TABLE webhook_deliveries (
    id               uuid PRIMARY KEY,
    endpoint_id      uuid NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    event_id         uuid NOT NULL REFERENCES outbox_events (id) ON DELETE CASCADE,
    attempt          integer NOT NULL DEFAULT 0,
    status_code      integer,
    response_excerpt text,
    last_error       text,
    next_attempt_at  timestamptz,
    delivered_at     timestamptz,
    failed_at        timestamptz,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX webhook_deliveries_endpoint_event_uidx ON webhook_deliveries (endpoint_id, event_id);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at)
    WHERE delivered_at IS NULL AND failed_at IS NULL;

CREATE TABLE idempotency_keys (
    scope                 text NOT NULL,
    key                   text NOT NULL,
    request_hash          text NOT NULL,
    response_status       integer,
    response_content_type text,
    response_body         bytea,
    created_at            timestamptz NOT NULL,
    expires_at            timestamptz NOT NULL,
    PRIMARY KEY (scope, key)
);
CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
DROP TABLE outbox_events;
DROP TABLE audit_log;
DROP FUNCTION audit_log_insert_only();
DROP TABLE api_keys;
DROP TABLE sessions;
DROP TABLE memberships;
DROP TABLE users;
DROP TABLE establishments;
DROP TABLE organizations;
