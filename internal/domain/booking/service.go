package booking

import (
	"context"
	"encoding/json"
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
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// DefaultHoldTTL is the booking hold duration when Deps.HoldTTL is zero.
const DefaultHoldTTL = 10 * time.Minute

// DefaultApprovalTTL is the duration of the long hold of a booking awaiting
// the manager's approval (spec 10.2) when Deps.ApprovalTTL is zero.
const DefaultApprovalTTL = 72 * time.Hour

// Page sizes of the booking list.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// maxCodeAttempts bounds the retries of the booking code allocation.
const maxCodeAttempts = 50

// Deps are the dependencies of the booking service. Every field is required
// except Log, HoldTTL, ApprovalTTL and Releases (nil: no season resale
// tracking).
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Inventory Inventory
	Pricer    Pricer
	Customers Customers
	Payments  Payments
	Releases  Releases
	// ExpiredRefunds refunds the online money left on a booking whose hold
	// expired (optional, see ExpiredRefunder).
	ExpiredRefunds ExpiredRefunder
	// CancelRefunds reserves the guaranteed provider refund of a
	// cancellation (optional, see CancelRefunds).
	CancelRefunds CancelRefunds
	// HoldTTL is ARENILE_HOLD_TTL (default DefaultHoldTTL).
	HoldTTL time.Duration
	// ApprovalTTL is the hold of bookings awaiting approval (default DefaultApprovalTTL).
	ApprovalTTL time.Duration
}

// Service implements the booking use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.HoldTTL <= 0 {
		d.HoldTTL = DefaultHoldTTL
	}
	if d.ApprovalTTL <= 0 {
		d.ApprovalTTL = DefaultApprovalTTL
	}
	return &Service{d: d}
}

// ExpiredRefunder is the OPTIONAL payment-side port of a hold that expires
// with money on the booking (INV-51): a provider payment below the
// confirmation threshold (the total changed after the payment started) or
// a partial payment. It schedules the refunds in tx (the booking is
// locked, audit and outbox of the expiry are written in the same
// transaction); the provider is called later. Implemented by
// *gateway.Service (internal/app wires it); without it the money stays on
// the expired booking for a manual refund.
type ExpiredRefunder interface {
	RefundExpiredBooking(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) error
}

// SetExpiredRefunder installs the refund path of expired holds after
// construction (the gateway is built after the booking service).
func (s *Service) SetExpiredRefunder(r ExpiredRefunder) { s.d.ExpiredRefunds = r }

// CancelRefunds is the OPTIONAL refund-side port of a cancellation that
// gives money back to an online payment (ADR 0019 feature B): it inserts
// the refund requests in tx (the cancellation transaction, booking
// locked) for up to amount on the provider payments of b and returns the
// reserved amount and the request ids; a job sends them until the
// provider confirms. Implemented by *refund.Service (internal/app wires
// it); without it the money refund stays due (refund_due).
type CancelRefunds interface {
	ReserveCancellationRefund(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, amount money.Cents, reason string, createdBy *string) (money.Cents, []string, error)
}

// SetCancelRefunds installs the guaranteed refund path after construction.
func (s *Service) SetCancelRefunds(r CancelRefunds) { s.d.CancelRefunds = r }

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

