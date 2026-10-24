package ticketing

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestSummarize_VoidedLeftOut(t *testing.T) {
	now := time.Date(2027, 7, 10, 9, 0, 0, 0, time.UTC)
	r22, r10 := pricing.NewDecimal(22, 0), pricing.NewDecimal(10, 0)
	vat := func(rate pricing.Decimal, taxable, tax int64) []pricing.VATBreakdown {
		return []pricing.VATBreakdown{{Rate: rate, Taxable: moneyOf(taxable), Tax: moneyOf(tax)}}
	}
	vs := []View{
		{Ticket: store.Ticket{Status: store.TicketValid, TotalAmount: 2000, PaymentMethod: store.TicketPaymentCash}, VAT: vat(r22, 1639, 361)},
		{Ticket: store.Ticket{Status: store.TicketValid, TotalAmount: 1100, PaymentMethod: store.TicketPaymentCard, NeedsAssignment: true}, VAT: vat(r10, 1000, 100)},
		{Ticket: store.Ticket{Status: store.TicketValid, TotalAmount: 2000, PaymentMethod: store.TicketPaymentCash, EnteredAt: &now}, VAT: vat(r22, 1639, 361)},
		{Ticket: store.Ticket{Status: store.TicketVoided, TotalAmount: 5000, PaymentMethod: store.TicketPaymentCash, VoidedAt: &now}, VAT: vat(r22, 4098, 902)},
	}
	s := Summarize(vs)
	if s.Issued != 3 || s.Voided != 1 || s.Unassigned != 1 || s.Entered != 1 || s.Total != 5100 {
		t.Fatalf("counters %+v", s)
	}
	if len(s.ByMethod) != 2 || s.ByMethod[0].Method != "card" || s.ByMethod[1].Amount != 4000 || s.ByMethod[1].Count != 2 {
		t.Fatalf("by method %+v", s.ByMethod)
	}
	if len(s.VAT) != 2 || s.VAT[0].Rate.String() != "22" || s.VAT[0].Taxable != 3278 || s.VAT[1].Tax != 100 {
		t.Fatalf("vat %+v", s.VAT)
	}
	if empty := Summarize(nil); empty.ByMethod == nil || empty.VAT == nil || empty.Issued != 0 {
		t.Fatalf("empty %+v", empty)
	}
}

func TestNormalizeCode_Payloads(t *testing.T) {
	tests := []struct{ in, want string }{
		{"LD-2027-000123", "LD-2027-000123"},
		{"  ld-2027-000123 \n", "LD-2027-000123"},
		{"https://lido.example/b/LD-2027-000123", "LD-2027-000123"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeCode(tt.in); got != tt.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatAddress_Parts(t *testing.T) {
	tests := []struct {
		a    store.Address
		want string
	}{
		{store.Address{Street: "Lungomare 1", PostalCode: "04024", City: "Gaeta", Province: "LT"}, "Lungomare 1, 04024 Gaeta (LT)"},
		{store.Address{City: "Gaeta"}, "Gaeta"},
		{store.Address{}, ""},
	}
	for _, tt := range tests {
		if got := formatAddress(tt.a); got != tt.want {
			t.Errorf("formatAddress(%+v) = %q, want %q", tt.a, got, tt.want)
		}
	}
}

func TestLocalized_Fallback(t *testing.T) {
	txt := store.I18nText{"it": "Grazie", "en": "Thanks"}
	if localized(txt, "en") != "Thanks" || localized(txt, "de") != "Grazie" || localized(store.I18nText{"fr": "Merci"}, "de") != "Merci" || localized(nil, "it") != "" {
		t.Fatal("localized fallback")
	}
}

func TestValidFormat_Values(t *testing.T) {
	for _, f := range []string{"", "thermal_80", "thermal_58", "a4", "a5"} {
		if !validFormat(f) {
			t.Errorf("%q refused", f)
		}
	}
	if validFormat("a3") {
		t.Error("a3 accepted")
	}
}

func moneyOf(v int64) money.Cents { return money.Cents(v) }
