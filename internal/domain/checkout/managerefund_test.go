package checkout

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestManageRefundOf_Aggregate(t *testing.T) {
	t0 := time.Date(2027, 7, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	req := func(status string, amount int64, created time.Time, done *time.Time) store.RefundRequest {
		return store.RefundRequest{Status: status, Amount: money.Cents(amount), CreatedAt: created, CompletedAt: done}
	}
	for _, tc := range []struct {
		name   string
		in     []store.RefundRequest
		status string
		amount int64
		done   bool
	}{
		{"none", nil, "", 0, false},
		{"cancelled only", []store.RefundRequest{req(store.RefundRequestCancelled, 100, t0, &t1)}, "", 0, false},
		{"all succeeded", []store.RefundRequest{req(store.RefundRequestSucceeded, 100, t0, &t1), req(store.RefundRequestSucceeded, 50, t1, &t1)}, store.RefundRequestSucceeded, 150, true},
		{"one processing", []store.RefundRequest{req(store.RefundRequestSucceeded, 100, t0, &t1), req(store.RefundRequestProcessing, 50, t1, nil)}, store.RefundRequestProcessing, 150, false},
		{"failed wins", []store.RefundRequest{req(store.RefundRequestPending, 100, t0, nil), req(store.RefundRequestFailed, 50, t1, nil)}, store.RefundRequestFailed, 150, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ManageRefundOf(tc.in)
			if tc.status == "" {
				if got != nil {
					t.Fatalf("got %+v", got)
				}
				return
			}
			if got == nil || got.Status != tc.status || int64(got.Amount) != tc.amount || (got.CompletedAt != nil) != tc.done || !got.RequestedAt.Equal(t0) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
