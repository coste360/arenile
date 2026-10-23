package booking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Use cases of the public channel (M4, spec 10.1, 12.4). Ownership: the
// "checkout" builder (internal/domain/checkout) implements these methods;
// they implement checkout.Bookings and are asserted in internal/app/m4.go.
//
// They take an audit.Actor (audit.ActorCustomer with the checkout session
// or magic link id, or audit.SystemActor) and an already resolved
// EstablishmentScope instead of an *auth.Principal: the public flow has no
// staff principal, and the checkout service resolves the scope from the
// slug and authenticates the session token or the magic link. Every method
// keeps the unit of work of doc.go (LockBooking, Transition, inventory,
// versioned update, audit + outbox in the caller's tx).
//
// Customer of a held online booking: bookings.customer_id is NOT NULL, but
// the customer types their details only at step 4. CreateOnline points the
// booking to the establishment's online placeholder customer
// (customer.Service, created on first use, hidden from lists, searches and
// exports); AttachCustomer re-points it to a customer created for that
// checkout (never an existing customer matched by email or phone: the
// public page must not reveal or impersonate stored customers, see
// customer.ResolveOnline).
//
// Zone-mode lines of an online booking stay pending while held (they
// consume the zone pool, the database decides the capacity) and get their
// spot at confirmation when the zone has auto_assign (spec 7.5):
// assignOnline runs inside the confirmation transaction (manual driver,
// payment webhook or manager approval), one savepoint per line, retried on
// a concurrent placement.

// Policy violation codes of the public channel (policy_violation).
const (
	// CodeModificationNotAllowed: the policy snapshot does not let the
	// customer modify the booking any more (spec 9 modification).
	CodeModificationNotAllowed policy.Code = "modification_not_allowed"
	// CodeCancellationClosed: the stay already started.
	CodeCancellationClosed policy.Code = "cancellation_closed"
)

// Audit actions of the public channel.
const (
	actionCustomerAttached = "booking.customer_attached"
	actionPaymentStarted   = "booking.payment_started"
)

// MaxPaymentHold bounds the hold while a payment is in progress (spec
// 10.2: pending_payment, at most 30 minutes from now).
const MaxPaymentHold = 30 * time.Minute

// maxAssignAttempts bounds the re-plans of the automatic assignment at
// confirmation when concurrent confirmations take the planned spots.
const maxAssignAttempts = 5

// OnlineCustomers resolves the customers of the public channel.
// Implemented by *customer.Service; the booking service type-asserts its
// Customers port to it, so test adapters without it simply cannot create
// online bookings.
type OnlineCustomers interface {
	OnlinePlaceholder(ctx context.Context, tx bun.IDB, es store.EstablishmentScope) (*store.Customer, error)
	ResolveOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID, currentCustomerID string, in customer.Input) (*store.Customer, error)
}

// errNoOnlineCustomers: the Customers port cannot serve the public channel.
var errNoOnlineCustomers = errors.New("booking: customers port does not support the public channel")

func (s *Service) onlineCustomers() (OnlineCustomers, error) {
	oc, ok := s.d.Customers.(OnlineCustomers)
	if !ok {
		return nil, errNoOnlineCustomers
	}
	return oc, nil
}

// OnlineCustomerInput is step 4 of the public flow (api.CheckoutCustomerInput).
type OnlineCustomerInput struct {
	Customer customer.Input
	// TermsVersion is the accepted version (must be the current one: the
	// checkout service checks it); TermsAcceptedAt is set by the service clock.
	TermsVersion  string
	Locale        string
	NotesCustomer string
	// Extras replaces the booking-level extras when ExtrasSet.
	Extras    []ExtraInput
	ExtrasSet bool
	// PromoCode applies a promo code when not nil and different from the
	// applied one ("" or the same code changes nothing; an applied code
	// cannot be replaced from the public page).
	PromoCode *string
}

// maxCustomerNotes bounds the notes typed on the public page.
const maxCustomerNotes = 1000

