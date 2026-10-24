package store

import (
	"testing"

	"github.com/coste360/arenile/internal/money"
)

func TestBalanceDue_Status(t *testing.T) {
	tests := []struct {
		status      string
		total, paid money.Cents
		want        money.Cents
	}{
		{"confirmed", 2700, 1000, 1700},
		{"confirmed", 2700, 3000, 0},
		{"checked_in", 2700, 0, 2700},
		{"no_show", 2700, 500, 2200},
		{"cancelled", 2700, 0, 0},
		{"cancelled", 2700, 700, 0},
		{"expired", 2700, 0, 0},
		{"rejected", 2700, 0, 0},
	}
	for _, tt := range tests {
		if got := BalanceDue(tt.status, tt.total, tt.paid); got != tt.want {
			t.Errorf("BalanceDue(%s, %d, %d) = %d, want %d", tt.status, tt.total, tt.paid, got, tt.want)
		}
	}
	b := Booking{Status: "cancelled", TotalAmount: 2700}
	if b.BalanceDue() != 0 {
		t.Errorf("cancelled booking owes %d", b.BalanceDue())
	}
}
