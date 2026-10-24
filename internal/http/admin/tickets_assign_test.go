package admin

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/inventory"
)

func TestOpTicketToAPI_NilAndSet(t *testing.T) {
	if opTicketToAPI(nil) != nil {
		t.Fatal("nil ticket mapped")
	}
	got := opTicketToAPI(&inventory.OperationalTicket{Number: 3, Label: "#003", NeedsAssignment: true})
	if got.Number != 3 || got.Label != "#003" || !got.NeedsAssignment {
		t.Fatalf("%+v", got)
	}
}
