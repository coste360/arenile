package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// M5 part B models (migration 00008): season rollover, renewal campaigns
// and proposals, waitlist and the customer credit balance. Query functions
// live in one file per area, owned by the matching domain builder:
// rollover.go, renewal.go, waitlist.go, credit.go.

// SeasonRollover records an applied season rollover (spec 8.7, 28). The
// unique (establishment_id, source_season_id, target_year) makes the apply
// idempotent; Result is the JSON of the counts and warnings returned then.
type SeasonRollover struct {
	bun.BaseModel `bun:"table:season_rollovers,alias:sro"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	SourceSeasonID  string    `bun:"source_season_id,notnull"`
	TargetSeasonID  string    `bun:"target_season_id,notnull"`
	TargetYear      int       `bun:"target_year,notnull"`
	Options         JSON      `bun:"options,notnull"`
	Result          JSON      `bun:"result,notnull"`
	CreatedByUserID *string   `bun:"created_by_user_id"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
}

// Renewal campaign statuses (renewal_campaigns.status).
const (
	RenewalCampaignOpen   = "open"
	RenewalCampaignClosed = "closed"
)

// RenewalCampaign is a "proponi rinnovi" run from a source season to a
// target season (spec 10.4).
type RenewalCampaign struct {
	bun.BaseModel `bun:"table:renewal_campaigns,alias:rcp"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	Name            string     `bun:"name,notnull"`
	Status          string     `bun:"status,notnull"`
	SourceSeasonID  string     `bun:"source_season_id,notnull"`
	TargetSeasonID  string     `bun:"target_season_id,notnull"`
	DeadlineAt      time.Time  `bun:"deadline_at,notnull"`
	Notes           string     `bun:"notes,notnull"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	ClosedAt        *time.Time `bun:"closed_at"`
	Version         int64      `bun:"version,notnull"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// Renewal proposal statuses (renewal_proposals.status).
const (
	RenewalProposed  = "proposed"
	RenewalAccepted  = "accepted"
	RenewalDeclined  = "declined"
	RenewalExpired   = "expired"
	RenewalConverted = "converted"
)

// Renewal response channels (renewal_proposals.response_channel).
const (
	RenewalResponseOnline = "online"
	RenewalResponseDesk   = "desk"
	RenewalResponseSystem = "system"
)

// RenewalProposal is the renewal offer of one season pass: it owns the held
// booking of the target season (BookingID, renewal_of_booking_id =
// SourceBookingID, hold_expires_at = DeadlineAt).
type RenewalProposal struct {
	bun.BaseModel `bun:"table:renewal_proposals,alias:rnp"`

	ID              string      `bun:"id,pk"`
	EstablishmentID string      `bun:"establishment_id,notnull"`
	CampaignID      string      `bun:"campaign_id,notnull"`
	SourceBookingID string      `bun:"source_booking_id,notnull"`
	BookingID       string      `bun:"booking_id,notnull"`
	CustomerID      string      `bun:"customer_id,notnull"`
	SpotID          string      `bun:"spot_id,notnull"`
	PackID          string      `bun:"pack_id,notnull"`
	Status          string      `bun:"status,notnull"`
	PreviousAmount  money.Cents `bun:"previous_amount,notnull"`
	TotalAmount     money.Cents `bun:"total_amount,notnull"`
	DepositDue      money.Cents `bun:"deposit_due,notnull"`
	DeadlineAt      time.Time   `bun:"deadline_at,notnull"`
	SendCount       int         `bun:"send_count,notnull"`
	LastSentAt      *time.Time  `bun:"last_sent_at"`
	RespondedAt     *time.Time  `bun:"responded_at"`
	ResponseChannel *string     `bun:"response_channel"`
	DeclineReason   string      `bun:"decline_reason,notnull"`
	Version         int64       `bun:"version,notnull"`
	CreatedAt       time.Time   `bun:"created_at,notnull"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull"`
}