// CreateOnline creates a held booking of channel online (or api) with a
// hold of in.HoldTTL (default ARENILE_HOLD_TTL) in tx: price and online
// policy (violations refused, never forced), online quota (inventory),
// placeholder customer, booking.created + booking.held.
func (s *Service) CreateOnline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, in CreateInput) (*Detail, error) {
	if in.Channel == "" {
		in.Channel = store.ChannelOnline
	}
	if in.Channel != store.ChannelOnline && in.Channel != store.ChannelAPI {
		return nil, configerr.Invalid("channel", "Canale non ammesso per la prenotazione online.")
	}
	in.Status, in.Force, in.HoldID = StatusHeld, false, ""
	in.CustomerID, in.Customer, in.NotesInternal, in.AgreementID = "", nil, "", ""
	in.IsSeasonPass, in.AllocationKind, in.SeasonID, in.RenewalOfBookingID = false, "", "", ""
	if err := validateCreate(&in); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(in.NotesCustomer)) > maxCustomerNotes {
		return nil, configerr.Invalid("notes", fmt.Sprintf("Al massimo %d caratteri.", maxCustomerNotes))
	}
	oc, err := s.onlineCustomers()
	if err != nil {
		return nil, err
	}
	cust, err := oc.OnlinePlaceholder(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	return s.createWith(ctx, tx, actor, nil, es, cust, in)
}

// AttachCustomer saves the customer, terms acceptance, marketing consent,
// notes, extras and promo code of a held online booking in tx
// (booking.customer_attached, event booking.modified). availability.ErrHoldExpired
// when the hold already elapsed; only a held booking accepts it.
func (s *Service) AttachCustomer(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, in OnlineCustomerInput) (*Detail, error) {
	oc, err := s.onlineCustomers()
	if err != nil {
		return nil, err
	}
	notes := strings.TrimSpace(in.NotesCustomer)
	if len([]rune(notes)) > maxCustomerNotes {
		return nil, configerr.Invalid("notes", fmt.Sprintf("Al massimo %d caratteri.", maxCustomerNotes))
	}
	return s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		now := s.now()
		if Status(b.Status) != StatusHeld {
			if Status(b.Status) == StatusExpired {
				return nil, availability.ErrHoldExpired
			}
			return nil, &TransitionError{From: Status(b.Status), To: StatusHeld}
		}
		if b.HoldExpiresAt == nil || !b.HoldExpiresAt.After(now) {
			return nil, availability.ErrHoldExpired
		}
		c, err := oc.ResolveOnline(ctx, tx, actor, es, b.ID, b.CustomerID, in.Customer)
		if err != nil {
			return nil, err
		}
		diff := map[string]any{
			"customer_attached": true, "terms_version": in.TermsVersion,
			"marketing_consent": in.Customer.MarketingConsent != nil, "notes": notes != "",
		}
		b.CustomerID = c.ID
		b.NotesCustomer = notes
		reprice := in.ExtrasSet
		if in.PromoCode != nil {
			applied, err := s.applyOnlinePromo(ctx, tx, es, b, strings.TrimSpace(*in.PromoCode), in, now)
			if err != nil {
				return nil, err
			}
			if applied {
				diff["promo_code_applied"] = true
				reprice = true
			}
		}
		var alloc *inventory.Allocation
		if reprice {
			res := &ChangeResult{}
			o, err := s.modifyLocked(ctx, tx, nil, es, b, PatchInput{Extras: in.Extras, ExtrasSet: in.ExtrasSet}, res)
			if err != nil {
				return nil, err
			}
			alloc = o.alloc
			diff["previous_total"], diff["new_total"] = int64(res.PreviousTotal), int64(res.NewTotal)
		}
		var version, locale *string
		if v := strings.TrimSpace(in.TermsVersion); v != "" {
			version = &v
		}
		if l := strings.TrimSpace(in.Locale); l != "" {
			locale = &l
		}
		at := now
		if err := store.SetBookingOnlineTerms(ctx, tx, es, b.ID, version, &at, locale); err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
		b.TermsVersion, b.TermsAcceptedAt, b.Locale = version, &at, locale
		return &outcome{action: actionCustomerAttached, event: eventModified, diff: diff, alloc: alloc}, nil
	})
}

