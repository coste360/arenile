//go:build integration

package pass_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestPriceOverride_AuditedPermissionGatedSnapshotImmutable covers INV-77:
// only establishment.configure may change the price of a pass, every change
// writes the audit (before/after, reason) and booking.modified in the same
// transaction, price_snapshot never changes and clearing restores the engine
// total.
func TestPriceOverride_AuditedPermissionGatedSnapshotImmutable(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		dt := e.createPass(t, pass.CreateInput{})
		id := dt.Pass.Booking.ID
		snapshot := string(dt.Pass.Booking.PriceSnapshot)
		v1 := dt.Pass.Booking.Version

		// The desk cannot change the price (cashier: bookings.write only).
		if _, err := e.svc.SetPriceOverride(e.ctx, e.cashier, "", e.estA.ID, id, &v1, 90000, "accordo"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier override: %v, want forbidden", err)
		}
		// Stale version: 412.
		stale := v1 - 1
		var vc *configerr.VersionConflictError
		if _, err := e.svc.SetPriceOverride(e.ctx, e.owner, "", e.estA.ID, id, &stale, 90000, "accordo"); !errors.As(err, &vc) {
			t.Fatalf("stale override: %v, want version conflict", err)
		}
		// A reason is mandatory.
		if _, err := e.svc.SetPriceOverride(e.ctx, e.owner, "", e.estA.ID, id, &v1, 90000, " "); err == nil {
			t.Fatal("override without reason accepted")
		}

		out, err := e.svc.SetPriceOverride(e.ctx, e.owner, "10.0.0.1", e.estA.ID, id, &v1, 90000, "cliente storico")
		if err != nil {
			t.Fatalf("override: %v", err)
		}
		b := out.Pass.Booking
		if b.TotalAmount != 90000 || b.PriceOverrideAmount == nil || *b.PriceOverrideAmount != 90000 ||
			b.PriceOverrideEngineAmount == nil || *b.PriceOverrideEngineAmount != passPrice ||
			b.PriceOverrideReason == nil || *b.PriceOverrideReason != "cliente storico" || b.Version <= v1 {
			t.Fatalf("overridden pass %+v", b)
		}
		if string(b.PriceSnapshot) != snapshot {
			t.Fatalf("price_snapshot changed: %s -> %s (INV-21)", snapshot, b.PriceSnapshot)
		}
		if n := e.count(t, "audit_log", "action = ? AND entity_id = ?", "season_pass.price_overridden", id); n != 1 {
			t.Fatalf("override audit rows %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.BookingModified); n != 1 {
			t.Fatalf("booking.modified events %d", n)
		}

		// Back to the price list: engine total, override columns cleared.
		v2 := b.Version
		back, err := e.svc.ClearPriceOverride(e.ctx, e.owner, "", e.estA.ID, id, &v2)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if back.Pass.Booking.TotalAmount != passPrice || back.Pass.Booking.PriceOverrideAmount != nil ||
			string(back.Pass.Booking.PriceSnapshot) != snapshot {
			t.Fatalf("cleared pass %+v", back.Pass.Booking)
		}
		if n := e.count(t, "audit_log", "action = ? AND entity_id = ?", "season_pass.price_override_cleared", id); n != 1 {
			t.Fatalf("clear audit rows %d", n)
		}
		v3 := back.Pass.Booking.Version
		if _, err := e.svc.ClearPriceOverride(e.ctx, e.owner, "", e.estA.ID, id, &v3); !errors.Is(err, pass.ErrNoOverride) {
			t.Fatalf("second clear: %v, want ErrNoOverride", err)
		}
	})
}
