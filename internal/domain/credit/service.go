package credit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// JobExpire is the name of the credit expiry job.
const JobExpire = "credits.expire"

// Audit actions of the credit ledger.
const (
	ActionIssued   = "credit.issued"
	ActionConsumed = "credit.consumed"
	ActionExpired  = "credit.expired"
	ActionGranted  = "credit.granted" // manual movement from the panel
)

// Limits.
const (
	maxNote = 500
	// maxManualAmount bounds one manual movement (100 000 EUR).
	maxManualAmount money.Cents = 10_000_000
	// expireBatch bounds the customers handled by one ExpireAll run.
	expireBatch = 1000
)

// ErrEstablishmentNotFound: unknown establishment or outside the scope (404).
var ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)

// ErrBookingNotFound: booking_id of a manual movement unknown (404).
var ErrBookingNotFound = fmt.Errorf("%w: prenotazione", configerr.ErrNotFound)

// Payments records a payment of method credit from a verified customer
// link (magic link). Implemented by *payment.Service (PayWithCredit), which
// writes the payment, its fiscal document, paid_amount, audit and
// payment.succeeded in one transaction and calls GrantCredit (redemption)
// inside it.
type Payments interface {
	PayWithCredit(ctx context.Context, es store.EstablishmentScope, bookingID string, amount money.Cents, idempotencyKey string) (*store.Payment, error)
	PayHeldWithCredit(ctx context.Context, es store.EstablishmentScope, bookingID string, maxAmount money.Cents, idempotencyKey string) (*store.Payment, error)
}

// Deps are the dependencies of the service.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Payments is optional: without it ApplyFromLink answers
	// configerr.ErrNotImplemented.
	Payments Payments
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

// SetPayments closes the credit -> payment loop after both services exist
// (the payment service uses this one, through customer.Service, as its
// Credits port).
func (s *Service) SetPayments(p Payments) { s.d.Payments = p }

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

