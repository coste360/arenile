-- M5 part B (SQLite dialect of postgres/00008): end of season -> new season cycle (spec 8.7, 10.4, 10.5, 13.4, 28).
--   * season_rollovers: one row per applied season rollover (copy of the
--     configuration into the next season). The unique key makes "apply"
--     idempotent: a second apply for the same source season and year finds
--     the row and returns the recorded result.
--   * renewal_campaigns / renewal_proposals: "proponi rinnovi". A proposal
--     owns a held booking of the target season (bookings.renewal_of_booking_id
--     = source pass, hold_expires_at = pre-emption deadline). Availability is
--     still enforced by spot_allocations (the held booking's allocations);
--     the tables only track the proposal lifecycle. At most one open
--     proposal (proposed/accepted) per source pass across campaigns.
--   * waitlist_settings / waitlist_entries / waitlist_offers: requests with
--     no availability, offered automatically when inventory frees. An offer
--     owns a hold (holds table, DB-enforced); at most one pending offer per
--     entry and per hold. waitlist_entries.access_token_hash is the SHA-256
--     (hex) of the 256-bit token of the customer's waitlist link.
--   * customer_credits (00003) is extended, not duplicated: new sources
--     goodwill / expiry / adjustment, sign checks per source, and
--     reference_credit_id (an expiry row points to the grant it expires; one
--     expiry per grant). customer_credit_balances keeps the running sum per
--     customer with CHECK (balance >= 0): every ledger insert updates it in
--     the same transaction with a conditional UPDATE, so the balance can
--     never become negative even under concurrency (like pool_usage).
--     Invariant: balance = SUM(customer_credits.amount) of the customer.

-- +goose Up
CREATE TABLE season_rollovers (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    source_season_id   TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    target_season_id   TEXT NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    target_year        INTEGER NOT NULL,
    -- {"include_rate_periods": true, ...} as requested.
    options            TEXT NOT NULL DEFAULT '{}',
    -- SeasonRolloverCounts + warnings of the applied copy.
    result             TEXT NOT NULL DEFAULT '{}',
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL,
    CHECK (source_season_id <> target_season_id)
);
CREATE UNIQUE INDEX season_rollovers_source_year_uidx ON season_rollovers (establishment_id, source_season_id, target_year);
CREATE UNIQUE INDEX season_rollovers_target_uidx ON season_rollovers (target_season_id);

CREATE TABLE renewal_campaigns (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    source_season_id   TEXT NOT NULL REFERENCES seasons (id) ON DELETE RESTRICT,
    target_season_id   TEXT NOT NULL REFERENCES seasons (id) ON DELETE RESTRICT,
    deadline_at        TIMESTAMP NOT NULL,
    notes              TEXT NOT NULL DEFAULT '',
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    closed_at          TIMESTAMP,
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK (source_season_id <> target_season_id),
    CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);
CREATE UNIQUE INDEX renewal_campaigns_seasons_uidx ON renewal_campaigns (establishment_id, source_season_id, target_season_id);
CREATE INDEX renewal_campaigns_establishment_created_idx ON renewal_campaigns (establishment_id, created_at);

CREATE TABLE renewal_proposals (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    campaign_id        TEXT NOT NULL REFERENCES renewal_campaigns (id) ON DELETE CASCADE,
    -- Season pass of the source season (booking with is_season_pass).
    source_booking_id  TEXT NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    -- Held booking of the target season created for the proposal.
    booking_id         TEXT NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    customer_id        TEXT NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,
    spot_id            TEXT NOT NULL REFERENCES spots (id) ON DELETE RESTRICT,
    pack_id            TEXT NOT NULL REFERENCES packs (id) ON DELETE RESTRICT,
    status             TEXT NOT NULL DEFAULT 'proposed'
                           CHECK (status IN ('proposed', 'accepted', 'declined', 'expired', 'converted')),
    previous_amount    INTEGER NOT NULL DEFAULT 0 CHECK (previous_amount >= 0),
    total_amount       INTEGER NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    deposit_due        INTEGER NOT NULL DEFAULT 0 CHECK (deposit_due >= 0),
    deadline_at        TIMESTAMP NOT NULL,
    send_count         INTEGER NOT NULL DEFAULT 0 CHECK (send_count >= 0),
    last_sent_at       TIMESTAMP,
    responded_at       TIMESTAMP,
    response_channel   TEXT CHECK (response_channel IN ('online', 'desk', 'system')),
    decline_reason     TEXT NOT NULL DEFAULT '',
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    CHECK ((status = 'proposed') = (responded_at IS NULL)),
    CHECK ((responded_at IS NULL) = (response_channel IS NULL))
);
CREATE UNIQUE INDEX renewal_proposals_campaign_source_uidx ON renewal_proposals (campaign_id, source_booking_id);
CREATE UNIQUE INDEX renewal_proposals_booking_uidx ON renewal_proposals (booking_id);
CREATE UNIQUE INDEX renewal_proposals_open_source_uidx ON renewal_proposals (source_booking_id)
    WHERE status IN ('proposed', 'accepted');
CREATE INDEX renewal_proposals_establishment_status_idx ON renewal_proposals (establishment_id, status);
CREATE INDEX renewal_proposals_campaign_status_idx ON renewal_proposals (campaign_id, status);
CREATE INDEX renewal_proposals_deadline_idx ON renewal_proposals (deadline_at) WHERE status = 'proposed';
CREATE INDEX renewal_proposals_customer_idx ON renewal_proposals (customer_id);

CREATE TABLE waitlist_settings (
    establishment_id   TEXT PRIMARY KEY REFERENCES establishments (id) ON DELETE CASCADE,
    enabled            INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    public_join        INTEGER NOT NULL DEFAULT 0 CHECK (public_join IN (0, 1)),
    auto_offer         INTEGER NOT NULL DEFAULT 1 CHECK (auto_offer IN (0, 1)),
    offer_hold_minutes INTEGER NOT NULL DEFAULT 120 CHECK (offer_hold_minutes BETWEEN 5 AND 10080),
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);

CREATE TABLE waitlist_entries (
    id                   TEXT PRIMARY KEY,
    establishment_id     TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id          TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    channel              TEXT NOT NULL DEFAULT 'desk' CHECK (channel IN ('desk', 'online', 'api')),
    starts_on            TEXT NOT NULL,
    ends_on              TEXT NOT NULL,
    slots                TEXT NOT NULL CHECK (slots IN ('AM', 'PM', 'FULL')),
    pack_id              TEXT NOT NULL REFERENCES packs (id) ON DELETE RESTRICT,
    zone_id              TEXT REFERENCES zones (id) ON DELETE SET NULL,
    spot_id              TEXT REFERENCES spots (id) ON DELETE SET NULL,
    quantity             INTEGER NOT NULL DEFAULT 1 CHECK (quantity BETWEEN 1 AND 20),
    priority             INTEGER NOT NULL DEFAULT 0,
    status               TEXT NOT NULL DEFAULT 'waiting'
                             CHECK (status IN ('waiting', 'offered', 'converted', 'expired', 'removed')),
    note                 TEXT NOT NULL DEFAULT '',
    locale               TEXT NOT NULL DEFAULT 'it',
    access_token_hash    TEXT,
    converted_booking_id TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    removed_at           TIMESTAMP,
    version              INTEGER NOT NULL DEFAULT 1,
    created_at           TIMESTAMP NOT NULL,
    updated_at           TIMESTAMP NOT NULL,
    CHECK (ends_on >= starts_on),
    CHECK ((status = 'converted') = (converted_booking_id IS NOT NULL)),
    CHECK ((status = 'removed') = (removed_at IS NOT NULL))
);
CREATE UNIQUE INDEX waitlist_entries_token_uidx ON waitlist_entries (access_token_hash) WHERE access_token_hash IS NOT NULL;
CREATE INDEX waitlist_entries_queue_idx ON waitlist_entries (establishment_id, status, priority, created_at);
CREATE INDEX waitlist_entries_establishment_dates_idx ON waitlist_entries (establishment_id, starts_on, ends_on);
CREATE INDEX waitlist_entries_customer_idx ON waitlist_entries (customer_id);

CREATE TABLE waitlist_offers (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    entry_id         TEXT NOT NULL REFERENCES waitlist_entries (id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'accepted', 'declined', 'expired', 'revoked')),
    hold_id          TEXT NOT NULL REFERENCES holds (id) ON DELETE CASCADE,
    spot_id          TEXT REFERENCES spots (id) ON DELETE SET NULL,
    booking_id       TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    cause            TEXT NOT NULL CHECK (cause IN ('cancellation', 'release', 'expiry', 'manual')),
    expires_at       TIMESTAMP NOT NULL,
    responded_at     TIMESTAMP,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    CHECK ((status = 'pending') = (responded_at IS NULL)),
    CHECK (status <> 'accepted' OR booking_id IS NOT NULL)
);
CREATE UNIQUE INDEX waitlist_offers_pending_entry_uidx ON waitlist_offers (entry_id) WHERE status = 'pending';
CREATE UNIQUE INDEX waitlist_offers_hold_uidx ON waitlist_offers (hold_id);
CREATE INDEX waitlist_offers_establishment_id_idx ON waitlist_offers (establishment_id);
CREATE INDEX waitlist_offers_pending_expires_idx ON waitlist_offers (expires_at) WHERE status = 'pending';

