package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Models of the customer area (spec 6.5, 13.4, 23): customers and their credit
// ledger. Repository functions live in internal/store/customer.go (owner:
// internal/domain/customer, see its doc.go).

// MarketingConsent is the optional, separate marketing consent (spec 23).
type MarketingConsent struct {
	GivenAt     time.Time `json:"given_at"`
	Source      string    `json:"source"`
	TextVersion string    `json:"text_version"`
}

// Customer is a customer of one establishment. Email is lowercase, phone
// E.164. SearchText is the lowercase, accent-free "last first email phone"
// maintained by the customer service on every write (portable LIKE search).
// After anonymization (GDPR) personal fields are replaced and AnonymizedAt
// is set; bookings and payments keep referencing the row.
type Customer struct {
	bun.BaseModel `bun:"table:customers,alias:cu"`

	ID               string            `bun:"id,pk"`
	EstablishmentID  string            `bun:"establishment_id,notnull"`
	FirstName        string            `bun:"first_name,notnull"`
	LastName         string            `bun:"last_name,notnull"`
	Email            *string           `bun:"email"`
	Phone            *string           `bun:"phone"`
	Locale           string            `bun:"locale,notnull"`
	Notes            string            `bun:"notes,notnull"`
	Tags             []string          `bun:"tags,type:json,notnull"`
	AgreementID      *string           `bun:"agreement_id"`
	MarketingConsent *MarketingConsent `bun:"marketing_consent,type:json"`
	SearchText       string            `bun:"search_text,notnull"`
	AnonymizedAt     *time.Time        `bun:"anonymized_at"`
	Version          int64             `bun:"version,notnull"`
	CreatedAt        time.Time         `bun:"created_at,notnull"`
	UpdatedAt        time.Time         `bun:"updated_at,notnull"`
}

// Credit sources (customer_credits.source).
const (
	CreditSourceCancellation  = "cancellation"
	CreditSourceSeasonRelease = "season_release"
	CreditSourceWeather       = "weather"
	CreditSourceManual        = "manual"
	CreditSourceRedemption    = "redemption"
	// M5B (migration 00008).
	CreditSourceGoodwill   = "goodwill"   // courtesy credit (amount > 0)
	CreditSourceExpiry     = "expiry"     // expiry of ReferenceCreditID (amount < 0)
	CreditSourceAdjustment = "adjustment" // manager correction (any sign)
)

// CustomerCredit is one movement of the customer credit ledger (spec 13.4):
// Amount > 0 grants credit, Amount < 0 consumes it (payment method
// "credit"). The balance is the sum of non-expired movements.
type CustomerCredit struct {
	bun.BaseModel `bun:"table:customer_credits,alias:cc"`

	ID              string         `bun:"id,pk"`
	EstablishmentID string         `bun:"establishment_id,notnull"`
	CustomerID      string         `bun:"customer_id,notnull"`
	Amount          money.Cents    `bun:"amount,notnull"`
	Source          string         `bun:"source,notnull"`
	BookingID       *string        `bun:"booking_id"`
	SeasonReleaseID *string        `bun:"season_release_id"`
	PaymentID       *string        `bun:"payment_id"`
	Note            string         `bun:"note,notnull"`
	ExpiresOn       *calendar.Date `bun:"expires_on"`
	CreatedByUserID *string        `bun:"created_by_user_id"`
	CreatedAt       time.Time      `bun:"created_at,notnull"`
	// ReferenceCreditID is set only on expiry rows: the grant they expire.
	ReferenceCreditID *string `bun:"reference_credit_id"`
}
