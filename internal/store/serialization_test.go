package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/uptrace/bun"
)

type fakePgErr struct{ code string }

func (e fakePgErr) Error() string       { return "pg: " + e.code }
func (e fakePgErr) Field(b byte) string { return map[byte]string{'C': e.code}[b] }

// TestIsSerializationFailure_Codes: deadlocks, serialization failures and
// lock timeouts of PostgreSQL are conflicts; anything else is not.
func TestIsSerializationFailure_Codes(t *testing.T) {
	for code, want := range map[string]bool{"40P01": true, "40001": true, "55P03": true, "23505": false, "42P01": false} {
		err := fmt.Errorf("wrapped: %w", fakePgErr{code})
		if got := IsSerializationFailure(err); got != want {
			t.Fatalf("code %s: %v, want %v", code, got, want)
		}
	}
	if IsSerializationFailure(nil) || IsSerializationFailure(errors.New("UNIQUE constraint failed")) {
		t.Fatal("plain errors must not be serialization failures")
	}
}

// TestRunInTx_SerializationFailureIsBusy: a transaction aborted by a
// concurrent one is ErrConflict and ErrBusy (409 concurrent_update, safe to
// retry); any other error passes through untouched.
func TestRunInTx_SerializationFailureIsBusy(t *testing.T) {
	db, err := Open(context.Background(), Options{SQLiteMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for code, busy := range map[string]bool{"40P01": true, "40001": true, "55P03": true, "23505": false} {
		err := RunInTx(context.Background(), db, func(context.Context, bun.Tx) error { return fakePgErr{code} })
		if errors.Is(err, ErrBusy) != busy || errors.Is(err, ErrConflict) != busy {
			t.Fatalf("code %s: %v (busy %v, conflict %v), want %v", code, err, errors.Is(err, ErrBusy), errors.Is(err, ErrConflict), busy)
		}
		if !errors.As(err, new(fakePgErr)) {
			t.Fatalf("code %s: pg error lost: %v", code, err)
		}
	}
}
