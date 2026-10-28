package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// M8 models (migration 00013, spec 30, ADR 0028): partners, customer
// categories, agreement versions and quota, partner payments, requirement
// verifications and the move undo log. The new columns of agreements,
// bookings, spots and extras are fields of their existing models
// (models_pricing.go, models_booking.go, models_mapping.go,
// models_catalog.go). Query functions live in one file per area, owned by
// the matching builder: partner.go (partners, categories, verifications,
// partner payments), agreement_quota.go (quota usage, INV-103),
// move_undo.go (undo log), report_m8.go (statement, partner report,
// agreements register).

// Partner kinds (partners.kind, spec 30.2.1).
const (
	PartnerKindAccommodation = "struttura_ricettiva"
	PartnerKindPublicBody    = "ente"
	PartnerKindCompany       = "azienda"
	PartnerKindAssociation   = "associazione"
	PartnerKindOther         = "altro"
)

// Agreement targets (agreements.target; NULL = AgreementTargetCustomers).
const (
	AgreementTargetPartner   = "partner"
	AgreementTargetCategory  = "customer_category"
	AgreementTargetCustomers = "individual_customers"
)

// Register categories (customer_categories.register_category,
// agreements.register_category).
const (
	RegisterResidents     = "residents"
	RegisterAccommodation = "accommodation"
	RegisterProtected     = "protected"
	RegisterCompanies     = "companies"
	RegisterAssociations  = "associations"
	RegisterOther         = "other"
)

// Partner share types (bookings.partner_share_type; NULL = none).
const (
	PartnerShareFull    = "full"
	PartnerSharePercent = "percent" // partner_share_value in basis points
	PartnerShareAmount  = "amount"  // partner_share_value in cents
)

// Move undo kinds (move_undo_log.kind) and the undo window (spec 30.7).
const (
	MoveUndoMove  = "move"
	MoveUndoSwap  = "swap"
	MoveUndoGroup = "group"

	MoveUndoWindow = 5 * time.Minute
)

// Partner is a counterpart of agreements (hotel, B&B, municipality,
// company, association), separate from customers. ContactName, Email and
// Phone are personal data: never logged.
type Partner struct {
	bun.BaseModel `bun:"table:partners,alias:pa"`

	ID                string    `bun:"id,pk"`
	EstablishmentID   string    `bun:"establishment_id,notnull"`
	Name              string    `bun:"name,notnull"`
	Kind              string    `bun:"kind,notnull"`
	AccommodationType *string   `bun:"accommodation_type"`
	Municipality      string    `bun:"municipality,notnull"`
	Address           string    `bun:"address,notnull"`
	ContactName       string    `bun:"contact_name,notnull"`
	Email             *string   `bun:"email"`
	Phone             *string   `bun:"phone"`
	VATNumber         string    `bun:"vat_number,notnull"`
	TaxCode           string    `bun:"tax_code,notnull"`
	Notes             string    `bun:"notes,notnull"`
	Active            bool      `bun:"active,notnull"`
	SearchText        string    `bun:"search_text,notnull"`
	Version           int64     `bun:"version,notnull"`
	CreatedAt         time.Time `bun:"created_at,notnull"`
	UpdatedAt         time.Time `bun:"updated_at,notnull"`
}

// CustomerCategory is a category of customers (residents, over 65...).
type CustomerCategory struct {
	bun.BaseModel `bun:"table:customer_categories,alias:cc"`

	ID               string    `bun:"id,pk"`
	EstablishmentID  string    `bun:"establishment_id,notnull"`
	Code             string    `bun:"code,notnull"`
	Name             string    `bun:"name,notnull"`
	Description      string    `bun:"description,notnull"`
	Requirement      string    `bun:"requirement,notnull"`
	RegisterCategory string    `bun:"register_category,notnull"`
	Active           bool      `bun:"active,notnull"`
	Version          int64     `bun:"version,notnull"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
	UpdatedAt        time.Time `bun:"updated_at,notnull"`
}

// CustomerCategoryMember assigns a customer to a category.
type CustomerCategoryMember struct {
	bun.BaseModel `bun:"table:customer_category_members,alias:ccm"`

	EstablishmentID string    `bun:"establishment_id,notnull"`
	CustomerID      string    `bun:"customer_id,pk"`
	CategoryID      string    `bun:"category_id,pk"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
}

