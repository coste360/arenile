//go:build integration

package admin_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestBookingsHTTP_PartnerShareType: a partner share with a missing or
// unknown type is a validation error on partner_share.type, never a booking
// quietly saved without a share.
func TestBookingsHTTP_PartnerShareType(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		h := &admin.Handlers{Bookings: f.Svc, Log: slog.New(slog.DiscardHandler)}
		est := f.Est.ID
		for _, share := range []map[string]any{{"type": "bogus"}, {"type": ""}, {"percent": "10"}} {
			create := map[string]any{
				"customer": map[string]any{"first_name": "Mario", "last_name": "Rossi"},
				"starts_on": "2027-07-10", "ends_on": "2027-07-11", "slots": "FULL", "formula": "multi_day",
				"lines":         []map[string]any{{"pack_id": f.Base.ID, "spot_id": f.Spots[0].ID}},
				"partner_share": share,
			}
			status, _, body := bkCall(t, f.Cashier, create, "", func(w http.ResponseWriter, r *http.Request) { h.CreateBooking(w, r, est, api.CreateBookingParams{}) })
			bkExpect(t, "create with a bad share", status, body, 400, "validation_failed")
			errs, _ := body["errors"].([]any)
			if len(errs) == 0 || errs[0].(map[string]any)["field"] != "partner_share.type" {
				t.Fatalf("share %v: errors %v", share, body["errors"])
			}
		}
		if n := f.Count((*store.Booking)(nil), ""); n != 0 {
			t.Fatalf("%d bookings written", n)
		}
	})
}
