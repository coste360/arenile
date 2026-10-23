package desktop

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/desktop/control"
)

func TestSetupToken_SingleUse(t *testing.T) {
	tok, err := NewSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	v := tok.Value()
	if err := tok.Consume("wrong", nil); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatal("wrong token accepted")
	}
	// A failed completion (owner creation) keeps the link usable.
	boom := errors.New("database is locked")
	if err := tok.Consume(v, func() error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("completion error not returned: %v", err)
	}
	if tok.Value() != v {
		t.Fatal("token burnt by a failed completion")
	}
	ran := 0
	if err := tok.Consume(v, func() error { ran++; return nil }); err != nil || ran != 1 {
		t.Fatal(err, ran)
	}
	if err := tok.Consume(v, func() error { ran++; return nil }); !errors.Is(err, control.ErrSetupTokenInvalid) || ran != 1 {
		t.Fatal("token reused")
	}
	var nilTok *SetupToken
	if err := nilTok.Consume(v, nil); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatal("nil token must reject")
	}
}
