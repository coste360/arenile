package printing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/printing/templates"
	"github.com/coste360/arenile/internal/store"
)

// TicketSource loads the printable data of a daily-sale ticket. The daily
// sale implements it at merge time (Deps.Tickets); Cash reports whether the
// ticket was paid in cash (drawer kick). ErrNotFound for unknown tickets.
type TicketSource interface {
	PrintableTicket(ctx context.Context, es store.EstablishmentScope, ticketID string) (doc templates.TicketDoc, cash bool, err error)
}

// errTicketsUnavailable: no TicketSource wired yet.
var errTicketsUnavailable = configerr.Invalid("document_kind", "La stampa dei biglietti non è ancora disponibile.")

// docContext is what every document needs from the establishment.
type docContext struct {
	est    *store.Establishment
	header templates.Header
	loc    *time.Location
}

func (s *Service) docContext(ctx context.Context, es store.EstablishmentScope) (docContext, error) {
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return docContext{}, err
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	h := templates.Header{Name: est.Name, Address: formatAddress(est.Address), Phone: est.ContactPhone}
	if org, err := store.GetOrganization(ctx, s.d.DB, est.OrganizationID); err == nil && org.VATNumber != nil {
		h.VATNumber = *org.VATNumber
	}
	return docContext{est: est, header: h, loc: loc}, nil
}

