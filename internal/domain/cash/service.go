package cash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
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

// Sentinel errors of the cash domain (mapped by internal/http/problem).
var (
	// ErrSessionClosed -> 409 cash_session_closed.
	ErrSessionClosed = errors.New("cash: session closed")
	// ErrSessionAlreadyOpen -> 409 cash_session_already_open.
	ErrSessionAlreadyOpen = errors.New("cash: a session is already open on this register")
	// ErrNotFound: unknown session (404).
	ErrNotFound = fmt.Errorf("%w: cash session", configerr.ErrNotFound)
	// ErrEstablishmentNotFound: unknown or out-of-scope establishment (404).
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
)

// Limits of the inputs.
const (
	maxNotes  = 500
	maxReason = 200
	// MaxAmount bounds floats, movements and counted amounts (1,000,000.00 EUR).
	MaxAmount money.Cents = 100_000_000
)

var registerRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// knownMethods are the payment methods accepted as keys of counted totals.
var knownMethods = map[string]bool{
	store.PaymentMethodCash: true, store.PaymentMethodCard: true, store.PaymentMethodBankTransfer: true,
	store.PaymentMethodWallet: true, store.PaymentMethodCredit: true, store.PaymentMethodOther: true,
}

// Deps are the dependencies of the cash service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the cash desk use cases.
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

// authorize resolves the establishment and checks that the principal holds
// at least one of perms (the first failure is reported otherwise).
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perms ...auth.Permission) (store.EstablishmentScope, error) {
	var first error
	for _, perm := range perms {
		es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
		if err == nil {
			return es, nil
		}
		if errors.Is(err, store.ErrNotFound) {
			return es, ErrEstablishmentNotFound
		}
		if !errors.Is(err, auth.ErrForbidden) {
			return es, err
		}
		if first == nil {
			first = err
		}
	}
	return store.EstablishmentScope{}, first
}

func userIDOf(p *auth.Principal) *string {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil
	}
	id := p.UserID
	return &id
}

// ---- payment.CashDesk ---------------------------------------------------------

// SessionForPayment implements payment.CashDesk in the payment transaction:
// with sessionID it locks that session (ErrNotFound when unknown,
// ErrSessionClosed when closed); otherwise it locks the open session of
// store.DefaultCashRegister. Method cash without an open session fails with
// ErrSessionClosed; other methods return nil when no session is open. The
// row lock makes a concurrent Close wait, so a payment never lands in a
// session whose expected totals have already been frozen (INV-25).
func (s *Service) SessionForPayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, sessionID *string, method string) (*string, error) {
	if sessionID != nil && *sessionID != "" {
		if err := s.lockOpen(ctx, tx, es, *sessionID); err != nil {
			return nil, err
		}
		id := *sessionID
		return &id, nil
	}
	cs, err := store.GetOpenCashSession(ctx, tx, es, store.DefaultCashRegister)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if method == store.PaymentMethodCash {
			return nil, ErrSessionClosed
		}
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("cash: %w", err)
	}
	if err := s.lockOpen(ctx, tx, es, cs.ID); err != nil {
		if errors.Is(err, ErrSessionClosed) && method != store.PaymentMethodCash {
			return nil, nil // closed meanwhile: card & co. do not need a session
		}
		return nil, err
	}
	return &cs.ID, nil
}

// lockOpen locks an open session or explains why it cannot.
func (s *Service) lockOpen(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string) error {
	ok, err := store.LockOpenCashSession(ctx, tx, es, id)
	if err != nil {
		return fmt.Errorf("cash: %w", err)
	}
	if ok {
		return nil
	}
	if _, err := store.GetCashSession(ctx, tx, es, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("cash: %w", err)
	}
	return ErrSessionClosed
}

// ---- open ---------------------------------------------------------------------

// OpenInput opens a session.
type OpenInput struct {
	// RegisterID is the register to open; when empty, Register (code,
	// deprecated) and then the default register are used.
	RegisterID   string
	Register     string // default store.DefaultCashRegister
	OpeningFloat money.Cents
	Notes        string
}