// AgreementVersion is the immutable snapshot of an agreement at one version
// (the api.AgreementInput JSON).
type AgreementVersion struct {
	bun.BaseModel `bun:"table:agreement_versions,alias:agv"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	AgreementID     string    `bun:"agreement_id,notnull"`
	Version         int64     `bun:"version,notnull"`
	Snapshot        JSON      `bun:"snapshot,notnull"`
	ChangedByUserID *string   `bun:"changed_by_user_id"`
	ChangedAt       time.Time `bun:"changed_at,notnull"`
}

// AgreementQuotaUsage is the counter of an agreement quota on one half-day
// (INV-103). Updated only with conditional UPDATEs (used + :n <= max_units,
// also a CHECK); Reserved is held back from general sale until ReleaseOn.
type AgreementQuotaUsage struct {
	bun.BaseModel `bun:"table:agreement_quota_usage,alias:aqu"`

	EstablishmentID string        `bun:"establishment_id,notnull"`
	AgreementID     string        `bun:"agreement_id,pk"`
	Date            calendar.Date `bun:"date,pk"`
	Slot            calendar.Slot `bun:"slot,pk"`
	Reserved        int           `bun:"reserved,notnull"`
	Used            int           `bun:"used,notnull"`
	MaxUnits        *int          `bun:"max_units"`
	ReleaseOn       calendar.Date `bun:"release_on,notnull"`
	UpdatedAt       time.Time     `bun:"updated_at,notnull"`
}

// AgreementQuotaAllocation is the per booking line detail of the agreement
// quota consumption (rebuilds agreement_quota_usage; deleted on release).
type AgreementQuotaAllocation struct {
	bun.BaseModel `bun:"table:agreement_quota_allocations,alias:aqa"`

	ID              string        `bun:"id,pk"`
	EstablishmentID string        `bun:"establishment_id,notnull"`
	AgreementID     string        `bun:"agreement_id,notnull"`
	BookingID       string        `bun:"booking_id,notnull"`
	BookingLineID   string        `bun:"booking_line_id,notnull"`
	Date            calendar.Date `bun:"date,notnull"`
	Slot            calendar.Slot `bun:"slot,notnull"`
	Quantity        int           `bun:"quantity,notnull"`
	CreatedAt       time.Time     `bun:"created_at,notnull"`
}

// PartnerPayment is a payment received from a partner (statement balance =
// charged - paid, INV-104). A voided payment keeps its row.
type PartnerPayment struct {
	bun.BaseModel `bun:"table:partner_payments,alias:pp"`

	ID               string        `bun:"id,pk"`
	EstablishmentID  string        `bun:"establishment_id,notnull"`
	PartnerID        string        `bun:"partner_id,notnull"`
	Amount           money.Cents   `bun:"amount,notnull"`
	ReceivedOn       calendar.Date `bun:"received_on,notnull"`
	Method           string        `bun:"method,notnull"`
	Reference        string        `bun:"reference,notnull"`
	Note             string        `bun:"note,notnull"`
	RecordedByUserID *string       `bun:"recorded_by_user_id"`
	VoidedAt         *time.Time    `bun:"voided_at"`
	VoidReason       *string       `bun:"void_reason"`
	VoidedByUserID   *string       `bun:"voided_by_user_id"`
	CreatedAt        time.Time     `bun:"created_at,notnull"`
	UpdatedAt        time.Time     `bun:"updated_at,notnull"`
}

// RequirementVerification records who verified an agreement or category
// requirement and when; never a copy of the document (spec 23).
type RequirementVerification struct {
	bun.BaseModel `bun:"table:requirement_verifications,alias:rv"`

	ID                 string    `bun:"id,pk"`
	EstablishmentID    string    `bun:"establishment_id,notnull"`
	Subject            string    `bun:"subject,notnull"` // booking | customer
	BookingID          *string   `bun:"booking_id"`
	CustomerID         *string   `bun:"customer_id"`
	AgreementID        *string   `bun:"agreement_id"`
	CustomerCategoryID *string   `bun:"customer_category_id"`
	Requirement        string    `bun:"requirement,notnull"`
	Note               string    `bun:"note,notnull"`
	VerifiedByUserID   *string   `bun:"verified_by_user_id"`
	VerifiedAt         time.Time `bun:"verified_at,notnull"`
}

// MoveUndoEntry records a move, swap or group move for the 5 minute undo
// (spec 30.7). Payload is the JSON of the moved lines (booking, line, spot
// before and after, dates, booking version after the move).
type MoveUndoEntry struct {
	bun.BaseModel `bun:"table:move_undo_log,alias:mul"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	UserID          *string    `bun:"user_id"`
	Kind            string     `bun:"kind,notnull"`
	Payload         JSON       `bun:"payload,notnull"`
	BookingIDs      []string   `bun:"booking_ids,type:json,notnull"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	ExpiresAt       time.Time  `bun:"expires_at,notnull"`
	UndoneAt        *time.Time `bun:"undone_at"`
}
