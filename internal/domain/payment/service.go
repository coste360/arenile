package payment

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
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// ProviderManual is the code of the desk payment driver (builtin.Manual).
const ProviderManual = "manual"

// Limits of the inputs.
const (
	// MaxAmount bounds one payment (1,000,000.00 EUR).
	MaxAmount money.Cents = 100_000_000
)

const (
	maxNote           = 500
	maxDescription    = 200
	maxIdempotencyKey = 255
)

// Errors of the payment package (mapped by internal/http/problem through
// the configerr kinds).
var (
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	ErrBookingNotFound       = fmt.Errorf("%w: booking", configerr.ErrNotFound)
	ErrPaymentNotFound       = fmt.Errorf("%w: payment", configerr.ErrNotFound)
	ErrCustomerNotFound      = fmt.Errorf("%w: customer", configerr.ErrNotFound)
	// ErrBookingNotPayable: cancelled, expired and rejected bookings take no
	// payments (refunds stay possible).
	ErrBookingNotPayable = fmt.Errorf("%w: la prenotazione non accetta pagamenti nel suo stato attuale", configerr.ErrConflict)
	// ErrIdempotencyMismatch: the key was already used for a different
	// payment (other booking, amount, method or direction).
	ErrIdempotencyMismatch = fmt.Errorf("%w: chiave di idempotenza già usata per un altro pagamento", configerr.ErrConflict)
	// ErrProviderMissing: the "manual" driver is not registered (wiring bug).
	ErrProviderMissing = errors.New("payment: manual provider not registered")
	// ErrRefundInProgress: a guaranteed refund of the booking (refund_requests,
	// ADR 0019) is still open; a manual refund would race with it (409
	// refund_in_progress).
	ErrRefundInProgress = errors.New("payment: refund in progress")
)

// Booking statuses the payment service reads. They mirror
// booking.Status values (booking imports payment, so payment cannot import
// booking).
const (
	bookingHeld           = "held"
	bookingPendingPayment = "pending_payment"
	bookingCancelled      = "cancelled"
	bookingExpired        = "expired"
	bookingRejected       = "rejected"
)

// methods accepted from the desk.
var methods = map[string]bool{
	store.PaymentMethodCash: true, store.PaymentMethodCard: true, store.PaymentMethodBankTransfer: true,
	store.PaymentMethodWallet: true, store.PaymentMethodCredit: true, store.PaymentMethodOther: true,
}

// Deps are the dependencies of the payment service. Every field is required
// except Log and Confirmer.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Bookings  BookingLedger
	Cash      CashDesk
	Fiscal    Fiscal
	Credits   Credits
	Providers Providers
	// Confirmer is the manual-driver confirmation path. When nil, Bookings
	// is used if it implements BookingConfirmer.
	Confirmer BookingConfirmer
}

// Service implements the payment use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Confirmer == nil {
		if c, ok := d.Bookings.(BookingConfirmer); ok {
			d.Confirmer = c
		}
	}
	return &Service{d: d}
}

// SetConfirmer installs the confirmation path after construction. For the
// composition root only: the booking service depends on payment, so it can
// be built only after this service (constructor cycle).
func (s *Service) SetConfirmer(c BookingConfirmer) { s.d.Confirmer = c }

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// Result is the outcome of a payment use case.
type Result struct {
	Payment store.Payment
	// Booking is the booking after the payment (nil for counter sales and
	// replays).
	Booking *store.Booking
	// Excess is the refund-to-credit row of an overpayment (nil otherwise).
	Excess *store.Payment
	// Confirmed is true when this payment confirmed the booking.
	Confirmed bool
	// Replayed is true when the idempotency key matched an existing payment:
	// nothing was written.
	Replayed bool
}

// ---- inputs -------------------------------------------------------------------

// Input is a manual payment of a booking (api.ManualPaymentInput).
type Input struct {
	Amount        money.Cents
	Method        string
	Note          string
	CashSessionID *string
	// IdempotencyKey makes the payment idempotent per establishment.
	IdempotencyKey string
	// OverpaymentAsCredit accepts an amount above the balance due: the
	// excess is refunded at once as customer credit (spec 13.4). Without it
	// the amount may not exceed the balance.
	OverpaymentAsCredit bool
}