// Open opens a session on a register with its float (auth.PermCashOperate).
// The register must exist and be active (validation error otherwise). A
// second open session on the same register is ErrSessionAlreadyOpen, also
// under concurrency (unique partial index cash_sessions_register_open_uidx,
// INV-73); sessions of different registers are independent.
func (s *Service) Open(ctx context.Context, p *auth.Principal, ip, establishmentID string, in OpenInput) (*store.CashSession, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	in.Register = strings.TrimSpace(in.Register)
	in.RegisterID = strings.TrimSpace(in.RegisterID)
	if in.Register == "" {
		in.Register = store.DefaultCashRegister
	}
	in.Notes = strings.TrimSpace(in.Notes)
	var ve configerr.ValidationError
	if !registerRe.MatchString(in.Register) {
		ve.Add("register", "Codice cassa: lettere minuscole, cifre, - e _ (massimo 40).")
	}
	if in.OpeningFloat < 0 || in.OpeningFloat > MaxAmount {
		ve.Add("opening_float", "Il fondo cassa deve essere tra 0 e 1.000.000,00.")
	}
	if len([]rune(in.Notes)) > maxNotes {
		ve.Add("notes", fmt.Sprintf("Massimo %d caratteri.", maxNotes))
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	reg, err := s.resolveRegister(ctx, s.d.DB, es, in.RegisterID, in.Register)
	if err != nil {
		return nil, err
	}
	if !reg.Active {
		return nil, configerr.Invalid("register_id", "La cassa è disattivata.")
	}
	in.Register = reg.Code
	now := s.now()
	regID := reg.ID
	cs := &store.CashSession{
		ID: store.NewID(), Register: reg.Code, RegisterID: &regID, RegisterName: reg.Name, OpenedBy: userIDOf(p), OpenedAt: now,
		OpeningFloat: in.OpeningFloat, ExpectedTotals: store.MethodTotals{store.PaymentMethodCash: in.OpeningFloat},
		Notes: in.Notes, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		// Row lock of the register: a concurrent deactivation waits (or has
		// already made the register inactive).
		if ok, err := store.LockActiveCashRegister(ctx, tx, es, regID); err != nil {
			return fmt.Errorf("cash: %w", err)
		} else if !ok {
			return configerr.Invalid("register_id", "La cassa è disattivata.")
		}
		if _, err := store.GetOpenCashSessionOfRegister(ctx, tx, es, regID); err == nil {
			return ErrSessionAlreadyOpen
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("cash: %w", err)
		}
		if err := store.InsertCashSession(ctx, tx, es, cs); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrSessionAlreadyOpen
			}
			return fmt.Errorf("cash: %w", err)
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: event.CashSessionOpened,
			EntityType: "cash_session", EntityID: cs.ID,
			Diff: map[string]any{"register": cs.Register, "register_id": regID, "opening_float": int64(cs.OpeningFloat)},
		}); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		_, err := s.d.Outbox.Write(ctx, tx, event.CashSessionOpened, es.ID(), 1, event.CashChange{
			CashSessionID: cs.ID, Register: cs.Register, Status: StatusOpen,
		})
		return err
	})
	if err != nil {
		// PostgreSQL aborts the loser of a concurrent open with a unique
		// violation that may surface from the commit.
		if store.IsUniqueViolation(err) {
			return nil, ErrSessionAlreadyOpen
		}
		return nil, err
	}
	return cs, nil
}

// Session statuses (api.CashSessionStatus).
const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)

// StatusOf is the status of a session.
func StatusOf(cs *store.CashSession) string {
	if cs.ClosedAt == nil {
		return StatusOpen
	}
	return StatusClosed
}

// ---- reads --------------------------------------------------------------------

// Detail is a session with its movements, its payments and the Z-style
// summary (spec 14.1).
type Detail struct {
	Session   store.CashSession
	Movements []store.CashMovement
	Payments  []store.Payment
	Summary   Summary
}

// Current returns the open session of register (default main), or
// ErrNotFound. Requires auth.PermCashOperate or auth.PermReportsRead.
func (s *Service) Current(ctx context.Context, p *auth.Principal, establishmentID, register string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	if register == "" {
		register = store.DefaultCashRegister
	}
	cs, err := store.GetOpenCashSession(ctx, s.d.DB, es, register)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("cash: %w", err)
	}
	return s.detail(ctx, s.d.DB, es, cs)
}

// CurrentOfRegister returns the open session of the register with id
// registerID, or ErrNotFound (also for an unknown register). Same
// permissions as Current.
func (s *Service) CurrentOfRegister(ctx context.Context, p *auth.Principal, establishmentID, registerID string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	cs, err := store.GetOpenCashSessionOfRegister(ctx, s.d.DB, es, registerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("cash: %w", err)
	}
	return s.detail(ctx, s.d.DB, es, cs)
}

// Get returns a session with its summary. Requires auth.PermCashOperate or
// auth.PermReportsRead.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	cs, err := store.GetCashSession(ctx, s.d.DB, es, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("cash: %w", err)
	}
	return s.detail(ctx, s.d.DB, es, cs)
}

