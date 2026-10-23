package checkout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// MaxPaymentHold bounds the hold extension while a payment is in progress
// (spec 10.2: pending_payment, max 30 minutes).
const MaxPaymentHold = 30 * time.Minute

// Errors of the checkout package (mapped by internal/http/problem).
var (
	// ErrSessionNotFound: missing or wrong X-Arenile-Checkout token, or a
	// token of another booking or establishment (404 checkout_not_found).
	ErrSessionNotFound = errors.New("checkout: session not found")
	// ErrDetailsMissing: pay called before customer details and terms (422).
	ErrDetailsMissing = fmt.Errorf("%w: inserisci i tuoi dati e accetta le condizioni", configerr.ErrValidation)
	// ErrTermsOutdated: the accepted terms version is not the current one (409).
	ErrTermsOutdated = fmt.Errorf("%w: le condizioni sono state aggiornate, rileggile e accettale di nuovo", configerr.ErrConflict)
	// ErrNothingToPay: no balance due (409).
	ErrNothingToPay = fmt.Errorf("%w: nessun importo da pagare", configerr.ErrConflict)
)

// Deps are the dependencies of the checkout service. Every field is
// required except Log, HoldTTL and Passes.
type Deps struct {
	DB       bun.IDB
	Clock    clock.Clock
	Log      *slog.Logger
	Settings Settings
	// Inventory, Pricer: reads of the online channel.
	Inventory Inventory
	Pricer    Pricer
	Bookings  Bookings
	Payments  Payments
	Notifier  Notifier
	Passes    SeasonPasses
	// HoldTTL is ARENILE_HOLD_TTL (countdown of the page).
	HoldTTL time.Duration
	// BaseURL is config.EffectiveBaseURL() (return URLs, manage links).
	BaseURL string
}

// Service implements the public flow and the manage area.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.HoldTTL <= 0 {
		d.HoldTTL = 10 * time.Minute
	}
	return &Service{d: d}
}

// ---- browse (browse.go) -----------------------------------------------------------

// Calendar returns the availability summary of a month for the online channel.
func (s *Service) Calendar(ctx context.Context, slug string, q CalendarQuery) (*Calendar, error) {
	return nil, configerr.ErrNotImplemented
}

// Map returns the public geometry of a map and its version (ETag).
func (s *Service) Map(ctx context.Context, slug, mapID string) (*MapView, error) {
	return nil, configerr.ErrNotImplemented
}

// Availability returns the online availability of an interval.
func (s *Service) Availability(ctx context.Context, slug string, q AvailabilityQuery) (*Availability, error) {
	return nil, configerr.ErrNotImplemented
}

// Catalog returns the online packs and extras compatible with a selection.
func (s *Service) Catalog(ctx context.Context, slug string, q CatalogQuery) (*Catalog, error) {
	return nil, configerr.ErrNotImplemented
}

// Quote prices a selection (no writes).
func (s *Service) Quote(ctx context.Context, slug string, in Selection) (*Quote, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- session (session.go) -----------------------------------------------------------

// Create creates the held booking and the checkout session; the token is
// returned once.
func (s *Service) Create(ctx context.Context, slug string, in CreateInput) (*Session, error) {
	return nil, configerr.ErrNotImplemented
}

// State returns the state of the session and its booking. It never
// confirms (INV-19).
func (s *Service) State(ctx context.Context, slug, bookingID, token string) (*State, error) {
	return nil, configerr.ErrNotImplemented
}

// UpdateCustomer saves customer details, terms acceptance, consents,
// extras and promo code of the held booking.
func (s *Service) UpdateCustomer(ctx context.Context, slug, bookingID, token string, in CustomerInput) (*State, error) {
	return nil, configerr.ErrNotImplemented
}

// Pay starts the provider payment or confirms without online payment.
func (s *Service) Pay(ctx context.Context, slug, bookingID, token string, in PayInput) (*PayOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// Abandon releases the hold of the session's booking.
func (s *Service) Abandon(ctx context.Context, slug, bookingID, token string) error {
	return configerr.ErrNotImplemented
}

// Document returns the QR (DocQRSVG / DocQRPNG) or the .ics (DocICS) of a
// confirmed booking of the session.
func (s *Service) Document(ctx context.Context, slug, bookingID, token, kind string) (*Document, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- manage area (manage.go) ----------------------------------------------------------

// Manage returns the booking of a magic link.
func (s *Service) Manage(ctx context.Context, slug, token string) (*Managed, error) {
	return nil, configerr.ErrNotImplemented
}

// ManageDocument returns the QR or .ics of the booking of a magic link.
func (s *Service) ManageDocument(ctx context.Context, slug, token, kind string) (*Document, error) {
	return nil, configerr.ErrNotImplemented
}

// ManagePay starts the payment of the balance.
func (s *Service) ManagePay(ctx context.Context, slug, token, idempotencyKey string) (*PayOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// ManageModify previews or applies a modification within the policy.
func (s *Service) ManageModify(ctx context.Context, slug, token string, expected *int64, in ModifyInput) (*ModifyOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// ManageCancel previews or applies a cancellation with refund or credit.
func (s *Service) ManageCancel(ctx context.Context, slug, token string, expected *int64, in CancelInput) (*CancelOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// ManageRelease previews or applies the release of season pass days.
func (s *Service) ManageRelease(ctx context.Context, slug, token string, in ReleaseInput) (*ReleaseOutcome, error) {
	return nil, configerr.ErrNotImplemented
}

// RequestLink sends a new manage link when email and code match; it
// returns nil also when they do not (no enumeration). ip feeds the rate
// limiter (never logged in clear with the email).
func (s *Service) RequestLink(ctx context.Context, slug, email, code, ip string) error {
	return configerr.ErrNotImplemented
}
