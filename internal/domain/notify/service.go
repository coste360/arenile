package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Template codes (message_log.template, notification_templates.template).
const (
	TemplateBookingConfirmed  = "booking_confirmed"
	TemplateBookingModified   = "booking_modified"
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

// Templates lists the templates shipped with built-in texts in every locale.
var Templates = []string{
	TemplateBookingConfirmed, TemplateBookingModified, TemplateBookingCancelled, TemplateBookingRejected,
	TemplateApprovalPending, TemplateRefundIssued, TemplateManageLink, TemplateBalanceReminder, TemplateDayBeforeReminder, TemplateTest,
	TemplateManagerNewBooking, TemplateManagerApproval, TemplateManagerPaymentFailed, TemplateManagerDeliveryFailed,
}

// managerTemplates go to the manager address in the establishment locale.
var managerTemplates = map[string]bool{
	TemplateManagerNewBooking: true, TemplateManagerApproval: true,
	TemplateManagerPaymentFailed: true, TemplateManagerDeliveryFailed: true, TemplateTest: true,
}

// Booking statuses read by notify (values of internal/domain/booking; not
// imported to keep notify independent of the booking service).
const (
	statusConfirmed        = "confirmed"
	statusAwaitingApproval = "awaiting_approval"
	statusCheckedIn        = "checked_in"
	statusCompleted        = "completed"
	statusCancelled        = "cancelled"
	statusRejected         = "rejected"
	statusExpired          = "expired"
	statusHeld             = "held"
	statusPendingPayment   = "pending_payment"
	statusDraft            = "draft"
)

// MessageChannel is the core channel code.
const MessageChannel = store.MessageChannelEmail

// headerTemplate is the email header naming the template (diagnostics).
const headerTemplate = "X-Arenile-Template"

// placeholderTag is customer.TagOnlinePlaceholder (the placeholder customer
// of held online bookings never receives messages).
const placeholderTag = "_online_placeholder"

// MagicLinkGrace is how long a magic link stays valid after the end of the
// booking (spec 12.4).
const MagicLinkGrace = 30 * 24 * time.Hour

// Delivery tuning (overridable in Deps for tests).
const (
	DefaultMaxAttempts = 8
	DefaultBatchSize   = 50
	// staleSending: a "sending" row older than this is reclaimed (the worker died).
	staleSending = 10 * time.Minute
	backoffBase  = time.Minute
	backoffCap   = 6 * time.Hour
)

// Errors of the notify package.
var (
	// ErrLinkNotFound: unknown, expired or revoked magic link (404, never
	// distinguishing the cases).
	ErrLinkNotFound = fmt.Errorf("%w: link", configerr.ErrNotFound)
	// ErrNoRecipient: the booking customer has no email.
	ErrNoRecipient = fmt.Errorf("%w: il cliente non ha un indirizzo email", configerr.ErrConflict)
	// ErrBookingNotConfirmed: resend of a confirmation for a booking that
	// is not confirmed.
	ErrBookingNotConfirmed = fmt.Errorf("%w: la prenotazione non è confermata", configerr.ErrConflict)
	// ErrBookingNotFound: unknown booking in the establishment.
	ErrBookingNotFound = fmt.Errorf("%w: booking", configerr.ErrNotFound)
	// ErrEstablishmentNotFound: unknown establishment or out of scope.
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrUnknownTemplate: no template with that code.
	ErrUnknownTemplate = errors.New("notify: unknown template")
	// errInvalidRecipient: malformed email address.
	errInvalidRecipient = fmt.Errorf("%w: recipient", configerr.ErrValidation)
	// errSkip: the message must not be sent (anonymized customer, booking
	// gone); the delivery cancels it instead of retrying.
	errSkip = errors.New("notify: message no longer applicable")
)

// ---- ports ------------------------------------------------------------------

// Settings returns the email settings of an establishment. Implemented by
// *online.Service.
type Settings interface {
	EmailConfigFor(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*online.EmailConfig, error)
}

// Email is one outgoing email, already rendered.
type Email struct {
	From      string // "Sender Name <address>"
	ReplyTo   string
	To        string
	Subject   string
	Text      string
	HTML      string
	Attach    []Attachment
	Headers   map[string]string
	DedupeID  string // message_log id (idempotency key of the transport)
	MessageID string // RFC 5322 Message-ID without angle brackets
	Date      time.Time
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
	// Send delivers e and returns the provider message id, if any. Errors
	// wrapping ErrPermanent are not retried.
	Send(ctx context.Context, e Email) (string, error)
}

// Deps are the dependencies of the notify service. Every field is required
// except Log, Notifiers, MaxAttempts and BatchSize.
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
	// Notifiers are extension channels (SMS, WhatsApp) from the registry; an
	// extension of channel "email" replaces Transport.
	Notifiers []ext.Notifier
	// MaxAttempts before the dead letter state (default DefaultMaxAttempts).
	MaxAttempts int
	// BatchSize of one delivery run (default DefaultBatchSize).
	BatchSize int
	// ReminderBatch is the page size of the reminder candidates (tests).
	ReminderBatch int
}