// detail loads movements and payments and computes the summary. For an open
// session ExpectedTotals is the live value (the stored one is frozen only
// on close).
func (s *Service) detail(ctx context.Context, db bun.IDB, es store.EstablishmentScope, cs *store.CashSession) (*Detail, error) {
	movs, err := store.ListCashMovements(ctx, db, es, cs.ID)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	pays, err := store.ListSessionPayments(ctx, db, es, cs.ID)
	if err != nil {
		return nil, fmt.Errorf("cash: %w", err)
	}
	sum := Summarize(cs.OpeningFloat, pays, movs, cs.CountedTotals)
	if cs.ClosedAt == nil {
		cs.ExpectedTotals = sum.Expected
	}
	if cs.RegisterID != nil && cs.RegisterName == "" {
		if r, err := store.GetCashRegister(ctx, db, es, *cs.RegisterID); err == nil {
			cs.RegisterName = r.Name
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("cash: %w", err)
		}
	}
	return &Detail{Session: *cs, Movements: movs, Payments: pays, Summary: sum}, nil
}

// ListFilter narrows List.
type ListFilter struct {
	Status     string // "", StatusOpen, StatusClosed
	Register   string
	RegisterID string
	From       *calendar.Date // opened on or after (establishment timezone)
	To         *calendar.Date // opened on or before
}

// List returns the session history, newest first. Open sessions carry their
// live expected totals. Requires auth.PermCashOperate or auth.PermReportsRead.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f ListFilter, cursor string, limit int) (store.Page[store.CashSession], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead)
	if err != nil {
		return store.Page[store.CashSession]{}, err
	}
	var sf store.CashSessionFilter
	switch f.Status {
	case "":
	case StatusOpen, StatusClosed:
		open := f.Status == StatusOpen
		sf.Open = &open
	default:
		return store.Page[store.CashSession]{}, configerr.Invalid("status", "Valori ammessi: open, closed.")
	}
	sf.Register = f.Register
	sf.RegisterID = f.RegisterID
	if f.From != nil || f.To != nil {
		loc, err := s.location(ctx, es)
		if err != nil {
			return store.Page[store.CashSession]{}, err
		}
		if f.From != nil {
			t := startOfDay(*f.From, loc)
			sf.OpenedFrom = &t
		}
		if f.To != nil {
			t := startOfDay(f.To.AddDays(1), loc)
			sf.OpenedBefore = &t
		}
	}
	page, err := store.ListCashSessions(ctx, s.d.DB, es, sf, cursor, limit)
	if err != nil {
		return page, fmt.Errorf("cash: %w", err)
	}
	names, err := s.registerNames(ctx, s.d.DB, es)
	if err != nil {
		return page, err
	}
	for i := range page.Items {
		cs := &page.Items[i]
		nameSession(cs, names)
		if cs.ClosedAt != nil {
			continue
		}
		d, err := s.detail(ctx, s.d.DB, es, cs)
		if err != nil {
			return page, err
		}
		cs.ExpectedTotals = d.Summary.Expected
	}
	return page, nil
}

func (s *Service) location(ctx context.Context, es store.EstablishmentScope) (*time.Location, error) {
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("cash: load establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("cash: timezone %q: %w", est.Timezone, err)
	}
	return loc, nil
}

func startOfDay(d calendar.Date, loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).UTC()
}

// ---- movements ----------------------------------------------------------------

// MovementInput is a cash deposit (in) or withdrawal (out).
type MovementInput struct {
	Kind   string // store.CashMovementIn | store.CashMovementOut
	Amount money.Cents
	Reason string
}

