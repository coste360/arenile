package ticketing

import (
	"context"

	"github.com/coste360/arenile/internal/printing/templates"
	"github.com/coste360/arenile/internal/store"
)

// PrintableTicket implements printing.TicketSource (ADR 0021): the ESC/POS
// version of the ticket the browser prints (PrintData), read in es. It
// carries the number ("#037"), the day, the zone and the spot (empty = "da
// assegnare"), slot, pack and extras (the ticket types of an entry ticket),
// total with the VAT split, payment method, the QR of the gate token and
// the footer text of the daily sale settings, marked as a reprint once the
// ticket was printed. The holder name is left out: printed documents carry
// no customer personal data. cash reports a cash ticket (drawer kick).
// The header and time zone are filled by the printing service. Unknown
// tickets return store.ErrNotFound. No write: the print count belongs to
// Reprint, which the panel calls before queuing a reprint.
func (s *Service) PrintableTicket(ctx context.Context, es store.EstablishmentScope, ticketID string) (templates.TicketDoc, bool, error) {
	t, err := store.GetTicket(ctx, s.d.DB, es, ticketID)
	if err != nil {
		return templates.TicketDoc{}, false, err
	}
	pd, err := s.printData(ctx, s.d.DB, es, t, "", t.PrintCount > 0)
	if err != nil {
		return templates.TicketDoc{}, false, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return templates.TicketDoc{}, false, err
	}
	return ticketDoc(pd, est.DefaultLocale), t.PaymentMethod == store.TicketPaymentCash, nil
}

// ticketDoc maps the print data of a ticket to the ESC/POS template in
// lang (names localized, Italian fallback).
func ticketDoc(pd *PrintData, lang string) templates.TicketDoc {
	v := pd.View
	name := func(t store.I18nText) string { return localized(t, lang) }
	d := templates.TicketDoc{
		Number: v.NumberLabel, Entry: v.Ticket.Kind == store.TicketKindEntry, Reprint: pd.Reprint,
		Voided: v.State == StateVoided, Date: v.Ticket.TicketDate, IssuedAt: pd.IssuedAt,
		Zone: name(v.ZoneName), Spot: v.SpotLabel, Total: v.Ticket.TotalAmount,
		PaymentMethod: templates.MethodLabel(lang, v.Ticket.PaymentMethod),
		BookingCode:   v.BookingCode, QRPayload: pd.QRPayload, Notes: pd.FooterText,
	}
	if v.Slots != "" {
		d.Formula = templates.SlotsLabel(lang, string(v.Slots))
	}
	if !d.Entry {
		d.Pack = name(v.PackName)
		// The pack line is the total less the extras (the price is kept
		// whole on the booking, extras have their own lines).
		d.PackAmount = v.Ticket.TotalAmount
		for _, x := range v.Extras {
			d.PackAmount -= x.Total
		}
	}
	for _, it := range v.EntryItems {
		d.Extras = append(d.Extras, templates.Line{Label: name(it.Name), Quantity: it.Quantity, Amount: it.Total})
	}
	for _, x := range v.Extras {
		d.Extras = append(d.Extras, templates.Line{Label: name(x.Name), Quantity: x.Quantity, Amount: x.Total})
	}
	for _, r := range v.VAT {
		if bp, ok := r.Rate.BasisPoints(); ok {
			d.VAT = append(d.VAT, templates.VATLine{RateBP: bp, Taxable: r.Taxable, Tax: r.Tax})
		}
	}
	return d
}
