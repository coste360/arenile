package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Template codes (message_log.template, notification_templates.template).
const (
	TemplateBookingConfirmed  = "booking_confirmed"
	TemplateBookingCancelled  = "booking_cancelled"
	TemplateBookingRejected   = "booking_rejected"
	TemplateApprovalPending   = "booking_approval_pending" // to the customer
	TemplateRefundIssued      = "refund_issued"
	TemplateBalanceReminder   = "balance_reminder"
	TemplateDayBeforeReminder = "day_before_reminder"
	TemplateManageLink        = "manage_link"
	TemplateTest              = "test"
	// Manager notifications (spec 19).
	TemplateManagerNewBooking     = "manager_new_booking"
	TemplateManagerApproval       = "manager_approval_requested"
	TemplateManagerPaymentFailed  = "manager_payment_failed"
	TemplateManagerDeliveryFailed = "manager_delivery_failed"
)

// MagicLinkGrace is how long a magic link stays valid after the end of the
// booking (spec 12.4).
const MagicLinkGrace = 30 * 24 * time.Hour

// Errors of the notify package.
var (
	// ErrLinkNotFound: unknown, expired or revoked magic link (404, never
	// distinguishing the cases).
	ErrLinkNotFound = fmt.Errorf("%w: link", configerr.ErrNotFound)
	// ErrNoRecipient: the booking customer has no email.
	ErrNoRecipient = fmt.Errorf("%w: il cliente non ha un indirizzo email", configerr.ErrConflict)
)

// ---- ports ------------------------------------------------------------------

// Settings returns the email settings of an establishment. Implemented by
// *online.Service.
type Settings interface {
	EmailConfigFor(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*online.EmailConfig, error)
}

// Email is one outgoing email, already rendered.
type Email struct {
	From     string // "Sender Name <address>"
	ReplyTo  string
	To       string
	Subject  string
	Text     string
	HTML     string
	Attach   []Attachment
	Headers  map[string]string
	DedupeID string // message_log id (Message-ID / idempotency)
}

// Attachment is an email attachment; Inline with ContentID for the QR.
type Attachment struct {
	Filename    string
	ContentType string
	ContentID   string
	Inline      bool
	Data        []byte
}

// Transport delivers emails (SMTP, file, log, memory).
type Transport interface {
	// Kind is "smtp", "file" or "log" (shown in the email settings).
	Kind() string
	// Send delivers e and returns the provider message id, if any.
	Send(ctx context.Context, e Email) (string, error)
}

// Deps are the dependencies of the notify service. Every field is required
// except Log and Notifiers.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Keys      *crypto.KeyRing
	Settings  Settings
	Transport Transport
	// MailFrom is ARENILE_MAIL_FROM (address of the sender).
	MailFrom string
	// BaseURL is config.EffectiveBaseURL() (manage links).
	BaseURL string
	// Notifiers are extension channels (SMS, WhatsApp) from the registry.
	Notifiers []ext.Notifier
}

// Service implements notifications, magic links, QR and .ics.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// MagicLink is a freshly issued link: Token and URL exist only here (and in
// the email); the database keeps the hash.
type MagicLink struct {
	ID        string
	Token     string
	URL       string
	ExpiresAt time.Time
}

// Enqueue is a message to queue in the caller's transaction.
type Enqueue struct {
	Template   string
	BookingID  string
	CustomerID string
	// Recipient is the email address (stored hashed + encrypted).
	Recipient string
	Locale    string
	DedupeKey string
	// Payload holds non-personal rendering inputs (ids, amounts).
	Payload map[string]any
}

// MessageFilter filters the message log.
type MessageFilter struct {
	BookingID string
	Status    string
}

// MessageView is a message_log row with the masked recipient and booking code.
type MessageView struct {
	Message         store.Message
	RecipientMasked string
	BookingCode     string
}

// ---- magic links ---------------------------------------------------------------

// IssueMagicLink creates a magic link for a booking in tx (expiry = end of
// the booking + MagicLinkGrace). createdBy is the staff user, nil for the
// system. Implements checkout.Notifier.
func (s *Service) IssueMagicLink(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, createdBy *string) (*MagicLink, error) {
	return nil, configerr.ErrNotImplemented
}

