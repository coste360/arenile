//go:build integration

package booking_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var d = bookingtest.D

func i64(v int64) *int64 { return &v }

// sameJSON compares two documents semantically (PostgreSQL jsonb reorders keys).
func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func allocCount(f *bookingtest.Fixture, bookingID string) int {
	return f.Count((*store.SpotAllocation)(nil), "booking_id = ?", bookingID)
}

func TestBookingCreate_ConfirmedDeskFlow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-12")
		in.Lines[0].Extras = []booking.ExtraInput{{ExtraID: f.Sunbed.ID, Quantity: 1}}
		det := f.MustCreate(in)
		b := det.Booking
		if b.Status != string(booking.StatusConfirmed) || b.Channel != store.ChannelDesk || b.ConfirmedAt == nil || b.HoldExpiresAt != nil {
			t.Fatalf("booking %+v", b)
		}
		if b.Code != "LD-2027-000001" {
			t.Fatalf("code %q", b.Code)
		}
		want := money.Cents(3 * (bookingtest.BasePerDay + bookingtest.SunbedPerDay))
		if b.TotalAmount != want || b.DepositDue != want || b.PaidAmount != 0 {
			t.Fatalf("totals %d %d %d, want %d", b.TotalAmount, b.DepositDue, b.PaidAmount, want)
		}
		if len(det.Lines) != 1 || det.Lines[0].LineTotal != want || det.Lines[0].PriceCategory != "front" || det.Lines[0].AssignmentStatus != store.AssignmentAssigned {
			t.Fatalf("lines %+v", det.Lines)
		}
		if len(det.Extras) != 1 || det.Extras[0].Total != 3*bookingtest.SunbedPerDay {
			t.Fatalf("extras %+v", det.Extras)
		}
		// INV-07 (through the port): FULL x 3 days = 6 rows, all kind booking.
		if len(det.SpotAllocations) != 6 {
			t.Fatalf("allocations %d", len(det.SpotAllocations))
		}
		for _, a := range det.SpotAllocations {
			if a.Kind != store.AllocationBooking || a.ExpiresAt != nil {
				t.Fatalf("allocation %+v", a)
			}
		}
		// Snapshots: breakdown total and the policy of the default rules.
		var snap struct {
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(b.PriceSnapshot, &snap); err != nil || snap.Total != int64(want) {
			t.Fatalf("price snapshot %s", b.PriceSnapshot)
		}
		if _, err := policy.ParseSnapshot(b.PolicySnapshot); err != nil {
			t.Fatalf("policy snapshot: %v", err)
		}
		// Audit + outbox in the same transaction (INV-18).
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ?", b.ID, event.BookingCreated) != 1 {
			t.Fatal("audit booking.created missing")
		}
		for _, typ := range []string{event.BookingCreated, event.BookingConfirmed} {
			if f.Count((*store.OutboxEvent)(nil), "type = ?", typ) != 1 {
				t.Fatalf("outbox %s missing", typ)
			}
		}
		if len(det.History) != 1 || det.History[0].Action != event.BookingCreated {
			t.Fatalf("history %+v", det.History)
		}
		// The next code of the series.
		det2 := f.MustCreate(f.Input(f.Spots[1], "2027-07-10", "2027-07-10"))
		if det2.Booking.Code != "LD-2027-000002" {
			t.Fatalf("second code %q", det2.Booking.Code)
		}
		// Existing customer, read back.
		in3 := f.Input(f.Spots[2], "2027-07-10", "2027-07-10")
		in3.Customer, in3.CustomerID = nil, b.CustomerID
		det3 := f.MustCreate(in3)
		if det3.Customer.ID != b.CustomerID {
			t.Fatal("existing customer not linked")
		}
		got, err := f.Svc.Get(f.Ctx, f.Viewer, f.Est.ID, b.ID)
		if err != nil || got.Booking.Version != 1 || len(got.SpotAllocations) != 6 || got.SpotLabels[f.Spots[0].ID] != "A1" {
			t.Fatalf("get: %v %+v", err, got)
		}
	})
}