// Summary is the balance view of a customer without movements (the public
// manage area: api.PublicCreditBalance).
type Summary struct {
	Balance   money.Cents
	Available money.Cents
	Expiring  []Expiring
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

// Change is the payload (version 1) of the credit.* events: ids, dates and
// cents only, never personal data.
type Change struct {
	CreditID          string `json:"credit_id"`
	CustomerID        string `json:"customer_id"`
	Amount            int64  `json:"amount"`
	Source            string `json:"source"`
	ExpiresOn         string `json:"expires_on,omitempty"`
	BookingID         string `json:"booking_id,omitempty"`
	PaymentID         string `json:"payment_id,omitempty"`
	ReferenceCreditID string `json:"reference_credit_id,omitempty"`
}

// ---- panel ----------------------------------------------------------------------

// Ledger returns balance, available amount, expiring credit and a page of
// movements (newest first). Permission bookings.read.
func (s *Service) Ledger(ctx context.Context, p *auth.Principal, establishmentID, customerID, cursor string, limit int) (*Ledger, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if err := s.customerExists(ctx, s.d.DB, es, customerID); err != nil {
		return nil, err
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	sum, err := s.summary(ctx, s.d.DB, es, customerID, today)
	if err != nil {
		return nil, err
	}
	page, err := store.PageCustomerCredits(ctx, s.d.DB, es, customerID, cursor, limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return nil, configerr.Invalid("cursor", "Cursore non valido.")
		}
		return nil, fmt.Errorf("credit: ledger: %w", err)
	}
	return &Ledger{CustomerID: customerID, Balance: sum.Balance, Available: sum.Available, Expiring: sum.Expiring, Items: page}, nil
}

// Grant records a manual movement (permission refunds.issue): audit
// "credit.granted" and event credit.issued (credit.consumed for a negative
// adjustment) in the same transaction. A negative adjustment can never make
// the balance negative (ErrInsufficient).
func (s *Service) Grant(ctx context.Context, p *auth.Principal, establishmentID, customerID string, in GrantInput) (*store.CustomerCredit, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermRefundsIssue)
	if err != nil {
		return nil, err
	}
	in.Note = strings.TrimSpace(in.Note)
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	if err := validateGrant(in, today); err != nil {
		return nil, err
	}
	c := &store.CustomerCredit{
		CustomerID: customerID, Amount: in.Amount, Source: in.Source, Note: in.Note, ExpiresOn: in.ExpiresOn,
		CreatedByUserID: userIDOf(p),
	}
	if in.BookingID != "" {
		b := in.BookingID
		c.BookingID = &b
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.LockCustomer(ctx, tx, es, customerID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrCustomerNotFound
			}
			return fmt.Errorf("credit: %w", err)
		}
		if in.BookingID != "" {
			if _, err := store.CreditBookingPolicy(ctx, tx, es, in.BookingID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return ErrBookingNotFound
				}
				return fmt.Errorf("credit: %w", err)
			}
		}
		return s.move(ctx, tx, es, c, p.Actor(in.IP), ActionGranted)
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func validateGrant(in GrantInput, today calendar.Date) error {
	var ve configerr.ValidationError
	switch in.Source {
	case store.CreditSourceGoodwill, store.CreditSourceManual:
		if in.Amount <= 0 {
			ve.Add("amount", "L'importo del credito deve essere positivo.")
		}
	case store.CreditSourceAdjustment:
		if in.Amount == 0 {
			ve.Add("amount", "L'importo della rettifica non può essere zero.")
		}
	default:
		ve.Add("source", "Valori ammessi: goodwill, manual, adjustment.")
	}
	if in.Amount > maxManualAmount || in.Amount < -maxManualAmount {
		ve.Add("amount", "Importo troppo alto.")
	}
	if in.ExpiresOn != nil {
		if in.Amount < 0 {
			ve.Add("expires_on", "Una rettifica negativa non ha scadenza.")
		} else if in.ExpiresOn.Before(today) {
			ve.Add("expires_on", "La scadenza non può essere nel passato.")
		}
	}
	if utf8.RuneCountInString(in.Note) > maxNote {
		ve.Add("note", fmt.Sprintf("Massimo %d caratteri.", maxNote))
	}
	return ve.OrNil()
}

// ---- ledger port (payment.Credits, pass.Credits) -----------------------------

// GrantCredit appends a movement inside tx and updates the balance with a
// conditional UPDATE (ErrInsufficient when it would go negative).
// Implements payment.Credits and pass.Credits.
//
// Before a debit the expired credit of the customer is materialised
// (ExpireDue), so expired credit is never spent. A negative movement of a
// credit source (cancellation, season_release, weather: the reversal of a
// credit granted earlier, e.g. pass.UnmarkResold) is capped at what is left
// of the balance: credit already spent cannot be taken back and the
// reversal must not block the cancellation that causes it. A reversal with
// nothing left to take writes no row.
func (s *Service) GrantCredit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit) error {
	if err := validateMovement(c); err != nil {
		return err
	}
	if err := s.customerExists(ctx, tx, es, c.CustomerID); err != nil {
		return err
	}
	actor := audit.SystemActor
	if c.CreatedByUserID != nil {
		actor = audit.Actor{Type: audit.ActorUser, ID: *c.CreatedByUserID}
	}
	action := ActionIssued
	if c.Amount < 0 {
		action = ActionConsumed
	}
	return s.move(ctx, tx, es, c, actor, action)
}

func validateMovement(c *store.CustomerCredit) error {
	if c == nil || c.Amount == 0 {
		return configerr.Invalid("amount", "L'importo del credito non può essere zero.")
	}
	switch c.Source {
	case store.CreditSourceCancellation, store.CreditSourceSeasonRelease, store.CreditSourceWeather,
		store.CreditSourceManual, store.CreditSourceAdjustment:
	case store.CreditSourceGoodwill:
		if c.Amount < 0 {
			return configerr.Invalid("amount", "Un credito di cortesia è sempre positivo.")
		}
	case store.CreditSourceRedemption:
		if c.Amount > 0 {
			return configerr.Invalid("amount", "L'uso del credito ha importo negativo.")
		}
	default:
		// expiry rows are written only by ExpireDue.
		return configerr.Invalid("source", "Origine del credito non valida.")
	}
	return nil
}

