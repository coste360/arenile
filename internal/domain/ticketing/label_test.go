package ticketing

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
)

func TestNumberLabel_Padding(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{{1, "#001"}, {37, "#037"}, {999, "#999"}, {1204, "#1204"}}
	for _, tt := range tests {
		if got := NumberLabel(tt.n); got != tt.want {
			t.Errorf("NumberLabel(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestStateOf_Precedence(t *testing.T) {
	now := time.Date(2027, 7, 3, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		t    store.Ticket
		want State
	}{
		{"sale assigned", store.Ticket{Kind: store.TicketKindSale, Status: store.TicketValid}, StateAssigned},
		{"sale unassigned", store.Ticket{Kind: store.TicketKindSale, Status: store.TicketValid, NeedsAssignment: true}, StateUnassigned},
		{"entry issued", store.Ticket{Kind: store.TicketKindEntry, Status: store.TicketValid}, StateIssued},
		{"entry entered", store.Ticket{Kind: store.TicketKindEntry, Status: store.TicketValid, EnteredAt: &now}, StateEntered},
		{"voided wins", store.Ticket{Kind: store.TicketKindSale, Status: store.TicketVoided, NeedsAssignment: true, VoidedAt: &now}, StateVoided},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StateOf(&tt.t); got != tt.want {
				t.Fatalf("StateOf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapLabel_NameOrNumber(t *testing.T) {
	if got := MapLabel(&store.Ticket{TicketNumber: 37}); got != "#037" {
		t.Fatalf("no name: %q", got)
	}
	if got := MapLabel(&store.Ticket{TicketNumber: 37, HolderName: "Rossi"}); got != "Rossi" {
		t.Fatalf("with name: %q", got)
	}
}