// applyOnlinePromo applies code to the held booking b (the caller
// re-prices). It reports whether something changed: an empty code or the
// code already applied changes nothing; replacing an applied code is a
// conflict; a code that does not apply to the request is a validation
// error (the price is computed with it first).
func (s *Service) applyOnlinePromo(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, code string, in OnlineCustomerInput, now time.Time) (bool, error) {
	current := ""
	if b.PromoCodeID != nil {
		if pc, err := store.GetPromoCode(ctx, tx, es, *b.PromoCodeID); err == nil {
			current = pc.Code
		}
	}
	if code == "" || strings.EqualFold(code, current) {
		return false, nil
	}
	if current != "" {
		return false, fmt.Errorf("%w: la prenotazione ha già un codice promozionale", configerr.ErrConflict)
	}
	rows, extras, _, err := s.currentLines(ctx, tx, es, b)
	if err != nil {
		return false, err
	}
	lineIn, bookingExtras := linesToInput(rows, extras)
	if in.ExtrasSet {
		bookingExtras = in.Extras
	}
	cat, err := loadCatalog(ctx, tx, es, lineIn, bookingExtras)
	if err != nil {
		return false, err
	}
	lines, err := cat.resolve(lineIn, "lines")
	if err != nil {
		return false, err
	}
	cust, err := store.GetCustomer(ctx, tx, es, b.CustomerID)
	if err != nil {
		return false, fmt.Errorf("booking: customer: %w", err)
	}
	st := stay{StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, Formula: b.Formula, Channel: b.Channel}
	if _, err := s.price(ctx, tx, es, st, lines, bookingExtras, cust.Tags, "", code, cat); err != nil {
		return false, err
	}
	if err := s.consumePromo(ctx, tx, es, b, code, now); err != nil {
		return false, err
	}
	return true, nil
}

// StartPayment moves a held booking to pending_payment and extends its
// hold to holdUntil (never shortened, at most MaxPaymentHold from now,
// spec 10.2) in tx. On a booking already pending_payment it only extends
// the hold (payment retry).
func (s *Service) StartPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string, holdUntil time.Time) (*Detail, error) {
	return s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		now := s.now()
		st := Status(b.Status)
		retry := st == StatusPendingPayment
		if !retry {
			if st == StatusExpired {
				return nil, availability.ErrHoldExpired
			}
			if err := Transition(st, StatusPendingPayment); err != nil {
				return nil, err
			}
		}
		if b.HoldExpiresAt == nil || !b.HoldExpiresAt.After(now) {
			return nil, availability.ErrHoldExpired
		}
		until := holdUntil.UTC()
		if limit := now.Add(MaxPaymentHold); until.After(limit) {
			until = limit
		}
		if until.Before(*b.HoldExpiresAt) {
			until = *b.HoldExpiresAt
		}
		if err := s.d.Inventory.ExtendHold(ctx, tx, es, b.ID, until); err != nil {
			return nil, err
		}
		b.Status = string(StatusPendingPayment)
		b.HoldExpiresAt = &until
		return &outcome{action: actionPaymentStarted, event: eventModified, diff: map[string]any{
			"hold_expires_at": until.Format(time.RFC3339), "retry": retry,
		}}, nil
	})
}

// ConfirmWithoutPayment confirms a held online booking whose policy needs
// no online payment (manual driver, spec 13.2): confirmed, or
// awaiting_approval with the long hold when approval is required (policy
// snapshot or a pack requiring approval).
func (s *Service) ConfirmWithoutPayment(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) (*Detail, error) {
	return s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		now := s.now()
		st := Status(b.Status)
		if st == StatusExpired {
			return nil, availability.ErrHoldExpired
		}
		if st != StatusHeld {
			return nil, &TransitionError{From: st, To: StatusConfirmed}
		}
		if b.HoldExpiresAt == nil || !b.HoldExpiresAt.After(now) {
			return nil, availability.ErrHoldExpired
		}
		approval, err := s.needsApproval(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		if approval {
			if err := Transition(st, StatusAwaitingApproval); err != nil {
				return nil, err
			}
			until := now.Add(s.d.ApprovalTTL)
			if err := s.d.Inventory.ExtendHold(ctx, tx, es, b.ID, until); err != nil {
				return nil, err
			}
			b.Status = string(StatusAwaitingApproval)
			b.HoldExpiresAt = &until
			return &outcome{action: eventApprovalRequested, diff: map[string]any{"channel": b.Channel, "settlement": "manual"}}, nil
		}
		o, err := s.confirmLocked(ctx, tx, es, b, "", false)
		if err != nil {
			return nil, err
		}
		o.diff["channel"] = b.Channel
		o.diff["settlement"] = "manual"
		return o, nil
	})
}