// isReversal reports a negative movement of a credit source.
func isReversal(c *store.CustomerCredit) bool {
	if c.Amount >= 0 {
		return false
	}
	switch c.Source {
	case store.CreditSourceCancellation, store.CreditSourceSeasonRelease, store.CreditSourceWeather:
		return true
	}
	return false
}

// move is the single write path of the ledger: lock the balance row,
// materialise the expired credit before a debit, derive the expiry of a
// grant from the policy of its booking, insert the row, apply it to the
// balance (conditional), audit and outbox, all in tx.
func (s *Service) move(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit, actor audit.Actor, action string) error {
	now := s.now()
	if err := store.LockCustomerCreditBalance(ctx, tx, es, c.CustomerID, now); err != nil {
		return fmt.Errorf("credit: %w", err)
	}
	today, err := s.today(ctx, tx, es)
	if err != nil {
		return err
	}
	if c.Amount < 0 {
		if _, err := s.expireDueLocked(ctx, tx, es, c.CustomerID, today); err != nil {
			return err
		}
	}
	if isReversal(c) {
		bal, err := store.GetCustomerCreditBalance(ctx, tx, es, c.CustomerID)
		if err != nil {
			return fmt.Errorf("credit: %w", err)
		}
		if bal <= 0 {
			s.d.Log.InfoContext(ctx, "credit reversal skipped: nothing left", slog.String("customer_id", c.CustomerID), slog.Int64("amount", int64(c.Amount)))
			return nil
		}
		if -c.Amount > bal {
			s.d.Log.InfoContext(ctx, "credit reversal capped at the balance", slog.String("customer_id", c.CustomerID), slog.Int64("amount", int64(c.Amount)), slog.Int64("balance", int64(bal)))
			c.Amount = -bal
		}
	}
	if c.Amount > 0 && c.ExpiresOn == nil && c.BookingID != nil {
		switch c.Source {
		case store.CreditSourceCancellation, store.CreditSourceSeasonRelease, store.CreditSourceWeather:
			exp, err := s.policyExpiry(ctx, tx, es, *c.BookingID, today)
			if err != nil {
				return err
			}
			c.ExpiresOn = exp
		}
	}
	return s.insert(ctx, tx, es, c, actor, action, now)
}

// insert writes the row and applies it to the balance.
func (s *Service) insert(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, c *store.CustomerCredit, actor audit.Actor, action string, now time.Time) error {
	if c.ID == "" {
		c.ID = store.NewID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if err := store.InsertCustomerCredit(ctx, tx, es, c); err != nil {
		return fmt.Errorf("credit: insert: %w", err)
	}
	ok, err := store.AddCustomerCreditBalance(ctx, tx, es, c.CustomerID, c.Amount, now)
	if err != nil {
		return fmt.Errorf("credit: %w", err)
	}
	if !ok {
		return ErrInsufficient
	}
	ch := changeOf(c)
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "customer_credit", EntityID: c.ID, Diff: ch,
	}); err != nil {
		return fmt.Errorf("credit: audit: %w", err)
	}
	typ := event.CreditIssued
	switch {
	case c.Source == store.CreditSourceExpiry:
		typ = event.CreditExpired
	case c.Amount < 0:
		typ = event.CreditConsumed
	}
	if _, err := s.d.Outbox.Write(ctx, tx, typ, es.ID(), 1, ch); err != nil {
		return fmt.Errorf("credit: outbox: %w", err)
	}
	return nil
}

