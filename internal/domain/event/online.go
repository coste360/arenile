package event

// Event types of the public page and online payments (M4). Payloads are
// version 1 and never contain personal data or secrets:
//
//   - config.online_changed: ConfigChange with Entity online_settings,
//     terms_version, payment_settings or email_settings (secret values are
//     never part of the payload);
//   - payment.pending: PaymentChange of a provider payment started from the
//     public page or the manage area (no ledger row yet);
//   - message.failed: MessageChange, a notification that exhausted its
//     retries (manager notification "webhook/email error", spec 19).
//
// payment.succeeded / payment.failed / payment.refunded (outbox.go) are
// also written by the payment gateway for provider outcomes.
const (
	ConfigOnlineChanged = "config.online_changed"
	PaymentPending      = "payment.pending"
	MessageFailed       = "message.failed"
)

// Entity values of ConfigChange for config.online_changed.
const (
	EntityOnlineSettings  = "online_settings"
	EntityTermsVersion    = "terms_version"
	EntityPaymentSettings = "payment_settings"
	EntityEmailSettings   = "email_settings"
)

// MessageChange is the payload (version 1) of message.failed: ids and the
// template only, never the recipient.
type MessageChange struct {
	MessageID string `json:"message_id"`
	Template  string `json:"template"`
	BookingID string `json:"booking_id,omitempty"`
	Attempts  int    `json:"attempts"`
}
