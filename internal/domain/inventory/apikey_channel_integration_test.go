//go:build integration

package inventory_test

import (
	"context"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// INV-62: a hold (or booking) made with an API key is always channel "api",
// even when the request asks for "desk", so it consumes and respects the
// online quota (ADR 0011 §1). Without auth.ChannelFor the key could bypass
// the quota by asking for the desk channel.
func TestInventory_APIKeyCannotBypassQuota(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		key := &auth.Principal{
			Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: f.est.ID,
			APIKeyOrganizationID: f.est.OrganizationID, Scopes: []auth.Permission{auth.PermBookingsWrite},
		}
		// The quantity zone allows 2 of 4 online: fill the quota online.
		_, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-08", to: "2027-07-08", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 2}}}, nil)
		must(t, err)
		in := inventory.HoldInput{
			Channel: store.ChannelDesk, StartsOn: d("2027-07-08"), EndsOn: d("2027-07-08"), Slots: calendar.SlotsAM,
			Items: []store.HoldItem{{ZoneID: f.qtyZone.ID, Quantity: 1}},
		}
		_, err = f.svc.CreateHold(ctx, key, f.est.ID, in)
		conflictOf(t, err, availability.ErrQuotaExceeded)
		// Staff on the desk may still sell the remaining capacity.
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, in)
		must(t, err)
		if h.Channel != store.ChannelDesk {
			t.Fatalf("staff hold channel = %s", h.Channel)
		}
		// On a free day the key's hold is recorded as "api".
		in.StartsOn, in.EndsOn = d("2027-07-10"), d("2027-07-10")
		h, err = f.svc.CreateHold(ctx, key, f.est.ID, in)
		must(t, err)
		if h.Channel != store.ChannelAPI {
			t.Fatalf("api key hold channel = %s, want api", h.Channel)
		}
	})
}