// CounterSaleInput is a desk sale without a booking (api.CounterSaleInput).
type CounterSaleInput struct {
	Amount         money.Cents
	Method         string
	Description    string
	CustomerID     string
	VATRateBP      *int64 // default: establishment default rate
	CashSessionID  *string
	IdempotencyKey string
}

// RefundAs is how a refund is settled.
type RefundAs string

// Refund settlements.
const (
	RefundAsRefund RefundAs = "refund" // money back (manual refund to complete)
	RefundAsCredit RefundAs = "credit" // customer credit (customer_credits)
)

// RefundRequest is a refund of a booking, from the API (RecordBookingRefund)
// or from a cancellation computed on the policy snapshot (booking service).
type RefundRequest struct {
	BookingID     string
	Amount        money.Cents
	Method        string // store.PaymentMethod*; ignored for RefundAsCredit
	As            RefundAs
	PaymentID     string // refunded payment (optional)
	Reason        string
	CashSessionID *string
	// IdempotencyKey, when set, makes the refund idempotent per establishment.
	IdempotencyKey string
	// CreditSource is the customer_credits.source of a RefundAsCredit
	// (default store.CreditSourceManual; the booking cancellation passes
	// store.CreditSourceCancellation).
	CreditSource string
	// CreditExpiresOn is the expiry of the credit (nil = never).
	CreditExpiresOn *calendar.Date
	// RefuseOpenRequests refuses the refund (ErrRefundInProgress) while a
	// guaranteed refund request of the booking is open; checked under the
	// booking lock (the manual panel refund sets it, the cancellation that
	// creates the request does not).
	RefuseOpenRequests bool
}

// ---- helpers ------------------------------------------------------------------

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func userIDOf(p *auth.Principal) *string {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil
	}
	id := p.UserID
	return &id
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// estInfo is what a payment needs from the establishment row.
type estInfo struct {
	est        *store.Establishment
	today      calendar.Date // business date in the establishment timezone
	defaultVAT int64
}

func (s *Service) establishment(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*estInfo, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("payment: load establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("payment: timezone %q: %w", est.Timezone, err)
	}
	settings, _ := establishment.ParseSettings(est.Settings, est.Currency)
	return &estInfo{est: est, today: calendar.In(s.d.Clock.Now(), loc), defaultVAT: settings.Pricing.DefaultVATRateBP}, nil
}

func (s *Service) manual() (ext.PaymentProvider, error) {
	pp, ok := s.d.Providers.PaymentProvider(ProviderManual)
	if !ok {
		return nil, ErrProviderMissing
	}
	return pp, nil
}

func validateKey(key string, ve *configerr.ValidationError) {
	if len(key) > maxIdempotencyKey {
		ve.Add("Idempotency-Key", "Chiave di idempotenza troppo lunga.")
	}
}

func validateAmount(field string, a money.Cents, ve *configerr.ValidationError) {
	if a <= 0 || a > MaxAmount {
		ve.Add(field, "L'importo deve essere positivo (massimo 1.000.000,00).")
	}
}

func euro(c money.Cents) string {
	v := int64(c)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d,%02d €", sign, v/100, v%100)
}

// replay returns the payment recorded with key when it describes the same
// operation (want), ErrIdempotencyMismatch when it does not, nil when the
// key is unused.
func replay(ctx context.Context, db bun.IDB, es store.EstablishmentScope, key string, want func(*store.Payment) bool) (*Result, error) {
	if key == "" {
		return nil, nil
	}
	pm, err := store.GetPaymentByIdempotencyKey(ctx, db, es, key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	if !want(pm) {
		return nil, ErrIdempotencyMismatch
	}
	return &Result{Payment: *pm, Replayed: true}, nil
}

// runIdempotent runs fn in a transaction. When two requests with the same
// key race, the loser hits the unique index payments(establishment_id,
// idempotency_key) and its transaction rolls back; the winner's payment is
// then returned as a replay.
func (s *Service) runIdempotent(ctx context.Context, es store.EstablishmentScope, key string, want func(*store.Payment) bool, fn func(ctx context.Context, tx bun.Tx) (*Result, error)) (*Result, error) {
	var res *Result
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		r, err := fn(ctx, tx)
		res = r
		return err
	})
	if err == nil {
		return res, nil
	}
	if key != "" && (errors.Is(err, store.ErrConflict) || store.IsUniqueViolation(err)) {
		if r, rerr := replay(ctx, s.d.DB, es, key, want); rerr != nil || r != nil {
			if rerr != nil {
				return nil, rerr
			}
			return r, nil
		}
	}
	return nil, err
}