-- Credit ledger extension. SQLite cannot alter CHECK constraints: the
-- table is rebuilt (create, copy, drop, rename; nothing references it).
CREATE TABLE customer_credits_new (
    id                  TEXT PRIMARY KEY,
    establishment_id    TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id         TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    amount              INTEGER NOT NULL CHECK (amount <> 0),
    source              TEXT NOT NULL CHECK (source IN ('cancellation', 'season_release', 'weather', 'manual', 'redemption', 'goodwill', 'expiry', 'adjustment')),
    booking_id          TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    season_release_id   TEXT REFERENCES season_releases (id) ON DELETE SET NULL,
    payment_id          TEXT REFERENCES payments (id) ON DELETE SET NULL,
    note                TEXT NOT NULL DEFAULT '',
    expires_on          TEXT,
    created_by_user_id  TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMP NOT NULL,
    reference_credit_id TEXT REFERENCES customer_credits (id) ON DELETE SET NULL,
    CHECK ((source IN ('redemption', 'expiry') AND amount < 0)
        OR (source = 'goodwill' AND amount > 0)
        -- cancellation / season_release / weather may be negative: reversals
        -- (e.g. pass.unmarkResold storno of a season release credit).
        OR source IN ('cancellation', 'season_release', 'weather', 'manual', 'adjustment')),
    CHECK ((source = 'expiry') = (reference_credit_id IS NOT NULL))
);
INSERT INTO customer_credits_new (id, establishment_id, customer_id, amount, source, booking_id, season_release_id,
                                  payment_id, note, expires_on, created_by_user_id, created_at)
