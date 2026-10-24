package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Models of payments and fiscal documents (spec 6.7, 13, 14.2). Ownership:
// payments -> internal/domain/payment (repository internal/store/payment.go);
// fiscal_documents -> internal/domain/fiscal (repository internal/store/fiscal.go).

// Payment values (payments.method, direction, status).
const (
	PaymentMethodCash         = "cash"
	PaymentMethodCard         = "card"
	PaymentMethodBankTransfer = "bank_transfer"
	PaymentMethodWallet       = "wallet"
	PaymentMethodCredit       = "credit"
	PaymentMethodOther        = "other"

	PaymentDirectionIn     = "in"
	PaymentDirectionRefund = "refund"

	PaymentPending   = "pending"
	PaymentSucceeded = "succeeded"
	PaymentFailed    = "failed"
	PaymentCancelled = "cancelled"
)

// VATShare is the part of a payment attributed to one VAT rate (gross =
// taxable + tax, INV-16), frozen when the payment is recorded.
type VATShare struct {
	RateBP  int64       `json:"rate_bp"`
	Gross   money.Cents `json:"gross"`
	Taxable money.Cents `json:"taxable"`
	Tax     money.Cents `json:"tax"`
}

// Payment is one movement of the payments ledger: an incoming payment
// (Direction in) or a refund (Direction refund); Amount is always > 0.
// BookingID is nil for counter sales. BusinessDate is the calendar date in
// the establishment timezone (corrispettivi export). IdempotencyKey is
// unique per establishment.
type Payment struct {
	bun.BaseModel `bun:"table:payments,alias:pm"`

	ID                string         `bun:"id,pk"`
	EstablishmentID   string         `bun:"establishment_id,notnull"`
	BookingID         *string        `bun:"booking_id"`
	CustomerID        *string        `bun:"customer_id"`
	Provider          string         `bun:"provider,notnull"`
	Method            string         `bun:"method,notnull"`
	Direction         string         `bun:"direction,notnull"`
	Amount            money.Cents    `bun:"amount,notnull"`
	Status            string         `bun:"status,notnull"`
	ProviderRef       *string        `bun:"provider_ref"`
	IdempotencyKey    *string        `bun:"idempotency_key"`
	CashSessionID     *string        `bun:"cash_session_id"`
	RefundOfPaymentID *string        `bun:"refund_of_payment_id"`
	ReceivedByUserID  *string        `bun:"received_by_user_id"`
	Description       string         `bun:"description,notnull"`
	Note              string         `bun:"note,notnull"`
	VATBreakdown      []VATShare     `bun:"vat_breakdown,type:json,notnull"`
	BusinessDate      calendar.Date  `bun:"business_date,notnull"`
	Metadata          map[string]any `bun:"metadata,type:json,notnull"`
	// Test marks a payment taken in test mode (00009): excluded from
	// corrispettivi, cash totals of reports and exports.
	Test      bool      `bun:"test,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// Signed is the payment amount with its sign in the ledger (refunds negative).
func (p *Payment) Signed() money.Cents {
	if p.Direction == PaymentDirectionRefund {
		return -p.Amount
	}
	return p.Amount
}

// Fiscal document statuses (fiscal_documents.status).
const (
	FiscalPending = "pending"
	FiscalIssued  = "issued"
	FiscalFailed  = "failed"
	FiscalVoid    = "void"
)

// FiscalDocument tracks one emission (or failure) of the fiscal driver for a
// payment (spec 14.2). Payload is the ext.FiscalDocumentRequest sent,
// Response the driver answer.
type FiscalDocument struct {
	bun.BaseModel `bun:"table:fiscal_documents,alias:fd"`

	ID              string        `bun:"id,pk"`
	EstablishmentID string        `bun:"establishment_id,notnull"`
	PaymentID       string        `bun:"payment_id,notnull"`
	Driver          string        `bun:"driver,notnull"`
	Status          string        `bun:"status,notnull"`
	DocumentNumber  *string       `bun:"document_number"`
	BusinessDate    calendar.Date `bun:"business_date,notnull"`
	Payload         JSON          `bun:"payload,notnull"`
	Response        JSON          `bun:"response"`
	Error           *string       `bun:"error"`
	CreatedAt       time.Time     `bun:"created_at,notnull"`
	UpdatedAt       time.Time     `bun:"updated_at,notnull"`
}