func TestBookingCreate_ConflictLeavesNoRows(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		bookings, customers := f.Count((*store.Booking)(nil), ""), f.Count((*store.Customer)(nil), "")
		audits, events := f.Count((*store.AuditEntry)(nil), ""), f.Count((*store.OutboxEvent)(nil), "")
		_, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, f.Input(f.Spots[0], "2027-07-12", "2027-07-13"))
		var ce *availability.ConflictError
		if !errors.As(err, &ce) || !errors.Is(err, availability.ErrSpotUnavailable) || len(ce.Conflicts) != 2 {
			t.Fatalf("err = %v", err)
		}
		// INV-18: nothing of the failed transaction remains (inline customer included).
		if f.Count((*store.Booking)(nil), "") != bookings || f.Count((*store.Customer)(nil), "") != customers ||
			f.Count((*store.AuditEntry)(nil), "") != audits || f.Count((*store.OutboxEvent)(nil), "") != events ||
			f.Count((*store.BookingLine)(nil), "") != 1 {
			t.Fatal("orphan rows after a refused creation")
		}
		// A quote reports the same conflicts without writing.
		q, err := f.Svc.Quote(f.Ctx, f.Viewer, f.Est.ID, f.Input(f.Spots[0], "2027-07-12", "2027-07-13"))
		if err != nil || len(q.Conflicts) != 2 || q.Result.Breakdown.Total != 2*bookingtest.BasePerDay {
			t.Fatalf("quote: %v %+v", err, q)
		}
	})
}

func TestBookingCreate_ValidationPolicyAndForce(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		// Seat zones need a spot; foreign spots do not exist here.
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.Lines[0].SpotID, in.Lines[0].ZoneID = "", f.SeatZone.ID
		if _, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, in); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("seat without spot: %v", err)
		}
		if _, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, f.Input(f.OtherSpot, "2027-07-10", "2027-07-10")); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign spot: %v", err)
		}
		// Default policy: at most 30 days (duration_too_long) unless forced.
		long := f.Input(f.Spots[1], "2027-07-01", "2027-08-05")
		_, err := f.Svc.Create(f.Ctx, f.Owner, f.Est.ID, long)
		var ve *policy.ViolationError
		if !errors.As(err, &ve) || !ve.Has(policy.CodeDurationTooLong) {
			t.Fatalf("violation: %v", err)
		}
		long.Force = true
		det := f.MustCreate(long)
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND CAST(diff AS TEXT) LIKE ?", det.Booking.ID, "%forced%") != 1 {
			t.Fatal("forced creation not audited")
		}
	})
}

// TestBookingConfirm_ClearsHoldExpiry covers INV-06 and INV-20 through the
// approval flow: awaiting_approval holds with an expiry, confirmation makes
// them stable and clears hold_expires_at.
func TestBookingConfirm_ClearsHoldExpiry(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.Lines[0].PackID = f.VIP.ID
		det := f.MustCreate(in)
		b := det.Booking
		if b.Status != string(booking.StatusAwaitingApproval) || b.HoldExpiresAt == nil || b.ConfirmedAt != nil {
			t.Fatalf("approval booking %+v", b)
		}
		for _, a := range det.SpotAllocations {
			if a.Kind != store.AllocationHold || a.ExpiresAt == nil {
				t.Fatalf("hold allocation %+v", a)
			}
		}
		if f.Count((*store.OutboxEvent)(nil), "type = ?", event.BookingApprovalRequested) != 1 {
			t.Fatal("approval_requested event missing")
		}
		// Check-in is not allowed before approval (409 invalid_transition).
		_, err := f.Svc.CheckIn(f.Ctx, f.Owner, f.Est.ID, b.ID, booking.ActionInput{})
		var te *booking.TransitionError
		if !errors.As(err, &te) || te.From != booking.StatusAwaitingApproval {
			t.Fatalf("check-in before approval: %v", err)
		}
		got, err := f.Svc.Approve(f.Ctx, f.Manager, f.Est.ID, b.ID, booking.ActionInput{ExpectedVersion: i64(1)})
		if err != nil {
			t.Fatal(err)
		}
		if got.Booking.Status != string(booking.StatusConfirmed) || got.Booking.HoldExpiresAt != nil || got.Booking.ConfirmedAt == nil || got.Booking.Version != 2 {
			t.Fatalf("approved %+v", got.Booking)
		}
		for _, a := range got.SpotAllocations {
			if a.Kind != store.AllocationBooking || a.ExpiresAt != nil {
				t.Fatalf("stable allocation %+v", a)
			}
		}
		// Reject path on another request releases the spot.
		in2 := f.Input(f.Spots[1], "2027-07-10", "2027-07-11")
		in2.Lines[0].PackID = f.VIP.ID
		r := f.MustCreate(in2)
		rej, err := f.Svc.Reject(f.Ctx, f.Manager, f.Est.ID, r.Booking.ID, booking.ActionInput{})
		if err != nil || rej.Booking.Status != string(booking.StatusRejected) || allocCount(f, r.Booking.ID) != 0 {
			t.Fatalf("reject: %v %+v", err, rej)
		}
	})
}

