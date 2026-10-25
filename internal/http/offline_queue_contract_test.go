package httpserver_test

import (
	"testing"

	"github.com/coste360/arenile/internal/http/api"
)

// TestOfflineQueueContract_IdempotencyKey guards the contract of the
// operations the panel and the mobile app queue offline (spec 21.2, 29.2):
// each declares the Idempotency-Key header, so the clients generated from
// api/openapi.yaml can send it. It fails to compile if a declaration is
// removed. The replay itself is the Idempotency middleware on every POST
// under /api/admin/ (TestAcceptance_OfflineQueueReplay,
// TestConcurrency_OfflineQueueReplay).
func TestOfflineQueueContract_IdempotencyKey(t *testing.T) {
	declared := map[string]*string{
		"checkInBooking":           api.CheckInBookingParams{}.IdempotencyKey,
		"checkInEntryTicket":       api.CheckInEntryTicketParams{}.IdempotencyKey,
		"addBookingNote":           api.AddBookingNoteParams{}.IdempotencyKey,
		"recordBookingPayment":     api.RecordBookingPaymentParams{}.IdempotencyKey,
		"createSpotBlock":          api.CreateSpotBlockParams{}.IdempotencyKey,
		"createSeasonPassReleases": api.CreateSeasonPassReleasesParams{}.IdempotencyKey,
	}
	if len(declared) != 6 {
		t.Fatalf("offline queue operations: %d", len(declared))
	}
}