// require checks perm on an already resolved establishment.
func require(p *auth.Principal, es store.EstablishmentScope, perm auth.Permission) error {
	if p == nil {
		return auth.ErrUnauthenticated
	}
	if !p.Can(es, perm) {
		return auth.ErrForbidden
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// ---- create ---------------------------------------------------------------------------

// Create creates a booking from the panel in one transaction (spec 10, 11.3):
// customer (existing or inline), price and policy, allocation (or panel hold
// conversion), rows, audit and outbox. Requires auth.PermBookingsWrite.
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		out, err = s.CreateInTx(ctx, tx, p, es, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateInTx creates a booking inside the caller's transaction (used by the
// pass service for season passes and by Create). It authorizes
// auth.PermBookingsWrite, resolves the customer, prices and validates the
// policy (Pricer), allocates (Inventory), writes the rows, the audit log and
// event.BookingCreated (+ BookingConfirmed / BookingHeld /
// BookingApprovalRequested). Implements pass.Bookings.
func (s *Service) CreateInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in CreateInput) (*Detail, error) {
	if err := require(p, es, auth.PermBookingsWrite); err != nil {
		return nil, err
	}
	in.Channel = auth.ChannelFor(p, in.Channel) // API keys always sell on "api" (ADR 0011)
	if err := validateCreate(&in); err != nil {
		return nil, err
	}
	if in.CustomerID == "" && in.Customer == nil {
		return nil, configerr.Invalid("customer_id", "Indica il cliente o i suoi dati.")
	}
	if in.CustomerID != "" && in.Customer != nil {
		return nil, configerr.Invalid("customer", "Indica il cliente esistente oppure i dati di un nuovo cliente, non entrambi.")
	}
	cust, err := s.d.Customers.Resolve(ctx, tx, p, es, in.CustomerID, in.Customer)
	if err != nil {
		return nil, err
	}
	var createdBy *string
	if p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		createdBy = &u
	}
	return s.createWith(ctx, tx, p.Actor(reqctx.From(ctx).ClientIP), createdBy, es, cust, in)
}

// createWith is the body of a creation once the actor and the customer are
// known (shared by the panel, CreateInTx, and the public channel,
// CreateOnline): catalog, price and policy, status, rows, promo,
// allocation, audit and outbox in tx.
func (s *Service) createWith(ctx context.Context, tx bun.IDB, actor audit.Actor, createdBy *string, es store.EstablishmentScope, cust *store.Customer, in CreateInput) (*Detail, error) {
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("booking: establishment: %w", err)
	}
	cat, err := loadCatalog(ctx, tx, es, in.Lines, in.Extras)
	if err != nil {
		return nil, err
	}
	lines, err := cat.resolve(in.Lines, "lines")
	if err != nil {
		return nil, err
	}
	st := stay{StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula, Channel: in.Channel, SeasonID: in.SeasonID}
	agreement := in.AgreementID
	if agreement == "" && cust.AgreementID != nil {
		agreement = *cust.AgreementID
	}
	pr, err := s.price(ctx, tx, es, st, lines, in.Extras, cust.Tags, agreement, in.PromoCode, cat)
	if err != nil {
		return nil, err
	}
	if len(pr.res.Violations) > 0 && !in.Force {
		return nil, &policy.ViolationError{Violations: pr.res.Violations}
	}
	now := s.now()
	holdTTL := s.d.HoldTTL
	if in.HoldTTL > 0 {
		holdTTL = in.HoldTTL
	}
	state, err := DecideInitialState(StateRequest{
		Requested: in.Status, ApprovalRequired: pr.approval, SeasonPass: in.IsSeasonPass, Kind: in.AllocationKind,
		HoldTTL: holdTTL, ApprovalTTL: s.d.ApprovalTTL,
	})
	if err != nil {
		return nil, configerr.Invalid("status", "Stato iniziale non ammesso: usa draft, held o confirmed.")
	}

	b := &store.Booking{
		ID: store.NewID(), CustomerID: cust.ID, Channel: st.Channel, Status: string(state.Status),
		StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula,
		IsSeasonPass: in.IsSeasonPass, TotalAmount: pr.res.Breakdown.Total, DepositDue: pr.res.DepositDue,
		NotesCustomer: strings.TrimSpace(in.NotesCustomer), NotesInternal: strings.TrimSpace(in.NotesInternal),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if pr.res.SeasonID != "" {
		sid := pr.res.SeasonID
		b.SeasonID = &sid
	}
	if in.RenewalOfBookingID != "" {
		r := in.RenewalOfBookingID
		b.RenewalOfBookingID = &r
	}
	if agreement != "" {
		a := agreement
		b.AgreementID = &a
	}
	b.CreatedByUserID = createdBy
	if state.Status == StatusConfirmed {
		b.ConfirmedAt = &now
	}
	var expiresAt *time.Time
	if state.Kind == inventory.KindHold {
		e := now.Add(state.HoldTTL)
		expiresAt = &e
		b.HoldExpiresAt = &e
	}
	if in.HoldID != "" {
		h := in.HoldID
		b.HoldID = &h
	}
	if err := setSnapshots(b, pr, st.Channel, now); err != nil {
		return nil, err
	}
	if err := s.insertWithCode(ctx, tx, es, b, CodeSeries(CodePrefix(est.Slug), in.StartsOn.Year())); err != nil {
		return nil, err
	}
	rows, extras := buildRows(b.ID, pr, cat, now)
	if err := store.InsertBookingLines(ctx, tx, es, rows); err != nil {
		return nil, fmt.Errorf("booking: %w", err)
	}
	if err := store.InsertBookingLineExtras(ctx, tx, es, extras); err != nil {
		return nil, fmt.Errorf("booking: %w", err)
	}
	if err := s.consumePromo(ctx, tx, es, b, in.PromoCode, now); err != nil {
		return nil, err
	}

	var alloc *inventory.Allocation
	if state.Kind != "" {
		req := allocRequest(b, state.Kind, expiresAt, rows, extras, cat)
		if in.HoldID != "" {
			alloc, err = s.d.Inventory.ConvertHold(ctx, tx, es, inventory.ConvertHoldRequest{
				HoldID: in.HoldID, BookingID: b.ID, Kind: state.Kind, ExpiresAt: expiresAt, Lines: req.Lines,
			})
		} else {
			alloc, err = s.d.Inventory.Allocate(ctx, tx, es, req)
		}
		if err != nil {
			return nil, err
		}
		if err := s.applyAllocation(ctx, tx, es, b, rows, alloc, state.Kind, now); err != nil {
			return nil, err
		}
	}

	diff := map[string]any{
		"code": b.Code, "status": b.Status, "channel": b.Channel, "starts_on": b.StartsOn.String(), "ends_on": b.EndsOn.String(),
		"slots": string(b.Slots), "total_amount": int64(b.TotalAmount), "deposit_due": int64(b.DepositDue), "lines": len(rows),
	}
	if in.Force && len(pr.res.Violations) > 0 {
		diff["forced"] = true
		diff["violations"] = pr.res.Violations
	}
	if in.HoldID != "" {
		diff["hold_id"] = in.HoldID
	}
	if in.IsSeasonPass {
		diff["season_pass"] = true
	}
	if err := s.recordAs(ctx, tx, actor, es, b, eventCreated, eventCreated, "", diff, alloc); err != nil {
		return nil, err
	}
	if state.Event != "" {
		if err := s.emit(ctx, tx, es, b, state.Event, "", alloc); err != nil {
			return nil, err
		}
	}
	return s.loadDetail(ctx, tx, es, b)
}

func validateCreate(in *CreateInput) error {
	v := &configerr.ValidationError{}
	if in.Channel == "" {
		in.Channel = store.ChannelDesk
	}
	switch in.Channel {
	case store.ChannelDesk, store.ChannelOnline, store.ChannelAPI:
	default:
		v.Add("channel", "Canale sconosciuto.")
	}
	validateStay(v, in.StartsOn, in.EndsOn, in.Slots, in.Formula)
	if len(in.Lines) == 0 {
		v.Add("lines", "Almeno una riga.")
	}
	if in.HoldTTL < 0 || in.HoldTTL > 24*time.Hour {
		v.Add("hold_ttl_seconds", "Durata dell'hold non valida (massimo 24 ore).")
	}
	return v.OrNil()
}

func validateStay(v *configerr.ValidationError, from, to calendar.Date, slots calendar.Slots, formula string) {
	if from.IsZero() {
		v.Add("starts_on", "Campo obbligatorio.")
	}
	if to.IsZero() {
		v.Add("ends_on", "Campo obbligatorio.")
	} else if !from.IsZero() && to.Before(from) {
		v.Add("ends_on", "Deve essere successiva o uguale a starts_on.")
	}
	if !from.IsZero() && !to.IsZero() && from.DaysUntil(to) > 366 {
		v.Add("ends_on", "Durata massima un anno.")
	}
	if !slots.Valid() {
		v.Add("slots", "Usa AM, PM o FULL.")
	}
	if formula == "" {
		v.Add("formula", "Campo obbligatorio.")
	}
}

// insertWithCode allocates the next human code of the series (unique per
// establishment) and inserts the booking. Each attempt runs in a savepoint:
// a concurrent creation that took the same code makes the insert fail on the
// unique index, the savepoint rolls back and the next code is tried.
func (s *Service) insertWithCode(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, series string) error {
	// Test mode (spec 15.4, ADR 0019): the switch is read in the creating
	// transaction and copied on the row.
	if !b.Test {
		on, err := store.TestModeEnabled(ctx, tx, es)
		if err != nil {
			return fmt.Errorf("booking: %w", err)
		}
		b.Test = on
	}
	for range maxCodeAttempts {
		last, err := store.LastBookingCode(ctx, tx, es, series)
		if err != nil {
			return fmt.Errorf("booking: %w", err)
		}
		b.Code = NextCode(series, last)
		err = tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
			return store.InsertBooking(ctx, sp, es, b)
		})
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return fmt.Errorf("booking: %w", err)
		}
		return nil
	}
	return ErrCodeExhausted
}

