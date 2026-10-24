package event

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// testModeEvent reports whether ev must stay out of the outgoing webhooks
// because of test mode (spec 15.4, ADR 0019, INV-74): events about a test
// booking or a test payment, and the test-mode events themselves. It runs
// when the event is written (Outbox.write, in the transaction that created
// the row it is about) and its answer is stored on the outbox row, so the
// dispatcher never looks the row up again: a purge that deletes the test
// booking before the dispatch cannot turn its events into real ones. The
// real-time bus and the in-process sinks still receive them (the panel
// shows test rows).
func testModeEvent(ctx context.Context, db bun.IDB, ev ext.Event) (bool, error) {
	if strings.HasPrefix(ev.Type, "establishment.test_") || ev.Type == "booking.test_bookings_purged" {
		return true, nil
	}
	var subject struct {
		BookingID string `json:"booking_id"`
		PaymentID string `json:"payment_id"`
	}
	if err := json.Unmarshal(ev.Data, &subject); err != nil {
		return false, nil //nolint:nilerr // payload without ids: not a test row
	}
	ok, err := store.IsTestEventSubject(ctx, db, ev.EstablishmentID, subject.BookingID, subject.PaymentID)
	if err != nil {
		return false, fmt.Errorf("event: %w", err)
	}
	return ok, nil
}
