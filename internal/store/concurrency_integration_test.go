//go:build integration

package store_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestConcurrency_TOTPStepAdvance: 100 parallel logins presenting the same
// TOTP code (same time step) are accepted exactly once (anti-replay).
func TestConcurrency_TOTPStepAdvance(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		u := &store.User{ID: store.NewID(), Email: "o@lido.it", PasswordHash: "x", Name: "O", Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertUser(ctx, db, u); err != nil {
			t.Fatal(err)
		}
		const workers = 100
		var accepted atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := store.AdvanceUserTOTPStep(ctx, db, u.ID, 57_000_000, t0)
				if err != nil {
					t.Error(err)
					return
				}
				if ok {
					accepted.Add(1)
				}
			}()
		}
		wg.Wait()
		if got := accepted.Load(); got != 1 {
			t.Fatalf("same TOTP step accepted %d times, want 1", got)
		}
	})
}

// TestConcurrency_IdempotencyKeyClaim: 100 parallel claims of the same key
// yield exactly one winner.
func TestConcurrency_IdempotencyKeyClaim(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		var won atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				k := &store.IdempotencyKey{Scope: "anon:1.2.3.4", Key: "key-00000001", RequestHash: "h", CreatedAt: t0, ExpiresAt: t0.AddDate(0, 0, 1)}
				claimed, existing, err := store.ClaimIdempotencyKey(ctx, db, k)
				if err != nil {
					t.Error(err)
					return
				}
				if claimed {
					won.Add(1)
				} else if existing == nil || existing.RequestHash != "h" {
					t.Errorf("loser must see the winner's row: %+v", existing)
				}
			}()
		}
		wg.Wait()
		if got := won.Load(); got != 1 {
			t.Fatalf("claims won = %d, want 1", got)
		}
	})
}
