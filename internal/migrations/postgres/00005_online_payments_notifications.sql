-- M4: public booking page and online payments (spec 10.1, 12, 13, 19, 23).
-- Online flow settings and terms versions, encrypted payment provider
-- credentials, checkout sessions of the public flow, magic links of the
-- customer manage area, provider payment intents (checkout references),
-- processed webhook events (deduplication), email deliveries (message_log)
-- and notification templates.
-- IDs are uuid; JSON as jsonb; booleans boolean; money bigint cents;
-- instants timestamptz.
--
-- Secrets and tokens are never stored in clear:
--   * checkout_sessions.token_hash and magic_link_tokens.token_hash are the
--     SHA-256 (hex) of 256-bit random tokens; the raw token exists only in
--     the client (sessionStorage) or in the email;
--   * payment_provider_settings.credentials is a key ring ciphertext
--     (internal/crypto, AES-256-GCM, AAD = "<establishment_id>:<provider>:<mode>");
--   * message_log.recipient_hash is the SHA-256 of the normalised address
--     (lookup without the address), recipient_enc its ciphertext (delivery).
-- bookings.manage_token_hash (00003) is superseded by magic_link_tokens,
-- which keeps every issued link with expiry and revocation.

-- +goose Up
CREATE TABLE terms_versions (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    -- Human version label shown to the customer and saved on bookings.terms_version.
    version            text NOT NULL,
    -- {"it": {"terms": "...", "privacy": "...", "marketing": "..."}, "en": {...}}
    texts              jsonb NOT NULL DEFAULT '{}',
    published_at       timestamptz NOT NULL,
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX terms_versions_establishment_version_uidx ON terms_versions (establishment_id, version);
CREATE INDEX terms_versions_establishment_published_idx ON terms_versions (establishment_id, published_at);

-- One row per establishment: online flow configuration (formulas, slots,
-- calendar options, payment preferences) and email settings. The theme and
-- allowed_embed_origins stay on establishments; the online service updates
-- them together with this row and bumps version (ETag of online settings).
CREATE TABLE online_settings (
    establishment_id         uuid PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    config                   jsonb NOT NULL DEFAULT '{}',
    email_config             jsonb NOT NULL DEFAULT '{}',
    current_terms_version_id uuid REFERENCES terms_versions (id) ON DELETE SET NULL,
    version                  integer NOT NULL DEFAULT 1,
    created_at               timestamptz NOT NULL,
    updated_at               timestamptz NOT NULL
);

-- Payment provider configuration per establishment, provider and mode.
-- At most one active row per establishment (the provider of the public page).
CREATE TABLE payment_provider_settings (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    provider         text NOT NULL,
    mode             text NOT NULL CHECK (mode IN ('test', 'live')),
    active           boolean NOT NULL DEFAULT false,
    -- Ciphertext of the JSON credentials map (ext.Credentials); '' = none.
    credentials      text NOT NULL DEFAULT '',
    -- Non-secret settings and hints: publishable key, secret key last4,
    -- webhook secret configured, embedded mode, account label.
    public_config    jsonb NOT NULL DEFAULT '{}',
    verified_at      timestamptz,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX payment_provider_settings_key_uidx ON payment_provider_settings (establishment_id, provider, mode);
CREATE UNIQUE INDEX payment_provider_settings_active_uidx ON payment_provider_settings (establishment_id) WHERE active;

-- Checkout session of the public flow: binds the browser (token sent in
-- the X-Arenile-Checkout header, spec 12.2) to a held booking.
CREATE TABLE checkout_sessions (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id       uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    token_hash       text NOT NULL,
    state            text NOT NULL CHECK (state IN ('open', 'paying', 'completed', 'expired', 'abandoned')),
    expires_at       timestamptz NOT NULL,
    payment_attempts integer NOT NULL DEFAULT 0 CHECK (payment_attempts >= 0),
    locale           text NOT NULL DEFAULT 'it',
    -- Origin of the embedding page, when verified (analytics of conversions).
    embed_origin     text,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX checkout_sessions_token_hash_uidx ON checkout_sessions (token_hash);
CREATE INDEX checkout_sessions_establishment_id_idx ON checkout_sessions (establishment_id);
CREATE INDEX checkout_sessions_booking_id_idx ON checkout_sessions (booking_id);
CREATE INDEX checkout_sessions_open_expires_idx ON checkout_sessions (expires_at) WHERE state IN ('open', 'paying');

-- Magic links of the customer manage area (spec 12.4): hash only, valid
-- until the end of the booking + 30 days, revocable (regeneration).
CREATE TABLE magic_link_tokens (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id         uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    token_hash         text NOT NULL,
    expires_at         timestamptz NOT NULL,
    revoked_at         timestamptz,
    last_used_at       timestamptz,
    created_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX magic_link_tokens_token_hash_uidx ON magic_link_tokens (token_hash);
CREATE INDEX magic_link_tokens_establishment_id_idx ON magic_link_tokens (establishment_id);
CREATE INDEX magic_link_tokens_booking_id_idx ON magic_link_tokens (booking_id);

-- Provider-side payments and refunds (Stripe Checkout sessions, payment
-- intents, refunds). The ledger row in payments is written when the
-- provider confirms (webhook or reconciliation); payment_id links it.
CREATE TABLE payment_intents (
    id                  uuid PRIMARY KEY,
    establishment_id    uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id          uuid REFERENCES bookings (id) ON DELETE RESTRICT,
    payment_id          uuid REFERENCES payments (id) ON DELETE SET NULL,
    refund_of_intent_id uuid REFERENCES payment_intents (id) ON DELETE SET NULL,
    provider            text NOT NULL,
    mode                text NOT NULL CHECK (mode IN ('test', 'live')),
    kind                text NOT NULL CHECK (kind IN ('payment', 'refund')),
    -- Provider reference (Stripe: cs_..., pi_..., re_...), unique per provider.
    external_id         text NOT NULL,
    -- Secondary provider reference (Stripe: the payment intent of a checkout session).
    external_ref        text,
    status              text NOT NULL CHECK (status IN ('created', 'pending', 'succeeded', 'failed', 'cancelled', 'expired')),
    amount              bigint NOT NULL CHECK (amount > 0),
    currency            text NOT NULL DEFAULT 'EUR',
    attempt             integer NOT NULL DEFAULT 1 CHECK (attempt >= 1),
    idempotency_key     text,
    -- Provider event ids applied to this intent, oldest first.
    raw_event_ids       jsonb NOT NULL DEFAULT '[]',
    failure_reason      text,
    expires_at          timestamptz,
    last_checked_at     timestamptz,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE UNIQUE INDEX payment_intents_provider_external_uidx ON payment_intents (provider, external_id);
CREATE UNIQUE INDEX payment_intents_establishment_idempotency_uidx ON payment_intents (establishment_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX payment_intents_establishment_created_idx ON payment_intents (establishment_id, created_at);
CREATE INDEX payment_intents_booking_id_idx ON payment_intents (booking_id);
CREATE INDEX payment_intents_open_idx ON payment_intents (status, created_at) WHERE status IN ('created', 'pending');

-- Processed provider webhook events (spec 13.3): the same event is never
-- processed twice; the insert happens in the transaction that applies it.
CREATE TABLE webhook_events (
    id                uuid PRIMARY KEY,
    establishment_id  uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    provider          text NOT NULL,
    event_id          text NOT NULL,
    event_type        text NOT NULL,
    payment_intent_id uuid REFERENCES payment_intents (id) ON DELETE SET NULL,
    status            text NOT NULL CHECK (status IN ('processed', 'ignored', 'failed')),
    error             text,
    received_at       timestamptz NOT NULL,
    processed_at      timestamptz
);
CREATE UNIQUE INDEX webhook_events_provider_event_uidx ON webhook_events (provider, event_id);
CREATE INDEX webhook_events_establishment_received_idx ON webhook_events (establishment_id, received_at);

-- Editable notification templates (spec 19): Markdown with variables, per
-- template and locale; absent rows fall back to the built-in templates.
CREATE TABLE notification_templates (
    id               uuid PRIMARY KEY,
    establishment_id uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    template         text NOT NULL,
    locale           text NOT NULL,
    subject          text NOT NULL,
    body_markdown    text NOT NULL,
    enabled          boolean NOT NULL DEFAULT true,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE UNIQUE INDEX notification_templates_key_uidx ON notification_templates (establishment_id, template, locale);

-- Outgoing messages and their delivery (spec 19 message_log): queued in the
-- transaction of the triggering change (or by the event sink, deduplicated
-- by dedupe_key), delivered by a job with retries.
CREATE TABLE message_log (
    id                  uuid PRIMARY KEY,
    establishment_id    uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    booking_id          uuid REFERENCES bookings (id) ON DELETE SET NULL,
    customer_id         uuid REFERENCES customers (id) ON DELETE SET NULL,
    channel             text NOT NULL DEFAULT 'email',
    template            text NOT NULL,
    locale              text NOT NULL DEFAULT 'it',
    recipient_hash      text NOT NULL,
    recipient_enc       text,
    -- e.g. "booking.confirmed:<event_id>"; one message per key and establishment.
    dedupe_key          text NOT NULL,
    status              text NOT NULL CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'cancelled')),
    attempts            integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at     timestamptz,
    last_error          text,
    provider_message_id text,
    -- Non-personal rendering inputs (ids, event type); personal data is
    -- read from the booking at send time.
    payload             jsonb NOT NULL DEFAULT '{}',
    sent_at             timestamptz,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE UNIQUE INDEX message_log_establishment_dedupe_uidx ON message_log (establishment_id, dedupe_key);
CREATE INDEX message_log_establishment_created_idx ON message_log (establishment_id, created_at);
CREATE INDEX message_log_booking_id_idx ON message_log (booking_id);
CREATE INDEX message_log_due_idx ON message_log (next_attempt_at) WHERE status IN ('queued', 'failed');

-- Terms acceptance instant (the accepted version is bookings.terms_version)
-- and language of the customer communications for this booking.
ALTER TABLE bookings ADD COLUMN terms_accepted_at timestamptz;
ALTER TABLE bookings ADD COLUMN locale text;

-- +goose Down
ALTER TABLE bookings DROP COLUMN locale;
ALTER TABLE bookings DROP COLUMN terms_accepted_at;
DROP TABLE message_log;
DROP TABLE notification_templates;
DROP TABLE webhook_events;
DROP TABLE payment_intents;
DROP TABLE magic_link_tokens;
DROP TABLE checkout_sessions;
DROP TABLE payment_provider_settings;
DROP TABLE online_settings;
DROP TABLE terms_versions;
