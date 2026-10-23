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
	if err := tok.Consume("wrong"); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatal("wrong token accepted")
	}
	if err := tok.Consume(v); err != nil {
		t.Fatal(err)
	}
	if err := tok.Consume(v); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatal("token reused")
	}
	var nilTok *SetupToken
	if err := nilTok.Consume(v); !errors.Is(err, control.ErrSetupTokenInvalid) {
		t.Fatal("nil token must reject")
	}
}
