package app

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestTZData_Embedded guards the time/tzdata import: hosts with a system
// zoneinfo (macOS, most Linux) cannot reveal its absence at run time, while on
// Windows every time zone would be rejected as unknown.
func TestTZData_Embedded(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "tzdata.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		if imp.Path.Value == `"time/tzdata"` {
			return
		}
	}
	t.Fatal(`internal/app/tzdata.go must import _ "time/tzdata"`)
}