// Service implements notifications, magic links, QR and .ics.
type Service struct {
	d       Deps
	limiter *auth.RateLimiter
	// wake signals the delivery job that messages were queued (buffer 1:
	// signals coalesce), so a confirmation email leaves within the second
	// instead of at the next 30 s tick.
	wake chan struct{}
}

// kick wakes the delivery job without blocking.
func (s *Service) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.MaxAttempts <= 0 {
		d.MaxAttempts = DefaultMaxAttempts
	}
	if d.BatchSize <= 0 {
		d.BatchSize = DefaultBatchSize
	}
	for _, n := range d.Notifiers {
		if n != nil && n.Channel() == MessageChannel {
			d.Transport = notifierTransport{n: n}
		}
	}
	return &Service{
		d:    d,
		wake: make(chan struct{}, 1),
		// "Send me a new link" (spec 12.4): 3 requests per booking per hour.
		limiter: auth.NewRateLimiter(d.Clock, 3, time.Hour),
	}
}

// TransportKind is the kind of the active transport.
func (s *Service) TransportKind() string { return s.d.Transport.Kind() }

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

// ---- messages ------------------------------------------------------------------

// Enqueue queues a message in tx (idempotent on DedupeKey: the existing row
// is returned). Implements checkout.Notifier.
func (s *Service) Enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m Enqueue) (*store.Message, error) {
	msg, _, err := s.enqueue(ctx, tx, es, m)
	return msg, err
}

// enqueue is Enqueue that also reports whether a new row was inserted
// (false: the dedupe key already existed).
func (s *Service) enqueue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, m Enqueue) (*store.Message, bool, error) {
	addr, err := normalizeRecipient(m.Recipient)
	if err != nil {
		return nil, false, err
	}
	if m.Template == "" || m.DedupeKey == "" {
		return nil, false, fmt.Errorf("notify: enqueue: template and dedupe key are required")
	}
	now := s.now()
	row := &store.Message{
		ID: store.NewID(), Channel: MessageChannel, Template: m.Template,
		Locale: pickLocale(m.Locale), RecipientHash: RecipientHash(es.ID(), addr),
		DedupeKey: m.DedupeKey, Status: store.MessageQueued, NextAttemptAt: &now,
		Payload: m.Payload, CreatedAt: now, UpdatedAt: now,
	}
	enc, err := s.d.Keys.Encrypt([]byte(addr), recipientAAD(es.ID()))
	if err != nil {
		return nil, false, fmt.Errorf("notify: encrypt recipient: %w", err)
	}
	row.RecipientEnc = &enc
	if m.BookingID != "" {
		row.BookingID = &m.BookingID
	}
	if m.CustomerID != "" {
		row.CustomerID = &m.CustomerID
	}
	stored, created, err := store.InsertMessage(ctx, tx, es, row)
	if err != nil {
		return nil, false, fmt.Errorf("notify: enqueue: %w", err)
	}
	return stored, created, nil
}