// needsApproval reports whether the manager must approve b: the policy
// snapshot or a pack of its lines requires it.
func (s *Service) needsApproval(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) (bool, error) {
	if snapshotRules(b).RequiresApproval() {
		return true, nil
	}
	rows, _, cat, err := s.currentLines(ctx, db, es, b)
	if err != nil {
		return false, err
	}
	for _, l := range rows {
		if cat.packs[l.PackID].RequiresApproval {
			return true, nil
		}
	}
	return false, nil
}

// Abandon expires a held or pending_payment online booking and releases its
// resources in tx (booking.expired, reason abandoned). Idempotent on an
// expired booking.
func (s *Service) Abandon(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, bookingID string) error {
	b, err := store.LockBooking(ctx, tx, es, bookingID)
	if err != nil {
		return notFound(err)
	}
	if Status(b.Status) == StatusExpired {
		return nil
	}
	_, err = s.mutateInTx(ctx, tx, actor, es, bookingID, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
		st := Status(b.Status)
		if st != StatusHeld && st != StatusPendingPayment {
			return nil, &TransitionError{From: st, To: StatusExpired}
		}
		alloc, err := s.releaseAll(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		b.Status = string(StatusExpired)
		b.HoldExpiresAt = nil
		return &outcome{action: eventExpired, diff: map[string]any{"reason": "abandoned"}, alloc: alloc}, nil
	})
	return err
}

// ModifyAsCustomer applies (or previews, in.DryRun) a modification asked
// from the manage area within the modification limits of the policy
// snapshot (policy_violation modification_not_allowed otherwise, never
// forced): confirmed bookings only, not season passes; the old and the new
// start date must both be inside the window. The fee of the policy is
// returned by policy.ModificationAllowed to the caller.
func (s *Service) ModifyAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, expected *int64, in PatchInput) (*ChangeResult, error) {
	in.Force, in.CustomerID, in.NotesInternal = false, nil, nil
	return s.changeAs(ctx, actor, es, bookingID, expected, in.DryRun, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error) {
		if Status(b.Status) != StatusConfirmed || b.IsSeasonPass {
			return nil, ErrNotModifiable
		}
		loc, err := s.location(ctx, tx, es)
		if err != nil {
			return nil, err
		}
		rules, now := snapshotRules(b), s.now()
		newStart := b.StartsOn
		if in.StartsOn != nil {
			newStart = *in.StartsOn
		}
		if !policy.ModificationAllowed(b.StartsOn, now, loc, rules).Allowed || !policy.ModificationAllowed(newStart, now, loc, rules).Allowed {
			return nil, &policy.ViolationError{Violations: []policy.Violation{{Code: CodeModificationNotAllowed}}}
		}
		o, err := s.modifyLocked(ctx, tx, nil, es, b, in, res)
		if err != nil {
			return nil, err
		}
		o.diff["by"] = "customer"
		return o, nil
	})
}

// refundablePaid is what a cancellation may give back: paid_amount minus
// the refunds already sent to the provider and not yet confirmed (an
// overpayment or late-payment refund in flight): INV-24 counts them.
func refundablePaid(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) (money.Cents, error) {
	inFlight, err := store.SumOpenRefundIntents(ctx, db, es, "", b.ID)
	if err != nil {
		return 0, fmt.Errorf("booking: refunds in flight: %w", err)
	}
	return b.PaidAmount - inFlight, nil
}

