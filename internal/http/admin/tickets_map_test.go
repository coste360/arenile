package admin

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
)

func TestWriteTicketsCSV_Format(t *testing.T) {
	at := time.Date(2027, 7, 10, 9, 5, 0, 0, time.UTC)
	items := []ticketing.View{
		{
			Ticket: store.Ticket{
				TicketNumber: 37, TicketDate: calendar.MustParse("2027-07-10"), Kind: store.TicketKindSale, TotalAmount: 2550,
				PaymentMethod: store.TicketPaymentCard, HolderName: "=HYPERLINK(1)", CreatedAt: at,
			},
			NumberLabel: "#037", State: ticketing.StateUnassigned, ZoneName: store.I18nText{"it": "Prima fila"}, BookingCode: "LD-2027-000001",
		},
		{
			Ticket: store.Ticket{
				TicketNumber: 38, TicketDate: calendar.MustParse("2027-07-10"), Kind: store.TicketKindEntry, TotalAmount: 500,
				PaymentMethod: store.TicketPaymentCash, CreatedAt: at,
			},
			NumberLabel: "#038", State: ticketing.StateVoided, SpotLabel: "", ZoneName: store.I18nText{"it": "Spiaggia libera"},
		},
	}
	var buf bytes.Buffer
	if err := WriteTicketsCSV(&buf, items, "it"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "\xEF\xBB\xBFnumero;data;ora;") {
		t.Fatalf("header %q", out)
	}
	for _, want := range []string{
		"#037;2027-07-10;09:05;vendita;Prima fila;da assegnare;25,50;carta;LD-2027-000001;'=HYPERLINK(1)",
		"#038;2027-07-10;09:05;ingresso;Spiaggia libera;annullato;5,00;contante;;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestEuroComma_Values(t *testing.T) {
	for in, want := range map[int64]string{0: "0,00", 5: "0,05", 2550: "25,50", -120: "-1,20"} {
		if got := euroComma(in); got != want {
			t.Errorf("euroComma(%d) = %q, want %q", in, got, want)
		}
	}
}