// SendTest queues a test email (auth.PermEstablishmentConfigure).
func (s *Service) SendTest(ctx context.Context, p *auth.Principal, establishmentID, to string) (*store.Message, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	addr, err := normalizeRecipient(to)
	if err != nil {
		return nil, configerr.Invalid("to", "Indirizzo email non valido.")
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}
	var msg *store.Message
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		msg, err = s.Enqueue(ctx, tx, es, Enqueue{
			Template: TemplateTest, Recipient: addr, Locale: est.DefaultLocale, DedupeKey: "test:" + store.NewID(),
		})
		if err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "notify.test_email_queued",
			EntityType: "message", EntityID: msg.ID, Diff: map[string]any{"recipient": MaskEmail(addr)},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("notify: send test: %w", err)
	}
	s.kick()
	return msg, nil
}

// ResendConfirmation queues the confirmation email again with a new magic
// link (auth.PermBookingsWrite); confirmed / checked-in bookings only.
func (s *Service) ResendConfirmation(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) (*store.Message, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var msg *store.Message
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		b, cu, err := s.bookingRecipient(ctx, tx, es, bookingID)
		if err != nil {
			return err
		}
		if b.Status != statusConfirmed && b.Status != statusCheckedIn {
			return ErrBookingNotConfirmed
		}
		if cu == nil {
			return ErrNoRecipient
		}
		msg, err = s.Enqueue(ctx, tx, es, Enqueue{
			Template: TemplateBookingConfirmed, BookingID: b.ID, CustomerID: cu.ID, Recipient: *cu.Email,
			Locale: bookingLocale(b, cu), DedupeKey: "resend:" + store.NewID(),
		})
		if err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "booking.confirmation_resent",
			EntityType: "booking", EntityID: b.ID, Diff: map[string]any{"message_id": msg.ID},
		})
	})
	if err != nil {
		return nil, err
	}
	s.kick()
	return msg, nil
}

// RegenerateLink revokes the links of a booking, issues a new one and
// optionally emails it (auth.PermBookingsWrite). The link is returned once;
// the email carries its own link and keeps this one valid.
func (s *Service) RegenerateLink(ctx context.Context, p *auth.Principal, establishmentID, bookingID string, sendEmail bool) (*MagicLink, bool, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, false, err
	}
	var link *MagicLink
	sent := false
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		b, cu, err := s.bookingRecipient(ctx, tx, es, bookingID)
		if err != nil {
			return err
		}
		if _, err := store.RevokeMagicLinks(ctx, tx, es, b.ID, s.now()); err != nil {
			return err
		}
		var createdBy *string
		if p.Kind == auth.PrincipalUser && p.UserID != "" {
			uid := p.UserID
			createdBy = &uid
		}
		if link, err = s.IssueMagicLink(ctx, tx, es, b.ID, createdBy); err != nil {
			return err
		}
		if sendEmail && cu != nil {
			if _, err := s.Enqueue(ctx, tx, es, Enqueue{
				Template: TemplateManageLink, BookingID: b.ID, CustomerID: cu.ID, Recipient: *cu.Email,
				Locale: bookingLocale(b, cu), DedupeKey: "regenerate:" + link.ID,
				Payload: map[string]any{"keep_link_id": link.ID},
			}); err != nil {
				return err
			}
			sent = true
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "booking.magic_link_regenerated",
			EntityType: "booking", EntityID: b.ID, Diff: map[string]any{"link_id": link.ID, "emailed": sent},
		})
	})
	if err != nil {
		return nil, false, err
	}
	if sent {
		s.kick()
	}
	return link, sent, nil
}

