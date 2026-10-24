package dailysale

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

func TestValidateSale_Fields(t *testing.T) {
	ok := SaleInput{Target: Target{SpotID: "s"}, Slots: calendar.SlotsFull, PackID: "p"}
	tests := []struct {
		name  string
		edit  func(*SaleInput)
		field string
	}{
		{"valid", func(*SaleInput) {}, ""},
		{"no target", func(in *SaleInput) { in.Target = Target{} }, "spot_id"},
		{"bad slots", func(in *SaleInput) { in.Slots = "XX" }, "slots"},
		{"no pack", func(in *SaleInput) { in.PackID = "" }, "pack_id"},
		{"bad extra", func(in *SaleInput) { in.Extras = []booking.ExtraInput{{ExtraID: "x"}} }, "extras[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.edit(&in)
			assertField(t, validateSale(&in), tt.field)
		})
	}
}

func TestValidatePayment_Methods(t *testing.T) {
	tendered := money.Cents(5000)
	negative := money.Cents(-1)
	tests := []struct {
		name  string
		pm    SalePayment
		field string
	}{
		{"cash", SalePayment{Method: "cash"}, ""},
		{"cash with tendered", SalePayment{Method: "cash", Tendered: &tendered}, ""},
		{"card", SalePayment{Method: "card"}, ""},
		{"credit not allowed", SalePayment{Method: "credit"}, "payment.method"},
		{"tendered on card", SalePayment{Method: "card", Tendered: &tendered}, "payment.tendered_amount"},
		{"negative tendered", SalePayment{Method: "cash", Tendered: &negative}, "payment.tendered_amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assertField(t, validatePayment(tt.pm), tt.field) })
	}
}

func TestValidateEntry_Items(t *testing.T) {
	d := calendar.MustParse("2027-07-10")
	tests := []struct {
		name  string
		in    EntryInput
		field string
	}{
		{"valid", EntryInput{Date: d, Slots: calendar.SlotsFull, Items: []EntryItem{{TypeID: "a", Quantity: 2}}}, ""},
		{"no items", EntryInput{Date: d, Slots: calendar.SlotsFull}, "items"},
		{"repeated type", EntryInput{Date: d, Slots: calendar.SlotsFull, Items: []EntryItem{{TypeID: "a", Quantity: 1}, {TypeID: "a", Quantity: 1}}}, "items[1].entry_ticket_type_id"},
		{"zero quantity", EntryInput{Date: d, Slots: calendar.SlotsFull, Items: []EntryItem{{TypeID: "a"}}}, "items[0].quantity"},
		{"no date", EntryInput{Slots: calendar.SlotsFull, Items: []EntryItem{{TypeID: "a", Quantity: 1}}}, "date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assertField(t, validateEntry(&tt.in), tt.field) })
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		return
	}
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a validation error on %s", err, field)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("fields %+v, want %s", ve.Fields, field)
}
