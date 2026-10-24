-- Editable customer email texts (SQLite dialect of postgres/00012; ADR 0022).
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
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    kind               TEXT NOT NULL,
    locale             TEXT NOT NULL CHECK (locale IN ('it', 'en', 'de', 'fr')),
    subject            TEXT NOT NULL,
    opening            TEXT NOT NULL DEFAULT '',
    closing            TEXT NOT NULL DEFAULT '',
    signature          TEXT NOT NULL DEFAULT '',
    active             INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version            INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX email_template_overrides_key_uidx ON email_template_overrides (establishment_id, kind, locale);

-- +goose Down
DROP TABLE email_template_overrides;