// consumePromo links the promo code used by the price and increments its
// uses with a conditional UPDATE (never beyond max_uses).
func (s *Service) consumePromo(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, code string, now time.Time) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	pc, err := store.GetPromoCodeByCode(ctx, tx, es, code)
	if errors.Is(err, store.ErrNotFound) {
		return configerr.Invalid("promo_code", "Codice promozionale inesistente.")
	}
	if err != nil {
		return fmt.Errorf("booking: promo code: %w", err)
	}
	ok, err := store.ConsumePromoCode(ctx, tx, es, pc.ID, now)
	if err != nil {
		return fmt.Errorf("booking: %w", err)
	}
	if !ok {
		return configerr.Invalid("promo_code", "Codice promozionale esaurito.")
	}
	id := pc.ID
	b.PromoCodeID = &id
	// The row was inserted without the promo link: write it now (same tx).
	_, err = tx.NewUpdate().Model((*store.Booking)(nil)).Set("promo_code_id = ?", id).
		Where("id = ?", b.ID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("booking: link promo code: %w", err)
	}
	return nil
}

// setSnapshots stores the price breakdown and the policy snapshot (INV-21).
func setSnapshots(b *store.Booking, pr *priced, channel string, now time.Time) error {
	bd, err := json.Marshal(pr.res.Breakdown)
	if err != nil {
		return fmt.Errorf("booking: price snapshot: %w", err)
	}
	ps, err := policy.NewSnapshot(pr.res.Policy, policy.Channel(channel), now).Marshal()
	if err != nil {
		return fmt.Errorf("booking: policy snapshot: %w", err)
	}
	b.PriceSnapshot, b.PolicySnapshot = bd, ps
	return nil
}

