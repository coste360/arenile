package ticketing

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/printing/templates"
	"github.com/coste360/arenile/internal/store"
)

func TestTicketDoc_Scenarios(t *testing.T) {
	issued := time.Date(2027, 7, 10, 7, 30, 0, 0, time.UTC)
	base := func(kind string) PrintData {
		return PrintData{
			View: View{
				Ticket: store.Ticket{
					Kind: kind, TicketDate: calendar.New(2027, 7, 10), TotalAmount: 3800,
					PaymentMethod: store.TicketPaymentCash, Status: store.TicketValid,
				},
				State: StateAssigned, NumberLabel: "#037", BookingCode: "ARN-7F3K",
				ZoneName: store.I18nText{"it": "Prima fila", "en": "Front row"}, SpotLabel: "A12",
				Slots: calendar.SlotsFull, PackName: store.I18nText{"it": "Base"},
				Extras: []ViewItem{{Name: store.I18nText{"it": "Lettino"}, Quantity: 2, Total: 1000}},
				VAT: []pricing.VATBreakdown{
					{Rate: pricing.NewDecimal(22, 0), Taxable: 2295, Tax: 505},
					{Rate: pricing.NewDecimal(55, 1), Taxable: 948, Tax: 52},
				},
			},
			QRPayload: "gt_token", IssuedAt: issued, FooterText: "Grazie",
		}
	}
	tests := []struct {
		name  string
		lang  string
		mut   func(*PrintData)
		check func(t *testing.T, d templates.TicketDoc)
	}{
		{"spot with extras", "it", nil, func(t *testing.T, d templates.TicketDoc) {
			if d.Number != "#037" || d.Spot != "A12" || d.Zone != "Prima fila" || d.Pack != "Base" ||
				d.PackAmount != money.Cents(2800) || d.Formula != "Giornata intera" || d.PaymentMethod != "Contanti" ||
				d.QRPayload != "gt_token" || d.Notes != "Grazie" || d.Entry || d.Reprint || d.Voided {
				t.Fatalf("doc %+v", d)
			}
			if len(d.Extras) != 1 || d.Extras[0].Label != "Lettino" || d.Extras[0].Quantity != 2 || d.Extras[0].Amount != 1000 {
				t.Fatalf("extras %+v", d.Extras)
			}
			if len(d.VAT) != 2 || d.VAT[0].RateBP != 2200 || d.VAT[1].RateBP != 550 || d.VAT[1].Tax != 52 {
				t.Fatalf("vat %+v", d.VAT)
			}
		}},
		{"zone first to assign", "it", func(p *PrintData) { p.View.SpotLabel, p.View.State = "", StateUnassigned }, func(t *testing.T, d templates.TicketDoc) {
			if d.Spot != "" || d.Zone != "Prima fila" {
				t.Fatalf("doc %+v", d)
			}
		}},
		{"entry ticket", "it", func(p *PrintData) {
			p.View.Ticket.Kind, p.View.SpotLabel, p.View.PackName, p.View.Extras = store.TicketKindEntry, "", nil, nil
			p.View.EntryItems = []ViewItem{{Name: store.I18nText{"it": "Adulto"}, Quantity: 2, Total: 3800}}
		}, func(t *testing.T, d templates.TicketDoc) {
			if !d.Entry || d.Pack != "" || d.PackAmount != 0 || len(d.Extras) != 1 || d.Extras[0].Label != "Adulto" {
				t.Fatalf("doc %+v", d)
			}
		}},
		{"voided reprint", "it", func(p *PrintData) { p.View.State, p.Reprint = StateVoided, true }, func(t *testing.T, d templates.TicketDoc) {
			if !d.Voided || !d.Reprint {
				t.Fatalf("doc %+v", d)
			}
		}},
		{"english with italian fallback", "en", nil, func(t *testing.T, d templates.TicketDoc) {
			if d.Zone != "Front row" || d.Pack != "Base" || d.Formula != "Full day" || d.PaymentMethod != "Cash" {
				t.Fatalf("doc %+v", d)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := base(store.TicketKindSale)
			if tt.mut != nil {
				tt.mut(&pd)
			}
			tt.check(t, ticketDoc(&pd, tt.lang))
		})
	}
}
