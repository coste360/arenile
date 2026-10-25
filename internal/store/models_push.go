package store

import (
	"time"

	"github.com/uptrace/bun"
)

// Models of the push notifications (migration 00013, ADR 0027). Models only:
// repository functions live in internal/store/push.go, owned by
// internal/domain/push.

// Push message keys (push_deliveries.message_key, api.PushTopic): a closed
// catalogue translated by the relay; never free text.
const (
	PushTopicNewOnlineBooking  = "booking_new_online"
	PushTopicApprovalRequested = "booking_approval_requested"
	PushTopicPaymentFailed     = "payment_failed"
)

// Push environments (push_registrations.environment): APNs production or
// sandbox (development builds).
const (
	PushEnvironmentProduction = "production"
	PushEnvironmentSandbox    = "sandbox"
)

// Push delivery statuses (push_deliveries.status).
const (
	PushDeliveryQueued  = "queued"
	PushDeliverySending = "sending"
	PushDeliverySent    = "sent"
	PushDeliveryFailed  = "failed"
	PushDeliveryExpired = "expired"
)

// PushTopics lists the message keys in catalogue order.
var PushTopics = []string{PushTopicNewOnlineBooking, PushTopicApprovalRequested, PushTopicPaymentFailed}

// PushSettings is the push opt-in of the manager of an establishment. No
// row means the defaults: disabled, every event on.
type PushSettings struct {
	bun.BaseModel `bun:"table:push_settings,alias:pss"`

	EstablishmentID   string    `bun:"establishment_id,pk"`
	Enabled           bool      `bun:"enabled,notnull"`
	NewOnlineBooking  bool      `bun:"notify_new_online_booking,notnull"`
	ApprovalRequested bool      `bun:"notify_approval_requested,notnull"`
	PaymentFailed     bool      `bun:"notify_payment_failed,notnull"`
	Version           int64     `bun:"version,notnull"`
	CreatedAt         time.Time `bun:"created_at,notnull"`
	UpdatedAt         time.Time `bun:"updated_at,notnull"`
}

// PushRegistration is the native push token of a paired mobile device.
// TokenCiphertext is encrypted with the key ring (AAD = DeviceID);
// TokenHash is its SHA-256 hex.
type PushRegistration struct {
	bun.BaseModel `bun:"table:push_registrations,alias:pr"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	DeviceID        string    `bun:"device_id,notnull"`
	UserID          string    `bun:"user_id,notnull"`
	Platform        string    `bun:"platform,notnull"`
	Environment     string    `bun:"environment,notnull"`
	Locale          string    `bun:"locale,notnull"`
	Topics          []string  `bun:"topics,type:json,notnull"`
	TokenCiphertext string    `bun:"token_ciphertext,notnull"`
	TokenHash       string    `bun:"token_hash,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// PushDelivery is one notification for one registration, sent by the
// push.deliver job with a lease. (RegistrationID, EventID) is unique.
type PushDelivery struct {
	bun.BaseModel `bun:"table:push_deliveries,alias:pd"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	RegistrationID  string     `bun:"registration_id,notnull"`
	EventID         string     `bun:"event_id,notnull"`
	MessageKey      string     `bun:"message_key,notnull"`
	Status          string     `bun:"status,notnull"`
	Attempts        int        `bun:"attempts,notnull"`
	NextAttemptAt   *time.Time `bun:"next_attempt_at"`
	LockedUntil     *time.Time `bun:"locked_until"`
	LastError       *string    `bun:"last_error"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
	SentAt          *time.Time `bun:"sent_at"`
}

// PushRecipient is a registration joined with what the sink and the
// deliverer re-check at send time: the device (revocation, idle expiry) and
// the user (active flag).
type PushRecipient struct {
	PushRegistration `bun:",extend"`

	DeviceRevokedAt  *time.Time `bun:"device_revoked_at"`
	DeviceLastUsedAt *time.Time `bun:"device_last_used_at"`
	DeviceCreatedAt  time.Time  `bun:"device_created_at"`
	UserActive       bool       `bun:"user_active"`
}
