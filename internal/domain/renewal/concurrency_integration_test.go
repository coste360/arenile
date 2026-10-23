//go:build integration

package renewal_test

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const parallel = 100

// TestConcurrency_RenewalAcceptVsPublicSale: the right of pre-emption is
// enforced by the database. While a proposal is open, 99 parallel sales of
// its spot on a day of the new season race with the holder's acceptance:
// every sale fails with a typed conflict and the acceptance converts the
// proposal (the held allocation becomes the season allocation, never a
// second row). Then a decline races with 99 sales of another proposed
// spot: the decline always succeeds and at most one sale gets the day
// (unique allocation key), never two.
func TestConcurrency_RenewalAcceptVsPublicSale(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 2)
		ctx, f := x.ctx, x.F
		res := x.campaign()
		if len(res.Proposals) != 2 {
			t.Fatalf("proposals: %+v", res)
		}
		const day = "2028-07-15"

		race := func(spot *store.Spot, decisive func() error) (sold, conflicts int, other []error, decisiveErr error) {
			var wg sync.WaitGroup
			var mu sync.Mutex
			start := make(chan struct{})
			for i := range parallel {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if i == parallel/2 {
						err := decisive()
						mu.Lock()
						decisiveErr = err
						mu.Unlock()
						return
					}
					err := x.sale(spot, day)
					mu.Lock()
					defer mu.Unlock()
					var ce *availability.ConflictError
					switch {
					case err == nil:
						sold++
					case errors.As(err, &ce):
						conflicts++
					default:
						other = append(other, err)
					}
				}()
			}
			close(start)
			wg.Wait()
			return sold, conflicts, other, decisiveErr
		}

		// Phase 1: acceptance against 99 sales.
		p0 := res.Proposals[0]
		sold, conflicts, other, err := race(f.Spots[0], func() error {
			_, err := x.rn.Accept(ctx, f.Owner, f.Est.ID, p0.ID, nil, renewal.AcceptInput{Confirm: true})
			return err
		})
		if err != nil || len(other) > 0 {
			t.Fatalf("accept: %v; unexpected sale errors %d: %v", err, len(other), other)
		}
		if sold != 0 || conflicts != parallel-1 {
			t.Fatalf("pre-emption broken: sold %d, conflicts %d", sold, conflicts)
		}
		if p := x.proposal(p0.ID); p.Status != store.RenewalConverted {
			t.Fatalf("proposal %s", p.Status)
		}
		rows, err := db.NewSelect().Model((*store.SpotAllocation)(nil)).Where("spot_id = ?", f.Spots[0].ID).Where("date = ?", day).Count(ctx)
		if err != nil || rows != 2 {
			t.Fatalf("allocations of the day: %d %v", rows, err)
		}

		// Phase 2: decline against 99 sales.
		p1 := res.Proposals[1]
		sold, conflicts, other, err = race(f.Spots[1], func() error {
			_, err := x.rn.Decline(ctx, f.Owner, f.Est.ID, p1.ID, nil, "", "")
			return err
		})
		if err != nil || len(other) > 0 {
			t.Fatalf("decline: %v; unexpected sale errors %d: %v", err, len(other), other)
		}
		if sold > 1 || sold+conflicts != parallel-1 {
			t.Fatalf("double sale after decline: sold %d, conflicts %d", sold, conflicts)
		}
		rows, err = db.NewSelect().Model((*store.SpotAllocation)(nil)).Where("spot_id = ?", f.Spots[1].ID).Where("date = ?", day).Count(ctx)
		if err != nil || rows != int64(2*sold) {
			t.Fatalf("allocations of the day after decline: %d (sold %d) %v", rows, sold, err)
		}
		if p := x.proposal(p1.ID); p.Status != store.RenewalDeclined {
			t.Fatalf("proposal %s", p.Status)
		}
	})
}

// TestConcurrency_RenewalProposalAccept (INV-68, INV-69): the desk (accept
// and confirm) and the customer (magic link) answer the same proposal 100
// times in parallel: the proposal converts exactly once, the booking is
// confirmed with its season allocations (never a second row), one
// season.renewal_accepted event and one conversion are recorded; every
// other attempt is refused as no longer open.
func TestConcurrency_RenewalProposalAccept(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db, 1)
		ctx, f := x.ctx, x.F
		p0 := x.campaign().Proposals[0]
		tok := x.link(p0.BookingID)
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				var err error
				for attempt := 0; attempt < 50; attempt++ {
					if i%2 == 0 {
						_, err = x.rn.Accept(ctx, f.Owner, f.Est.ID, p0.ID, nil, renewal.AcceptInput{Confirm: true})
					} else {
						_, err = x.rn.AcceptByToken(ctx, checkouttestSlug, tok, renewal.PublicAcceptInput{Payment: renewal.PaymentFull})
					}
					if err == nil || !retryable(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
				var vc *configerr.VersionConflictError
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, renewal.ErrNotOpen), errors.As(err, &vc):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if ok.Load() < 1 || ok.Load()+refused.Load() != parallel {
			t.Fatalf("ok %d refused %d", ok.Load(), refused.Load())
		}
		if p := x.proposal(p0.ID); p.Status != store.RenewalConverted {
			t.Fatalf("proposal %s", p.Status)
		}
		if b := x.Booking(p0.BookingID); b.Status != "confirmed" {
			t.Fatalf("booking %s", b.Status)
		}
		rows, err := db.NewSelect().Model((*store.SpotAllocation)(nil)).Where("spot_id = ?", f.Spots[0].ID).Where("date = ?", "2028-07-15").Count(ctx)
		if err != nil || rows != 2 {
			t.Fatalf("allocations of the day: %d %v", rows, err)
		}
		// PostgreSQL prints jsonb with a space after the colon: strip spaces.
		if n := x.Count((*store.AuditEntry)(nil), "action = 'renewal_proposal.converted' OR (action = 'renewal_proposal.accepted' AND REPLACE(CAST(diff AS TEXT), ' ', '') LIKE '%\"confirmed\":true%')"); n != 1 {
			t.Fatalf("conversions recorded %d", n)
		}
		if n := x.Count((*store.OutboxEvent)(nil), "type = ?", event.SeasonRenewalAccepted); n != 1 {
			t.Fatalf("accepted events %d", n)
		}
	})
}

// retryable reports a transient lock error of the file-backed SQLite
// database (single writer) under heavy contention.
func retryable(err error) bool {
	for _, s := range []string{"database is locked", "SQLITE_BUSY", "deadlock detected", "could not serialize"} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