func samePayment(bookingID *string, direction, method string, amount money.Cents) func(*store.Payment) bool {
	return func(pm *store.Payment) bool {
		sameBooking := (bookingID == nil && pm.BookingID == nil) ||
			(bookingID != nil && pm.BookingID != nil && *bookingID == *pm.BookingID)
		return sameBooking && pm.Direction == direction && pm.Method == method && pm.Amount == amount
	}
}

// record writes the audit entry and the outbox event of pm.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, action, eventType string, pm *store.Payment, extra map[string]any) error {
	diff := map[string]any{
		"amount": int64(pm.Amount), "method": pm.Method, "direction": pm.Direction, "provider": pm.Provider,
		"business_date": pm.BusinessDate.String(),
	}
	if pm.BookingID != nil {
		diff["booking_id"] = *pm.BookingID
	}
	if pm.CashSessionID != nil {
		diff["cash_session_id"] = *pm.CashSessionID
	}
	if pm.RefundOfPaymentID != nil {
		diff["refund_of_payment_id"] = *pm.RefundOfPaymentID
	}
	for k, v := range extra {
		diff[k] = v
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: action, EntityType: "payment", EntityID: pm.ID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("payment: %w", err)
	}
	ch := event.PaymentChange{
		PaymentID: pm.ID, Provider: pm.Provider, Method: pm.Method, Direction: pm.Direction,
		Amount: int64(pm.Amount), Status: pm.Status,
	}
	if pm.BookingID != nil {
		ch.BookingID = *pm.BookingID
	}
	if pm.CashSessionID != nil {
		ch.CashSessionID = *pm.CashSessionID
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, ch); err != nil {
		return fmt.Errorf("payment: %w", err)
	}
	return nil
}

// insert writes pm and its fiscal document.
func (s *Service) insert(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, pm *store.Payment) error {
	// Test mode (spec 15.4, ADR 0019): a payment inherits the flag of its
	// booking; a counter sale takes the switch read in this transaction.
	if !pm.Test {
		var err error
		if pm.BookingID != nil {
			pm.Test, err = store.BookingIsTest(ctx, tx, es, *pm.BookingID)
		} else {
			pm.Test, err = store.TestModeEnabled(ctx, tx, es)
		}
		if err != nil {
			return fmt.Errorf("payment: %w", err)
		}
	}
	if err := store.InsertPayment(ctx, tx, es, pm); err != nil {
		return fmt.Errorf("payment: %w", err)
	}
	if err := s.d.Fiscal.RecordPayment(ctx, tx, es, pm); err != nil {
		return fmt.Errorf("payment: fiscal: %w", err)
	}
	return nil
}

// sessionFor resolves the cash session of a payment; method credit moves no
// money through the desk and never attaches to a session.
func (s *Service) sessionFor(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, sessionID *string, method string) (*string, error) {
	if method == store.PaymentMethodCredit {
		return nil, nil
	}
	return s.d.Cash.SessionForPayment(ctx, tx, es, sessionID, method)
}

// redeemCredit checks the customer credit balance and consumes amount. The
// customer row is locked first (store.LockCustomer): credit movements of one
// customer serialize, so two payments of different bookings cannot both
// spend the same balance (INV-47, the ledger has no balance constraint).
func (s *Service) redeemCredit(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, info *estInfo, customerID string, amount money.Cents, pm *store.Payment) error {
	if err := store.LockCustomer(ctx, tx, es, customerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrCustomerNotFound
		}
		return fmt.Errorf("payment: %w", err)
	}
	bal, err := s.d.Credits.CreditBalance(ctx, tx, es, customerID, info.today)
	if err != nil {
		return fmt.Errorf("payment: credit balance: %w", err)
	}
	if bal < amount {
		return configerr.Invalid("amount", fmt.Sprintf("Credito del cliente insufficiente (disponibile %s).", euro(bal)))
	}
	return s.grantCredit(ctx, tx, p, es, customerID, -amount, store.CreditSourceRedemption, nil, pm, "")
}