func TestBookingHeld_ExpiresAndReleases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		in.Status = booking.StatusHeld
		det := f.MustCreate(in)
		if det.Booking.Status != string(booking.StatusHeld) || det.Booking.HoldExpiresAt == nil || allocCount(f, det.Booking.ID) != 2 {
			t.Fatalf("held %+v", det.Booking)
		}
		if n, err := f.Svc.ExpireHolds(f.Ctx); err != nil || n != 0 {
			t.Fatalf("early expiry: %d %v", n, err)
		}
		f.Clock.Advance(booking.DefaultHoldTTL + time.Second)
		if n, err := f.Svc.ExpireHolds(f.Ctx); err != nil || n != 1 {
			t.Fatalf("expiry: %d %v", n, err)
		}
		got, err := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, det.Booking.ID)
		if err != nil || got.Booking.Status != string(booking.StatusExpired) || got.Booking.HoldExpiresAt != nil || len(got.SpotAllocations) != 0 {
			t.Fatalf("expired: %v %+v", err, got)
		}
		if f.Count((*store.OutboxEvent)(nil), "type = ?", event.BookingExpired) != 1 {
			t.Fatal("booking.expired missing")
		}
		// Draft: no resources, confirm allocates (draft -> held -> confirmed).
		dr := f.Input(f.Spots[0], "2027-07-10", "2027-07-10")
		dr.Status = booking.StatusDraft
		dd := f.MustCreate(dr)
		if allocCount(f, dd.Booking.ID) != 0 {
			t.Fatal("draft occupies resources")
		}
		c, err := f.Svc.Confirm(f.Ctx, f.Cashier, f.Est.ID, dd.Booking.ID, booking.ActionInput{})
		if err != nil || c.Booking.Status != string(booking.StatusConfirmed) || len(c.SpotAllocations) != 2 {
			t.Fatalf("confirm draft: %v", err)
		}
	})
}

