package online

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the online package (mapped by internal/http/problem through the
// configerr kinds).
var (
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrBookingDisabled: the establishment exists but online booking is off
	// (the public API answers 404 for every flow endpoint except the profile).
	ErrBookingDisabled = fmt.Errorf("%w: online booking disabled", configerr.ErrNotFound)
	// ErrTermsVersionExists: a terms version with the same label exists.
	ErrTermsVersionExists = fmt.Errorf("%w: versione delle condizioni già pubblicata", configerr.ErrConflict)
	// ErrUnknownProvider: the provider code is not registered.
	ErrUnknownProvider = fmt.Errorf("%w: provider di pagamento sconosciuto", configerr.ErrValidation)
)

// Providers lists the registered payment provider codes. Implemented by
// *app.Registry.
type Providers interface {
	PaymentProviderCodes() []string
}

// Deps are the dependencies of the online service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Keys encrypts and decrypts payment provider credentials.
	Keys      *crypto.KeyRing
	Providers Providers
	// BaseURL is config.EffectiveBaseURL() (page URL, embed snippets,
	// webhook URL shown to the manager).
	BaseURL string
	// Transport is the email transport chosen by the server configuration.
	Transport Transport
}

// Service implements the online settings use cases and the read ports used
// by checkout, the payment gateway and notify.
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

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// ---- admin use cases (internal/http/admin/online.go, paymentsettings.go, email.go)

// Get returns the online settings (auth.PermBookingsRead).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID string) (*Settings, error) {
	return nil, configerr.ErrNotImplemented
}

// Update replaces the online settings with optimistic concurrency
// (expected = If-Match version, nil = any) (auth.PermEstablishmentConfigure).
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in Input) (*Settings, error) {
	return nil, configerr.ErrNotImplemented
}

// ListTerms lists the terms versions, newest first (auth.PermBookingsRead).
func (s *Service) ListTerms(ctx context.Context, p *auth.Principal, establishmentID string) ([]store.TermsVersion, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// PublishTerms publishes an immutable terms version, optionally making it
// current (auth.PermEstablishmentConfigure).
func (s *Service) PublishTerms(ctx context.Context, p *auth.Principal, establishmentID string, in TermsInput) (*store.TermsVersion, error) {
	return nil, configerr.ErrNotImplemented
}

// GetPayments returns the payment settings without secrets (auth.PermPaymentsConfigure).
func (s *Service) GetPayments(ctx context.Context, p *auth.Principal, establishmentID string) (*PaymentSettings, error) {
	return nil, configerr.ErrNotImplemented
}

// UpdatePayments stores the provider choice and the encrypted credentials
// (auth.PermPaymentsConfigure).
func (s *Service) UpdatePayments(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in PaymentSettingsInput) (*PaymentSettings, error) {
	return nil, configerr.ErrNotImplemented
}

// TestPayments checks the stored credentials with the provider
// (auth.PermPaymentsConfigure). Drivers that cannot check credentials
// answer OK=false with a message.
func (s *Service) TestPayments(ctx context.Context, p *auth.Principal, establishmentID string) (*TestResult, error) {
	return nil, configerr.ErrNotImplemented
}

// GetEmail returns the email settings (auth.PermBookingsRead).
func (s *Service) GetEmail(ctx context.Context, p *auth.Principal, establishmentID string) (*EmailSettings, error) {
	return nil, configerr.ErrNotImplemented
}

// UpdateEmail replaces the email settings (auth.PermEstablishmentConfigure).
func (s *Service) UpdateEmail(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, in EmailConfig) (*EmailSettings, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- read ports (no principal: public flows and jobs) --------------------

// ResolvePublic resolves a slug for the public flow: ErrEstablishmentNotFound
// for unknown or archived establishments, ErrBookingDisabled when online
// booking is off. Implements checkout.Settings.
func (s *Service) ResolvePublic(ctx context.Context, slug string) (*PublicEstablishment, error) {
	return nil, configerr.ErrNotImplemented
}

// ActiveProvider returns the decrypted configuration of the active payment
// provider of es; the "manual" configuration when none is active.
// Implements gateway.ProviderSettings and checkout.Settings.
func (s *Service) ActiveProvider(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*ProviderConfig, error) {
	return nil, configerr.ErrNotImplemented
}

// ProviderConfig returns the decrypted configuration of provider for es,
// active or not (webhooks of a provider just switched off must still
// verify). store.ErrNotFound when the provider was never configured.
// Implements gateway.ProviderSettings.
func (s *Service) ProviderConfig(ctx context.Context, db bun.IDB, es store.EstablishmentScope, provider string) (*ProviderConfig, error) {
	return nil, configerr.ErrNotImplemented
}

// CurrentTerms returns the current terms version of es (nil, nil when the
// establishment has none). Implements checkout.Settings.
func (s *Service) CurrentTerms(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*store.TermsVersion, error) {
	return nil, configerr.ErrNotImplemented
}

// EmailConfigFor returns the email settings of es with defaults applied.
// Implements notify.Settings.
func (s *Service) EmailConfigFor(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*EmailConfig, error) {
	return nil, configerr.ErrNotImplemented
}
