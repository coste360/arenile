package app

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/establishment"
)

// TestTZData_ValidatesEuropeRome loads a zone through the validation used
// when an establishment is created. CI also runs it on Windows from a binary
// built with -trimpath and without GOROOT, like the shipped one: there the
// embedded database is the only source of time zones.
func TestTZData_ValidatesEuropeRome(t *testing.T) {
	if err := establishment.ValidateTimezone("Europe/Rome"); err != nil {
		t.Fatal(err)
	}
}
