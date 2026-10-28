package store

import (
	"errors"
	"fmt"
	"testing"
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
