package httpserver_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestOfflineQueueContract_IdempotencyKey guards the operations the panel
// and the mobile app queue offline (spec 21.2, 29.2, INV-99): each declares
// the Idempotency-Key header (so the clients generated from
// api/openapi.yaml send it; the build fails if a declaration is removed),
// and each route goes through the Idempotency middleware: a repeat with the
// same key replays the first answer (here the 4xx of unknown ids, stored
// like any non-5xx answer) instead of running the handler again.
func TestOfflineQueueContract_IdempotencyKey(t *testing.T) {
	declared := []any{
		api.CheckInBookingParams{}.IdempotencyKey,
		api.CheckInEntryTicketParams{}.IdempotencyKey,
		api.AddBookingNoteParams{}.IdempotencyKey,
		api.RecordBookingPaymentParams{}.IdempotencyKey,
		api.CreateSpotBlockParams{}.IdempotencyKey,
		api.CreateSeasonPassReleasesParams{}.IdempotencyKey,
	}
	for _, d := range declared {
		if reflect.TypeOf(d) != reflect.TypeFor[*string]() {
			t.Fatalf("Idempotency-Key declared as %T", d)
		}
	}

	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
		t.Fatal(body)
	}
	base := "/api/admin/v1/establishments/" + e.estA.ID
	id := store.NewID()
	// The routes this server answers without the booking, inventory and
	// payment services (check-in, cash collection and spot blocks replay
	// successfully in TestAcceptance_OfflineQueueReplay on the full app).
	for i, path := range []string{
		"/entry-gate/check-in",
		"/bookings/" + id + "/notes",
		"/season-passes/" + id + "/releases",
	} {
		hdr := map[string]string{"X-CSRF-Token": c.csrf(), "If-Match": `"1"`, "Idempotency-Key": fmt.Sprintf("contract-key-%04d", i)}
		first, firstBody := c.do(http.MethodPost, base+path, map[string]any{}, hdr)
		again, againBody := c.do(http.MethodPost, base+path, map[string]any{}, hdr)
		if first.StatusCode >= 500 || first.Header.Get("Idempotent-Replayed") != "" {
			t.Fatalf("%s: first answer %d %v", path, first.StatusCode, firstBody)
		}
		if again.StatusCode != first.StatusCode || again.Header.Get("Idempotent-Replayed") != "true" || !reflect.DeepEqual(againBody, firstBody) {
			t.Fatalf("%s: repeat %d (replayed %q) %v, want the stored %d %v", path, again.StatusCode,
				again.Header.Get("Idempotent-Replayed"), againBody, first.StatusCode, firstBody)
		}
	}
}
