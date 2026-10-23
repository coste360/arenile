package fiscal

import (
	"errors"
	"fmt"
	"testing"
)

type deferredErr struct{}

func (deferredErr) Error() string  { return "deferred" }
func (deferredErr) Deferred() bool { return true }

func TestIsDeferred_Scenarios(t *testing.T) {
	if !isDeferred(fmt.Errorf("wrap: %w", deferredErr{})) {
		t.Fatal("wrapped deferred error not recognised")
	}
	if isDeferred(errors.New("device offline")) || isDeferred(nil) {
		t.Fatal("plain errors are not deferred")
	}
}
