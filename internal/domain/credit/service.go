package credit

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// JobExpire is the name of the credit expiry job.
const JobExpire = "credits.expire"

// Deps are the dependencies of the service.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the customer credit use cases.
type Service struct {
	d Deps
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

// Expiring is the amount of credit that expires on a date.
type Expiring struct {
	ExpiresOn calendar.Date
	Amount    money.Cents
}

// Ledger is the credit view of a customer (api.CustomerCreditLedger).
type Ledger struct {
	CustomerID string
	// Balance is SUM(amount) (customer_credit_balances.balance).
	Balance money.Cents
	// Available is what can be spent today (Balance minus expired credit
	// not yet materialised as expiry rows).
	Available money.Cents
	Expiring  []Expiring
	Items     store.Page[store.CustomerCredit]
}

// GrantInput is a manual movement from the panel (api.CustomerCreditCreate).
type GrantInput struct {
	Amount    money.Cents
	Source    string // store.CreditSourceGoodwill | Manual | Adjustment
	ExpiresOn *calendar.Date
	BookingID string
	Note      string
	IP        string
}

// Ledger returns balance, available amount, expiring credit and a page of
// movements (newest first). Permission bookings.read.
func (s *Service) Ledger(ctx context.Context, p *auth.Principal, establishmentID, customerID, cursor string, limit int) (*Ledger, error) {
	return nil, configerr.ErrNotImplemented
}

// Grant records a manual movement (permission refunds.issue): audit
// "credit.granted" and event credit.issued in the same transaction.
func (s *Service) Grant(ctx context.Context, p *auth.Principal, establishmentID, customerID string, in GrantInput) (*store.CustomerCredit, error) {
	return nil, configerr.ErrNotImplemented
}

// GrantCredit appends a movement inside tx and updates the balance with a
// conditional UPDATE (ErrInsufficient when it would go negative).
// Implements payment.Credits and pass.Credits.
func (s *Service) GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error {
	return configerr.ErrNotImplemented
}

// CreditBalance is the amount spendable on today (Available). Implements
// payment.Credits.
func (s *Service) CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	return 0, configerr.ErrNotImplemented
}

// ExpireDue materialises the expired credit of one customer inside tx (the
// caller holds store.LockCustomer) and returns the expired amount.
func (s *Service) ExpireDue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	return 0, configerr.ErrNotImplemented
}

// ExpireAll runs ExpireDue for every customer with expired credit (one
// transaction per customer) and returns how many customers it touched.
func (s *Service) ExpireAll(ctx context.Context) (int, error) {
	return 0, configerr.ErrNotImplemented
}

// Jobs returns the credit expiry job (hourly). Registered by internal/app
// once implemented.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobExpire, Interval: time.Hour, RunOnStart: true, Timeout: 5 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.ExpireAll(ctx)
		return err
	}}}
}
