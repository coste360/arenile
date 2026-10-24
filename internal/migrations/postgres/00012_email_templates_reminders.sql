-- Editable customer email texts (PostgreSQL dialect of sqlite/00012; ADR 0022).
--   * email_template_overrides: the texts an establishment changed for one
--     customer email kind and locale (subject, opening, closing, signature).
--     Only overrides are stored: no row (or active = false) = the built-in
--     texts of the binary. "Restore default" sets active = false and bumps
--     version instead of deleting the row, so an ETag is never reused after
--     a restore (no ABA on If-Match). Plain text with {placeholders}
--     from the safe set of internal/domain/notify, validated on save and
--     escaped on render; the fixed blocks (QR, summary, amounts, manage
--     link, legal footer) are not stored here. version is the ETag of the
--     row (If-Match); one row per (establishment, kind, locale).
-- Automatic reminders (day before, balance due) need no table: their
-- schedule lives in online_settings.email_config and the once-only send is
-- the unique message_log (establishment_id, dedupe_key) of migration 00005
-- with the key "reminder:<kind>:<booking id>".

-- +goose Up
CREATE TABLE email_template_overrides (
    id                 uuid PRIMARY KEY,
    establishment_id   uuid NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind               text NOT NULL,
    locale             text NOT NULL CHECK (locale IN ('it', 'en', 'de', 'fr')),
    subject            text NOT NULL,
    opening            text NOT NULL DEFAULT '',
    closing            text NOT NULL DEFAULT '',
    signature          text NOT NULL DEFAULT '',
    active             boolean NOT NULL DEFAULT true,
    version            bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);
CREATE UNIQUE INDEX email_template_overrides_key_uidx ON email_template_overrides (establishment_id, kind, locale);

-- +goose Down
DROP TABLE email_template_overrides;