// TestBookingModify_KeepsOriginalSnapshot covers INV-21 and the atomic
// re-allocation of spec 10.3.
func TestBookingModify_KeepsOriginalSnapshot(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		id, snap := det.Booking.ID, det.Booking.PriceSnapshot
		other := f.MustCreate(f.Input(f.Spots[1], "2027-07-14", "2027-07-14"))

		ends := d("2027-07-14")
		// Dry run: price and allocation computed, nothing written.
		res, err := f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, i64(1), booking.PatchInput{EndsOn: &ends, DryRun: true})
		if err != nil || res.Applied || res.NewTotal != 5*bookingtest.BasePerDay || res.PriceDifference() != 2*bookingtest.BasePerDay {
			t.Fatalf("dry run: %v %+v", err, res)
		}
		if allocCount(f, id) != 6 {
			t.Fatal("dry run wrote allocations")
		}
		// Applied: 5 days, 10 rows, version 2, snapshot unchanged, total updated.
		res, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, i64(1), booking.PatchInput{EndsOn: &ends})
		if err != nil || !res.Applied {
			t.Fatalf("update: %v", err)
		}
		b := res.Detail.Booking
		if b.Version != 2 || b.TotalAmount != 5*bookingtest.BasePerDay || len(res.Detail.SpotAllocations) != 10 || !sameJSON(b.PriceSnapshot, snap) {
			t.Fatalf("modified %+v allocations %d", b, len(res.Detail.SpotAllocations))
		}
		if res.Detail.Lines[0].ID != det.Lines[0].ID || res.Detail.Lines[0].LineTotal != 5*bookingtest.BasePerDay {
			t.Fatalf("line kept and re-priced: %+v", res.Detail.Lines)
		}
		// Stale If-Match: 412 version_conflict, nothing changes.
		_, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, i64(1), booking.PatchInput{EndsOn: &ends})
		var vc *configerr.VersionConflictError
		if !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale version: %v", err)
		}
		// Moving the lines onto a spot taken on 14 July fails and leaves the
		// original allocations intact; the dry run reports the conflict.
		lines := []booking.LineInput{{PackID: f.Base.ID, SpotID: f.Spots[1].ID}}
		res, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, i64(2), booking.PatchInput{Lines: lines, DryRun: true})
		if err != nil || res.Applied || len(res.Conflicts) != 2 {
			t.Fatalf("dry run conflict: %v %+v", err, res)
		}
		_, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, i64(2), booking.PatchInput{Lines: lines})
		if !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("conflict: %v", err)
		}
		after, _ := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, id)
		if after.Booking.Version != 2 || len(after.SpotAllocations) != 10 || *after.Lines[0].SpotID != f.Spots[0].ID || allocCount(f, other.Booking.ID) != 2 {
			t.Fatal("failed modification changed the booking")
		}
		// Replace the line with A3 + a sunbed: new line id, extras priced.
		lines = []booking.LineInput{{PackID: f.Base.ID, SpotID: f.Spots[2].ID, Extras: []booking.ExtraInput{{ExtraID: f.Sunbed.ID, Quantity: 2}}}}
		res, err = f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, id, nil, booking.PatchInput{Lines: lines})
		if err != nil || res.NewTotal != 5*(bookingtest.BasePerDay+2*bookingtest.SunbedPerDay) || *res.Detail.Lines[0].SpotID != f.Spots[2].ID {
			t.Fatalf("replace lines: %v %+v", err, res)
		}
		if f.Count((*store.SpotAllocation)(nil), "spot_id = ?", f.Spots[0].ID) != 0 {
			t.Fatal("old spot not released")
		}
	})
}

func TestBookingCancel_RefundTiersAndCredit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		// 2027-06-01 -> 2027-07-10: more than 14 days, 100 % refund.
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		f.PayBooking(det.Booking.ID, 6000)
		pre, err := f.Svc.Cancel(f.Ctx, f.Manager, f.Est.ID, det.Booking.ID, booking.CancelInput{DryRun: true})
		if err != nil || pre.Applied || pre.Refund.Amount != 6000 || pre.Refund.As != booking.SettleRefund || allocCount(f, det.Booking.ID) != 6 {
			t.Fatalf("preview: %v %+v", err, pre)
		}
		res, err := f.Svc.Cancel(f.Ctx, f.Manager, f.Est.ID, det.Booking.ID, booking.CancelInput{RefundMethod: store.PaymentMethodCash, Reason: "meteo"})
		if err != nil || !res.Applied || res.Detail.Booking.Status != string(booking.StatusCancelled) || res.Recorded == nil {
			t.Fatalf("cancel: %v %+v", err, res)
		}
		// INV-20: no allocations; INV-22 through the ledger: paid back to zero.
		if allocCount(f, det.Booking.ID) != 0 || res.Detail.Booking.PaidAmount != 0 || f.Pay.RefundCount() != 1 {
			t.Fatalf("after cancel: %+v", res.Detail.Booking)
		}
		// A cancelled booking cannot be cancelled again (INV-17).
		if _, err := f.Svc.Cancel(f.Ctx, f.Manager, f.Est.ID, det.Booking.ID, booking.CancelInput{}); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Fatalf("double cancel: %v", err)
		}
		// 5 days before: 50 % as credit, recorded even without a method.
		f.Clock.Set(time.Date(2027, 7, 5, 8, 0, 0, 0, time.UTC))
		c := f.MustCreate(f.Input(f.Spots[1], "2027-07-10", "2027-07-11"))
		f.PayBooking(c.Booking.ID, 4000)
		res, err = f.Svc.Cancel(f.Ctx, f.Manager, f.Est.ID, c.Booking.ID, booking.CancelInput{})
		if err != nil || res.Refund.Amount != 2000 || res.Refund.As != booking.SettleCredit || res.Recorded == nil || f.Pay.Refunds[1].As != "credit" {
			t.Fatalf("credit: %v %+v", err, res)
		}
		// A cashier (no refunds.issue) cancels: the refund stays due.
		f.Clock.Set(time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC))
		k := f.MustCreate(f.Input(f.Spots[2], "2027-07-20", "2027-07-20"))
		f.PayBooking(k.Booking.ID, 2000)
		res, err = f.Svc.Cancel(f.Ctx, f.Cashier, f.Est.ID, k.Booking.ID, booking.CancelInput{RefundMethod: store.PaymentMethodCash})
		if err != nil || res.Recorded != nil || res.Detail.Booking.PaidAmount != 2000 || f.Pay.RefundCount() != 2 {
			t.Fatalf("cashier cancel: %v %+v", err, res)
		}
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND CAST(diff AS TEXT) LIKE ?", k.Booking.ID, "%refund_due%") != 1 {
			t.Fatal("refund due not audited")
		}
	})
}

