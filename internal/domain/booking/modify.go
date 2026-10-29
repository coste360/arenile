package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/priceoverride"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// errDryRun rolls back the transaction of a dry run after computing it: a
// dry run executes exactly the code of the real change (same re-pricing,
// same release + allocation against the database constraints) and then
// discards every write, so the preview can never disagree with the outcome.
var errDryRun = errors.New("booking: dry run")

// changeInTx runs a modification of booking id as one unit of work, or as a
// dry run (rolled back). Conflicts of a dry run are returned in the result
// instead of failing.
func (s *Service) changeInTx(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, dryRun bool,
	fn func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error),
) (*ChangeResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	return s.changeAs(ctx, p.Actor(reqctx.From(ctx).ClientIP), es, id, expected, dryRun, fn)
}

// changeAs is changeInTx once the actor and the establishment are resolved
// (panel principal or public-channel actor).
func (s *Service) changeAs(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, id string, expected *int64, dryRun bool,
	fn func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error),
) (*ChangeResult, error) {
	res := &ChangeResult{}
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		d, err := s.mutateInTx(ctx, tx, actor, es, id, expected, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
			return fn(ctx, tx, es, b, res)
		})
		var ce *availability.ConflictError
		if dryRun && errors.As(err, &ce) {
			res.Conflicts = ce.Conflicts
			return errDryRun
		}
		if err != nil {
			return err
		}
		res.Detail = d
		if dryRun {
			return errDryRun
		}
		return nil
	})
	switch {
	case errors.Is(err, errDryRun):
		if res.Detail == nil {
			b, err := store.GetBooking(ctx, s.d.DB, es, id)
			if err != nil {
				return nil, notFound(err)
			}
			if res.Detail, err = s.loadDetail(ctx, s.d.DB, es, b); err != nil {
				return nil, err
			}
		}
		if res.Conflicts == nil {
			res.Conflicts = []availability.Conflict{}
		}
		return res, nil
	case err != nil:
		return nil, err
	}
	res.Applied = true
	res.Conflicts = []availability.Conflict{}
	return res, nil
}

// modifiable reports whether dates, lines and customer may still change.
func modifiable(st Status) bool {
	switch st {
	case StatusDraft, StatusHeld, StatusPendingPayment, StatusAwaitingApproval, StatusConfirmed, StatusCheckedIn:
		return true
	}
	return false
}

// allocationKind is the kind of the allocations of a booking in its status.
func allocationKind(b *store.Booking) inventory.Kind {
	switch {
	case Status(b.Status).IsHold():
		return inventory.KindHold
	case b.IsSeasonPass:
		return inventory.KindSeason
	}
	return inventory.KindBooking
}

// Update modifies dates, slots, formula, lines, extras, customer or notes
// (spec 10.3) with re-pricing and atomic re-allocation: the old allocations
// are released and the new ones taken in the same transaction, so a
// conflict leaves the original booking intact. The snapshots of a confirmed
// booking are kept (INV-21); the new total yields the difference to collect
// or refund. expected is the If-Match version. Requires auth.PermBookingsWrite.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in PatchInput) (*ChangeResult, error) {
	return s.changeInTx(ctx, p, establishmentID, id, expected, in.DryRun, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error) {
		if err := refuseTicketOwned(ctx, tx, es, b); err != nil {
			return nil, err
		}
		return s.modifyLocked(ctx, tx, p, es, b, in, res)
	})
}

// refuseTicketOwned is ErrTicketOwned when a daily ticket owns b (ADR 0020):
// the generic cancel, modify and move would leave the ticket row (status,
// date, zone, needs_assignment) out of step, so they are refused and the
// ticket endpoints, which change both in one transaction, are the only
// way. Called with the booking row locked.
func refuseTicketOwned(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) error {
	owned, err := store.TicketOwnsBooking(ctx, tx, es, b.ID)
	if err != nil {
		return err
	}
	if owned {
		return ErrTicketOwned
	}
	return nil
}