// applyAllocation writes back the spot and assignment chosen by the
// inventory for each line and tracks the resale of released season days.
func (s *Service) applyAllocation(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, rows []store.BookingLine, alloc *inventory.Allocation, kind inventory.Kind, now time.Time) error {
	if alloc == nil {
		return nil
	}
	byID := make(map[string]*store.BookingLine, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	for _, la := range alloc.Lines {
		l, ok := byID[la.LineID]
		if !ok {
			continue
		}
		changed := false
		if la.AssignmentStatus != "" && la.AssignmentStatus != l.AssignmentStatus {
			l.AssignmentStatus = la.AssignmentStatus
			changed = true
		}
		if la.SpotID != "" && (l.SpotID == nil || *l.SpotID != la.SpotID) {
			sp := la.SpotID
			l.SpotID = &sp
			changed = true
		}
		if changed {
			l.UpdatedAt = now
			if err := store.UpdateBookingLine(ctx, tx, es, l); err != nil {
				return fmt.Errorf("booking: %w", err)
			}
		}
	}
	if s.d.Releases == nil || kind == inventory.KindSeason {
		return nil
	}
	for i := range rows {
		l := &rows[i]
		if l.SpotID == nil || l.AssignmentStatus != store.AssignmentAssigned {
			continue
		}
		if err := s.d.Releases.MarkResold(ctx, tx, es, l.ID, *l.SpotID, calendar.Range(b.StartsOn, b.EndsOn), b.Slots); err != nil {
			return err
		}
	}
	return nil
}

// unmarkResold is the counterpart of applyAllocation when lines release.
func (s *Service) unmarkResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineIDs []string) error {
	if s.d.Releases == nil {
		return nil
	}
	for _, id := range lineIDs {
		if err := s.d.Releases.UnmarkResold(ctx, tx, es, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- audit and outbox ---------------------------------------------------------------

// recordAs writes the audit entry and the outbox event of a change in tx
// (INV-18) with the given actor (a principal's or the public channel's).
func (s *Service) recordAs(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, b *store.Booking, action, eventType, prevStatus string, diff map[string]any, alloc *inventory.Allocation) error {
	if diff == nil {
		diff = map[string]any{}
	}
	if prevStatus != "" && prevStatus != b.Status {
		diff["from_status"] = prevStatus
		diff["to_status"] = b.Status
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "booking", EntityID: b.ID, Diff: diff,
	}); err != nil {
		return err
	}
	if eventType == "" {
		return nil
	}
	return s.emit(ctx, tx, es, b, eventType, prevStatus, alloc)
}

// emit writes the outbox event of a booking (payload event.BookingChange,
// no personal data) with the spot and date hints of alloc.
func (s *Service) emit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, eventType, prevStatus string, alloc *inventory.Allocation) error {
	payload := event.BookingChange{
		BookingID: b.ID, Code: b.Code, Status: b.Status, PreviousStatus: prevStatus, Channel: b.Channel,
		CustomerID: b.CustomerID, StartsOn: b.StartsOn.String(), EndsOn: b.EndsOn.String(),
		TotalAmount: int64(b.TotalAmount), PaidAmount: int64(b.PaidAmount), IsSeasonPass: b.IsSeasonPass, Version: b.Version,
	}
	if alloc != nil {
		payload.SpotIDs = append([]string(nil), alloc.SpotIDs...)
		for _, d := range alloc.Dates {
			payload.Dates = append(payload.Dates, d.String())
		}
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, payload); err != nil {
		return fmt.Errorf("booking: outbox %s: %w", eventType, err)
	}
	return nil
}

// mergeAlloc combines the hints of two allocation summaries (release + allocate).
func mergeAlloc(a, b *inventory.Allocation) *inventory.Allocation {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := &inventory.Allocation{Lines: b.Lines, SpotRows: a.SpotRows + b.SpotRows, PoolRows: a.PoolRows + b.PoolRows}
	seenS := map[string]bool{}
	for _, id := range append(append([]string{}, a.SpotIDs...), b.SpotIDs...) {
		if !seenS[id] {
			seenS[id] = true
			out.SpotIDs = append(out.SpotIDs, id)
		}
	}
	seenP := map[string]bool{}
	for _, id := range append(append([]string{}, a.PoolIDs...), b.PoolIDs...) {
		if !seenP[id] {
			seenP[id] = true
			out.PoolIDs = append(out.PoolIDs, id)
		}
	}
	seenD := map[calendar.Date]bool{}
	for _, d := range append(append([]calendar.Date{}, a.Dates...), b.Dates...) {
		if !seenD[d] {
			seenD[d] = true
			out.Dates = append(out.Dates, d)
		}
	}
	return out
}

// ---- reads ------------------------------------------------------------------------

// Get returns the detail of a booking (auth.PermBookingsRead).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	b, err := store.GetBooking(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.loadDetail(ctx, s.d.DB, es, b)
}

// List returns a page of bookings (auth.PermBookingsRead).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, in ListInput) (store.Page[store.BookingRow], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.BookingRow]{}, err
	}
	if in.Limit <= 0 {
		in.Limit = DefaultPageSize
	}
	if in.Limit > MaxPageSize {
		return store.Page[store.BookingRow]{}, configerr.Invalid("limit", "Deve essere tra 1 e 200.")
	}
	for _, st := range in.Filter.Statuses {
		if !Status(st).Valid() {
			return store.Page[store.BookingRow]{}, configerr.Invalid("status", "Stato sconosciuto.")
		}
	}
	return store.ListBookings(ctx, s.d.DB, es, in.Filter, in.Cursor, in.Limit)
}