// RevokeMagicLinks revokes every active link of a booking in tx.
func (s *Service) RevokeMagicLinks(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error {
	return configerr.ErrNotImplemented
}

// ResolveMagicLink returns the active link with the token hash in es and
// marks it used; ErrLinkNotFound otherwise. Implements checkout.Notifier.
func (s *Service) ResolveMagicLink(ctx context.Context, db bun.IDB, es store.EstablishmentScope, token string) (*store.MagicLinkToken, error) {
	return nil, configerr.ErrNotImplemented
}

// RequestLink sends a new manage link when email and code match a booking
// of es; it returns nil also when they do not (no enumeration). Implements
// checkout.Notifier.
func (s *Service) RequestLink(ctx context.Context, es store.EstablishmentScope, email, code string) error {
	return configerr.ErrNotImplemented
}

// ---- messages ------------------------------------------------------------------

// Enqueue queues a message in tx (idempotent on DedupeKey). Implements
// checkout.Notifier.
func (s *Service) Enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m Enqueue) (*store.Message, error) {
	return nil, configerr.ErrNotImplemented
}

// SendTest queues a test email (auth.PermEstablishmentConfigure).
func (s *Service) SendTest(ctx context.Context, p *auth.Principal, establishmentID, to string) (*store.Message, error) {
	return nil, configerr.ErrNotImplemented
}

// ResendConfirmation queues the confirmation email again with a new magic
// link (auth.PermBookingsWrite); confirmed / checked-in bookings only.
func (s *Service) ResendConfirmation(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) (*store.Message, error) {
	return nil, configerr.ErrNotImplemented
}

// RegenerateLink revokes the links of a booking, issues a new one and
// optionally emails it (auth.PermBookingsWrite). The link is returned once.
func (s *Service) RegenerateLink(ctx context.Context, p *auth.Principal, establishmentID, bookingID string, sendEmail bool) (*MagicLink, bool, error) {
	return nil, false, configerr.ErrNotImplemented
}

// ListMessages lists the message log (auth.PermBookingsRead), newest first.
func (s *Service) ListMessages(ctx context.Context, p *auth.Principal, establishmentID string, f MessageFilter, cursor string, limit int) (store.Page[MessageView], error) {
	return store.Page[MessageView]{}, configerr.ErrNotImplemented
}

// Deliver sends the due messages (job body); it returns how many were sent.
func (s *Service) Deliver(ctx context.Context) (int, error) {
	return 0, configerr.ErrNotImplemented
}

// Jobs returns the background jobs of notify (delivery, reminders).
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     "notify.deliver",
		Interval: 30 * time.Second,
		Run: func(ctx context.Context) error {
			_, err := s.Deliver(ctx)
			if errors.Is(err, configerr.ErrNotImplemented) {
				return nil
			}
			return err
		},
	}}
}

// ---- event sink ----------------------------------------------------------------

// Sink returns the ext.EventSink that turns outbox events into messages.
func (s *Service) Sink() ext.EventSink { return sink{s: s} }

type sink struct{ s *Service }

func (sink) Name() string { return "notify" }

// Handle implements ext.EventSink (idempotent on ev.ID through DedupeKey).
func (k sink) Handle(ctx context.Context, ev ext.Event) error {
	return nil
}

// ---- QR and .ics -----------------------------------------------------------------

// QR formats.
const (
	QRSVG = "svg"
	QRPNG = "png"
)

// QRPayload is the text encoded in the QR of a booking (check-in at the desk).
func QRPayload(bookingCode string) string { return "ARENILE:" + bookingCode }

// QR renders payload as SVG or PNG and returns the content type.
// Implements checkout.Notifier.
func (s *Service) QR(payload, format string) ([]byte, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// ICS renders the .ics file of a booking of es; manageURL is included when
// not empty. Implements checkout.Notifier.
func (s *Service) ICS(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID, manageURL string) ([]byte, error) {
	return nil, configerr.ErrNotImplemented
}
