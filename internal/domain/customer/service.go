package customer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	ErrAnonymized = fmt.Errorf("%w: cliente anonimizzato, non modificabile", configerr.ErrConflict)
	// ErrEstablishmentNotFound: unknown establishment or outside the scope.
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
)

// DuplicateError is returned by Create when a customer with the same email
// or phone already exists (dedupe hint, spec 6.5): the client shows the
// existing customer and may repeat the request with force. It is a
// configerr.ErrConflict (HTTP 409). Error() carries only the id of the
// existing customer, never its data.
type DuplicateError struct {
	ExistingID string
	// Field is "email" or "phone".
	Field string
}

func (e *DuplicateError) Error() string {
	what := "lo stesso telefono"
	if e.Field == "email" {
		what = "la stessa email"
	}
	return fmt.Sprintf("%s: esiste già un cliente con %s (existing_customer_id=%s)", configerr.ErrConflict, what, e.ExistingID)
}

// Is makes errors.Is(err, configerr.ErrConflict) true.
func (e *DuplicateError) Is(target error) bool { return target == configerr.ErrConflict }

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
	// ledger, when set (UseCreditLedger, M5B), owns GrantCredit and
	// CreditBalance (credit_ledger.go).
	ledger CreditLedger
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil && d.Clock != nil {
		d.Outbox = event.NewOutbox(d.Clock)
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
// booking (api.CustomerInput). Empty strings mean "not set".
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
// of customerID and in must be set. Inline creation skips the dedupe hint:
// the booking flow must not be interrupted (the staff merges later).
// Implements booking.Customers.
func (s *Service) Resolve(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, in *Input) (*store.Customer, error) {
	switch {
	case customerID != "" && in != nil:
		return nil, configerr.Invalid("customer", "Indica un cliente esistente oppure i dati di un nuovo cliente, non entrambi.")
	case customerID != "":
		c, err := store.GetCustomer(ctx, tx, es, customerID)
		if err != nil {
			return nil, notFound(err)
		}
		if c.AnonymizedAt != nil {
			return nil, ErrAnonymized
		}
		return c, nil
	case in != nil:
		if !p.Can(es, auth.PermBookingsWrite) {
			return nil, auth.ErrForbidden
		}
		est, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return nil, fmt.Errorf("customer: resolve: %w", err)
		}
		now := s.now()
		f, err := validate(in, est.DefaultLocale, now)
		if err != nil {
			return nil, prefixFields(err, "customer.")
		}
		if err := s.checkAgreement(ctx, tx, es, f.AgreementID); err != nil {
			return nil, prefixFields(err, "customer.")
		}
		return s.insert(ctx, tx, p, "", es, &f, now)
	}
	return nil, configerr.Invalid("customer_id", "Il cliente è obbligatorio.")
}

// GrantCredit appends a movement to the customer credit ledger inside tx
// (positive = credit granted, negative = credit used). ID and CreatedAt are
// filled when empty. The caller audits the operation that caused it.
// Implements payment.Credits and pass.Credits.
func (s *Service) GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error {
	if s.ledger != nil {
		return s.ledger.GrantCredit(ctx, tx, es, c)
	}
	if c == nil || c.Amount == 0 {
		return configerr.Invalid("amount", "L'importo del credito non può essere zero.")
	}
	switch c.Source {
	case store.CreditSourceCancellation, store.CreditSourceSeasonRelease, store.CreditSourceWeather,
		store.CreditSourceManual, store.CreditSourceRedemption:
	default:
		return configerr.Invalid("source", "Origine del credito non valida.")
	}
	if _, err := store.GetCustomer(ctx, tx, es, c.CustomerID); err != nil {
		return notFound(err)
	}
	if c.ID == "" {
		c.ID = store.NewID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = s.now()
	}
	if err := store.InsertCustomerCredit(ctx, tx, es, c); err != nil {
		return fmt.Errorf("customer: grant credit: %w", err)
	}
	return nil
}

// CreditBalance is the sum of the non-expired credit movements of a
// customer on date today. Implements payment.Credits.
func (s *Service) CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	if s.ledger != nil {
		return s.ledger.CreditBalance(ctx, db, es, customerID, today)
	}
	sum, err := store.SumCustomerCredits(ctx, db, es, customerID, today)
	if err != nil {
		return 0, fmt.Errorf("customer: credit balance: %w", err)
	}
	return sum, nil
}

// ---- helpers ------------------------------------------------------------------

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// today is the current date in the establishment timezone.
func (s *Service) today(est *store.Establishment) calendar.Date {
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return calendar.In(s.d.Clock.Now(), loc)
}

// notFound maps store.ErrNotFound to ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("customer: %w", err)
}

// inTx runs fn in a transaction of the service database.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return store.RunInTx(ctx, s.d.DB, fn)
}

// checkAgreement verifies that the agreement (if any) belongs to es.
func (s *Service) checkAgreement(ctx context.Context, db bun.IDB, es store.EstablishmentScope, agreementID *string) error {
	if agreementID == nil {
		return nil
	}
	ok, err := store.CustomerAgreementExists(ctx, db, es, *agreementID)
	if err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	if !ok {
		return configerr.Invalid("agreement_id", "Convenzione inesistente in questo stabilimento.")
	}
	return nil
}

// record writes the audit entry and the outbox event of a customer change
// in tx. Diff holds field names and flags only, never personal data.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, action, eventType string, c *store.Customer, diff any) error {
	err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: action,
		EntityType: "customer", EntityID: c.ID, Diff: diff,
	})
	if err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	if eventType == "" {
		return nil
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, event.CustomerChange{CustomerID: c.ID, Version: c.Version}); err != nil {
		return fmt.Errorf("customer: %w", err)
	}
	return nil
}

// prefixFields prefixes the field paths of a validation error (inline
// customer of a booking: "customer.email").
func prefixFields(err error, prefix string) error {
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := &configerr.ValidationError{}
	for _, f := range ve.Fields {
		out.Add(prefix+strings.TrimPrefix(f.Field, prefix), f.Message)
	}
	return out
}