// loadDetail reads everything the detail page shows through db.
func (s *Service) loadDetail(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) (*Detail, error) {
	d := &Detail{Booking: *b}
	c, err := store.GetCustomer(ctx, db, es, b.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("booking: customer: %w", err)
	}
	d.Customer = *c
	if d.Lines, err = store.ListBookingLines(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	if d.Extras, err = store.ListBookingLineExtras(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	if d.SpotAllocations, err = store.ListBookingSpotAllocations(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	if d.PoolAllocations, err = store.ListBookingPoolAllocations(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	if d.Payments, err = s.d.Payments.ListByBooking(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	if d.Payments == nil {
		d.Payments = []store.Payment{}
	}
	paymentIDs := make([]string, 0, len(d.Payments))
	for i := range d.Payments {
		paymentIDs = append(paymentIDs, d.Payments[i].ID)
	}
	if d.History, err = store.ListBookingHistory(ctx, db, es, b.ID, paymentIDs); err != nil {
		return nil, err
	}
	if d.RefundRequests, err = store.ListBookingRefundRequests(ctx, db, es, b.ID); err != nil {
		return nil, err
	}
	var spotIDs []string
	for _, l := range d.Lines {
		if l.SpotID != nil {
			spotIDs = append(spotIDs, *l.SpotID)
		}
	}
	if d.SpotLabels, err = store.SpotLabels(ctx, db, es, spotIDs); err != nil {
		return nil, err
	}
	d.PackCodes, d.ExtraCodes = map[string]string{}, map[string]string{}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("booking: packs: %w", err)
	}
	for _, pk := range packs {
		d.PackCodes[pk.ID] = pk.Code
	}
	extras, err := store.ListAllExtras(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("booking: extras: %w", err)
	}
	for _, x := range extras {
		d.ExtraCodes[x.ID] = x.Code
	}
	return d, nil
}

// ---- status changes -------------------------------------------------------------

// outcome is what a use case did to a locked booking (see mutate).
type outcome struct {
	action string // audit action (and event type unless event is set)
	event  string // outbox event ("" = action; "-" = none)
	diff   map[string]any
	alloc  *inventory.Allocation
	// after runs once the booking row is saved (refunds read paid_amount).
	after func(ctx context.Context, tx bun.IDB, b *store.Booking) error
}

// mutate is the unit of work of every change of an existing booking: one
// transaction that locks the row, checks If-Match, lets fn change the
// booking and its resources, saves the row with a conditional UPDATE on the
// version and writes audit + outbox (INV-18). A failure anywhere rolls back
// everything (the original booking and its allocations stay intact).
func (s *Service) mutate(ctx context.Context, p *auth.Principal, establishmentID, id string, perm auth.Permission, expected *int64,
	fn func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error),
) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, perm)
	if err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		out, err = s.mutateInTx(ctx, tx, p.Actor(reqctx.From(ctx).ClientIP), es, id, expected, fn)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) mutateInTx(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, id string, expected *int64,
	fn func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error),
) (*Detail, error) {
	b, err := store.LockBooking(ctx, tx, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	if err := configerr.CheckVersion(expected, b.Version); err != nil {
		return nil, err
	}
	prev, version := b.Status, b.Version
	o, err := fn(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	b.UpdatedAt = s.now()
	ok, err := store.UpdateBookingVersioned(ctx, tx, es, b, version)
	if err != nil {
		return nil, fmt.Errorf("booking: %w", err)
	}
	if !ok {
		cur, _ := store.GetBooking(ctx, tx, es, id)
		return nil, &configerr.VersionConflictError{Current: cur.Version}
	}
	if o.after != nil {
		if err := o.after(ctx, tx, b); err != nil {
			return nil, err
		}
		if b, err = store.GetBooking(ctx, tx, es, id); err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
	}
	ev := o.event
	switch ev {
	case "":
		ev = o.action
	case "-":
		ev = ""
	}
	if err := s.recordAs(ctx, tx, actor, es, b, o.action, ev, prev, o.diff, o.alloc); err != nil {
		return nil, err
	}
	return s.loadDetail(ctx, tx, es, b)
}

// Confirm confirms a booking with the manual driver (panel): from held,
// pending_payment or awaiting_approval the hold allocations become stable;
// a draft is allocated now (draft -> held -> confirmed). Never called from a
// browser return URL (INV-19). Requires auth.PermBookingsWrite.
func (s *Service) Confirm(ctx context.Context, p *auth.Principal, establishmentID, id string, in ActionInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermBookingsWrite, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		switch Status(b.Status) {
		case StatusDraft, StatusHeld, StatusPendingPayment, StatusAwaitingApproval:
		default:
			return s.confirmLocked(ctx, tx, es, b, in.Note, false)
		}
		needed, err := s.needsApproval(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		// Approval rule R2/R3 (approval.go): a booking that needs approval
		// waits for the manager; confirming one awaiting approval is approving.
		switch DecideApproval(ApprovalCase{Op: ApprovalOnConfirm, From: Status(b.Status), Needed: needed, Approver: canApprove(p, es)}) {
		case ApprovalRefuse:
			return nil, ErrApprovalRequired
		case ApprovalAwait:
			o, err := s.awaitApprovalLocked(ctx, tx, es, b, "confirm")
			if err == nil {
				noteDiff(o.diff, in.Note)
			}
			return o, err
		case ApprovalProceedApproved:
			return s.confirmLocked(ctx, tx, es, b, in.Note, true)
		}
		return s.confirmLocked(ctx, tx, es, b, in.Note, false)
	})
}

// ConfirmFromPayment implements payment.BookingConfirmer: inside the
// payment transaction, a held or pending_payment booking whose payments
// reached the threshold becomes confirmed (manual driver path, spec 13.2,
// INV-19), or awaiting_approval when it needs the manager's approval (rule
// R2, approval.go). Bookings in any other status are left alone
// (awaiting_approval still needs the manager; confirmed ones need nothing).
func (s *Service) ConfirmFromPayment(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, bookingID string, pm *store.Payment) error {
	b, err := store.LockBooking(ctx, tx, es, bookingID)
	if err != nil {
		return notFound(err)
	}
	if st := Status(b.Status); st != StatusHeld && st != StatusPendingPayment {
		return nil
	}
	_, err = s.mutateInTx(ctx, tx, p.Actor(reqctx.From(ctx).ClientIP), es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		needed, err := s.needsApproval(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		var o *outcome
		if DecideApproval(ApprovalCase{Op: ApprovalOnConfirm, From: Status(b.Status), Needed: needed}) == ApprovalAwait {
			// Rule R2: the money stays recorded, the manager still decides.
			o, err = s.awaitApprovalLocked(ctx, tx, es, b, "payment")
		} else {
			o, err = s.confirmLocked(ctx, tx, es, b, "", false)
		}
		if err != nil {
			return nil, err
		}
		o.diff["driver"] = pm.Provider
		o.diff["payment_id"] = pm.ID
		return o, nil
	})
	return err
}

// Approve confirms a booking awaiting approval: the manager's decision,
// auth.PermEstablishmentConfigure (approval rule R3, approval.go).
func (s *Service) Approve(ctx context.Context, p *auth.Principal, establishmentID, id string, in ActionInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermEstablishmentConfigure, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		if Status(b.Status) != StatusAwaitingApproval {
			return nil, &TransitionError{From: Status(b.Status), To: StatusConfirmed}
		}
		return s.confirmLocked(ctx, tx, es, b, in.Note, true)
	})
}