func formatAddress(a store.Address) string {
	city := strings.TrimSpace(strings.Join(nonEmpty(a.PostalCode, a.City), " "))
	if a.Province != "" {
		city = strings.TrimSpace(city + " (" + a.Province + ")")
	}
	return strings.Join(nonEmpty(a.Street, city), ", ")
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// renderDocument loads the document and renders it for pr. The drawer is
// kicked only if requested, the printer has a drawer and the document is a
// cash one (cash payment, cash closing, cash ticket).
func (s *Service) renderDocument(ctx context.Context, es store.EstablishmentScope, pr *store.Printer, kind, docID string, copies int, kick bool) ([]byte, error) {
	dc, err := s.docContext(ctx, es)
	if err != nil {
		return nil, err
	}
	layout := templates.Layout{
		PaperWidthMM: pr.PaperWidthMM, CharsPerLine: pr.CharsPerLine, Codepage: escpos.Codepage(pr.Codepage),
		CodepageTable: pr.CodepageTable, Cut: escpos.CutMode(pr.CutMode), FeedLines: pr.FeedLines,
		Copies: copies, Lang: dc.est.DefaultLocale,
	}
	if pr.PrintLogo {
		layout.Logo = s.logo(ctx, pr, dc.est)
	}
	pin := 0
	if kick && pr.DrawerPin != nil {
		pin = *pr.DrawerPin
	}
	notFound := func(err error) error {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	switch kind {
	case store.PrintDocTestPage:
		return templates.TestPage(templates.TestPageDoc{
			Header: dc.header, PrinterName: pr.Name, Host: pr.Host, Port: pr.Port, PrintedAt: s.now(), Location: dc.loc,
		}, layout)
	case store.PrintDocPaymentReceipt:
		doc, cash, err := s.receiptDoc(ctx, es, dc, docID)
		if err != nil {
			return nil, notFound(err)
		}
		if cash {
			layout.KickDrawerPin = pin
		}
		return templates.Receipt(doc, layout)
	case store.PrintDocCashZ:
		doc, err := s.zDoc(ctx, es, dc, docID)
		if err != nil {
			return nil, notFound(err)
		}
		layout.KickDrawerPin = pin
		return templates.ZSummary(doc, layout)
	case store.PrintDocDailyTicket:
		if s.d.Tickets == nil {
			return nil, errTicketsUnavailable
		}
		doc, cash, err := s.d.Tickets.PrintableTicket(ctx, es, docID)
		if err != nil {
			return nil, notFound(err)
		}
		if doc.Header == (templates.Header{}) {
			doc.Header = dc.header
		}
		if doc.Location == nil {
			doc.Location = dc.loc
		}
		if cash {
			layout.KickDrawerPin = pin
		}
		return templates.Ticket(doc, layout)
	}
	return nil, configerr.Invalid("document_kind", "Tipo di documento non valido.")
}

// receiptDoc builds the non-fiscal receipt of an incoming payment.
func (s *Service) receiptDoc(ctx context.Context, es store.EstablishmentScope, dc docContext, paymentID string) (templates.ReceiptDoc, bool, error) {
	pm, err := store.GetPayment(ctx, s.d.DB, es, paymentID)
	if err != nil {
		return templates.ReceiptDoc{}, false, err
	}
	lang := dc.est.DefaultLocale
	doc := templates.ReceiptDoc{
		Header: dc.header, Number: templates.ShortID(pm.ID), IssuedAt: pm.CreatedAt, Location: dc.loc,
		Total: pm.Signed(), PaymentMethod: templates.MethodLabel(lang, pm.Method),
	}
	desc := strings.TrimSpace(pm.Description)
	if pm.BookingID != nil {
		if b, err := store.GetBooking(ctx, s.d.DB, es, *pm.BookingID); err == nil {
			doc.BookingCode = b.Code
		}
	}
	if desc == "" {
		desc = map[string]string{"it": "Pagamento", "en": "Payment", "de": "Zahlung", "fr": "Paiement"}[langOf(lang)]
	}
	doc.Lines = []templates.Line{{Label: desc, Amount: pm.Signed()}}
	for _, v := range pm.VATBreakdown {
		sign := money.Cents(1)
		if pm.Direction == store.PaymentDirectionRefund {
			sign = -1
		}
		doc.VAT = append(doc.VAT, templates.VATLine{RateBP: v.RateBP, Taxable: v.Taxable * sign, Tax: v.Tax * sign})
	}
	if pm.ReceivedByUserID != nil {
		if u, err := store.GetUser(ctx, s.d.DB, *pm.ReceivedByUserID); err == nil {
			doc.Operator = u.Name
		}
	}
	return doc, pm.Method == store.PaymentMethodCash, nil
}

// zDoc builds the closing summary of a cash session (draft when open).
func (s *Service) zDoc(ctx context.Context, es store.EstablishmentScope, dc docContext, sessionID string) (templates.ZSummaryDoc, error) {
	cs, err := store.GetCashSession(ctx, s.d.DB, es, sessionID)
	if err != nil {
		return templates.ZSummaryDoc{}, err
	}
	lang := dc.est.DefaultLocale
	doc := templates.ZSummaryDoc{
		Header: dc.header, Register: cs.Register, SessionID: cs.ID, OpenedAt: cs.OpenedAt, ClosedAt: cs.ClosedAt,
		Location: dc.loc, OpeningFloat: cs.OpeningFloat,
	}
	if cs.RegisterID != nil {
		if name, err := store.GetCashRegisterName(ctx, s.d.DB, es, *cs.RegisterID); err == nil && name != "" {
			doc.Register = name
		}
	}
	for _, uid := range []*string{cs.OpenedBy, cs.ClosedBy} {
		if uid == nil {
			continue
		}
		if u, err := store.GetUser(ctx, s.d.DB, *uid); err == nil && !slices.Contains(doc.Operators, u.Name) {
			doc.Operators = append(doc.Operators, u.Name)
		}
	}
	doc.ByMethod = methodLines(lang, cs.ExpectedTotals)
	doc.Counted = methodLines(lang, cs.CountedTotals)
	if cs.Difference != nil {
		doc.Difference = *cs.Difference
	}
	mvs, err := store.ListCashMovements(ctx, s.d.DB, es, cs.ID)
	if err != nil {
		return templates.ZSummaryDoc{}, err
	}
	for _, m := range mvs {
		amt := m.Amount
		if m.Kind == store.CashMovementOut {
			amt = -amt
		}
		doc.Movements = append(doc.Movements, templates.Line{Label: m.Reason, Amount: amt})
	}
	return doc, nil
}

// methodOrder is the printing order of the payment methods.
var methodOrder = []string{
	store.PaymentMethodCash, store.PaymentMethodCard, store.PaymentMethodBankTransfer,
	store.PaymentMethodWallet, store.PaymentMethodCredit, store.PaymentMethodOther,
}

func methodLines(lang string, t store.MethodTotals) []templates.Line {
	var out []templates.Line
	seen := map[string]bool{}
	for _, m := range methodOrder {
		if v, ok := t[m]; ok {
			out = append(out, templates.Line{Label: templates.MethodLabel(lang, m), Amount: v})
			seen[m] = true
		}
	}
	var rest []string
	for m := range t {
		if !seen[m] {
			rest = append(rest, m)
		}
	}
	slices.Sort(rest)
	for _, m := range rest {
		out = append(out, templates.Line{Label: templates.MethodLabel(lang, m), Amount: t[m]})
	}
	return out
}

func langOf(l string) string {
	if len(l) >= 2 {
		switch l[:2] {
		case "en", "de", "fr":
			return l[:2]
		}
	}
	return "it"
}

// logError logs a non-fatal rendering problem (never the payload).
func (s *Service) logError(ctx context.Context, msg, printerID string, err error) {
	s.d.Log.WarnContext(ctx, msg, slog.String("printer_id", printerID), slog.String("error", fmt.Sprint(err)))
}