// AddMovement records a movement on an open session (auth.PermCashOperate):
// ErrSessionClosed on a closed session; a withdrawal larger than the cash
// expected in the drawer is a validation error.
func (s *Service) AddMovement(ctx context.Context, p *auth.Principal, ip, establishmentID, sessionID string, in MovementInput) (*store.CashMovement, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	var ve configerr.ValidationError
	if in.Kind != store.CashMovementIn && in.Kind != store.CashMovementOut {
		ve.Add("kind", "Valori ammessi: in, out.")
	}
	if in.Amount <= 0 || in.Amount > MaxAmount {
		ve.Add("amount", "L'importo deve essere positivo (massimo 1.000.000,00).")
	}
	if in.Reason == "" {
		ve.Add("reason", "Indica la causale del movimento.")
	} else if len([]rune(in.Reason)) > maxReason {
		ve.Add("reason", fmt.Sprintf("Massimo %d caratteri.", maxReason))
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	now := s.now()
	m := &store.CashMovement{
		ID: store.NewID(), CashSessionID: sessionID, Kind: in.Kind, Amount: in.Amount, Reason: in.Reason,
		CreatedByUserID: userIDOf(p), CreatedAt: now,
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := s.lockOpen(ctx, tx, es, sessionID); err != nil {
			return err
		}
		cs, err := store.GetCashSession(ctx, tx, es, sessionID)
		if err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if in.Kind == store.CashMovementOut {
			d, err := s.detail(ctx, tx, es, cs)
			if err != nil {
				return err
			}
			if avail := d.Summary.Expected[store.PaymentMethodCash]; in.Amount > avail {
				return configerr.Invalid("amount", fmt.Sprintf("Il prelievo supera il denaro atteso in cassa (%s).", euro(avail)))
			}
		}
		if err := store.InsertCashMovement(ctx, tx, es, m); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: event.CashMovementRecorded,
			EntityType: "cash_session", EntityID: cs.ID,
			Diff: map[string]any{"movement_id": m.ID, "kind": m.Kind, "amount": int64(m.Amount), "reason": m.Reason},
		}); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		_, err = s.d.Outbox.Write(ctx, tx, event.CashMovementRecorded, es.ID(), 1, event.CashChange{
			CashSessionID: cs.ID, Register: cs.Register, Status: StatusOpen, MovementID: m.ID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ---- close --------------------------------------------------------------------

// CloseInput closes a session with the counted amounts per method.
type CloseInput struct {
	Counted store.MethodTotals
	Notes   string
}

// Close closes an open session (auth.PermCashOperate): it locks the row,
// computes the expected totals (float + session takings +/- movements),
// stores counted, expected and difference = sum(counted) - sum(expected)
// with a conditional UPDATE (still open, version = ifMatch when given) and
// writes the audit log and cash_session.closed (INV-25). Methods missing
// from Counted count as 0.
func (s *Service) Close(ctx context.Context, p *auth.Principal, ip, establishmentID, sessionID string, ifMatch *int64, in CloseInput) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	in.Notes = strings.TrimSpace(in.Notes)
	var ve configerr.ValidationError
	for m, v := range in.Counted {
		if !knownMethods[m] {
			ve.Add("counted_totals."+m, "Metodo di pagamento sconosciuto.")
		} else if v < 0 || v > MaxAmount {
			ve.Add("counted_totals."+m, "L'importo contato deve essere tra 0 e 1.000.000,00.")
		}
	}
	if len([]rune(in.Notes)) > maxNotes {
		ve.Add("notes", fmt.Sprintf("Massimo %d caratteri.", maxNotes))
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := s.lockOpen(ctx, tx, es, sessionID); err != nil {
			return err
		}
		cs, err := store.GetCashSession(ctx, tx, es, sessionID)
		if err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if err := configerr.CheckVersion(ifMatch, cs.Version); err != nil {
			return err
		}
		counted := store.MethodTotals{}
		for m, v := range in.Counted {
			counted[m] = v
		}
		d, err := s.detail(ctx, tx, es, cs)
		if err != nil {
			return err
		}
		sum := Summarize(cs.OpeningFloat, d.Payments, d.Movements, counted)
		now := s.now()
		old := cs.Version
		diff := sum.Difference()
		cs.ClosedAt, cs.ClosedBy = &now, userIDOf(p)
		cs.ExpectedTotals, cs.CountedTotals, cs.Difference = sum.Expected, sum.Counted, &diff
		if in.Notes != "" {
			if cs.Notes != "" {
				cs.Notes += "\n"
			}
			cs.Notes += in.Notes
		}
		cs.Version, cs.UpdatedAt = old+1, now
		ok, err := store.CloseCashSession(ctx, tx, es, cs, old)
		if err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		if !ok { // impossible while the row lock is held; kept as a guard
			return &configerr.VersionConflictError{Current: old}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: event.CashSessionClosed,
			EntityType: "cash_session", EntityID: cs.ID,
			Diff: map[string]any{
				"expected_totals": cents(sum.Expected), "counted_totals": cents(sum.Counted), "difference": int64(diff),
			},
		}); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		d64 := int64(diff)
		if _, err := s.d.Outbox.Write(ctx, tx, event.CashSessionClosed, es.ID(), 1, event.CashChange{
			CashSessionID: cs.ID, Register: cs.Register, Status: StatusClosed, Difference: &d64,
			Expected: cents(sum.Expected), Counted: cents(sum.Counted),
		}); err != nil {
			return fmt.Errorf("cash: %w", err)
		}
		out = &Detail{Session: *cs, Movements: d.Movements, Payments: d.Payments, Summary: sum}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func cents(t store.MethodTotals) map[string]int64 {
	out := make(map[string]int64, len(t))
	for k, v := range t {
		out[k] = int64(v)
	}
	return out
}

func euro(c money.Cents) string {
	v := int64(c)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d,%02d €", sign, v/100, v%100)
}