// CancelAsCustomer cancels (or previews, in.DryRun) from the manage area
// with the refund or credit of the policy snapshot (the customer may only
// choose credit instead of the policy outcome, never a larger refund).
// Credits are recorded at once; a money refund is left due (refund_due in
// the audit diff): the checkout service then refunds through the payment
// gateway. confirmed and awaiting_approval bookings only, until the first
// day of the stay included.
func (s *Service) CancelAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, bookingID string, in CancelInput) (*CancelResult, error) {
	settle := SettlePolicy
	if in.Settlement == SettleCredit {
		settle = SettleCredit
	}
	res := &CancelResult{}
	compute := func(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) error {
		st := Status(b.Status)
		if st != StatusConfirmed && st != StatusAwaitingApproval {
			return &TransitionError{From: st, To: StatusCancelled}
		}
		loc, err := s.location(ctx, db, es)
		if err != nil {
			return err
		}
		days := calendar.In(s.now(), loc).DaysUntil(b.StartsOn)
		if days < 0 {
			return &policy.ViolationError{Violations: []policy.Violation{{Code: CodeCancellationClosed}}}
		}
		paid, err := refundablePaid(ctx, db, es, b)
		if err != nil {
			return err
		}
		res.Refund = ComputeRefund(snapshotRules(b), paid, days, CancelRequest{Settlement: settle})
		return nil
	}
	if in.DryRun {
		b, err := store.GetBooking(ctx, s.d.DB, es, bookingID)
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
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		d, err := s.mutateInTx(ctx, tx, actor, es, bookingID, in.ExpectedVersion, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
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
				if len([]rune(r)) > maxCustomerNotes {
					r = string([]rune(r)[:maxCustomerNotes])
				}
				b.CancelReason = &r
			}
			rf := res.Refund
			diff := map[string]any{
				"refund_amount": int64(rf.Amount), "refund_as": string(rf.As), "refund_percent": rf.RefundPercent,
				"paid_amount": int64(rf.PaidAmount), "settlement": string(settle), "by": "customer",
			}
			o := &outcome{action: eventCancelled, diff: diff, alloc: alloc}
			if rf.Amount <= 0 || rf.As == SettleNone {
				return o, nil
			}
			if rf.As != SettleCredit {
				diff["refund_due"] = int64(rf.Amount)
				return o, nil
			}
			o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
				pm, err := s.d.Payments.RecordRefund(ctx, tx, nil, es, refundRequest(b.ID, rf, CancelInput{Reason: "customer_cancellation"}))
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
			return err
		}
		res.Detail = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	res.Applied = true
	return res, nil
}

// ---- automatic assignment at confirmation (spec 7.5) ------------------------------

// assignOnline places the pending zone-mode lines of a confirmed online
// booking whose zone has auto_assign, inside the confirmation transaction
// (b is locked, its status already confirmed). Each placement runs in a
// savepoint: release the line (zone pool) and allocate it again on the
// chosen spot (zone pool + spot rows, the unique allocation key decides);
// a conflict rolls the savepoint back and the remaining lines are planned
// again on fresh data, up to maxAssignAttempts. Lines that cannot be
// placed stay pending for the manager (never an error: the booking is
// sold, only its spot is open).
func (s *Service) assignOnline(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, now time.Time) (*inventory.Allocation, error) {
	rows, extras, cat, err := s.currentLines(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	pending := map[string]int{}
	var zoneIDs []string
	for i, l := range rows {
		z, ok := cat.zones[l.ZoneID]
		if l.AssignmentStatus != store.AssignmentPending || !ok || z.AssignmentMode != modeZone || !z.AutoAssign {
			continue
		}
		pending[l.ID] = i
		if !contains(zoneIDs, l.ZoneID) {
			zoneIDs = append(zoneIDs, l.ZoneID)
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}
	kind := allocationKind(b)
	var merged *inventory.Allocation
	for attempt := 0; attempt < maxAssignAttempts && len(pending) > 0; attempt++ {
		plan, err := s.assignPlan(ctx, tx, es, b, rows, pending, zoneIDs, cat, now)
		if err != nil {
			return nil, err
		}
		if len(plan) == 0 {
			break
		}
		for lineID, spotID := range plan {
			idx := pending[lineID]
			var placed *inventory.Allocation
			err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
				rel, err := s.d.Inventory.Release(ctx, sp, es, inventory.ReleaseRequest{BookingID: b.ID, LineIDs: []string{lineID}})
				if err != nil {
					return err
				}
				row := rows[idx]
				spot := spotID
				row.SpotID = &spot
				req := allocRequest(b, kind, b.HoldExpiresAt, []store.BookingLine{row}, lineExtras(extras, lineID, idx == 0), cat)
				a, err := s.d.Inventory.Allocate(ctx, sp, es, req)
				if err != nil {
					return err
				}
				one := []store.BookingLine{rows[idx]}
				if err := s.applyAllocation(ctx, sp, es, b, one, a, kind, now); err != nil {
					return err
				}
				rows[idx] = one[0]
				placed = mergeAlloc(rel, a)
				return nil
			})
			var ce *availability.ConflictError
			switch {
			case errors.As(err, &ce), errors.Is(err, availability.ErrSpotUnavailable):
				continue // taken meanwhile: plan again
			case err != nil:
				return nil, err
			}
			delete(pending, lineID)
			merged = mergeAlloc(merged, placed)
		}
	}
	return merged, nil
}

