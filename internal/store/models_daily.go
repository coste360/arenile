package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Daily sale and ticket models (migration 00010). Ownership:
//   - daily_ticket_counters, tickets: internal/domain/ticketing;
//   - daily_sale_settings: internal/domain/dailysale;
//   - entry_ticket_types: internal/domain/ticketing (entry tickets).
//
// The daily number is enforced by the schema: the counter row is advanced
// with one atomic upsert in the sale transaction and tickets carries
// UNIQUE (establishment_id, ticket_date, ticket_number).

// Ticket kinds (tickets.kind).
const (
	TicketKindSale  = "sale"
	TicketKindEntry = "entry"
)

// WalkInCustomerTag tags the shared customer of the quick sales without a
// name ("Cliente al banco", internal/domain/dailysale): a system row that
// the customer rankings leave out.
const WalkInCustomerTag = "walk-in"

// Ticket statuses (tickets.status). A voided ticket keeps its number.
const (
	TicketValid  = "valid"
	TicketVoided = "voided"
)

// Ticket payment methods (tickets.payment_method), both via the manual driver.
const (
	TicketPaymentCash = "cash"
	TicketPaymentCard = "card"
)

// Ticket print formats (daily_sale_settings.print_format).
const (
	PrintThermal80 = "thermal_80"
	PrintThermal58 = "thermal_58"
	PrintA4        = "a4"
	PrintA5        = "a5"
)

// DailyTicketCounter is the last number handed out for one establishment
// and calendar day (establishment timezone).
type DailyTicketCounter struct {
	bun.BaseModel `bun:"table:daily_ticket_counters,alias:dtc"`

	EstablishmentID string        `bun:"establishment_id,pk"`
	TicketDate      calendar.Date `bun:"ticket_date,pk"`
	LastNumber      int64         `bun:"last_number,notnull"`
	UpdatedAt       time.Time     `bun:"updated_at,notnull"`
}

// Ticket is the receipt of one walk-in sale or entry sale; it owns exactly
// one booking. NeedsAssignment marks a zone-first sale ("da assegnare");
// ProvisionalSpotID is the placeholder allocation of a seat-mode zone.
// HolderName is the optional map label (never an email or phone).
type Ticket struct {
	bun.BaseModel `bun:"table:tickets,alias:tk"`

	ID                string        `bun:"id,pk"`
	EstablishmentID   string        `bun:"establishment_id,notnull"`
	BookingID         string        `bun:"booking_id,notnull"`
	TicketDate        calendar.Date `bun:"ticket_date,notnull"`
	TicketNumber      int64         `bun:"ticket_number,notnull"`
	Kind              string        `bun:"kind,notnull"`
	Status            string        `bun:"status,notnull"`
	ZoneID            string        `bun:"zone_id,notnull"`
	NeedsAssignment   bool          `bun:"needs_assignment,notnull"`
	ProvisionalSpotID *string       `bun:"provisional_spot_id"`
	HolderName        string        `bun:"holder_name,notnull"`
	TotalAmount       money.Cents   `bun:"total_amount,notnull"`
	PaymentMethod     string        `bun:"payment_method,notnull"`
	PaymentID         *string       `bun:"payment_id"`
	CashSessionID     *string       `bun:"cash_session_id"`
	PrintCount        int           `bun:"print_count,notnull"`
	CreatedByUserID   *string       `bun:"created_by_user_id"`
	VoidedAt          *time.Time    `bun:"voided_at"`
	VoidedByUserID    *string       `bun:"voided_by_user_id"`
	VoidReason        string        `bun:"void_reason,notnull"`
	RefundPaymentID   *string       `bun:"refund_payment_id"`
	EnteredAt         *time.Time    `bun:"entered_at"`
	EnteredByUserID   *string       `bun:"entered_by_user_id"`
	// GateToken is the random QR payload (ticketing.NewGateToken); the gate
	// admits by it, never by the sequential booking code alone.
	GateToken string    `bun:"gate_token,notnull"`
	Version   int64     `bun:"version,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// EntryTicketType is an entry-only ticket type (Adulto, Ridotto, ...): a
// pack priced by the pricing engine, sold on a quantity-mode zone whose pool
// is the daily capacity shared with the online channel.
type EntryTicketType struct {
	bun.BaseModel `bun:"table:entry_ticket_types,alias:ett"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Code            string    `bun:"code,notnull"`
	Name            I18nText  `bun:"name,type:json,notnull"`
	Description     I18nText  `bun:"description,type:json,notnull"`
	PackID          string    `bun:"pack_id,notnull"`
	ZoneID          string    `bun:"zone_id,notnull"`
	SortOrder       int       `bun:"sort_order,notnull"`
	Active          bool      `bun:"active,notnull"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// DailySaleSettings are the per-establishment options of the quick sale.
// WalkInCustomerID is the customer used when no name is given (created on
// first use by internal/domain/dailysale).
type DailySaleSettings struct {
	bun.BaseModel `bun:"table:daily_sale_settings,alias:dss"`

	EstablishmentID  string    `bun:"establishment_id,pk"`
	WalkInCustomerID *string   `bun:"walk_in_customer_id"`
	PrintFormat      string    `bun:"print_format,notnull"`
	AutoPrint        bool      `bun:"auto_print,notnull"`
	ZoneFirstEnabled bool      `bun:"zone_first_enabled,notnull"`
	FooterText       I18nText  `bun:"footer_text,type:json,notnull"`
	Version          int64     `bun:"version,notnull"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
	UpdatedAt        time.Time `bun:"updated_at,notnull"`
}