func changeOf(c *store.CustomerCredit) Change {
	ch := Change{CreditID: c.ID, CustomerID: c.CustomerID, Amount: int64(c.Amount), Source: c.Source}
	if c.ExpiresOn != nil {
		ch.ExpiresOn = c.ExpiresOn.String()
	}
	if c.BookingID != nil {
		ch.BookingID = *c.BookingID
	}
	if c.PaymentID != nil {
		ch.PaymentID = *c.PaymentID
	}
	if c.ReferenceCreditID != nil {
		ch.ReferenceCreditID = *c.ReferenceCreditID
	}
	return ch
}

// policyExpiry is the expiry of a credit caused by bookingID issued today
// (policy credit_validity_days of its snapshot; nil = never).
func (s *Service) policyExpiry(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string, today calendar.Date) (*calendar.Date, error) {
	raw, err := store.CreditBookingPolicy(ctx, db, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("credit: %w", err)
	}
	snap, err := policy.ParseSnapshot(raw)
	if err != nil {
		return nil, nil //nolint:nilerr // a malformed snapshot means "no expiry", never a failed refund
	}
	return snap.Rules.CreditExpiry(today), nil
}

// CreditBalance is the amount spendable on today (Available). Implements
// payment.Credits.
func (s *Service) CreditBalance(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	mv, err := store.LedgerCustomerCredits(ctx, db, es, customerID)
	if err != nil {
		return 0, fmt.Errorf("credit: balance: %w", err)
	}
	return allocate(mv).available(today), nil
}

// Summary returns balance, available amount and expiring credit of a
// customer of an establishment (no permission check: the caller proved the
// identity, e.g. with a magic link).
func (s *Service) Summary(ctx context.Context, establishmentID, customerID string) (*Summary, error) {
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, establishmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrEstablishmentNotFound
		}
		return nil, fmt.Errorf("credit: %w", err)
	}
	today, err := s.today(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	return s.summary(ctx, s.d.DB, es, customerID, today)
}

func (s *Service) summary(ctx context.Context, db bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (*Summary, error) {
	mv, err := store.LedgerCustomerCredits(ctx, db, es, customerID)
	if err != nil {
		return nil, fmt.Errorf("credit: summary: %w", err)
	}
	bal, err := store.GetCustomerCreditBalance(ctx, db, es, customerID)
	if err != nil {
		return nil, fmt.Errorf("credit: summary: %w", err)
	}
	a := allocate(mv)
	if a.Balance != bal {
		// Drift between the aggregate and the ledger means a write bypassed
		// GrantCredit: report the ledger (the source of truth) and log it.
		s.d.Log.WarnContext(ctx, "credit balance differs from the ledger", slog.String("customer_id", customerID),
			slog.Int64("balance", int64(bal)), slog.Int64("ledger", int64(a.Balance)))
	}
	return &Summary{Balance: a.Balance, Available: a.available(today), Expiring: a.expiring(today)}, nil
}

// ApplyFromLink pays (part of) the balance due of a booking with the
// customer's credit, for the holder of a verified link (magic link): amount
// 0 = the minimum of available credit and balance due. The payment service
// records the payment of method credit and the redemption in one
// transaction.
func (s *Service) ApplyFromLink(ctx context.Context, establishmentID, bookingID string, amount money.Cents, idempotencyKey string) (*store.Payment, error) {
	if s.d.Payments == nil {
		return nil, configerr.ErrNotImplemented
	}
	if amount < 0 {
		return nil, configerr.Invalid("amount", "L'importo non può essere negativo.")
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, establishmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrEstablishmentNotFound
		}
		return nil, fmt.Errorf("credit: %w", err)
	}
	return s.d.Payments.PayWithCredit(ctx, es, bookingID, amount, idempotencyKey)
}

// ApplyCreditToBooking spends up to maxAmount (0 = no cap) of the
// customer credit on a booking of es, also while it is held (renewal or
// waitlist acceptance from a verified link), and returns what it applied
// (0 when no credit is available). Implements renewal.CreditApplier.
func (s *Service) ApplyCreditToBooking(ctx context.Context, es store.EstablishmentScope, bookingID string, maxAmount money.Cents, idempotencyKey string) (money.Cents, error) {
	if s.d.Payments == nil {
		return 0, configerr.ErrNotImplemented
	}
	pm, err := s.d.Payments.PayHeldWithCredit(ctx, es, bookingID, maxAmount, idempotencyKey)
	if err != nil || pm == nil {
		return 0, err
	}
	return pm.Amount, nil
}

