//go:build integration

package dailysale_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/printing"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/printing/printertest"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestTicketing_PrintsOnThermalPrinter (ADR 0020, ADR 0021): the ticketing
// service is the printing.TicketSource. A spot ticket, a zone-first ticket
// and an entry ticket queued as daily_ticket jobs reach the network printer
// with number, day, spot or "da assegnare", pack or ticket types, amount,
// the QR of the gate token and the non-fiscal line; the drawer opens only
// for the first print of a cash ticket (never on a reprint); the optional
// holder surname is printed like on the browser ticket; a reprint says so; unknown
// tickets are not found; the route is offered only on the desktop edition.
func TestTicketing_PrintsOnThermalPrinter(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		f, ctx, est := e.f, e.f.Ctx, e.f.Est.ID
		e.openSession("")

		spotIn := e.spotSale(f.Spots[0])
		spotIn.LastName = "Rossi"
		spot, err := e.sales.CreateDaily(ctx, f.Cashier, est, spotIn)
		if err != nil {
			t.Fatal(err)
		}
		zoneIn := e.spotSale(f.Spots[0])
		zoneIn.Target = dailysale.Target{ZoneID: f.SeatZone.ID}
		zoneIn.Payment.Method = store.TicketPaymentCard
		zone, err := e.sales.CreateDaily(ctx, f.Cashier, est, zoneIn)
		if err != nil {
			t.Fatal(err)
		}
		entry := mustEntry(t, e, dailysale.EntryInput{
			Date: spotIn.Date, Slots: "FULL", Payment: dailysale.SalePayment{Method: store.TicketPaymentCash},
			Items: []dailysale.EntryItem{{TypeID: e.adult.ID, Quantity: 2}},
		})

		svc := printing.NewService(printing.Deps{
			DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Available: true, AllowLoopback: true, Tickets: e.tickets,
		})
		if !svc.TicketsAvailable() {
			t.Fatal("tickets must be printable on the desktop edition")
		}
		if printing.NewService(printing.Deps{DB: db, Clock: f.Clock, Tickets: e.tickets}).TicketsAvailable() {
			t.Fatal("tickets must not be printable on the server edition")
		}
		fp := printertest.Start(t)
		pin := 2
		pr, err := svc.CreatePrinter(ctx, f.Owner, est, printing.PrinterInput{
			Name: "Cassa", Host: fp.Host(), Port: fp.Port(), PaperWidthMM: 80, DrawerPin: &pin,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdateRoutes(ctx, f.Owner, est, nil, []printing.Route{
			{DocumentKind: store.PrintDocDailyTicket, PrinterID: pr.ID, KickDrawer: true},
		}); err != nil {
			t.Fatal(err)
		}
		enqueue := func(id string) {
			t.Helper()
			if _, _, err := svc.Enqueue(ctx, f.Cashier, est, printing.JobInput{DocumentKind: store.PrintDocDailyTicket, DocumentID: id}, ""); err != nil {
				t.Fatalf("enqueue %s: %v", id, err)
			}
		}
		if _, _, err := svc.Enqueue(ctx, f.Cashier, est, printing.JobInput{DocumentKind: store.PrintDocDailyTicket, DocumentID: store.NewID()}, ""); !errors.Is(err, printing.ErrNotFound) {
			t.Fatalf("unknown ticket: %v", err)
		}
		for _, id := range []string{spot.Ticket.ID, zone.Ticket.ID, entry.Ticket.ID} {
			enqueue(id)
		}
		if n, err := svc.DeliverDue(ctx, 10); err != nil || n != 3 {
			t.Fatalf("delivered %d: %v", n, err)
		}
		jobs := fp.WaitJobs(3, 5*time.Second)
		if len(jobs) != 3 {
			t.Fatalf("printer received %d jobs", len(jobs))
		}
		find := func(sub string) []byte {
			t.Helper()
			for _, j := range jobs {
				if bytes.Contains(j, []byte(sub)) {
					return j
				}
			}
			t.Fatalf("no job contains %q", sub)
			return nil
		}
		kick := []byte{escpos.ESC, 'p', 0}
		qr := func(token string) []byte { return append([]byte("1P0"), token...) }
		euro := func(c int64) string { return fmt.Sprintf("%d,%02d", c/100, c%100) }

		s := find("BIGLIETTO #001")
		for _, want := range [][]byte{
			[]byte("10/07/2027"), []byte("A1"), []byte("Giornata intera"), []byte(euro(int64(spot.Ticket.TotalAmount))),
			[]byte("Contanti"), []byte("Non valido ai fini fiscali"), qr(spot.Ticket.GateToken), kick, []byte("Rossi"),
		} {
			if !bytes.Contains(s, want) {
				t.Errorf("spot ticket lacks %q", want)
			}
		}
		z := find("BIGLIETTO #002")
		if !bytes.Contains(z, []byte("da assegnare")) || !bytes.Contains(z, []byte("Carta")) || bytes.Contains(z, kick) ||
			!bytes.Contains(z, qr(zone.Ticket.GateToken)) {
			t.Errorf("zone ticket:\n%s", escpos.Dump(z))
		}
		en := find("INGRESSO #003")
		if !bytes.Contains(en, []byte("2 x adulto")) || bytes.Contains(en, []byte("Postazione")) ||
			!bytes.Contains(en, qr(entry.Ticket.GateToken)) || !bytes.Contains(en, kick) {
			t.Errorf("entry ticket:\n%s", escpos.Dump(en))
		}
		for _, j := range jobs {
			if bytes.Contains(j, []byte("RISTAMPA")) {
				t.Errorf("reprint mark on a first print:\n%s", escpos.Dump(j))
			}
		}

		// A counted reprint is marked as such on the thermal paper too.
		if _, err := e.tickets.Reprint(ctx, f.Cashier, est, spot.Ticket.ID, ""); err != nil {
			t.Fatal(err)
		}
		enqueue(spot.Ticket.ID)
		if _, err := svc.DeliverDue(ctx, 10); err != nil {
			t.Fatal(err)
		}
		jobs = fp.WaitJobs(4, 5*time.Second)
		if len(jobs) != 4 || !bytes.Contains(jobs[3], []byte("RISTAMPA")) || !bytes.Contains(jobs[3], []byte("BIGLIETTO #001")) {
			t.Fatalf("reprint: %d jobs", len(jobs))
		}
		// A reprint of a cash ticket takes no money: the drawer stays closed.
		if bytes.Contains(jobs[3], kick) {
			t.Error("reprint opened the cash drawer")
		}
	})
}