// assignPlan runs the pure greedy engine (availability.AutoAssign) for the
// pending lines of b on the spots of their zones free in the database
// (valid allocations only, INV-04). Split assignments are not applied.
func (s *Service) assignPlan(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking, rows []store.BookingLine, pending map[string]int, zoneIDs []string, cat *catalog, now time.Time) (map[string]string, error) {
	spots, err := store.ListZoneSpots(ctx, db, es, zoneIDs)
	if err != nil {
		return nil, err
	}
	if len(spots) == 0 {
		return nil, nil
	}
	all := availability.Interval{From: b.StartsOn, To: b.EndsOn, Slots: calendar.SlotsFull}
	free := availability.NewFreeSpots()
	engineSpots := make([]availability.Spot, 0, len(spots))
	spotIDs := make([]string, 0, len(spots))
	for _, sp := range spots {
		id, zid, err := parseIDs(sp.ID, sp.ZoneID)
		if err != nil {
			return nil, err
		}
		spotIDs = append(spotIDs, sp.ID)
		engineSpots = append(engineSpots, availability.Spot{
			ID: id, ZoneID: zid, Label: sp.Label, Row: sp.RowLabel, Number: sp.Number, Kind: sp.Kind, Attributes: sp.Attributes, X: sp.X, Y: sp.Y,
		})
		free.AddInterval(all, id)
	}
	occupied, err := store.OccupiedHalfDays(ctx, db, es, spotIDs, b.StartsOn, b.EndsOn, now)
	if err != nil {
		return nil, err
	}
	for _, o := range occupied {
		if id, err := uuidOf(o.SpotID); err == nil {
			free.Remove(o.Date, o.Slot, id)
		}
	}
	bid, cid, err := parseIDs(b.ID, b.CustomerID)
	if err != nil {
		return nil, err
	}
	lines := make([]availability.Line, 0, len(pending))
	for lineID, idx := range pending {
		l := rows[idx]
		lid, zid, err := parseIDs(lineID, l.ZoneID)
		if err != nil {
			return nil, err
		}
		lines = append(lines, availability.Line{
			ID: lid, BookingID: bid, CustomerID: cid, ZoneID: zid,
			Interval:  availability.Interval{From: b.StartsOn, To: b.EndsOn, Slots: b.Slots},
			SpotKinds: cat.packs[l.PackID].RequiresSpotKind,
		})
	}
	res := availability.AutoAssign(availability.AssignInput{Lines: lines, Spots: engineSpots, Free: free})
	out := map[string]string{}
	for _, a := range res.Assignments {
		if a.Split() || len(a.Segments) == 0 {
			continue
		}
		out[a.LineID.String()] = a.SpotID().String()
	}
	return out, nil
}

// DetailIn loads the detail of a booking of es through db (no permission
// check: the caller authenticated a checkout session or a magic link).
func (s *Service) DetailIn(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*Detail, error) {
	b, err := store.GetBooking(ctx, db, es, bookingID)
	if err != nil {
		return nil, notFound(err)
	}
	return s.loadDetail(ctx, db, es, b)
}