func TestBookingLifecycle_CheckInNoShowMoveNotes(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		a := f.MustCreate(f.Input(f.Spots[0], "2027-06-01", "2027-06-03"))
		ci, err := f.Svc.CheckIn(f.Ctx, f.Staff, f.Est.ID, a.Booking.ID, booking.ActionInput{ExpectedVersion: i64(1)})
		if err != nil || ci.Booking.Status != string(booking.StatusCheckedIn) || ci.Booking.CheckedInAt == nil {
			t.Fatalf("check-in: %v", err)
		}
		// checked_in -> no_show is not an edge; nothing changes.
		if _, err := f.Svc.MarkNoShow(f.Ctx, f.Staff, f.Est.ID, a.Booking.ID, booking.ActionInput{}); !errors.Is(err, booking.ErrInvalidTransition) {
			t.Fatalf("no-show after check-in: %v", err)
		}
		// No-show releases from today (2027-06-01) on: here everything.
		past := f.Input(f.Spots[1], "2027-05-31", "2027-06-02")
		past.Force = true // started yesterday: lead_time_too_short without force
		ns := f.MustCreate(past)
		got, err := f.Svc.MarkNoShow(f.Ctx, f.Cashier, f.Est.ID, ns.Booking.ID, booking.ActionInput{})
		if err != nil || got.Booking.Status != string(booking.StatusNoShow) || len(got.SpotAllocations) != 2 {
			t.Fatalf("no-show: %v %d", err, len(got.SpotAllocations))
		}
		// Move a line from 2027-07-11 to A4 (mid stay), then the whole line.
		m := f.MustCreate(f.Input(f.Spots[2], "2027-07-10", "2027-07-12"))
		from := d("2027-07-11")
		res, err := f.Svc.Move(f.Ctx, f.Owner, f.Est.ID, m.Booking.ID, booking.MoveInput{LineID: m.Lines[0].ID, SpotID: f.Spots[3].ID, From: &from})
		if err != nil || !res.Applied {
			t.Fatalf("move: %v", err)
		}
		if f.Count((*store.SpotAllocation)(nil), "booking_id = ? AND spot_id = ?", m.Booking.ID, f.Spots[2].ID) != 2 ||
			f.Count((*store.SpotAllocation)(nil), "booking_id = ? AND spot_id = ?", m.Booking.ID, f.Spots[3].ID) != 4 {
			t.Fatal("mid-stay move allocations")
		}
		// Moving onto a busy spot fails (dry run reports it).
		res, err = f.Svc.Move(f.Ctx, f.Owner, f.Est.ID, m.Booking.ID, booking.MoveInput{LineID: m.Lines[0].ID, SpotID: f.Spots[0].ID, DryRun: true})
		if err != nil || res.Applied {
			t.Fatalf("move dry run: %v %+v", err, res)
		}
		if _, err := f.Svc.Move(f.Ctx, f.Owner, f.Est.ID, m.Booking.ID, booking.MoveInput{LineID: "nope", SpotID: f.Spots[0].ID}); !errors.Is(err, booking.ErrLineNotFound) {
			t.Fatalf("unknown line: %v", err)
		}
		// Notes: appended, versioned, text not in the audit diff.
		n, err := f.Svc.AddNote(f.Ctx, f.Cashier, f.Est.ID, m.Booking.ID, booking.NoteInput{Text: "Cliente abituale"})
		if err != nil || n.Booking.NotesInternal != "Cliente abituale" {
			t.Fatalf("note: %v", err)
		}
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND CAST(diff AS TEXT) LIKE ?", m.Booking.ID, "%abituale%") != 0 {
			t.Fatal("note text copied into the audit log")
		}
	})
}

