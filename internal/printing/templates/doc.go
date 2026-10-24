// Package templates renders Arenile documents as ESC/POS bytes for 58 and
// 80 mm paper (ADR 0021) on top of internal/escpos: payment receipt (non
// fiscale), cash Z summary, test page and the generic ticket (TicketDoc)
// used by the daily sale. Pure functions: data and Layout in, bytes out; no
// DB, no clock (dates and instants are passed in), covered by golden tests
// for both widths.
//
// Labels are printed in the establishment language (Lang: it default, en,
// de, fr) from the label tables of this package; amounts are formatted
// "€ 1.234,50" from money.Cents; dates dd/MM/yyyy.
//
// Every document ends with the Arenile mono mark line ("Arenile" in small
// type, ADR 0018), then Layout.FeedLines, the cut and, if requested, the
// drawer kick.
//
// Ownership: ESC/POS builders (templates). Ticket is fed by the ticketing
// service (ticketing.Service.PrintableTicket) through printing.TicketSource.
package templates