// ListMessages lists the message log (auth.PermBookingsRead), newest first.
func (s *Service) ListMessages(ctx context.Context, p *auth.Principal, establishmentID string, f MessageFilter, cursor string, limit int) (store.Page[MessageView], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[MessageView]{}, err
	}
	page, err := store.ListMessages(ctx, s.d.DB, es, store.MessageFilter{BookingID: f.BookingID, Status: f.Status}, cursor, limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return store.Page[MessageView]{}, configerr.Invalid("cursor", "Cursore non valido.")
		}
		return store.Page[MessageView]{}, fmt.Errorf("notify: list messages: %w", err)
	}
	var ids []string
	for _, m := range page.Items {
		if m.BookingID != nil {
			ids = append(ids, *m.BookingID)
		}
	}
	codes, err := store.CustomerBookingCodes(ctx, s.d.DB, es, ids)
	if err != nil {
		return store.Page[MessageView]{}, fmt.Errorf("notify: list messages: %w", err)
	}
	out := store.Page[MessageView]{Items: make([]MessageView, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, m := range page.Items {
		v := MessageView{Message: m, RecipientMasked: "***"}
		if addr, err := s.recipient(&m); err == nil {
			v.RecipientMasked = MaskEmail(addr)
		}
		if m.BookingID != nil {
			v.BookingCode = codes[*m.BookingID]
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// ---- helpers -------------------------------------------------------------------

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

// bookingRecipient loads a booking and its customer; the customer is nil
// when it must not receive messages (placeholder, anonymized, no email).
func (s *Service) bookingRecipient(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*store.Booking, *store.Customer, error) {
	b, err := store.GetBooking(ctx, db, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("notify: %w", err)
	}
	cu, err := store.GetCustomer(ctx, db, es, b.CustomerID)
	if err != nil {
		return nil, nil, fmt.Errorf("notify: %w", err)
	}
	if !canReceive(cu) {
		return b, nil, nil
	}
	return b, cu, nil
}

// canReceive: GDPR (spec 23) and the online placeholder customer.
func canReceive(cu *store.Customer) bool {
	if cu == nil || cu.AnonymizedAt != nil || cu.Email == nil || strings.TrimSpace(*cu.Email) == "" {
		return false
	}
	for _, t := range cu.Tags {
		if t == placeholderTag {
			return false
		}
	}
	return true
}

func bookingLocale(b *store.Booking, cu *store.Customer) string {
	var bl string
	if b.Locale != nil {
		bl = *b.Locale
	}
	return pickLocale(bl, cu.Locale)
}

// normalizeRecipient validates and normalises an address (bare address only).
func normalizeRecipient(addr string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(addr))
	if err != nil || a.Name != "" || !strings.Contains(a.Address, "@") {
		return "", errInvalidRecipient
	}
	return store.NormalizeEmail(a.Address), nil
}

// RecipientHash is message_log.recipient_hash: SHA-256 hex of the
// establishment id and the normalised address (lookups per establishment
// without storing the address; the salt prevents cross-tenant joins).
func RecipientHash(establishmentID, normalizedAddr string) string {
	sum := sha256.Sum256([]byte(establishmentID + ":" + normalizedAddr))
	return hex.EncodeToString(sum[:])
}

func recipientAAD(establishmentID string) []byte {
	return []byte("message_log:" + establishmentID)
}

// recipient decrypts the recipient of a message.
func (s *Service) recipient(m *store.Message) (string, error) {
	if m.RecipientEnc == nil {
		return "", errSkip
	}
	b, err := s.d.Keys.Decrypt(*m.RecipientEnc, recipientAAD(m.EstablishmentID))
	if err != nil {
		return "", fmt.Errorf("notify: decrypt recipient: %w", err)
	}
	return string(b), nil
}

// emailConfig returns the email settings of es, with built-in defaults when
// the settings service has none yet.
func (s *Service) emailConfig(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (online.EmailConfig, error) {
	if s.d.Settings != nil {
		cfg, err := s.d.Settings.EmailConfigFor(ctx, db, es)
		switch {
		case err == nil && cfg != nil:
			return *cfg, nil
		case err != nil && !errors.Is(err, configerr.ErrNotImplemented) && !errors.Is(err, store.ErrNotFound):
			return online.EmailConfig{}, fmt.Errorf("notify: email settings: %w", err)
		}
	}
	return DefaultEmailConfig(), nil
}

// DefaultEmailConfig enables every customer notification shipped in M4
// and the manager notifications about online bookings and failures.
func DefaultEmailConfig() online.EmailConfig {
	return online.EmailConfig{
		ToCustomer: online.CustomerNotifications{Confirmation: true, Cancellation: true, Refund: true},
		ToManager:  online.ManagerNotifications{NewOnlineBooking: true, ApprovalRequested: true, PaymentFailed: true, WebhookError: true},
	}
}