func TestBookingAutoAssign_PendingZoneLines(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.Lines[0].SpotID, in.Lines[0].ZoneID = "", f.FreeZone.ID
		z := f.MustCreate(in)
		if z.Lines[0].AssignmentStatus != store.AssignmentPending || z.Lines[0].PriceCategory != "back" {
			t.Fatalf("zone line %+v", z.Lines[0])
		}
		// B1 is taken on the 11th: the engine picks B2.
		f.MustCreate(f.Input(f.FreeSpots[0], "2027-07-11", "2027-07-11"))
		plan, err := f.Svc.AutoAssign(f.Ctx, f.Owner, f.Est.ID, booking.AutoAssignInput{From: d("2027-07-01"), To: d("2027-07-31"), DryRun: true})
		if err != nil || plan.Applied || len(plan.Assigned) != 1 || plan.Assigned[0].SpotID != f.FreeSpots[1].ID {
			t.Fatalf("plan: %v %+v", err, plan)
		}
		res, err := f.Svc.AutoAssign(f.Ctx, f.Owner, f.Est.ID, booking.AutoAssignInput{From: d("2027-07-01"), To: d("2027-07-31")})
		if err != nil || !res.Applied || len(res.Assigned) != 1 {
			t.Fatalf("apply: %v %+v", err, res)
		}
		got, _ := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, z.Booking.ID)
		if got.Lines[0].AssignmentStatus != store.AssignmentAssigned || *got.Lines[0].SpotID != f.FreeSpots[1].ID || len(got.SpotAllocations) != 4 {
			t.Fatalf("assigned %+v", got.Lines[0])
		}
	})
}

func TestBookingList_FiltersAndCursor(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		var ids []string
		for i, day := range []string{"2027-07-01", "2027-07-02", "2027-07-03", "2027-07-04", "2027-07-05"} {
			ids = append(ids, f.MustCreate(f.Input(f.Spots[i%4], day, day)).Booking.ID)
		}
		if _, err := f.Svc.CheckIn(f.Ctx, f.Owner, f.Est.ID, ids[0], booking.ActionInput{}); err != nil {
			t.Fatal(err)
		}
		var seen []string
		cursor := ""
		for {
			page, err := f.Svc.List(f.Ctx, f.Viewer, f.Est.ID, booking.ListInput{Cursor: cursor, Limit: 2})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range page.Items {
				seen = append(seen, r.ID)
				if r.CustomerLastName != "Rossi" || len(r.SpotLabels) != 1 {
					t.Fatalf("row %+v", r)
				}
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if len(seen) != 5 || seen[0] != ids[0] || seen[4] != ids[4] {
			t.Fatalf("pagination order %v", seen)
		}
		from, to := d("2027-07-02"), d("2027-07-03")
		cases := []struct {
			name string
			f    store.BookingFilter
			want int
		}{
			{"status", store.BookingFilter{Statuses: []string{"checked_in"}}, 1},
			{"dates", store.BookingFilter{From: &from, To: &to}, 2},
			{"spot", store.BookingFilter{SpotID: f.Spots[0].ID}, 2},
			{"code", store.BookingFilter{Q: "ld-2027-000003"}, 1},
			{"customer name", store.BookingFilter{Q: "ross"}, 5},
			{"balance due", store.BookingFilter{BalanceDue: ptr(true)}, 5},
			{"season passes", store.BookingFilter{SeasonPass: ptr(true)}, 0},
		}
		for _, tc := range cases {
			page, err := f.Svc.List(f.Ctx, f.Viewer, f.Est.ID, booking.ListInput{Filter: tc.f})
			if err != nil || len(page.Items) != tc.want {
				t.Errorf("%s: %v %d, want %d", tc.name, err, len(page.Items), tc.want)
			}
		}
		if _, err := f.Svc.List(f.Ctx, f.Viewer, f.Est.ID, booking.ListInput{Cursor: "garbage!"}); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestBookingService_CrossTenantAndPermissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
		id := det.Booking.ID
		// Viewer and beach staff cannot create or modify (spec 16.1).
		for _, p := range []*auth.Principal{f.Viewer, f.Staff} {
			if _, err := f.Svc.Create(f.Ctx, p, f.Est.ID, f.Input(f.Spots[1], "2027-07-10", "2027-07-10")); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("create: %v", err)
			}
			if _, err := f.Svc.Cancel(f.Ctx, p, f.Est.ID, id, booking.CancelInput{}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("cancel: %v", err)
			}
			if _, err := f.Svc.AddNote(f.Ctx, p, f.Est.ID, id, booking.NoteInput{Text: "x"}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("note: %v", err)
			}
		}
		if _, err := f.Svc.CheckIn(f.Ctx, f.Viewer, f.Est.ID, id, booking.ActionInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer check-in: %v", err)
		}
		if _, err := f.Svc.Get(f.Ctx, f.Viewer, f.Est.ID, id); err != nil {
			t.Fatalf("viewer get: %v", err)
		}
		// Another organization never sees the booking (INV-26).
		if _, err := f.Svc.Get(f.Ctx, f.Outsider, f.Est.ID, id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("outsider get: %v", err)
		}
		if _, err := f.Svc.Get(f.Ctx, f.Outsider, f.Other.ID, id); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("outsider get via own establishment: %v", err)
		}
		if _, err := f.Svc.Cancel(f.Ctx, f.Outsider, f.Other.ID, id, booking.CancelInput{}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("outsider cancel: %v", err)
		}
		page, err := f.Svc.List(f.Ctx, f.Outsider, f.Other.ID, booking.ListInput{})
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("outsider list: %v %d", err, len(page.Items))
		}
		// Inline customer of another tenant cannot be referenced.
		in := f.Input(f.Spots[1], "2027-07-10", "2027-07-10")
		in.Customer, in.CustomerID = nil, det.Customer.ID
		if _, err := f.Svc.Create(f.Ctx, f.Outsider, f.Other.ID, in); err == nil {
			t.Fatal("foreign customer accepted")
		}
	})
}