// withoutTicketLines drops the pending lines whose booking is owned by a
// daily ticket (see refuseTicketOwned).
func withoutTicketLines(ctx context.Context, db bun.IDB, es store.EstablishmentScope, lines []store.PendingLine) ([]store.PendingLine, error) {
	if len(lines) == 0 {
		return lines, nil
	}
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.BookingID)
	}
	owned, err := store.TicketOwnedBookingIDs(ctx, db, es, ids)
	if err != nil {
		return nil, err
	}
	if len(owned) == 0 {
		return lines, nil
	}
	out := lines[:0]
	for _, l := range lines {
		if !owned[l.BookingID] {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *Service) modifyLocked(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking, in PatchInput, res *ChangeResult) (*outcome, error) {
	st := Status(b.Status)
	if !modifiable(st) {
		return nil, ErrNotModifiable
	}
	diff := map[string]any{}
	newStay := stay{StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, Formula: b.Formula, Channel: b.Channel}
	if b.IsSeasonPass && b.SeasonID != nil {
		newStay.SeasonID = *b.SeasonID
	}
	if in.StartsOn != nil {
		newStay.StartsOn = *in.StartsOn
	}
	if in.EndsOn != nil {
		newStay.EndsOn = *in.EndsOn
	}
	if in.Slots != nil {
		newStay.Slots = *in.Slots
	}
	if in.Formula != nil {
		newStay.Formula = *in.Formula
	}
	v := &configerr.ValidationError{}
	validateStay(v, newStay.StartsOn, newStay.EndsOn, newStay.Slots, newStay.Formula)
	if in.Lines != nil && len(in.Lines) == 0 {
		v.Add("lines", "Almeno una riga.")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	if err := applyPatchM8(ctx, tx, es, b, in, diff); err != nil {
		return nil, err
	}
	if in.CustomerID != nil && *in.CustomerID != b.CustomerID {
		c, err := s.d.Customers.Resolve(ctx, tx, p, es, *in.CustomerID, nil)
		if err != nil {
			return nil, err
		}
		diff["customer_changed"] = true
		b.CustomerID = c.ID
	}
	rows, extras, _, err := s.currentLines(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	lineIn, bookingExtras := linesToInput(rows, extras)
	replace := in.Lines != nil
	if replace {
		lineIn = in.Lines
		for i, l := range lineIn {
			if len(l.Segments) > 0 {
				if msg := coverError(l.Segments, newStay.StartsOn, newStay.EndsOn); msg != "" {
					return nil, configerr.Invalid(fmt.Sprintf("lines[%d].segments", i), msg)
				}
			}
		}
	}
	if in.ExtrasSet {
		bookingExtras = in.Extras
	}
	// Split stays and scheduled moves (INV-102, INV-106): the lines that are
	// kept keep their segments, fitted to the new dates when these change;
	// each day stays priced with the spot of that day and the allocations
	// before a scheduled move are never rewritten onto the destination.
	var kept map[string][]SegmentInput
	spotsIn := lineIn
	if !replace && st.OccupiesResources() {
		cur, err := allocSegments(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		kept = map[string][]SegmentInput{}
		spotsIn = slices.Clone(lineIn)
		for _, r := range rows {
			sg := fitSegments(cur[r.ID], newStay.StartsOn, newStay.EndsOn)
			if len(sg) == 0 {
				continue
			}
			kept[r.ID] = sg
			for _, x := range sg {
				spotsIn = append(spotsIn, LineInput{SpotID: x.SpotID})
			}
		}
	}
	cat, err := loadCatalog(ctx, tx, es, spotsIn, bookingExtras)
	if err != nil {
		return nil, err
	}
	lines, err := cat.resolve(lineIn, "lines")
	if err != nil {
		return nil, err
	}
	if kept != nil {
		withSegments(lines, rows, kept)
	}
	pr, err := s.repriceAs(ctx, tx, es, b, newStay, lines, bookingExtras, cat)
	if err != nil {
		return nil, err
	}
	if len(pr.res.Violations) > 0 && !in.Force {
		return nil, &policy.ViolationError{Violations: pr.res.Violations}
	}
	if in.Force && len(pr.res.Violations) > 0 {
		diff["forced"] = true
		diff["violations"] = pr.res.Violations
	}
	// Approval rule R1/R4 (approval.go): force never waives the approval.
	if err := s.checkModifyApproval(ctx, tx, p, es, b, pr.approval, in.Force, diff); err != nil {
		return nil, err
	}
	res.PreviousTotal, res.NewTotal = b.TotalAmount, pr.res.Breakdown.Total

	// Resources: release everything, rewrite the lines, allocate again.
	var released *inventory.Allocation
	if st.OccupiesResources() {
		if released, err = s.releaseAll(ctx, tx, es, b); err != nil {
			return nil, err
		}
	}
	if diffDates(b, newStay) {
		diff["from"] = map[string]any{"starts_on": b.StartsOn.String(), "ends_on": b.EndsOn.String(), "slots": string(b.Slots), "formula": b.Formula}
		diff["to"] = map[string]any{"starts_on": newStay.StartsOn.String(), "ends_on": newStay.EndsOn.String(), "slots": string(newStay.Slots), "formula": newStay.Formula}
	}
	b.StartsOn, b.EndsOn, b.Slots, b.Formula = newStay.StartsOn, newStay.EndsOn, newStay.Slots, newStay.Formula
	now := s.now()
	rows, err = s.rewriteRows(ctx, tx, es, b, rows, pr, cat, now, replace)
	if err != nil {
		return nil, err
	}
	if replace {
		diff["lines_replaced"] = len(rows)
	}
	if in.ExtrasSet {
		diff["extras_replaced"] = len(bookingExtras)
	}
	var allocated *inventory.Allocation
	if st.OccupiesResources() {
		kind := allocationKind(b)
		var exp *time.Time
		if kind == inventory.KindHold {
			exp = b.HoldExpiresAt
			if exp == nil {
				e := now.Add(s.d.HoldTTL)
				exp = &e
				b.HoldExpiresAt = exp
			}
		}
		newExtras, err := store.ListBookingLineExtras(ctx, tx, es, b.ID)
		if err != nil {
			return nil, err
		}
		if hasSegments(pr.lines) {
			// One allocation per segment (the kept ones, or those of the
			// replaced lines), the plain lines in one request.
			if allocated, err = s.allocateSplit(ctx, tx, es, b, kind, exp, rows, newExtras, cat, pr.lines, now, byDesk(p)); err != nil {
				return nil, err
			}
		} else {
			req := allocRequest(b, kind, exp, rows, newExtras, cat)
			req.ByDesk = byDesk(p)
			if allocated, err = s.d.Inventory.Allocate(ctx, tx, es, req); err != nil {
				return nil, err
			}
			if err := s.applyAllocation(ctx, tx, es, b, rows, allocated, kind, now); err != nil {
				return nil, err
			}
		}
	}
	if err := s.applyTotals(b, pr, now, diff); err != nil {
		return nil, err
	}
	res.NewTotal = b.TotalAmount // a kept manual price (ADR 0019 E)
	if in.NotesCustomer != nil {
		b.NotesCustomer = strings.TrimSpace(*in.NotesCustomer)
		diff["notes_customer_changed"] = true
	}
	if in.NotesInternal != nil {
		b.NotesInternal = strings.TrimSpace(*in.NotesInternal)
		diff["notes_internal_changed"] = true
	}
	diff["previous_total"] = int64(res.PreviousTotal)
	diff["new_total"] = int64(res.NewTotal)
	diff["price_difference"] = int64(res.PriceDifference())
	if b.ConfirmedAt != nil {
		// INV-21: the snapshot stays; the audit keeps the new computation.
		diff["new_breakdown"] = pr.res.Breakdown
	}
	return &outcome{action: eventModified, diff: diff, alloc: mergeAlloc(released, allocated)}, nil
}

// fitSegments fits the segments of a line to a (possibly new) stay from..to:
// the days outside are dropped, the first segment starts with the stay and
// the last one ends with it (a stay extended on either side extends the
// spot of that side). nil when segs is empty or lies entirely outside.
func fitSegments(segs []SegmentInput, from, to calendar.Date) []SegmentInput {
	out := clipBefore(clipFrom(segs, from), to.AddDays(1))
	if len(out) == 0 {
		return nil
	}
	out[0].StartsOn = from
	out[len(out)-1].EndsOn = to
	return out
}

// byDesk reports whether p is a panel user: the cash desk may place a
// booking of any channel on a spot excluded from online sale (spec 30.11),
// while an API key or a public-channel actor follows the online rules.
func byDesk(p *auth.Principal) bool {
	return p != nil && p.Kind == auth.PrincipalUser
}

func diffDates(b *store.Booking, st stay) bool {
	return !b.StartsOn.Equal(st.StartsOn) || !b.EndsOn.Equal(st.EndsOn) || b.Slots != st.Slots || b.Formula != st.Formula
}

// repriceAs prices lines for b with its customer tags, agreement and promo code.
func (s *Service) repriceAs(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, st stay, lines []PricedLine, extras []ExtraInput, cat *catalog) (*priced, error) {
	cust, err := store.GetCustomer(ctx, tx, es, b.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("booking: customer: %w", err)
	}
	agreement := ""
	if b.AgreementID != nil {
		agreement = *b.AgreementID
	}
	promo := ""
	if b.PromoCodeID != nil {
		if pc, err := store.GetPromoCode(ctx, tx, es, *b.PromoCodeID); err == nil {
			promo = pc.Code
		}
	}
	pr, err := s.price(ctx, tx, es, st, lines, extras, cust.Tags, agreement, promo, cat)
	if err != nil && promo != "" && errors.Is(err, configerr.ErrValidation) {
		// A promo code no longer applicable (expired, other dates) must not
		// block a modification: price without it.
		pr, err = s.price(ctx, tx, es, st, lines, extras, cust.Tags, agreement, "", cat)
	}
	return pr, err
}

// rewriteRows stores the lines and extras of pr: replace deletes the old
// lines and inserts new ones, otherwise the existing lines keep their ids
// (same order) and get the new spot, zone, category and totals. Extras are
// always rewritten. The caller has released the allocations.
func (s *Service) rewriteRows(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, old []store.BookingLine, pr *priced, cat *catalog, now time.Time, replace bool) ([]store.BookingLine, error) {
	rows, extras := buildRows(b.ID, pr, cat, now)
	if replace || len(rows) != len(old) {
		if err := store.DeleteBookingLines(ctx, tx, es, b.ID); err != nil {
			return nil, err
		}
		if err := store.InsertBookingLines(ctx, tx, es, rows); err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
		if err := store.InsertBookingLineExtras(ctx, tx, es, extras); err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
		return rows, nil
	}
	if err := store.DeleteBookingExtras(ctx, tx, es, b.ID); err != nil {
		return nil, err
	}
	remap := make(map[string]string, len(rows))
	for i := range rows {
		remap[rows[i].ID] = old[i].ID
		rows[i].ID, rows[i].CreatedAt = old[i].ID, old[i].CreatedAt
		if err := store.UpdateBookingLine(ctx, tx, es, &rows[i]); err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
	}
	for i := range extras {
		if extras[i].BookingLineID != nil {
			id := remap[*extras[i].BookingLineID]
			extras[i].BookingLineID = &id
		}
	}
	if err := store.InsertBookingLineExtras(ctx, tx, es, extras); err != nil {
		return nil, fmt.Errorf("booking: %w", err)
	}
	return rows, nil
}

// applyTotals sets the new total and deposit. Before the confirmation the
// snapshots are replaced; after it they stay (INV-21) and the re-quote is
// stored as price_current, so that total_amount always equals the total of
// the breakdown that prices the booking (INV-12, ADR 0011) and payments split
// their VAT on the current rate mix.
//
// A manual price of a season pass (ADR 0019 E, INV-77) survives every
// re-pricing edit, whoever makes it: only SetPriceOverride and
// ClearPriceOverride (establishment.configure, audited) change or remove
// it. The engine total beneath it is updated and price_current is rebuilt
// on the new engine breakdown, so that "Torna al listino" restores the
// price of the stay as it is now; the audit diff records price_override_kept
// and the new engine_total.
func (s *Service) applyTotals(b *store.Booking, pr *priced, now time.Time, diff map[string]any) error {
	engineTotal := pr.res.Breakdown.Total
	if pr.res.SeasonID != "" {
		sid := pr.res.SeasonID
		b.SeasonID = &sid
	}
	if b.PriceOverrideAmount != nil {
		amount := *b.PriceOverrideAmount
		var prev json.RawMessage
		if b.ConfirmedAt == nil {
			if err := setSnapshots(b, pr, b.Channel, now); err != nil {
				return err
			}
		} else {
			cur, err := json.Marshal(pr.res.Breakdown)
			if err != nil {
				return fmt.Errorf("booking: price current: %w", err)
			}
			prev = cur
		}
		cur, err := priceoverride.Apply(pr.res.Breakdown, prev, amount)
		if err != nil {
			return fmt.Errorf("booking: %w", err)
		}
		b.PriceCurrent, b.TotalAmount = cur, amount
		b.PriceOverrideEngineAmount = &engineTotal
		b.DepositDue = policy.Deposit(amount, snapshotRules(b))
		if diff != nil {
			diff["price_override_kept"] = true
			diff["engine_total"] = int64(engineTotal)
		}
		return nil
	}
	b.TotalAmount = engineTotal
	if b.ConfirmedAt != nil {
		b.DepositDue = policy.Deposit(b.TotalAmount, snapshotRules(b))
		cur, err := json.Marshal(pr.res.Breakdown)
		if err != nil {
			return fmt.Errorf("booking: price current: %w", err)
		}
		b.PriceCurrent = cur
		return nil
	}
	b.DepositDue = pr.res.DepositDue
	b.PriceCurrent = nil
	return setSnapshots(b, pr, b.Channel, now)
}

// ---- move -------------------------------------------------------------------------

// Move moves one line to another spot (spec 11.3 drag and drop), for the
// whole stay or from a date; for a pending line of a zone-mode zone it is the
// assignment. The line is re-priced (the price category may change) and the
// release + allocation happen in one transaction. Requires
// auth.PermBookingsWrite.
func (s *Service) Move(ctx context.Context, p *auth.Principal, establishmentID, id string, in MoveInput) (*ChangeResult, error) {
	return s.move(ctx, p, establishmentID, id, in, true)
}

// move is Move; undoable writes the 5 minute undo entry (spec 30.7, the
// panel move; not the auto-assignment).
func (s *Service) move(ctx context.Context, p *auth.Principal, establishmentID, id string, in MoveInput, undoable bool) (*ChangeResult, error) {
	if in.LineID == "" {
		return nil, configerr.Invalid("line_id", "Campo obbligatorio.")
	}
	if in.SpotID == "" {
		return nil, configerr.Invalid("spot_id", "Campo obbligatorio.")
	}
	res, err := s.changeInTx(ctx, p, establishmentID, id, in.ExpectedVersion, in.DryRun, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error) {
		// A ticket booking is placed only through MoveThen (ticket update in
		// the same transaction).
		if err := refuseTicketOwned(ctx, tx, es, b); err != nil {
			return nil, err
		}
		if undoable {
			return s.moveRecorded(ctx, tx, p, es, b, in, res)
		}
		return s.moveLocked(ctx, tx, p, es, b, in, res)
	})
	if res != nil && !res.Applied {
		res.Undo = nil // a dry run wrote nothing
	}
	return res, err
}

func (s *Service) moveLocked(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking, in MoveInput, res *ChangeResult) (*outcome, error) {
	st := Status(b.Status)
	if !modifiable(st) {
		return nil, ErrNotModifiable
	}
	rows, extras, _, err := s.currentLines(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i := range rows {
		if rows[i].ID == in.LineID {
			idx = i
		}
	}
	if idx < 0 {
		return nil, ErrLineNotFound
	}
	// The days already past never change (spec 30.7): an explicit from in
	// the past is refused; a move without from of a stay in progress starts
	// today and leaves the past days where they were.
	today, err := s.today(ctx, tx, es)
	if err != nil {
		return nil, err
	}
	if in.From != nil && in.From.Before(today) {
		return nil, configerr.Invalid("from", "La data non può essere nel passato.")
	}
	if in.From == nil && today.After(b.StartsOn) && !today.After(b.EndsOn) && st.OccupiesResources() {
		in.From = &today
	}
	partial := in.From != nil && in.From.After(b.StartsOn)
	if in.From != nil && in.From.After(b.EndsOn) {
		return nil, configerr.Invalid("from", "La data deve rientrare nel periodo della prenotazione.")
	}
	lineIn, bookingExtras := linesToInput(rows, extras)
	lineIn[idx].SpotID, lineIn[idx].ZoneID = in.SpotID, ""
	cat, err := loadCatalog(ctx, tx, es, lineIn, bookingExtras)
	if err != nil {
		return nil, err
	}
	lines, err := cat.resolve(lineIn, "lines")
	if err != nil {
		var ve *configerr.ValidationError
		if errors.As(err, &ve) {
			return nil, configerr.Invalid("spot_id", "Postazione non valida per questa riga: "+ve.Fields[0].Message)
		}
		return nil, err
	}
	// Split stays (ADR 0028): each day is priced with the spot of that day,
	// the moved line keeps its spots before from.
	cur, err := allocSegments(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	if partial {
		cur[in.LineID] = joinSegments(clipBefore(cur[in.LineID], *in.From), []SegmentInput{{SpotID: in.SpotID, StartsOn: *in.From, EndsOn: b.EndsOn}})
	} else {
		delete(cur, in.LineID)
	}
	withSegments(lines, rows, cur)
	st2 := stay{StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, Formula: b.Formula, Channel: b.Channel}
	if b.IsSeasonPass && b.SeasonID != nil {
		st2.SeasonID = *b.SeasonID
	}
	pr, err := s.repriceAs(ctx, tx, es, b, st2, lines, bookingExtras, cat)
	if err != nil {
		return nil, err
	}
	// Approval rule R4 (approval.go): a move onto a spot whose policy needs
	// approval is refused to the desk on a confirmed booking.
	approvedDiff := map[string]any{}
	if err := s.checkModifyApproval(ctx, tx, p, es, b, pr.approval, false, approvedDiff); err != nil {
		return nil, err
	}
	res.PreviousTotal, res.NewTotal = b.TotalAmount, pr.res.Breakdown.Total
	now := s.now()

	var released, allocated *inventory.Allocation
	from := b.StartsOn
	if partial {
		from = *in.From
	}
	if st.OccupiesResources() {
		rr := inventory.ReleaseRequest{BookingID: b.ID, LineIDs: []string{in.LineID}}
		if partial {
			rr.From = &from
		}
		if released, err = s.d.Inventory.Release(ctx, tx, es, rr); err != nil {
			return nil, err
		}
		if err := s.unmarkResold(ctx, tx, es, []string{in.LineID}); err != nil {
			return nil, err
		}
	}
	prevSpot := ""
	if rows[idx].SpotID != nil {
		prevSpot = *rows[idx].SpotID
	}
	if rows, err = s.rewriteRows(ctx, tx, es, b, rows, pr, cat, now, false); err != nil {
		return nil, err
	}
	if st.OccupiesResources() {
		kind := allocationKind(b)
		newExtras, err := store.ListBookingLineExtras(ctx, tx, es, b.ID)
		if err != nil {
			return nil, err
		}
		// The first line also carries the booking-level components.
		req := allocRequest(b, kind, b.HoldExpiresAt, rows[idx:idx+1], lineExtras(newExtras, rows[idx].ID, idx == 0), cat)
		req.StartsOn, req.ByDesk = from, byDesk(p)
		// A season pass keeps the days its holder released (INV-10).
		if req.Skip, err = seasonSkips(ctx, tx, es, b, in.LineID, from, b.EndsOn); err != nil {
			return nil, err
		}
		if allocated, err = s.d.Inventory.Allocate(ctx, tx, es, req); err != nil {
			return nil, err
		}
		if err := s.applyAllocation(ctx, tx, es, b, rows[idx:idx+1], allocated, kind, now); err != nil {
			return nil, err
		}
		if err := s.moveSeasonReleases(ctx, tx, es, b, kind, in.LineID, []SegmentInput{{SpotID: in.SpotID, StartsOn: from, EndsOn: b.EndsOn}}); err != nil {
			return nil, err
		}
	}
	diff := map[string]any{"line_id": in.LineID, "from_spot_id": prevSpot, "to_spot_id": in.SpotID, "from": from.String()}
	prevTotal, prevCurrent, prevDeposit := b.TotalAmount, b.PriceCurrent, b.DepositDue
	if err := s.applyTotals(b, pr, now, diff); err != nil {
		return nil, err
	}
	if in.KeepPrice && b.PriceOverrideAmount == nil {
		// Daily ticket (ADR 0020): the guest paid the ticket at the desk, so
		// the placement never changes the total; the engine price of the
		// new spot stays in the audit diff only.
		diff["price_kept"], diff["engine_total"] = true, int64(pr.res.Breakdown.Total)
		b.TotalAmount, b.PriceCurrent, b.DepositDue = prevTotal, prevCurrent, prevDeposit
	}
	res.NewTotal = b.TotalAmount // a kept manual price (ADR 0019 E) or a kept ticket price
	diff["previous_total"], diff["new_total"] = int64(res.PreviousTotal), int64(res.NewTotal)
	for k, v := range approvedDiff {
		diff[k] = v
	}
	return &outcome{action: actionMoved, event: eventModified, diff: diff, alloc: mergeAlloc(released, allocated)}, nil
}

// lineExtras keeps the extras of one line, plus the booking-level ones when
// withBooking (they are allocated with the first line, see allocRequest).
func lineExtras(xs []store.BookingLineExtra, lineID string, withBooking bool) []store.BookingLineExtra {
	var out []store.BookingLineExtra
	for _, x := range xs {
		if (x.BookingLineID != nil && *x.BookingLineID == lineID) || (withBooking && x.BookingLineID == nil) {
			out = append(out, x)
		}
	}
	return out
}

func uuidOf(s string) (uuid.UUID, error) { return uuid.Parse(s) }

func parseIDs(a, b string) (uuid.UUID, uuid.UUID, error) {
	x, err := uuid.Parse(a)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("booking: id %q: %w", a, err)
	}
	y, err := uuid.Parse(b)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("booking: id %q: %w", b, err)
	}
	return x, y, nil
}

// ---- quote ------------------------------------------------------------------------

// Quote previews a creation without writing: price, policy evaluation and
// the conflicts the allocation would hit (auth.PermBookingsRead).
func (s *Service) Quote(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Quote, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	in.Channel = auth.ChannelFor(p, in.Channel) // the channel the creation would use (ADR 0011)
	if err := validateCreate(&in); err != nil {
		return nil, err
	}
	var tags []string
	agreement := in.AgreementID
	if in.CustomerID != "" {
		c, err := store.GetCustomer(ctx, s.d.DB, es, in.CustomerID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("customer_id", "Cliente inesistente.")
		}
		if err != nil {
			return nil, fmt.Errorf("booking: customer: %w", err)
		}
		tags = c.Tags
		if agreement == "" && c.AgreementID != nil {
			agreement = *c.AgreementID
		}
	} else if in.Customer != nil {
		tags = in.Customer.Tags
		if agreement == "" {
			agreement = in.Customer.AgreementID
		}
	}
	cat, err := loadCatalog(ctx, s.d.DB, es, in.Lines, in.Extras)
	if err != nil {
		return nil, err
	}
	lines, err := cat.resolve(in.Lines, "lines")
	if err != nil {
		return nil, err
	}
	st := stay{StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula, Channel: in.Channel, SeasonID: in.SeasonID}
	pr, err := s.price(ctx, s.d.DB, es, st, lines, in.Extras, tags, agreement, in.PromoCode, cat)
	if err != nil {
		return nil, err
	}
	out := &Quote{Result: pr.res, Conflicts: []availability.Conflict{}}
	if in.Status == StatusDraft || in.HoldID != "" {
		return out, nil
	}
	b := &store.Booking{Channel: in.Channel, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots}
	rows, extras := buildRows("", pr, cat, s.now())
	req := allocRequest(b, inventory.KindBooking, nil, rows, extras, cat)
	for i := range req.Lines {
		req.Lines[i].LineID = ""
	}
	conflicts, err := s.checkSplit(ctx, s.d.DB, es, req, pr.lines, cat)
	if err != nil {
		return nil, err
	}
	if conflicts != nil {
		out.Conflicts = conflicts
	}
	return out, nil
}

// ---- auto-assignment ------------------------------------------------------------------

// maxAutoAssignDays bounds the interval of one auto-assignment run.
const maxAutoAssignDays = 62

// AutoAssign places the pending lines of zone-mode zones (spec 7.5) with the
// pure greedy engine availability.AutoAssign on the spots free in the
// database; applying runs one Move per line (own transaction, audit, outbox),
// so a line taken meanwhile by someone else is reported as unassigned and
// never double-booked (the unique allocation key decides). DryRun only
// computes the plan. Requires auth.PermBookingsWrite.
func (s *Service) AutoAssign(ctx context.Context, p *auth.Principal, establishmentID string, in AutoAssignInput) (*AutoAssignResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	v := &configerr.ValidationError{}
	if in.From.IsZero() {
		v.Add("from", "Campo obbligatorio.")
	}
	if in.To.IsZero() || in.To.Before(in.From) {
		v.Add("to", "Deve essere successiva o uguale a from.")
	} else if in.From.DaysUntil(in.To) >= maxAutoAssignDays {
		v.Add("to", fmt.Sprintf("Intervallo massimo %d giorni.", maxAutoAssignDays))
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	plan, labels, err := s.autoAssignPlan(ctx, es, in)
	if err != nil {
		return nil, err
	}
	out := &AutoAssignResult{Assigned: []AutoAssignment{}, Unassigned: plan.unassigned}
	if in.DryRun {
		for _, a := range plan.assigned {
			a.SpotLabel = labels[a.SpotID]
			out.Assigned = append(out.Assigned, a)
		}
		return out, nil
	}
	for _, a := range plan.assigned {
		_, err := s.move(ctx, p, establishmentID, a.BookingID, MoveInput{LineID: a.LineID, SpotID: a.SpotID}, false)
		var ce *availability.ConflictError
		switch {
		case errors.As(err, &ce), errors.Is(err, availability.ErrSpotUnavailable), errors.Is(err, configerr.ErrVersionConflict):
			out.Unassigned = append(out.Unassigned, AutoUnassigned{BookingID: a.BookingID, LineID: a.LineID, Reason: "conflict"})
			continue
		case err != nil:
			return nil, err
		}
		a.SpotLabel = labels[a.SpotID]
		out.Assigned = append(out.Assigned, a)
	}
	out.Applied = true
	return out, nil
}

type autoPlan struct {
	assigned   []AutoAssignment
	unassigned []AutoUnassigned
}

// occupyingStatuses are the statuses whose lines hold resources.
var occupyingStatuses = []string{
	string(StatusHeld), string(StatusPendingPayment), string(StatusAwaitingApproval), string(StatusConfirmed), string(StatusCheckedIn),
}

func (s *Service) autoAssignPlan(ctx context.Context, es store.EstablishmentScope, in AutoAssignInput) (*autoPlan, map[string]string, error) {
	db := s.d.DB
	pending, err := store.ListPendingLines(ctx, db, es, in.From, in.To, in.ZoneID, occupyingStatuses)
	if err != nil {
		return nil, nil, err
	}
	// Zone-first daily tickets are pending lines too, but they are placed
	// by their own auto-assignment (internal/domain/ticketassign), which
	// updates the ticket row in the same transaction: leave them out here.
	if pending, err = withoutTicketLines(ctx, db, es, pending); err != nil {
		return nil, nil, err
	}
	plan := &autoPlan{unassigned: []AutoUnassigned{}}
	if len(pending) == 0 {
		return plan, map[string]string{}, nil
	}
	var zoneIDs []string
	from, to := pending[0].StartsOn, pending[0].EndsOn
	for _, l := range pending {
		if !contains(zoneIDs, l.ZoneID) {
			zoneIDs = append(zoneIDs, l.ZoneID)
		}
		if l.StartsOn.Before(from) {
			from = l.StartsOn
		}
		if l.EndsOn.After(to) {
			to = l.EndsOn
		}
	}
	spots, err := store.ListZoneSpots(ctx, db, es, zoneIDs)
	if err != nil {
		return nil, nil, err
	}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return nil, nil, fmt.Errorf("booking: packs: %w", err)
	}
	kinds := map[string][]string{}
	for _, pk := range packs {
		kinds[pk.ID] = pk.RequiresSpotKind
	}
	spotIDs := make([]string, len(spots))
	labels := make(map[string]string, len(spots))
	engineSpots := make([]availability.Spot, 0, len(spots))
	free := availability.NewFreeSpots()
	all := availability.Interval{From: from, To: to, Slots: calendar.SlotsFull}
	for i, sp := range spots {
		spotIDs[i] = sp.ID
		labels[sp.ID] = sp.Label
		id, zid, err := parseIDs(sp.ID, sp.ZoneID)
		if err != nil {
			return nil, nil, err
		}
		engineSpots = append(engineSpots, availability.Spot{
			ID: id, ZoneID: zid, Label: sp.Label, Row: sp.RowLabel, Number: sp.Number, Kind: sp.Kind, Attributes: sp.Attributes,
			OnlineExcluded: sp.OnlineExcluded,
		})
		free.AddInterval(all, id)
	}
	occupied, err := store.OccupiedHalfDays(ctx, db, es, spotIDs, from, to, s.now())
	if err != nil {
		return nil, nil, err
	}
	for _, o := range occupied {
		if id, err := uuidOf(o.SpotID); err == nil {
			free.Remove(o.Date, o.Slot, id)
		}
	}
	lines := make([]availability.Line, 0, len(pending))
	owners := map[string]string{}
	for _, l := range pending {
		lid, bid, err := parseIDs(l.ID, l.BookingID)
		if err != nil {
			return nil, nil, err
		}
		zid, _ := uuidOf(l.ZoneID)
		cid, _ := uuidOf(l.CustomerID)
		owners[l.ID] = l.BookingID
		lines = append(lines, availability.Line{
			ID: lid, BookingID: bid, CustomerID: cid, ZoneID: zid,
			Interval:    availability.Interval{From: l.StartsOn, To: l.EndsOn, Slots: l.Slots},
			SpotKinds:   kinds[l.PackID],
			SelfService: l.Channel != store.ChannelDesk, // never an excluded spot (INV-105)
		})
	}
	res := availability.AutoAssign(availability.AssignInput{Lines: lines, Spots: engineSpots, Free: free})
	for _, a := range res.Assignments {
		lineID := a.LineID.String()
		if a.Split() {
			plan.unassigned = append(plan.unassigned, AutoUnassigned{BookingID: owners[lineID], LineID: lineID, Reason: string(availability.ReasonNoSpotForWholePeriod)})
			continue
		}
		plan.assigned = append(plan.assigned, AutoAssignment{BookingID: owners[lineID], LineID: lineID, SpotID: a.SpotID().String()})
	}
	for _, u := range res.Unassigned {
		lineID := u.LineID.String()
		plan.unassigned = append(plan.unassigned, AutoUnassigned{BookingID: owners[lineID], LineID: lineID, Reason: string(u.Reason)})
	}
	return plan, labels, nil
}