// WaitlistSettings is the waitlist configuration of an establishment (one
// row; absent = defaults of migration 00008).
type WaitlistSettings struct {
	bun.BaseModel `bun:"table:waitlist_settings,alias:wls"`

	EstablishmentID  string    `bun:"establishment_id,pk"`
	Enabled          bool      `bun:"enabled,notnull"`
	PublicJoin       bool      `bun:"public_join,notnull"`
	AutoOffer        bool      `bun:"auto_offer,notnull"`
	OfferHoldMinutes int       `bun:"offer_hold_minutes,notnull"`
	Version          int64     `bun:"version,notnull"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
	UpdatedAt        time.Time `bun:"updated_at,notnull"`
}

// Waitlist entry statuses (waitlist_entries.status).
const (
	WaitlistWaiting   = "waiting"
	WaitlistOffered   = "offered"
	WaitlistConverted = "converted"
	WaitlistExpired   = "expired"
	WaitlistRemoved   = "removed"
)

// WaitlistEntry is a customer request with no availability (spec 10.5).
// AccessTokenHash is the SHA-256 (hex) of the customer's waitlist link token.
type WaitlistEntry struct {
	bun.BaseModel `bun:"table:waitlist_entries,alias:wle"`

	ID                 string         `bun:"id,pk"`
	EstablishmentID    string         `bun:"establishment_id,notnull"`
	CustomerID         string         `bun:"customer_id,notnull"`
	Channel            string         `bun:"channel,notnull"`
	StartsOn           calendar.Date  `bun:"starts_on,notnull"`
	EndsOn             calendar.Date  `bun:"ends_on,notnull"`
	Slots              calendar.Slots `bun:"slots,notnull"`
	PackID             string         `bun:"pack_id,notnull"`
	ZoneID             *string        `bun:"zone_id"`
	SpotID             *string        `bun:"spot_id"`
	Quantity           int            `bun:"quantity,notnull"`
	Priority           int            `bun:"priority,notnull"`
	Status             string         `bun:"status,notnull"`
	Note               string         `bun:"note,notnull"`
	Locale             string         `bun:"locale,notnull"`
	AccessTokenHash    *string        `bun:"access_token_hash"`
	ConvertedBookingID *string        `bun:"converted_booking_id"`
	RemovedAt          *time.Time     `bun:"removed_at"`
	Version            int64          `bun:"version,notnull"`
	CreatedAt          time.Time      `bun:"created_at,notnull"`
	UpdatedAt          time.Time      `bun:"updated_at,notnull"`
}

// Waitlist offer statuses (waitlist_offers.status).
const (
	WaitlistOfferPending  = "pending"
	WaitlistOfferAccepted = "accepted"
	WaitlistOfferDeclined = "declined"
	WaitlistOfferExpired  = "expired"
	WaitlistOfferRevoked  = "revoked"
)

// Waitlist offer causes (waitlist_offers.cause): what freed the inventory.
const (
	WaitlistCauseCancellation = "cancellation"
	WaitlistCauseRelease      = "release"
	WaitlistCauseExpiry       = "expiry"
	WaitlistCauseManual       = "manual"
)

// WaitlistOffer reserves inventory for a waitlist entry with a hold (holds
// table, DB-enforced) until ExpiresAt.
type WaitlistOffer struct {
	bun.BaseModel `bun:"table:waitlist_offers,alias:wlo"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	EntryID         string     `bun:"entry_id,notnull"`
	Status          string     `bun:"status,notnull"`
	HoldID          string     `bun:"hold_id,notnull"`
	SpotID          *string    `bun:"spot_id"`
	BookingID       *string    `bun:"booking_id"`
	Cause           string     `bun:"cause,notnull"`
	ExpiresAt       time.Time  `bun:"expires_at,notnull"`
	RespondedAt     *time.Time `bun:"responded_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// CustomerCreditBalance is the running sum of the credit ledger of a
// customer (CHECK balance >= 0). Invariant: Balance = SUM(customer_credits.
// amount) of the customer; updated in the same transaction as every ledger
// insert with a conditional UPDATE (never negative, even concurrently).
type CustomerCreditBalance struct {
	bun.BaseModel `bun:"table:customer_credit_balances,alias:ccb"`

	EstablishmentID string      `bun:"establishment_id,pk"`
	CustomerID      string      `bun:"customer_id,pk"`
	Balance         money.Cents `bun:"balance,notnull"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull"`
}