func (s *Service) grantCredit(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, customerID string, amount money.Cents, source string, expires *calendar.Date, pm *store.Payment, note string) error {
	c := &store.CustomerCredit{
		ID: store.NewID(), EstablishmentID: es.ID(), CustomerID: customerID, Amount: amount, Source: source,
		BookingID: pm.BookingID, PaymentID: &pm.ID, Note: note, ExpiresOn: expires,
		CreatedByUserID: userIDOf(p), CreatedAt: s.now(),
	}
	if err := s.d.Credits.GrantCredit(ctx, tx, es, c); err != nil {
		return fmt.Errorf("payment: credit: %w", err)
	}
	return nil
}

// ---- record a booking payment -------------------------------------------------

// RecordPayment records a manual payment of a booking (driver "manual",
// auth.PermCashOperate) in one transaction: lock the booking, check the
// balance, attach the cash session (cash requires an open one), freeze the
// VAT split, write the payment and its fiscal document, update paid_amount
// (INV-22), write audit + payment.succeeded and, when the payment reaches
// the deposit (or the total), confirm the booking through the
// BookingConfirmer (manual driver confirmation, INV-19).
func (s *Service) RecordPayment(ctx context.Context, p *auth.Principal, ip, establishmentID, bookingID string, in Input) (*Result, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	in.Note = strings.TrimSpace(in.Note)
	var ve configerr.ValidationError
	validateAmount("amount", in.Amount, &ve)
	if !methods[in.Method] {
		ve.Add("method", "Metodo di pagamento non valido.")
	}
	if len([]rune(in.Note)) > maxNote {
		ve.Add("note", fmt.Sprintf("Massimo %d caratteri.", maxNote))
	}
	validateKey(in.IdempotencyKey, &ve)
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	want := samePayment(&bookingID, store.PaymentDirectionIn, in.Method, in.Amount)
	return s.runIdempotent(ctx, es, in.IdempotencyKey, want, func(ctx context.Context, tx bun.Tx) (*Result, error) {
		b, err := s.d.Bookings.LoadForPayment(ctx, tx, es, bookingID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrBookingNotFound
			}
			return nil, fmt.Errorf("payment: load booking: %w", err)
		}
		if r, err := replay(ctx, tx, es, in.IdempotencyKey, want); err != nil || r != nil {
			return r, err
		}
		switch b.Status {
		case bookingCancelled, bookingExpired, bookingRejected:
			return nil, ErrBookingNotPayable
		}
		due := b.TotalAmount - b.PaidAmount
		excess := in.Amount - max(due, 0)
		if excess > 0 && !in.OverpaymentAsCredit {
			if due <= 0 {
				return nil, configerr.Invalid("amount", "La prenotazione è già saldata.")
			}
			return nil, configerr.Invalid("amount", fmt.Sprintf("L'importo supera il saldo dovuto (%s).", euro(due)))
		}
		info, err := s.establishment(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		session, err := s.sessionFor(ctx, tx, es, in.CashSessionID, in.Method)
		if err != nil {
			return nil, err
		}
		pp, err := s.manual()
		if err != nil {
			return nil, err
		}
		intent, err := pp.CreatePayment(ctx, ext.CreatePaymentRequest{
			EstablishmentID: es.ID(), BookingID: b.ID, BookingCode: b.Code, Amount: int64(in.Amount),
			Currency: info.est.Currency, IdempotencyKey: in.IdempotencyKey, PreferredMode: ext.PaymentModeNone,
		})
		if err != nil {
			return nil, fmt.Errorf("payment: manual driver: %w", err)
		}
		now := s.now()
		pm := &store.Payment{
			ID: store.NewID(), BookingID: &b.ID, CustomerID: optional(b.CustomerID), Provider: pp.Code(),
			Method: in.Method, Direction: store.PaymentDirectionIn, Amount: in.Amount, Status: store.PaymentSucceeded,
			ProviderRef: optional(intent.ProviderRef), IdempotencyKey: optional(in.IdempotencyKey),
			CashSessionID: session, ReceivedByUserID: userIDOf(p), Description: "Prenotazione " + b.Code, Note: in.Note,
			VATBreakdown: SplitBySnapshot(b.CurrentPrice(), in.Amount, info.defaultVAT), BusinessDate: info.today,
			Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.insert(ctx, tx, es, pm); err != nil {
			return nil, err
		}
		if in.Method == store.PaymentMethodCredit {
			if err := s.redeemCredit(ctx, tx, p, es, info, b.CustomerID, in.Amount, pm); err != nil {
				return nil, err
			}
		}
		updated, err := s.d.Bookings.ApplyPayment(ctx, tx, es, b.ID, in.Amount)
		if err != nil {
			return nil, fmt.Errorf("payment: apply to booking: %w", err)
		}
		if err := s.record(ctx, tx, p, ip, es, "payment.recorded", event.PaymentSucceeded, pm, nil); err != nil {
			return nil, err
		}
		res := &Result{Payment: *pm, Booking: updated}
		if excess > 0 {
			ex, err := s.refundInTx(ctx, tx, p, ip, es, b, info, RefundRequest{
				BookingID: b.ID, Amount: excess, As: RefundAsCredit, Method: store.PaymentMethodCredit, PaymentID: pm.ID,
				Reason: "Eccedenza del pagamento accreditata al cliente", CreditSource: store.CreditSourceManual,
			})
			if err != nil {
				return nil, err
			}
			res.Excess, res.Booking = &ex.Payment, ex.Booking
		}
		confirmed, err := s.maybeConfirm(ctx, tx, p, es, res.Booking, pm)
		if err != nil {
			return nil, err
		}
		res.Confirmed = confirmed
		return res, nil
	})
}

// maybeConfirm confirms a held / pending_payment booking whose paid amount
// reached its threshold: deposit_due when the policy asks for a deposit,
// the total otherwise.
func (s *Service) maybeConfirm(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking, pm *store.Payment) (bool, error) {
	if s.d.Confirmer == nil || b == nil {
		return false, nil
	}
	if b.Status != bookingHeld && b.Status != bookingPendingPayment {
		return false, nil
	}
	threshold := b.TotalAmount
	if b.DepositDue > 0 {
		threshold = b.DepositDue
	}
	if b.PaidAmount < threshold {
		return false, nil
	}
	if err := s.d.Confirmer.ConfirmFromPayment(ctx, tx, p, es, b.ID, pm); err != nil {
		return false, fmt.Errorf("payment: confirm booking: %w", err)
	}
	return true, nil
}

// ---- refunds ------------------------------------------------------------------

// RefundBooking is the API use case of a manual refund
// (auth.PermRefundsIssue): one transaction around RecordRefund.
func (s *Service) RefundBooking(ctx context.Context, p *auth.Principal, ip, establishmentID string, req RefundRequest) (*Result, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermRefundsIssue)
	if err != nil {
		return nil, err
	}
	if err := validateRefund(&req); err != nil {
		return nil, err
	}
	want := samePayment(&req.BookingID, store.PaymentDirectionRefund, req.Method, req.Amount)
	req.RefuseOpenRequests = true
	return s.runIdempotent(ctx, es, req.IdempotencyKey, want, func(ctx context.Context, tx bun.Tx) (*Result, error) {
		return s.refund(ctx, tx, p, ip, es, req)
	})
}

