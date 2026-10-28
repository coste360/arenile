package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/uptrace/bun/driver/pgdriver"
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

// TestWithLockTimeout_BelowReadTimeout: a lock wait ends as lock_not_available
// (a conflict) before the driver read timeout turns it into a network error;
// a lock_timeout in the URL wins and other URL parameters are kept.
func TestWithLockTimeout_BelowReadTimeout(t *testing.T) {
	cases := map[string]struct {
		dsn  string
		want string
	}{
		"default read timeout": {dsn: "postgres://u:p@localhost/db?search_path=x", want: "5000ms"},
		"read timeout in url":  {dsn: "postgres://u:p@localhost/db?search_path=x&read_timeout=30s", want: "15000ms"},
		"lock timeout in url":  {dsn: "postgres://u:p@localhost/db?search_path=x&lock_timeout=2s", want: "2s"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			conf := pgdriver.NewConnector(pgdriver.WithDSN(tc.dsn), withLockTimeout).Config()
			if got := conf.ConnParams["lock_timeout"]; got != tc.want {
				t.Fatalf("lock_timeout = %v, want %s", got, tc.want)
			}
			if conf.ConnParams["search_path"] != "x" {
				t.Fatalf("other params lost: %v", conf.ConnParams)
			}
		})
	}
}
