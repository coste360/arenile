package admin

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/inventory"
)

func TestOpTicketToAPI_NilAndSet(t *testing.T) {
	if opTicketToAPI(nil) != nil {
		t.Fatal("nil ticket mapped")
	}
	got := opTicketToAPI(&inventory.OperationalTicket{ID: "t3", Number: 3, Label: "#003", NeedsAssignment: true, Version: 2})
	if got.Id != "t3" || got.Number != 3 || got.Label != "#003" || !got.NeedsAssignment || got.Version != 2 {
		t.Fatalf("%+v", got)
	}
}