func (s *Service) confirmLocked(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, note string, approved bool) (*outcome, error) {
	from := Status(b.Status)
	now := s.now()
	var alloc *inventory.Allocation
	switch from {
	case StatusDraft:
		if err := Transition(StatusDraft, StatusHeld); err != nil {
			return nil, err
		}
		if err := Transition(StatusHeld, StatusConfirmed); err != nil {
			return nil, err
		}
		rows, extras, cat, err := s.currentLines(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		kind := inventory.KindBooking
		if b.IsSeasonPass {
			kind = inventory.KindSeason
		}
		if alloc, err = s.d.Inventory.Allocate(ctx, tx, es, allocRequest(b, kind, nil, rows, extras, cat)); err != nil {
			return nil, err
		}
		if err := s.applyAllocation(ctx, tx, es, b, rows, alloc, kind, now); err != nil {
			return nil, err
		}
	default:
		if err := Transition(from, StatusConfirmed); err != nil {
			return nil, err
		}
		if err := s.d.Inventory.Confirm(ctx, tx, es, b.ID); err != nil {
			return nil, err
		}
	}
	b.Status = string(StatusConfirmed)
	b.ConfirmedAt = &now
	b.HoldExpiresAt = nil
	if b.Channel != store.ChannelDesk {
		// Public channel: zone-mode lines get their spot now (spec 7.5,
		// auto_assign zones; see assignOnline in online.go).
		assigned, err := s.assignOnline(ctx, tx, es, b, now)
		if err != nil {
			return nil, err
		}
		alloc = mergeAlloc(alloc, assigned)
	}
	diff := map[string]any{"driver": "manual"}
	if approved {
		diff["approved"] = true
	}
	noteDiff(diff, note)
	return &outcome{action: eventConfirmed, diff: diff, alloc: alloc}, nil
}

// noteDiff records that an operator note accompanied an action without
// copying its text into the insert-only audit log (it may contain personal
// data that a GDPR anonymization could never remove, INV-30): only the
// presence and the length are kept, as AddNote does.
func noteDiff(diff map[string]any, note string) {
	if note == "" {
		return
	}
	diff["note"] = true
	diff["note_length"] = len(note)
}

// Reject refuses a booking awaiting approval and releases its resources
// (auth.PermBookingsWrite).
func (s *Service) Reject(ctx context.Context, p *auth.Principal, establishmentID, id string, in ActionInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermBookingsWrite, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		if err := Transition(Status(b.Status), StatusRejected); err != nil {
			return nil, err
		}
		alloc, err := s.releaseAll(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		b.Status = string(StatusRejected)
		b.HoldExpiresAt = nil
		diff := map[string]any{}
		noteDiff(diff, in.Note)
		return &outcome{action: eventRejected, diff: diff, alloc: alloc}, nil
	})
}

