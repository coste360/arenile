package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/money"
)

// Models of the public booking page, online payments and notifications
// (migration 00005, spec 10.1, 12, 13, 19, 23). Models only: repository
// functions live in internal/store/{online,checkout,payment_intent,notify}.go,
// each owned by the matching domain package:
//
//   - terms_versions, online_settings, payment_provider_settings
//     -> internal/domain/online
//   - checkout_sessions -> internal/domain/checkout
//   - payment_intents, webhook_events -> internal/domain/payment/gateway
//   - magic_link_tokens, notification_templates, message_log
//     -> internal/domain/notify
//
// Tokens and secrets are never stored in clear (see the migration header):
// TokenHash fields hold the SHA-256 hex of a 256-bit random token,
// Credentials and RecipientEnc hold internal/crypto ciphertexts.

// TermsTexts are the texts of one locale of a terms version.
type TermsTexts struct {
	Terms     string `json:"terms"`
	Privacy   string `json:"privacy,omitempty"`
	Marketing string `json:"marketing,omitempty"`
}

// TermsVersion is an immutable, published version of the booking terms,
// privacy notice and marketing consent text (spec 12.3, 23). Version is the
// label saved on bookings.terms_version when the customer accepts it.
type TermsVersion struct {
	bun.BaseModel `bun:"table:terms_versions,alias:tv"`

	ID              string                `bun:"id,pk"`
	EstablishmentID string                `bun:"establishment_id,notnull"`
	Version         string                `bun:"version,notnull"`
	Texts           map[string]TermsTexts `bun:"texts,type:json,notnull"`
	PublishedAt     time.Time             `bun:"published_at,notnull"`
	CreatedByUserID *string               `bun:"created_by_user_id"`
	CreatedAt       time.Time             `bun:"created_at,notnull"`
}

// OnlineSettings is the online configuration of one establishment (one row
// per establishment). Config and EmailConfig are owned by
// internal/domain/online (typed there: online.Config, online.EmailConfig).
// Version is the ETag of GET/PUT online-settings and email-settings.
type OnlineSettings struct {
	bun.BaseModel `bun:"table:online_settings,alias:os"`

	EstablishmentID       string    `bun:"establishment_id,pk"`
	Config                JSON      `bun:"config,notnull"`
	EmailConfig           JSON      `bun:"email_config,notnull"`
	CurrentTermsVersionID *string   `bun:"current_terms_version_id"`
	Version               int64     `bun:"version,notnull"`
	CreatedAt             time.Time `bun:"created_at,notnull"`
	UpdatedAt             time.Time `bun:"updated_at,notnull"`
}

// Payment provider modes (payment_provider_settings.mode, payment_intents.mode).
const (
	ProviderModeTest = "test"
	ProviderModeLive = "live"
)

// PaymentProviderSettings holds the configuration of one payment provider
// for an establishment and mode. Credentials is the ciphertext of the JSON
// ext.Credentials map (AAD "<establishment_id>:<provider>:<mode>"), never
// returned by the API; PublicConfig holds the non-secret hints (publishable
// key, secret key last4, webhook secret configured, embedded mode).
type PaymentProviderSettings struct {
	bun.BaseModel `bun:"table:payment_provider_settings,alias:pps"`

	ID              string         `bun:"id,pk"`
	EstablishmentID string         `bun:"establishment_id,notnull"`
	Provider        string         `bun:"provider,notnull"`
	Mode            string         `bun:"mode,notnull"`
	Active          bool           `bun:"active,notnull"`
	Credentials     string         `bun:"credentials,notnull"`
	PublicConfig    map[string]any `bun:"public_config,type:json,notnull"`
	VerifiedAt      *time.Time     `bun:"verified_at"`
	Version         int64          `bun:"version,notnull"`
	CreatedAt       time.Time      `bun:"created_at,notnull"`
	UpdatedAt       time.Time      `bun:"updated_at,notnull"`
}

// Checkout session states (checkout_sessions.state).
const (
	CheckoutOpen      = "open"
	CheckoutPaying    = "paying"
	CheckoutCompleted = "completed"
	CheckoutExpired   = "expired"
	CheckoutAbandoned = "abandoned"
)

