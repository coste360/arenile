package inventory

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
)

func TestHoldStatus_Derivation(t *testing.T) {
	exp := t0.Add(10 * time.Minute)
	before := exp.Add(-time.Minute)
	after := exp.Add(time.Second)
	booking := "b-1"
	tests := []struct {
		name string
		h    store.Hold
		now  time.Time
		want string
	}{
		{name: "active", h: store.Hold{ExpiresAt: exp}, now: before, want: HoldActive},
		{name: "lapsed but not swept", h: store.Hold{ExpiresAt: exp}, now: after, want: HoldExpired},
		{name: "released in time", h: store.Hold{ExpiresAt: exp, ReleasedAt: &before}, now: after, want: HoldReleased},
		{name: "closed by the sweeper", h: store.Hold{ExpiresAt: exp, ReleasedAt: &after}, now: after, want: HoldExpired},
		{name: "closed exactly at expiry", h: store.Hold{ExpiresAt: exp, ReleasedAt: &exp}, now: after, want: HoldExpired},
		{name: "converted wins", h: store.Hold{ExpiresAt: exp, ReleasedAt: &after, ConvertedBookingID: &booking}, now: after, want: HoldConverted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := holdStatus(&tc.h, tc.now); got != tc.want {
				t.Fatalf("holdStatus = %q, want %q", got, tc.want)
			}
		})
	}
}
