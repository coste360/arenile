package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Models of the deferred features (migration 00009, ADR 0019): test mode,
// guaranteed refunds, cash registers, webhook delivery attempts, CSV
// imports and in-panel notifications. Ownership is noted on each model;
// repositories live next to the existing ones in internal/store.

// TestModeSettings is the test-mode switch of an establishment (spec 15.4).
// No row = test mode off. PublicEnabled can be true only while Enabled
// (CHECK). Ownership: internal/domain/testmode.
type TestModeSettings struct {
	bun.BaseModel `bun:"table:test_mode_settings,alias:tms"`

	EstablishmentID string     `bun:"establishment_id,pk"`
	Enabled         bool       `bun:"enabled,notnull"`
	PublicEnabled   bool       `bun:"public_enabled,notnull"`
	EnabledAt       *time.Time `bun:"enabled_at"`
	EnabledByUserID *string    `bun:"enabled_by_user_id"`
	Version         int64      `bun:"version,notnull"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// Refund request statuses (api RefundRequestStatus).
const (
	RefundRequestPending    = "pending"
	RefundRequestProcessing = "processing"
	RefundRequestSucceeded  = "succeeded"
	RefundRequestFailed     = "failed"
	RefundRequestCancelled  = "cancelled"
)

// Refund request reasons.
const (
	RefundReasonCustomerCancellation = "customer_cancellation"
	RefundReasonPanelCancellation    = "panel_cancellation"
	RefundReasonExpiredHold          = "expired_hold"
)

// RefundRequest is a provider refund reserved in the transaction of the
// cancellation and sent by a job until the provider confirms. The unique
// key (booking_id, payment_intent_id) over non-cancelled rows forbids a
// second refund of the same payment; IdempotencyKey is sent to the provider
// on every attempt. Ownership: internal/domain/refund.
type RefundRequest struct {
	bun.BaseModel `bun:"table:refund_requests,alias:rr"`

	ID              string      `bun:"id,pk"`
	EstablishmentID string      `bun:"establishment_id,notnull"`
	BookingID       string      `bun:"booking_id,notnull"`
	PaymentIntentID string      `bun:"payment_intent_id,notnull"`
	RefundIntentID  *string     `bun:"refund_intent_id"`
	PaymentID       *string     `bun:"payment_id"`
	Provider        string      `bun:"provider,notnull"`
	Amount          money.Cents `bun:"amount,notnull"`
	Currency        string      `bun:"currency,notnull"`
	Reason          string      `bun:"reason,notnull"`
	Status          string      `bun:"status,notnull"`
	IdempotencyKey  string      `bun:"idempotency_key,notnull"`
	Attempts        int         `bun:"attempts,notnull"`
	NextAttemptAt   *time.Time  `bun:"next_attempt_at"`
	LastError       *string     `bun:"last_error"`
	Test            bool        `bun:"test,notnull"`
	CreatedByUserID *string     `bun:"created_by_user_id"`
	Version         int64       `bun:"version,notnull"`
	CreatedAt       time.Time   `bun:"created_at,notnull"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull"`
	CompletedAt     *time.Time  `bun:"completed_at"`
}

// DefaultCashRegisterName is the name of the register created for every
// establishment (code DefaultCashRegister).
const DefaultCashRegisterName = "Cassa principale"

