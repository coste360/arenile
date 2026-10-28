//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestLockBookings_WaitEndsAsConflict: on PostgreSQL a LockBookings queued
// behind a transaction that keeps the booking locked gives up with
// ErrConflict before the driver read timeout (no network error, no 500),
// and the statement limit does not outlive the lock.
func TestLockBookings_WaitEndsAsConflict(t *testing.T) {
	db := storetest.Postgres(t)
	ctx := context.Background()
	_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-lock", "Lido Lock", t0)
	if err != nil {
		t.Fatal(err)
	}
	cust := &store.Customer{
		ID: store.NewID(), EstablishmentID: est.ID, LastName: "Rossi", Locale: "it", Tags: []string{},
		SearchText: "rossi", Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	bk := &store.Booking{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "AR-LOCK01", CustomerID: cust.ID, Channel: store.ChannelDesk,
		Status: "confirmed", StartsOn: calendar.MustParse("2027-08-10"), EndsOn: calendar.MustParse("2027-08-16"),
		Slots: calendar.SlotsFull, Formula: "week", PriceSnapshot: store.JSON(`{"total":0}`),
		PolicySnapshot: store.JSON(`{"rules":{}}`), Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	for _, m := range []any{cust, bk} {
		if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
	if err != nil {
		t.Fatal(err)
	}

	// No contention: the lock is taken and the session limit is back.
	if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.LockBookings(ctx, tx, es, []string{bk.ID}); err != nil {
			return err
		}
		var timeout string
		if err := tx.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&timeout); err != nil {
			return err
		}
		if timeout != "0" {
			t.Errorf("statement_timeout after the lock = %q, want 0", timeout)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Contention: the holder keeps the row past the limit.
	holder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := store.LockBookings(ctx, holder, es, []string{bk.ID}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		return store.LockBookings(ctx, tx, es, []string{bk.ID})
	})
	waited := time.Since(start)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("queued lock: want ErrConflict, got %v", err)
	}
	if waited >= 9*time.Second {
		t.Fatalf("queued lock waited %s, want less than the driver read timeout", waited)
	}
}