// TestBookingService_AuditAndOutboxAtomic covers INV-18: a change that fails
// after the booking row was written leaves no audit entry, no outbox event
// and no allocation.
func TestBookingService_AuditAndOutboxAtomic(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		f.MustCreate(f.Input(f.Spots[1], "2027-07-13", "2027-07-13"))
		audits, events := f.Count((*store.AuditEntry)(nil), ""), f.Count((*store.OutboxEvent)(nil), "")
		ends := d("2027-07-13")
		lines := []booking.LineInput{{PackID: f.Base.ID, SpotID: f.Spots[1].ID}}
		if _, err := f.Svc.Update(f.Ctx, f.Owner, f.Est.ID, det.Booking.ID, nil, booking.PatchInput{EndsOn: &ends, Lines: lines}); err == nil {
			t.Fatal("conflicting modification accepted")
		}
		if f.Count((*store.AuditEntry)(nil), "") != audits || f.Count((*store.OutboxEvent)(nil), "") != events || allocCount(f, det.Booking.ID) != 6 {
			t.Fatal("partial writes after a failed modification")
		}
		got, _ := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, det.Booking.ID)
		if got.Booking.Version != 1 || !got.Booking.EndsOn.Equal(d("2027-07-12")) || got.Lines[0].ID != det.Lines[0].ID {
			t.Fatal("booking changed by a failed modification")
		}
		_ = calendar.SlotsFull
	})
}

func TestLedger_ApplyPayment(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
		es, _ := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
		b, err := f.Ledger.LoadForPayment(f.Ctx, db, es, det.Booking.ID)
		if err != nil || b.Version != 1 {
			t.Fatal(err)
		}
		b, err = f.Ledger.ApplyPayment(f.Ctx, db, es, det.Booking.ID, 1500)
		if err != nil || b.PaidAmount != 1500 || b.Version != 2 || b.BalanceDue() != bookingtest.BasePerDay-1500 {
			t.Fatalf("apply: %v %+v", err, b)
		}
		other, _ := store.SystemEstablishmentScope(f.Ctx, db, f.Other.ID)
		if _, err := f.Ledger.LoadForPayment(f.Ctx, db, other, det.Booking.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant ledger: %v", err)
		}
	})
}

