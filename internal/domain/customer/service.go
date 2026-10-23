package customer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Sentinel errors of the customer domain.
var (
	// ErrNotFound: unknown customer or outside the caller's establishment.
	ErrNotFound = fmt.Errorf("%w: customer", configerr.ErrNotFound)
	// ErrAnonymized: the customer was anonymized and is read-only.
	ErrAnonymized = fmt.Errorf("%w: customer anonymized", configerr.ErrConflict)
)

// Deps are the dependencies of the customer service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the customer use cases.
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

// WithDB returns a copy of the service bound to db (composition in a caller
// transaction: the service's own transactions become savepoints).
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// Input is the customer data of create, update and inline creation from a
// booking (api.CustomerInput).
type Input struct {
	FirstName        string
	LastName         string
	Email            string
	Phone            string
	Locale           string
	Notes            string
	Tags             []string
	AgreementID      string
	MarketingConsent *store.MarketingConsent
}

// ---- ports used by other domain packages -----------------------------------

// Resolve returns the customer to attach to a booking inside tx: the
// existing customer customerID (in es, not anonymized) or, when in is not
// nil, a new customer created in tx (audit + customer.created). Exactly one
// of customerID and in must be set. Implements booking.Customers.
func (s *Service) Resolve(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, in *Input) (*store.Customer, error) {
	return nil, fmt.Errorf("customer: resolve: %w", configerr.ErrNotImplemented)
}

// GrantCredit appends a movement to the customer credit ledger inside tx
// (positive = credit granted, negative = credit used). Implements
// payment.Credits and pass.Credits.
func (s *Service) GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error {
	return fmt.Errorf("customer: grant credit: %w", configerr.ErrNotImplemented)
}

// CreditBalance is the sum of the non-expired credit movements of a
// customer on date today. Implements payment.Credits.
func (s *Service) CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	return 0, fmt.Errorf("customer: credit balance: %w", configerr.ErrNotImplemented)
}

// ---- helpers ------------------------------------------------------------------

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// notFound maps store.ErrNotFound to ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// inTx runs fn in a transaction of the service database.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return store.RunInTx(ctx, s.d.DB, fn)
}

// The helpers above are for the use cases still to be implemented (owner:
// customers builder); drop this block once they are used.
var (
	_ = (*Service).authorize
	_ = (*Service).now
	_ = notFound
	_ = (*Service).inTx
)