// CashRegister is a till of an establishment. Code is stable and equals
// CashSession.Register; exactly one register per establishment has
// IsDefault (unique partial index), and it stays active. At most one open
// session per register (cash_sessions_register_open_uidx). Ownership:
// internal/domain/cash.
type CashRegister struct {
	bun.BaseModel `bun:"table:cash_registers,alias:cr"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Code            string    `bun:"code,notnull"`
	Name            string    `bun:"name,notnull"`
	Active          bool      `bun:"active,notnull"`
	IsDefault       bool      `bun:"is_default,notnull"`
	SortOrder       int       `bun:"sort_order,notnull"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// WebhookDeliveryAttempt is one HTTP attempt of a webhook delivery (full
// history; webhook_deliveries keeps the last attempt summary). Purged by
// the retention job. Ownership: internal/domain/event (deliverer).
type WebhookDeliveryAttempt struct {
	bun.BaseModel `bun:"table:webhook_delivery_attempts,alias:wda"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	EndpointID      string    `bun:"endpoint_id,notnull"`
	DeliveryID      string    `bun:"delivery_id,notnull"`
	Attempt         int       `bun:"attempt,notnull"`
	StatusCode      *int      `bun:"status_code"`
	Error           *string   `bun:"error"`
	ResponseExcerpt *string   `bun:"response_excerpt"`
	DurationMS      int64     `bun:"duration_ms,notnull"`
	AttemptedAt     time.Time `bun:"attempted_at,notnull"`
}

// Import kinds and statuses (api ImportKind, ImportJobStatus).
const (
	ImportKindCustomers    = "customers"
	ImportKindSeasonPasses = "season_passes"

	ImportStatusUploaded  = "uploaded"
	ImportStatusValidated = "validated"
	ImportStatusApplying  = "applying"
	ImportStatusApplied   = "applied"
	ImportStatusFailed    = "failed"
	ImportStatusDiscarded = "discarded"
)

// ImportJob is a CSV import. Content (personal data) is kept only until
// apply/discard and at most 7 days; Report holds row numbers, columns and
// codes only. The same file (SHA256) is applied at most once per kind
// (unique partial index). Ownership: internal/domain/importer.
type ImportJob struct {
	bun.BaseModel `bun:"table:import_jobs,alias:ij"`

	ID              string         `bun:"id,pk"`
	EstablishmentID string         `bun:"establishment_id,notnull"`
	Kind            string         `bun:"kind,notnull"`
	Status          string         `bun:"status,notnull"`
	Filename        string         `bun:"filename,notnull"`
	SizeBytes       int64          `bun:"size_bytes,notnull"`
	SHA256          string         `bun:"sha256,notnull"`
	Content         *string        `bun:"content"`
	Options         map[string]any `bun:"options,type:json,notnull"`
	Report          map[string]any `bun:"report,type:json,nullzero"`
	CreatedByUserID *string        `bun:"created_by_user_id"`
	AppliedAt       *time.Time     `bun:"applied_at"`
	Version         int64          `bun:"version,notnull"`
	CreatedAt       time.Time      `bun:"created_at,notnull"`
	UpdatedAt       time.Time      `bun:"updated_at,notnull"`
}

// Notification audiences.
const (
	NotificationAudienceStaff = "staff"
	NotificationAudienceOwner = "owner"
	// NotificationAudienceManager: whoever can act on refunds (refunds.issue:
	// owner and manager), e.g. a guaranteed refund that needs a manual retry.
	NotificationAudienceManager = "manager"
)

// AdminNotification is an in-panel notification of an establishment.
// DedupeKey (unique per establishment) makes producers idempotent; Params
// never contain personal data (ids and labels only). Read state is shared
// by the staff. Ownership: internal/domain/notify (panel notifications).
type AdminNotification struct {
	bun.BaseModel `bun:"table:admin_notifications,alias:an"`

	ID              string            `bun:"id,pk"`
	EstablishmentID string            `bun:"establishment_id,notnull"`
	Kind            string            `bun:"kind,notnull"`
	Severity        string            `bun:"severity,notnull"`
	Audience        string            `bun:"audience,notnull"`
	Params          map[string]string `bun:"params,type:json,notnull"`
	ResourceType    *string           `bun:"resource_type"`
	ResourceID      *string           `bun:"resource_id"`
	DedupeKey       string            `bun:"dedupe_key,notnull"`
	ReadAt          *time.Time        `bun:"read_at"`
	ReadByUserID    *string           `bun:"read_by_user_id"`
	CreatedAt       time.Time         `bun:"created_at,notnull"`
}