// TestBookingLifecycle_CheckInOutsidePeriod: check-in only on a day of the
// stay and no-show never before it (establishment timezone, spec 10.2); a
// refusal is typed (422 booking_outside_period) and writes nothing. Operator
// notes of confirm / reject never reach the audit log (spec 23, INV-30).
func TestBookingLifecycle_CheckInOutsidePeriod(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db) // today 2027-06-01 in Europe/Rome
		future := f.MustCreate(f.Input(f.Spots[0], "2027-06-10", "2027-06-12"))
		audits := f.Count((*store.AuditEntry)(nil), "")
		var ope *booking.OutsidePeriodError
		_, err := f.Svc.CheckIn(f.Ctx, f.Staff, f.Est.ID, future.Booking.ID, booking.ActionInput{})
		if !errors.As(err, &ope) || !errors.Is(err, booking.ErrOutsidePeriod) || ope.Action != "check_in" || ope.Today.String() != "2027-06-01" {
			t.Fatalf("early check-in: %v", err)
		}
		_, err = f.Svc.MarkNoShow(f.Ctx, f.Staff, f.Est.ID, future.Booking.ID, booking.ActionInput{})
		if !errors.As(err, &ope) || ope.Action != "no_show" {
			t.Fatalf("early no-show: %v", err)
		}
		got, _ := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, future.Booking.ID)
		if got.Booking.Status != string(booking.StatusConfirmed) || got.Booking.Version != 1 || len(got.SpotAllocations) != 6 || f.Count((*store.AuditEntry)(nil), "") != audits {
			t.Fatal("a refused check-in or no-show changed the booking")
		}
		// The first day of the stay (Rome): check-in accepted; the day after
		// the stay: refused again (the completion job closes it).
		f.Clock.Advance(9 * 24 * time.Hour)
		if _, err := f.Svc.CheckIn(f.Ctx, f.Staff, f.Est.ID, future.Booking.ID, booking.ActionInput{}); err != nil {
			t.Fatalf("check-in on the first day: %v", err)
		}
		late := f.MustCreate(f.Input(f.Spots[1], "2027-06-10", "2027-06-10"))
		f.Clock.Advance(24 * time.Hour)
		if _, err := f.Svc.CheckIn(f.Ctx, f.Staff, f.Est.ID, late.Booking.ID, booking.ActionInput{}); !errors.Is(err, booking.ErrOutsidePeriod) {
			t.Fatalf("late check-in: %v", err)
		}
		// No-show after the start releases from today, with the note kept
		// out of the audit diff.
		if _, err := f.Svc.MarkNoShow(f.Ctx, f.Staff, f.Est.ID, late.Booking.ID, booking.ActionInput{}); err != nil {
			t.Fatalf("no-show after the stay: %v", err)
		}

		// Confirm and reject with a note: the text is not in audit_log.
		in := f.Input(f.Spots[2], "2027-07-01", "2027-07-01")
		in.Status = booking.StatusHeld
		held := f.MustCreate(in)
		if _, err := f.Svc.Confirm(f.Ctx, f.Cashier, f.Est.ID, held.Booking.ID, booking.ActionInput{Note: "telefono segreto 3339998877"}); err != nil {
			t.Fatalf("confirm: %v", err)
		}
		vip := f.Input(f.Spots[3], "2027-07-01", "2027-07-01")
		vip.Lines[0].PackID = f.VIP.ID
		awaiting := f.MustCreate(vip)
		if awaiting.Booking.Status != string(booking.StatusAwaitingApproval) {
			t.Fatalf("vip status %s", awaiting.Booking.Status)
		}
		if _, err := f.Svc.Reject(f.Ctx, f.Manager, f.Est.ID, awaiting.Booking.ID, booking.ActionInput{Note: "motivo riservato 3339998877"}); err != nil {
			t.Fatalf("reject: %v", err)
		}
		if f.Count((*store.AuditEntry)(nil), "CAST(diff AS TEXT) LIKE ?", "%3339998877%") != 0 {
			t.Fatal("operator note copied into the audit log")
		}
		if f.Count((*store.AuditEntry)(nil), "entity_id = ? AND action = ? AND CAST(diff AS TEXT) LIKE ?", held.Booking.ID, event.BookingConfirmed, "%note_length%") != 1 {
			t.Fatal("confirm note not recorded as a flag")
		}
	})
}
