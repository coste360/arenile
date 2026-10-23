package dailysale

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
)

func TestFormulaFor_Slots(t *testing.T) {
	tests := []struct {
		s    calendar.Slots
		want string
	}{{calendar.SlotsFull, FormulaDay}, {calendar.SlotsAM, FormulaHalfDay}, {calendar.SlotsPM, FormulaHalfDay}}
	for _, tt := range tests {
		if got := FormulaFor(tt.s); got != tt.want {
			t.Errorf("FormulaFor(%v) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestTarget_Validate(t *testing.T) {
	tests := []struct {
		name      string
		t         Target
		wantErr   bool
		zoneFirst bool
	}{
		{"spot first", Target{SpotID: "s"}, false, false},
		{"zone first", Target{ZoneID: "z"}, false, true},
		{"none", Target{}, true, false},
		{"both", Target{SpotID: "s", ZoneID: "z"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.t.Validate()
			if (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, ErrTarget)) {
				t.Fatalf("Validate = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.t.ZoneFirst() != tt.zoneFirst {
				t.Fatalf("ZoneFirst = %v", tt.t.ZoneFirst())
			}
		})
	}
}

func TestHolderName_Normalise(t *testing.T) {
	tests := []struct{ first, last, want string }{
		{"", "", ""},
		{"Mario", "", "Mario"},
		{"Mario", " Rossi ", "Rossi"},
	}
	for _, tt := range tests {
		if got := HolderName(tt.first, tt.last); got != tt.want {
			t.Errorf("HolderName(%q, %q) = %q, want %q", tt.first, tt.last, got, tt.want)
		}
	}
}