SELECT id, establishment_id, customer_id, amount, source, booking_id, season_release_id,
       payment_id, note, expires_on, created_by_user_id, created_at
FROM customer_credits;
DROP TABLE customer_credits;
ALTER TABLE customer_credits_new RENAME TO customer_credits;
CREATE INDEX customer_credits_establishment_customer_idx ON customer_credits (establishment_id, customer_id);
CREATE UNIQUE INDEX customer_credits_expiry_ref_uidx ON customer_credits (reference_credit_id) WHERE source = 'expiry';
CREATE INDEX customer_credits_expires_idx ON customer_credits (establishment_id, expires_on) WHERE expires_on IS NOT NULL;

CREATE TABLE customer_credit_balances (
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id      TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    balance          INTEGER NOT NULL DEFAULT 0 CHECK (balance >= 0),
    updated_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (establishment_id, customer_id)
);
CREATE UNIQUE INDEX customer_credit_balances_customer_uidx ON customer_credit_balances (customer_id);

INSERT INTO customer_credit_balances (establishment_id, customer_id, balance, updated_at)
SELECT establishment_id, customer_id, MAX(SUM(amount), 0), MAX(created_at)
FROM customer_credits
GROUP BY establishment_id, customer_id;

-- +goose Down
DROP TABLE customer_credit_balances;
CREATE TABLE customer_credits_old (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    customer_id        TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    amount             INTEGER NOT NULL CHECK (amount <> 0),
    source             TEXT NOT NULL CHECK (source IN ('cancellation', 'season_release', 'weather', 'manual', 'redemption')),
    booking_id         TEXT REFERENCES bookings (id) ON DELETE SET NULL,
    season_release_id  TEXT REFERENCES season_releases (id) ON DELETE SET NULL,
    payment_id         TEXT REFERENCES payments (id) ON DELETE SET NULL,
    note               TEXT NOT NULL DEFAULT '',
    expires_on         TEXT,
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL
);
INSERT INTO customer_credits_old (id, establishment_id, customer_id, amount, source, booking_id, season_release_id,
                                  payment_id, note, expires_on, created_by_user_id, created_at)
SELECT id, establishment_id, customer_id, amount, source, booking_id, season_release_id,
       payment_id, note, expires_on, created_by_user_id, created_at
FROM customer_credits WHERE source NOT IN ('goodwill', 'expiry', 'adjustment');
DROP TABLE customer_credits;
ALTER TABLE customer_credits_old RENAME TO customer_credits;
CREATE INDEX customer_credits_establishment_customer_idx ON customer_credits (establishment_id, customer_id);
DROP TABLE waitlist_offers;
DROP TABLE waitlist_entries;
DROP TABLE waitlist_settings;
DROP TABLE renewal_proposals;
DROP TABLE renewal_campaigns;
DROP TABLE season_rollovers;