// CheckIn records the arrival (confirmed -> checked_in, auth.PermOperationsRun).
// Only on a day of the stay in the establishment timezone (spec 10.2, 11.3):
// otherwise *OutsidePeriodError.
func (s *Service) CheckIn(ctx context.Context, p *auth.Principal, establishmentID, id string, in ActionInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermOperationsRun, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		if err := Transition(Status(b.Status), StatusCheckedIn); err != nil {
			return nil, err
		}
		today, err := s.today(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		if today.Before(b.StartsOn) || today.After(b.EndsOn) {
			return nil, &OutsidePeriodError{Action: "check_in", Today: today, StartsOn: b.StartsOn, EndsOn: b.EndsOn}
		}
		now := s.now()
		b.Status = string(StatusCheckedIn)
		b.CheckedInAt = &now
		return &outcome{action: eventCheckedIn, diff: map[string]any{"day": today.String()}}, nil
	})
}

// MarkNoShow marks a confirmed booking as not shown and releases the
// allocations from today on (auth.PermOperationsRun). Not before the stay
// starts (*OutsidePeriodError): a future booking is cancelled, with the
// refund of its policy, not marked as no-show.
func (s *Service) MarkNoShow(ctx context.Context, p *auth.Principal, establishmentID, id string, in ActionInput) (*Detail, error) {
	return s.mutate(ctx, p, establishmentID, id, auth.PermOperationsRun, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		if err := Transition(Status(b.Status), StatusNoShow); err != nil {
			return nil, err
		}
		from, err := s.today(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		if from.Before(b.StartsOn) {
			return nil, &OutsidePeriodError{Action: "no_show", Today: from, StartsOn: b.StartsOn, EndsOn: b.EndsOn}
		}
		var alloc *inventory.Allocation
		if !from.After(b.EndsOn) {
			if alloc, err = s.d.Inventory.Release(ctx, tx, es, inventory.ReleaseRequest{BookingID: b.ID, From: &from}); err != nil {
				return nil, err
			}
		}
		b.Status = string(StatusNoShow)
		return &outcome{action: eventNoShow, diff: map[string]any{"released_from": from.String()}, alloc: alloc}, nil
	})
}

// AddNote appends a note (auth.PermBookingsWrite); the text is not copied
// into the audit log (it may contain personal data).
func (s *Service) AddNote(ctx context.Context, p *auth.Principal, establishmentID, id string, in NoteInput) (*Detail, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, configerr.Invalid("text", "Scrivi il testo della nota.")
	}
	if len(text) > 4000 {
		return nil, configerr.Invalid("text", "Nota troppo lunga (massimo 4000 caratteri).")
	}
	vis := in.Visibility
	if vis == "" {
		vis = "internal"
	}
	if vis != "internal" && vis != "customer" {
		return nil, configerr.Invalid("visibility", "Usa internal o customer.")
	}
	return s.mutate(ctx, p, establishmentID, id, auth.PermBookingsWrite, nil, func(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		target := &b.NotesInternal
		if vis == "customer" {
			target = &b.NotesCustomer
		}
		if *target != "" {
			*target += "\n"
		}
		*target += text
		return &outcome{action: actionNoteAdded, event: eventModified, diff: map[string]any{"visibility": vis, "length": len(text)}}, nil
	})
}

// releaseAll frees every allocation of an occupying booking.
func (s *Service) releaseAll(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*inventory.Allocation, error) {
	if !Status(b.Status).OccupiesResources() {
		return nil, nil
	}
	alloc, err := s.d.Inventory.Release(ctx, tx, es, inventory.ReleaseRequest{BookingID: b.ID})
	if err != nil {
		return nil, err
	}
	lines, err := store.ListBookingLines(ctx, tx, es, b.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.ID
	}
	if err := s.unmarkResold(ctx, tx, es, ids); err != nil {
		return nil, err
	}
	return alloc, nil
}