// ---- expiry ---------------------------------------------------------------------

// ExpireDue materialises the expired credit of one customer inside tx and
// returns the expired amount: one expiry row per grant expired before today
// with something left (amount = -remaining, reference_credit_id = grant;
// unique per grant, so it is idempotent).
func (s *Service) ExpireDue(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	if err := store.LockCustomerCreditBalance(ctx, tx, es, customerID, s.now()); err != nil {
		return 0, fmt.Errorf("credit: %w", err)
	}
	return s.expireDueLocked(ctx, tx, es, customerID, today)
}

func (s *Service) expireDueLocked(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	mv, err := store.LedgerCustomerCredits(ctx, tx, es, customerID)
	if err != nil {
		return 0, fmt.Errorf("credit: expire: %w", err)
	}
	var total money.Cents
	now := s.now()
	for _, g := range allocate(mv).dueExpiries(today) {
		ref := g.ID
		row := &store.CustomerCredit{
			CustomerID: customerID, Amount: -g.Remaining, Source: store.CreditSourceExpiry,
			ReferenceCreditID: &ref, CreatedAt: now,
		}
		if err := s.insert(ctx, tx, es, row, audit.SystemActor, ActionExpired, now); err != nil {
			return total, err
		}
		total += g.Remaining
	}
	return total, nil
}

// ExpireAll runs ExpireDue for every customer with expired credit (one
// transaction per customer) and returns how many customers it touched.
func (s *Service) ExpireAll(ctx context.Context) (int, error) {
	// The latest "today" on earth: candidates are a superset, each one is
	// checked against the date of its establishment.
	horizon := calendar.In(s.d.Clock.Now(), time.UTC).AddDays(1)
	list, err := store.CustomersWithExpiredCredit(ctx, s.d.DB, horizon, expireBatch)
	if err != nil {
		return 0, fmt.Errorf("credit: expire all: %w", err)
	}
	touched := 0
	scopes := map[string]store.EstablishmentScope{}
	for _, c := range list {
		if err := ctx.Err(); err != nil {
			return touched, err
		}
		es, ok := scopes[c.EstablishmentID]
		if !ok {
			if es, err = store.SystemEstablishmentScope(ctx, s.d.DB, c.EstablishmentID); err != nil {
				return touched, fmt.Errorf("credit: expire all: %w", err)
			}
			scopes[c.EstablishmentID] = es
		}
		var n money.Cents
		err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			today, err := s.today(ctx, tx, es)
			if err != nil {
				return err
			}
			n, err = s.ExpireDue(ctx, tx, es, c.CustomerID, today)
			return err
		})
		if err != nil {
			return touched, err
		}
		if n > 0 {
			touched++
		}
	}
	if touched > 0 {
		s.d.Log.InfoContext(ctx, "expired customer credit", slog.Int("customers", touched))
	}
	return touched, nil
}

// Jobs returns the credit expiry job (hourly). Registered by internal/app.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobExpire, Interval: time.Hour, RunOnStart: true, Timeout: 5 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.ExpireAll(ctx)
		return err
	}}}
}

// ---- helpers ----------------------------------------------------------------------

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func (s *Service) customerExists(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) error {
	if _, err := store.GetCustomer(ctx, db, es, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrCustomerNotFound
		}
		return fmt.Errorf("credit: %w", err)
	}
	return nil
}

// today is the current date in the establishment timezone.
func (s *Service) today(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (calendar.Date, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("credit: establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return calendar.In(s.d.Clock.Now(), loc), nil
}

func userIDOf(p *auth.Principal) *string {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil
	}
	id := p.UserID
	return &id
}