// CheckNoOpenRefundRequest is ErrRefundInProgress while a guaranteed
// refund of the booking (refund_requests: pending, processing or failed,
// ADR 0019) is open: a manual refund from the panel would race with it.
// Call it with the booking locked (INV-75): a cancellation committing
// between the check and the refund would otherwise slip through. The
// cancellation itself (credit-paid part, M5B) and the refund job do not go
// through this check.
func CheckNoOpenRefundRequest(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) error {
	open, err := store.HasOpenRefundRequest(ctx, db, es, bookingID)
	if err != nil {
		return fmt.Errorf("payment: %w", err)
	}
	if open {
		return ErrRefundInProgress
	}
	return nil
}

// RecordRefund records a refund (or credit) of a booking in tx, applies it to
// paid_amount and writes audit + payment.refunded. The caller (booking
// cancellation, or the payment API use case) has already checked
// auth.PermRefundsIssue. Implements booking.Payments.
func (s *Service) RecordRefund(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, req RefundRequest) (*store.Payment, error) {
	if err := validateRefund(&req); err != nil {
		return nil, err
	}
	r, err := s.refund(ctx, tx, p, "", es, req)
	if err != nil {
		return nil, err
	}
	return &r.Payment, nil
}

func validateRefund(req *RefundRequest) error {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.As == "" {
		req.As = RefundAsRefund
	}
	var ve configerr.ValidationError
	validateAmount("amount", req.Amount, &ve)
	switch req.As {
	case RefundAsCredit:
		req.Method = store.PaymentMethodCredit
		if req.CreditSource == "" {
			req.CreditSource = store.CreditSourceManual
		}
	case RefundAsRefund:
		if !methods[req.Method] || req.Method == store.PaymentMethodCredit {
			ve.Add("method", "Metodo di rimborso non valido (per il credito usa as=credit).")
		}
	default:
		ve.Add("as", "Valori ammessi: refund, credit.")
	}
	if len([]rune(req.Reason)) > maxNote {
		ve.Add("reason", fmt.Sprintf("Massimo %d caratteri.", maxNote))
	}
	validateKey(req.IdempotencyKey, &ve)
	return ve.OrNil()
}

