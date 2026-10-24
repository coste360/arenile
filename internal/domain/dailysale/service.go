package dailysale

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
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// WalkInLastName is the name of the walk-in customer, the owner of the
// bookings of quick sales without a customer (the map label comes from the
// ticket: holder name or number).
const WalkInLastName = "Cliente al banco"

// WalkInTag tags the walk-in customer.
const WalkInTag = store.WalkInCustomerTag

// Deps are the dependencies of the daily sale service; every field is
// required except Log.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Log       *slog.Logger
	Bookings  Bookings
	Payments  Payments
	Tickets   Tickets
	Customers Customers
	// AvailabilityIn binds the availability reads to a transaction (the
	// candidate spots of a zone-first sale are read in the sale
	// transaction). internal/app wires (*inventory.Service).WithDB.
	AvailabilityIn func(db bun.IDB) Availability
}

// Service implements the quick walk-in sale and the entry sale.
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

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perms ...auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perms[0])
	if err != nil {
		return es, err
	}
	for _, perm := range perms[1:] {
		if !p.Can(es, perm) {
			return es, auth.ErrForbidden
		}
	}
	return es, nil
}

// checkDate refuses a sale for a day already past in the establishment
// timezone (a ticket is numbered on its day of use).
func (s *Service) checkDate(ctx context.Context, es store.EstablishmentScope, d calendar.Date) error {
	if d.IsZero() {
		return configerr.Invalid("date", "Campo obbligatorio.")
	}
	today, err := ticketing.Today(ctx, s.d.DB, es, s.d.Clock)
	if err != nil {
		return err
	}
	if d.Before(today) {
		return configerr.Invalid("date", "Non si vendono biglietti per un giorno passato.")
	}
	return nil
}

// walkIn returns the walk-in customer of es, creating it (and the settings
// row) on first use. The settings row is locked while the customer is
// created, so concurrent first sales create exactly one.
func (s *Service) walkIn(ctx context.Context, p *auth.Principal, es store.EstablishmentScope) (string, error) {
	if st, err := store.GetDailySaleSettings(ctx, s.d.DB, es); err == nil && st.WalkInCustomerID != nil {
		if c, err := store.GetCustomer(ctx, s.d.DB, es, *st.WalkInCustomerID); err == nil && c.AnonymizedAt == nil {
			return c.ID, nil
		}
	}
	var id string
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		if err := store.EnsureDailySaleSettings(ctx, tx, es, now); err != nil {
			return err
		}
		st, err := store.LockDailySaleSettings(ctx, tx, es)
		if err != nil {
			return err
		}
		if st.WalkInCustomerID != nil {
			c, err := store.GetCustomer(ctx, tx, es, *st.WalkInCustomerID)
			if err == nil && c.AnonymizedAt == nil {
				id = c.ID
				return nil
			}
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		c, err := s.d.Customers.Resolve(ctx, tx, p, es, "", &customer.Input{LastName: WalkInLastName, Tags: []string{WalkInTag}})
		if err != nil {
			return fmt.Errorf("dailysale: walk-in customer: %w", err)
		}
		if err := store.SetDailySaleWalkIn(ctx, tx, es, c.ID, now); err != nil {
			return err
		}
		id = c.ID
		return nil
	})
	return id, err
}

// PaymentRecorder implements Payments over the payment service: the manual
// payment runs in the sale transaction (as a savepoint), with the payment
// service's own checks (cash.operate, open cash session for cash, VAT
// split, fiscal document, audit and payment.succeeded).
type PaymentRecorder struct {
	Svc *payment.Service
}

// RecordManualInTx implements Payments.
func (r PaymentRecorder) RecordManualInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in PaymentInput) (*store.Payment, error) {
	res, err := r.Svc.WithDB(tx).RecordPayment(ctx, p, reqctx.From(ctx).ClientIP, es.ID(), in.BookingID, payment.Input{
		Amount: in.Amount, Method: in.Method, CashSessionID: in.CashSessionID, Note: in.Note,
	})
	if err != nil {
		return nil, err
	}
	return &res.Payment, nil
}