// CheckoutSession binds a browser (token in the X-Arenile-Checkout header,
// spec 12.2) to the held booking of the public flow. ExpiresAt follows the
// booking hold (extended while paying, spec 10.2).
type CheckoutSession struct {
	bun.BaseModel `bun:"table:checkout_sessions,alias:cks"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	BookingID       string    `bun:"booking_id,notnull"`
	TokenHash       string    `bun:"token_hash,notnull"`
	State           string    `bun:"state,notnull"`
	ExpiresAt       time.Time `bun:"expires_at,notnull"`
	PaymentAttempts int       `bun:"payment_attempts,notnull"`
	Locale          string    `bun:"locale,notnull"`
	EmbedOrigin     *string   `bun:"embed_origin"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// MagicLinkToken is one issued link of the customer manage area (spec
// 12.4): valid until ExpiresAt (end of the booking + 30 days) unless
// revoked (regeneration revokes the previous ones).
type MagicLinkToken struct {
	bun.BaseModel `bun:"table:magic_link_tokens,alias:mlt"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	BookingID       string     `bun:"booking_id,notnull"`
	TokenHash       string     `bun:"token_hash,notnull"`
	ExpiresAt       time.Time  `bun:"expires_at,notnull"`
	RevokedAt       *time.Time `bun:"revoked_at"`
	LastUsedAt      *time.Time `bun:"last_used_at"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
}

// Payment intent kinds and statuses (payment_intents.kind, status).
const (
	IntentKindPayment = "payment"
	IntentKindRefund  = "refund"

	IntentCreated   = "created"
	IntentPending   = "pending"
	IntentSucceeded = "succeeded"
	IntentFailed    = "failed"
	IntentCancelled = "cancelled"
	IntentExpired   = "expired"
)

// PaymentIntent is a provider-side payment or refund (Stripe Checkout
// session, payment intent, refund). ExternalID is unique per provider; the
// ledger row (payments) is written only when the provider confirms, and
// PaymentID then points to it. RawEventIDs lists the provider events
// applied, oldest first.
type PaymentIntent struct {
	bun.BaseModel `bun:"table:payment_intents,alias:pi"`

	ID               string      `bun:"id,pk"`
	EstablishmentID  string      `bun:"establishment_id,notnull"`
	BookingID        *string     `bun:"booking_id"`
	PaymentID        *string     `bun:"payment_id"`
	RefundOfIntentID *string     `bun:"refund_of_intent_id"`
	Provider         string      `bun:"provider,notnull"`
	Mode             string      `bun:"mode,notnull"`
	Kind             string      `bun:"kind,notnull"`
	ExternalID       string      `bun:"external_id,notnull"`
	ExternalRef      *string     `bun:"external_ref"`
	Status           string      `bun:"status,notnull"`
	Amount           money.Cents `bun:"amount,notnull"`
	Currency         string      `bun:"currency,notnull"`
	Attempt          int         `bun:"attempt,notnull"`
	IdempotencyKey   *string     `bun:"idempotency_key"`
	RawEventIDs      []string    `bun:"raw_event_ids,type:json,notnull"`
	FailureReason    *string     `bun:"failure_reason"`
	ExpiresAt        *time.Time  `bun:"expires_at"`
	LastCheckedAt    *time.Time  `bun:"last_checked_at"`
	CreatedAt        time.Time   `bun:"created_at,notnull"`
	UpdatedAt        time.Time   `bun:"updated_at,notnull"`
}

// Webhook event statuses (webhook_events.status).
const (
	WebhookEventProcessed = "processed"
	WebhookEventIgnored   = "ignored"
	WebhookEventFailed    = "failed"
)

// WebhookEvent records a processed provider notification (spec 13.3):
// (Provider, EventID) is unique, so the same event is never applied twice.
type WebhookEvent struct {
	bun.BaseModel `bun:"table:webhook_events,alias:whe"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	Provider        string     `bun:"provider,notnull"`
	EventID         string     `bun:"event_id,notnull"`
	EventType       string     `bun:"event_type,notnull"`
	PaymentIntentID *string    `bun:"payment_intent_id"`
	Status          string     `bun:"status,notnull"`
	Error           *string    `bun:"error"`
	ReceivedAt      time.Time  `bun:"received_at,notnull"`
	ProcessedAt     *time.Time `bun:"processed_at"`
}

// NotificationTemplate overrides a built-in template for one locale (spec
// 19): Markdown body with variables such as {{booking.code}}.
type NotificationTemplate struct {
	bun.BaseModel `bun:"table:notification_templates,alias:ntt"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	Template        string    `bun:"template,notnull"`
	Locale          string    `bun:"locale,notnull"`
	Subject         string    `bun:"subject,notnull"`
	BodyMarkdown    string    `bun:"body_markdown,notnull"`
	Enabled         bool      `bun:"enabled,notnull"`
	Version         int64     `bun:"version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Message statuses (message_log.status).
const (
	MessageQueued    = "queued"
	MessageSending   = "sending"
	MessageSent      = "sent"
	MessageFailed    = "failed"
	MessageCancelled = "cancelled"
)

// MessageChannelEmail is the core delivery channel (message_log.channel).
const MessageChannelEmail = "email"

// Message is one outgoing notification and its delivery state (spec 19
// message_log). RecipientHash identifies the recipient without the address;
// RecipientEnc is its ciphertext, used only for delivery. DedupeKey makes
// enqueueing idempotent per establishment (e.g. "booking.confirmed:<event id>").
// Payload holds non-personal rendering inputs only.
type Message struct {
	bun.BaseModel `bun:"table:message_log,alias:msg"`

	ID                string         `bun:"id,pk"`
	EstablishmentID   string         `bun:"establishment_id,notnull"`
	BookingID         *string        `bun:"booking_id"`
	CustomerID        *string        `bun:"customer_id"`
	Channel           string         `bun:"channel,notnull"`
	Template          string         `bun:"template,notnull"`
	Locale            string         `bun:"locale,notnull"`
	RecipientHash     string         `bun:"recipient_hash,notnull"`
	RecipientEnc      *string        `bun:"recipient_enc"`
	DedupeKey         string         `bun:"dedupe_key,notnull"`
	Status            string         `bun:"status,notnull"`
	Attempts          int            `bun:"attempts,notnull"`
	NextAttemptAt     *time.Time     `bun:"next_attempt_at"`
	LastError         *string        `bun:"last_error"`
	ProviderMessageID *string        `bun:"provider_message_id"`
	Payload           map[string]any `bun:"payload,type:json,notnull"`
	SentAt            *time.Time     `bun:"sent_at"`
	CreatedAt         time.Time      `bun:"created_at,notnull"`
	UpdatedAt         time.Time      `bun:"updated_at,notnull"`
}
