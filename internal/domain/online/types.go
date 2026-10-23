package online

import (
	"time"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Config is the online flow configuration stored as JSON in
// online_settings.config (api.OnlineFlowConfig).
type Config struct {
	// Formulas offered online; empty = every formula allowed by online packs.
	Formulas []string `json:"formulas,omitempty"`
	// Slots offered online (calendar.Slots values); empty = FULL, AM, PM.
	Slots []string `json:"slots,omitempty"`
	// ShowRatePeriods highlights rate periods in the public calendar.
	ShowRatePeriods bool `json:"show_rate_periods"`
	// MaxDaysAhead limits how far ahead the page sells (0 = policy only).
	MaxDaysAhead      int  `json:"max_days_ahead,omitempty"`
	PromoCodesEnabled bool `json:"promo_codes_enabled"`
	RequirePhone      bool `json:"require_phone"`
}

// ManagerNotifications toggles the notifications sent to the manager (spec 19).
type ManagerNotifications struct {
	NewOnlineBooking  bool `json:"new_online_booking"`
	ApprovalRequested bool `json:"approval_requested"`
	PaymentFailed     bool `json:"payment_failed"`
	WebhookError      bool `json:"webhook_error"`
}

// CustomerNotifications toggles the notifications sent to customers (spec 19).
type CustomerNotifications struct {
	Confirmation      bool `json:"confirmation"`
	BalanceReminder   bool `json:"balance_reminder"`
	DayBeforeReminder bool `json:"day_before_reminder"`
	Cancellation      bool `json:"cancellation"`
	Refund            bool `json:"refund"`
}

// EmailConfig is stored as JSON in online_settings.email_config
// (api.EmailSettingsInput). The transport itself comes from the server
// configuration (ARENILE_SMTP_URL, ARENILE_MAIL_FROM, ARENILE_MAIL_DIR).
type EmailConfig struct {
	SenderName string                `json:"sender_name"`
	ReplyTo    string                `json:"reply_to,omitempty"`
	Manager    string                `json:"manager_email,omitempty"`
	ToManager  ManagerNotifications  `json:"manager_notifications"`
	ToCustomer CustomerNotifications `json:"customer_notifications"`
}

// Transport describes the email transport chosen by the server
// configuration (read-only in the API).
type Transport struct {
	// Kind is "smtp", "log" or "file".
	Kind           string
	SMTPConfigured bool
	MailFrom       string
}

// Settings is the view of GET online-settings (api.OnlineSettings).
type Settings struct {
	EstablishmentID     string
	Slug                string
	BookingEnabled      bool
	AllowedEmbedOrigins []string
	Theme               store.Theme
	Flow                Config
	CurrentTerms        *store.TermsVersion
	// PageURL is <ARENILE_BASE_URL>/b/<slug>; the snippets are built from it.
	PageURL string
	Version int64
}

// Input is PUT online-settings (api.OnlineSettingsInput).
type Input struct {
	BookingEnabled      bool
	AllowedEmbedOrigins []string
	Theme               store.Theme
	Flow                Config
}

// TermsInput publishes a new terms version (api.TermsVersionInput).
type TermsInput struct {
	Version     string
	Texts       map[string]store.TermsTexts
	MakeCurrent bool
}

// ProviderConfig is the decrypted configuration of a payment provider for
// one establishment, for the payment gateway and the checkout only. Never
// serialise it.
type ProviderConfig struct {
	EstablishmentID string
	// Code is the ext.PaymentProvider code ("manual" when nothing online is active).
	Code string
	// Mode is store.ProviderModeTest or store.ProviderModeLive.
	Mode        string
	Credentials ext.Credentials
	// Embedded enables the in-page form (Stripe Payment Element) instead of redirect.
	Embedded       bool
	PublishableKey string
	// Online reports whether the provider takes payments online.
	Online bool
}

// StripeView is the non-secret state of the Stripe credentials (api.StripeSettings).
type StripeView struct {
	Configured              bool
	PublishableKey          string
	SecretKeyLast4          string
	WebhookSecretConfigured bool
	EmbeddedMode            bool
	VerifiedAt              *time.Time
}

// PaymentSettings is GET payment-settings (api.PaymentSettings).
type PaymentSettings struct {
	Provider           string
	AvailableProviders []string
	Mode               string
	Stripe             StripeView
	// WebhookURL is <base>/webhooks/payments/<provider>/<establishment_id>.
	WebhookURL string
	Version    int64
}

// StripeInput carries the Stripe fields of an update: nil = unchanged,
// pointer to "" = clear (api.StripeSettingsInput).
type StripeInput struct {
	PublishableKey *string
	SecretKey      *string
	WebhookSecret  *string
	EmbeddedMode   *bool
}

// PaymentSettingsInput is PUT payment-settings (api.PaymentSettingsInput).
type PaymentSettingsInput struct {
	Provider string
	Mode     string
	Stripe   *StripeInput
}

// TestResult is the outcome of a credentials check (api.PaymentSettingsTestResult).
type TestResult struct {
	OK           bool
	Provider     string
	Mode         string
	Message      string
	AccountLabel string
}

// EmailSettings is GET email-settings (api.EmailSettings).
type EmailSettings struct {
	Transport Transport
	Config    EmailConfig
	Version   int64
}

// PublicEstablishment is everything the public flow needs to serve a slug:
// the scope, the establishment row, the flow configuration, the current
// terms and the payment configuration (without credentials).
type PublicEstablishment struct {
	Scope         store.EstablishmentScope
	Establishment *store.Establishment
	Flow          Config
	Terms         *store.TermsVersion
	// Provider is the active provider code, Online whether it takes online
	// payments, Embedded/PublishableKey for the in-page form, TestMode for a
	// banner on the page.
	Provider       string
	Online         bool
	Embedded       bool
	PublishableKey string
	TestMode       bool
}