func (s *Service) refund(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, req RefundRequest) (*Result, error) {
	b, err := s.d.Bookings.LoadForPayment(ctx, tx, es, req.BookingID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("payment: load booking: %w", err)
	}
	if r, err := replay(ctx, tx, es, req.IdempotencyKey, samePayment(&req.BookingID, store.PaymentDirectionRefund, req.Method, req.Amount)); err != nil || r != nil {
		return r, err
	}
	if req.RefuseOpenRequests {
		// Under the booking lock taken by LoadForPayment (INV-75).
		if err := CheckNoOpenRefundRequest(ctx, tx, es, b.ID); err != nil {
			return nil, err
		}
	}
	info, err := s.establishment(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	return s.refundInTx(ctx, tx, p, ip, es, b, info, req)
}

// refundInTx records a manual refund or a credit of the (locked) booking b:
// the money is handed back at the desk (or credited), so the row is
// recorded as succeeded. INV-24: the refunds of a booking never exceed its
// payments, and the refunds of one payment never exceed that payment; both
// caps count the provider refunds still in flight (refund intents created
// or pending at the provider: an overpayment or late-payment refund the
// gateway scheduled), so a credit and a provider refund never add up above
// what was paid. The booking lock held by the caller serialises this path
// with gateway.reserveRefund, which locks the booking after the intent.
func (s *Service) refundInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, b *store.Booking, info *estInfo, req RefundRequest) (*Result, error) {
	// M5B: what was paid with customer credit goes back as credit (credit.go).
	if credited, done, err := s.splitCreditRefund(ctx, tx, p, ip, es, b, info, &req); err != nil || done {
		return credited, err
	}
	totals, err := store.BookingPaymentTotals(ctx, tx, es, b.ID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	inFlight, err := store.SumOpenRefundIntents(ctx, tx, es, "", b.ID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	if left := totals.Net() - inFlight; req.Amount > left {
		return nil, configerr.Invalid("amount", fmt.Sprintf("Il rimborso supera quanto pagato (massimo %s).", euro(max(left, 0))))
	}
	weights := rateWeights(b.CurrentPrice())
	var refundOf *string
	if req.PaymentID != "" {
		orig, err := store.GetPayment(ctx, tx, es, req.PaymentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrPaymentNotFound
			}
			return nil, fmt.Errorf("payment: %w", err)
		}
		if orig.BookingID == nil || *orig.BookingID != b.ID || orig.Direction != store.PaymentDirectionIn || orig.Status != store.PaymentSucceeded {
			return nil, configerr.Invalid("payment_id", "Il pagamento scelto non è un incasso di questa prenotazione.")
		}
		done, err := store.RefundedOfPayment(ctx, tx, es, orig.ID)
		if err != nil {
			return nil, fmt.Errorf("payment: %w", err)
		}
		if pi, err := store.GetPaymentIntentByPaymentID(ctx, tx, es, orig.ID); err == nil {
			open, err := store.SumOpenRefundIntents(ctx, tx, es, pi.ID, "")
			if err != nil {
				return nil, fmt.Errorf("payment: %w", err)
			}
			done += open
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("payment: %w", err)
		}
		if done+req.Amount > orig.Amount {
			return nil, configerr.Invalid("amount", fmt.Sprintf("Il rimborso supera il pagamento scelto (residuo %s).", euro(max(orig.Amount-done, 0))))
		}
		if len(orig.VATBreakdown) > 0 {
			weights = orig.VATBreakdown
		}
		refundOf = &orig.ID
	}
	session, err := s.sessionFor(ctx, tx, es, req.CashSessionID, req.Method)
	if err != nil {
		return nil, err
	}
	pp, err := s.manual()
	if err != nil {
		return nil, err
	}
	now := s.now()
	desc := "Rimborso prenotazione " + b.Code
	if req.As == RefundAsCredit {
		desc = "Credito cliente da prenotazione " + b.Code
	}
	pm := &store.Payment{
		ID: store.NewID(), BookingID: &b.ID, CustomerID: optional(b.CustomerID), Provider: pp.Code(),
		Method: req.Method, Direction: store.PaymentDirectionRefund, Amount: req.Amount, Status: store.PaymentSucceeded,
		IdempotencyKey: optional(req.IdempotencyKey), CashSessionID: session, RefundOfPaymentID: refundOf,
		ReceivedByUserID: userIDOf(p), Description: desc, Note: req.Reason,
		VATBreakdown: SplitByWeights(weights, req.Amount, info.defaultVAT), BusinessDate: info.today,
		Metadata: map[string]any{"settlement": string(req.As), "manual_refund": true}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.insert(ctx, tx, es, pm); err != nil {
		return nil, err
	}
	if req.As == RefundAsCredit {
		if err := s.grantCredit(ctx, tx, p, es, b.CustomerID, req.Amount, req.CreditSource, req.CreditExpiresOn, pm, req.Reason); err != nil {
			return nil, err
		}
	}
	updated, err := s.d.Bookings.ApplyPayment(ctx, tx, es, b.ID, -req.Amount)
	if err != nil {
		return nil, fmt.Errorf("payment: apply to booking: %w", err)
	}
	if err := s.record(ctx, tx, p, ip, es, "payment.refunded", event.PaymentRefunded, pm, map[string]any{"settlement": string(req.As)}); err != nil {
		return nil, err
	}
	return &Result{Payment: *pm, Booking: updated}, nil
}

// ---- counter sales ------------------------------------------------------------

// RecordCounterSale records a desk sale without a booking (spec 14.1: an
// extra sunbed on the fly, a daily ticket for the generic "Cliente cassa"),
// auth.PermCashOperate. The VAT rate is the given one or the establishment
// default; method credit requires a customer with enough credit.
func (s *Service) RecordCounterSale(ctx context.Context, p *auth.Principal, ip, establishmentID string, in CounterSaleInput) (*Result, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	in.Description = strings.TrimSpace(in.Description)
	var ve configerr.ValidationError
	validateAmount("amount", in.Amount, &ve)
	if !methods[in.Method] {
		ve.Add("method", "Metodo di pagamento non valido.")
	}
	if in.Description == "" {
		ve.Add("description", "Indica cosa è stato venduto.")
	} else if len([]rune(in.Description)) > maxDescription {
		ve.Add("description", fmt.Sprintf("Massimo %d caratteri.", maxDescription))
	}
	if in.VATRateBP != nil && (*in.VATRateBP < 0 || *in.VATRateBP > 10000) {
		ve.Add("vat_rate_bp", "Aliquota in punti base tra 0 e 10000.")
	}
	if in.Method == store.PaymentMethodCredit && in.CustomerID == "" {
		ve.Add("customer_id", "Il pagamento con credito richiede il cliente.")
	}
	validateKey(in.IdempotencyKey, &ve)
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	want := samePayment(nil, store.PaymentDirectionIn, in.Method, in.Amount)
	return s.runIdempotent(ctx, es, in.IdempotencyKey, want, func(ctx context.Context, tx bun.Tx) (*Result, error) {
		if r, err := replay(ctx, tx, es, in.IdempotencyKey, want); err != nil || r != nil {
			return r, err
		}
		if in.CustomerID != "" {
			ok, err := store.PaymentCustomerExists(ctx, tx, es, in.CustomerID)
			if err != nil {
				return nil, fmt.Errorf("payment: %w", err)
			}
			if !ok {
				return nil, ErrCustomerNotFound
			}
		}
		info, err := s.establishment(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		rate := info.defaultVAT
		if in.VATRateBP != nil {
			rate = *in.VATRateBP
		}
		session, err := s.sessionFor(ctx, tx, es, in.CashSessionID, in.Method)
		if err != nil {
			return nil, err
		}
		pp, err := s.manual()
		if err != nil {
			return nil, err
		}
		intent, err := pp.CreatePayment(ctx, ext.CreatePaymentRequest{
			EstablishmentID: es.ID(), Amount: int64(in.Amount), Currency: info.est.Currency,
			Description: in.Description, IdempotencyKey: in.IdempotencyKey, PreferredMode: ext.PaymentModeNone,
		})
		if err != nil {
			return nil, fmt.Errorf("payment: manual driver: %w", err)
		}
		now := s.now()
		pm := &store.Payment{
			ID: store.NewID(), CustomerID: optional(in.CustomerID), Provider: pp.Code(), Method: in.Method,
			Direction: store.PaymentDirectionIn, Amount: in.Amount, Status: store.PaymentSucceeded,
			ProviderRef: optional(intent.ProviderRef), IdempotencyKey: optional(in.IdempotencyKey),
			CashSessionID: session, ReceivedByUserID: userIDOf(p), Description: in.Description,
			VATBreakdown: FlatSplit(in.Amount, rate), BusinessDate: info.today,
			Metadata: map[string]any{"counter_sale": true}, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.insert(ctx, tx, es, pm); err != nil {
			return nil, err
		}
		if in.Method == store.PaymentMethodCredit {
			if err := s.redeemCredit(ctx, tx, p, es, info, in.CustomerID, in.Amount, pm); err != nil {
				return nil, err
			}
		}
		if err := s.record(ctx, tx, p, ip, es, "payment.counter_sale", event.PaymentSucceeded, pm, nil); err != nil {
			return nil, err
		}
		return &Result{Payment: *pm}, nil
	})
}

// ---- reads --------------------------------------------------------------------

// ListByBooking returns the payments and refunds of a booking, oldest first.
// Implements booking.Payments (no permission check: the caller has done it).
func (s *Service) ListByBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) ([]store.Payment, error) {
	items, err := store.ListBookingPayments(ctx, db, es, bookingID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	return items, nil
}

// BookingPayments is the API read of the ledger of a booking
// (auth.PermBookingsRead); ErrBookingNotFound for unknown bookings.
func (s *Service) BookingPayments(ctx context.Context, p *auth.Principal, establishmentID, bookingID string) ([]store.Payment, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	ok, err := store.PaymentBookingExists(ctx, s.d.DB, es, bookingID)
	if err != nil {
		return nil, fmt.Errorf("payment: %w", err)
	}
	if !ok {
		return nil, ErrBookingNotFound
	}
	return s.ListByBooking(ctx, s.d.DB, es, bookingID)
}

// List lists the payments of the establishment, newest first
// (auth.PermBookingsRead).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f store.PaymentFilter, cursor string, limit int) (store.Page[store.PaymentWithBooking], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.PaymentWithBooking]{}, err
	}
	var ve configerr.ValidationError
	if f.Method != "" && !methods[f.Method] {
		ve.Add("method", "Metodo di pagamento non valido.")
	}
	if f.Direction != "" && f.Direction != store.PaymentDirectionIn && f.Direction != store.PaymentDirectionRefund {
		ve.Add("direction", "Valori ammessi: in, refund.")
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		ve.Add("to", "La data finale non può precedere quella iniziale.")
	}
	if err := ve.OrNil(); err != nil {
		return store.Page[store.PaymentWithBooking]{}, err
	}
	page, err := store.ListPayments(ctx, s.d.DB, es, f, cursor, limit)
	if err != nil {
		return page, fmt.Errorf("payment: %w", err)
	}
	return page, nil
}