// today is the current date in the establishment timezone.
func (s *Service) today(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (calendar.Date, error) {
	loc, err := s.location(ctx, db, es)
	if err != nil {
		return calendar.Date{}, err
	}
	return calendar.In(s.now(), loc), nil
}

// location loads the establishment timezone.
func (s *Service) location(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*time.Location, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("booking: establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("booking: timezone %q: %w", est.Timezone, err)
	}
	return loc, nil
}

// snapshotRules decodes the policy snapshot of a booking (default rules
// when absent or malformed: a stored booking must stay cancellable).
func snapshotRules(b *store.Booking) policy.Rules {
	snap, err := policy.ParseSnapshot(b.PolicySnapshot)
	if err != nil {
		return policy.Rules{}
	}
	return snap.Rules
}

// ---- cancellation -------------------------------------------------------------------

// Cancel cancels a booking (auth.PermBookingsWrite): releases its resources
// and computes the refund on the policy snapshot (spec 9). A credit
// settlement is recorded at once; a money refund is recorded when
// RefundMethod is given, otherwise it is left due (audit + event) for the
// cashier. Recording any refund requires auth.PermRefundsIssue (without it
// the cancellation records no refund). DryRun only previews.
func (s *Service) Cancel(ctx context.Context, p *auth.Principal, establishmentID, id string, in CancelInput) (*CancelResult, error) {
	if in.Settlement == "" {
		in.Settlement = SettlePolicy
	}
	switch in.Settlement {
	case SettlePolicy, SettleRefund, SettleCredit, SettleNone:
	default:
		return nil, configerr.Invalid("settlement", "Usa policy, refund, credit o none.")
	}
	if in.RefundAmount != nil && *in.RefundAmount < 0 {
		return nil, configerr.Invalid("refund_amount", "Importo non valido.")
	}
	res := &CancelResult{}
	compute := func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) error {
		if err := Transition(Status(b.Status), StatusCancelled); err != nil {
			return err
		}
		loc, err := s.location(ctx, tx, es)
		if err != nil {
			return err
		}
		days := calendar.In(s.now(), loc).DaysUntil(b.StartsOn)
		paid, err := refundablePaid(ctx, tx, es, b)
		if err != nil {
			return err
		}
		res.Refund = ComputeRefund(snapshotRules(b), paid, days, CancelRequest{Settlement: in.Settlement, Weather: in.Weather, Override: in.RefundAmount})
		return nil
	}
	if in.DryRun {
		es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
		if err != nil {
			return nil, err
		}
		b, err := store.GetBooking(ctx, s.d.DB, es, id)
		if err != nil {
			return nil, notFound(err)
		}
		if err := configerr.CheckVersion(in.ExpectedVersion, b.Version); err != nil {
			return nil, err
		}
		if err := compute(ctx, s.d.DB, es, b); err != nil {
			return nil, err
		}
		if res.Detail, err = s.loadDetail(ctx, s.d.DB, es, b); err != nil {
			return nil, err
		}
		return res, nil
	}
	d, err := s.mutate(ctx, p, establishmentID, id, auth.PermBookingsWrite, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		// A ticket booking is cancelled only by the void of its ticket
		// (ADR 0020), which marks the ticket in the same transaction.
		if !in.FromTicket {
			if err := refuseTicketOwned(ctx, tx, es, b); err != nil {
				return nil, err
			}
		}
		if err := compute(ctx, tx, es, b); err != nil {
			return nil, err
		}
		alloc, err := s.releaseAll(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		now := s.now()
		b.Status = string(StatusCancelled)
		b.CancelledAt = &now
		b.HoldExpiresAt = nil
		if r := strings.TrimSpace(in.Reason); r != "" {
			b.CancelReason = &r
		}
		rf := res.Refund
		diff := map[string]any{
			"refund_amount": int64(rf.Amount), "refund_as": string(rf.As), "refund_percent": rf.RefundPercent,
			"paid_amount": int64(rf.PaidAmount), "weather": in.Weather, "settlement": string(in.Settlement),
		}
		o := &outcome{action: eventCancelled, diff: diff, alloc: alloc}
		if rf.Amount <= 0 || rf.As == SettleNone {
			return o, nil
		}
		// M5B: the credit-paid part of a money refund goes back as credit.
		creditPart, err := creditRefundPart(ctx, tx, es, b.ID, rf)
		if err != nil {
			return nil, err
		}
		if creditPart > 0 && p.Can(es, auth.PermRefundsIssue) {
			rf.Amount -= creditPart
			diff["refund_credit_part"] = int64(creditPart)
			o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
				_, err := s.recordCreditRefund(ctx, tx, p, es, b.ID, creditPart, in.Weather)
				return err
			}
			if rf.Amount <= 0 {
				return o, nil
			}
		}
		record := rf.As == SettleCredit || in.RefundMethod != ""
		if !record || !p.Can(es, auth.PermRefundsIssue) {
			if rf.As == SettleRefund && p.Can(es, auth.PermRefundsIssue) {
				// ADR 0019: a money refund without a desk method goes back to
				// the online payments as guaranteed provider refunds; what no
				// provider payment can take stays due.
				by := p.UserID
				return o, s.reserveRefund(ctx, tx, es, b, rf.Amount, store.RefundReasonPanelCancellation, &by, diff, res)
			}
			diff["refund_due"] = int64(rf.Amount)
			res.RefundDue = rf.Amount
			return o, nil
		}
		creditAfter := o.after
		o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
			if creditAfter != nil {
				if err := creditAfter(ctx, tx, b); err != nil {
					return err
				}
			}
			req := refundRequest(b.ID, rf, in)
			pm, err := s.d.Payments.RecordRefund(ctx, tx, p, es, req)
			if err != nil {
				return err
			}
			res.Recorded = pm
			diff["refund_recorded"] = true
			return nil
		}
		return o, nil
	})
	if err != nil {
		return nil, err
	}
	res.Applied, res.Detail = true, d
	return res, nil
}
