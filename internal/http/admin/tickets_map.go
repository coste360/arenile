package admin

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// Mapping of the ticketing and daily sale views to the API (spec 14.3).

// TicketToAPI maps a ticket view.
func TicketToAPI(v *ticketing.View) api.Ticket {
	t := &v.Ticket
	out := api.Ticket{
		Id: t.ID, Kind: api.TicketKind(t.Kind), Status: api.TicketStatus(t.Status), State: api.TicketState(v.State),
		TicketDate: t.TicketDate, TicketNumber: t.TicketNumber, NumberLabel: v.NumberLabel,
		BookingId: t.BookingID, BookingCode: v.BookingCode, ZoneId: t.ZoneID, ZoneName: catalogI18nToAPI(v.ZoneName),
		SpotId: invOpt(v.SpotID), SpotLabel: invOpt(v.SpotLabel), NeedsAssignment: t.NeedsAssignment,
		HolderName: invOpt(t.HolderName), Slots: api.Slots(v.Slots), Formula: api.Formula(v.Formula), PackId: invOpt(v.PackID),
		Extras: make([]api.TicketExtra, 0, len(v.Extras)), EntryItems: make([]api.TicketEntryItem, 0, len(v.EntryItems)),
		TotalAmount: int64(t.TotalAmount), Vat: vatToAPI(v.VAT), PaymentMethod: api.TicketPaymentMethod(t.PaymentMethod),
		PaymentId: t.PaymentID, CashSessionId: t.CashSessionID, PrintCount: t.PrintCount, CreatedByUserId: t.CreatedByUserID,
		VoidedAt: utcPtr(t.VoidedAt), VoidReason: invOpt(t.VoidReason), RefundPaymentId: t.RefundPaymentID,
		EnteredAt: utcPtr(t.EnteredAt), Version: t.Version, CreatedAt: t.CreatedAt.UTC(),
	}
	if v.PackID != "" {
		n := catalogI18nToAPI(v.PackName)
		out.PackName = &n
	}
	for _, x := range v.Extras {
		out.Extras = append(out.Extras, api.TicketExtra{ExtraId: x.ID, Name: catalogI18nToAPI(x.Name), Quantity: x.Quantity, Total: int64(x.Total)})
	}
	for _, x := range v.EntryItems {
		out.EntryItems = append(out.EntryItems, api.TicketEntryItem{EntryTicketTypeId: x.ID, Name: catalogI18nToAPI(x.Name), Quantity: x.Quantity, Total: int64(x.Total)})
	}
	return out
}

func vatToAPI(vs []pricing.VATBreakdown) []api.VatBreakdown {
	out := make([]api.VatBreakdown, 0, len(vs))
	for _, x := range vs {
		out = append(out, api.VatBreakdown{Rate: x.Rate.String(), Taxable: int64(x.Taxable), Tax: int64(x.Tax)})
	}
	return out
}

// TicketListToAPI maps a page of the daily list.
func TicketListToAPI(l *ticketing.List) api.TicketList {
	out := api.TicketList{Items: make([]api.Ticket, 0, len(l.Items)), NextCursor: nextCursor(l.NextCursor)}
	for i := range l.Items {
		out.Items = append(out.Items, TicketToAPI(&l.Items[i]))
	}
	s := l.Summary
	out.Summary = api.TicketDaySummary{
		Issued: s.Issued, Voided: s.Voided, Unassigned: s.Unassigned, Entered: s.Entered, TotalAmount: int64(s.Total),
		ByMethod: make([]api.TicketMethodTotal, 0, len(s.ByMethod)), Vat: vatToAPI(s.VAT),
	}
	for _, m := range s.ByMethod {
		out.Summary.ByMethod = append(out.Summary.ByMethod, api.TicketMethodTotal{Method: api.TicketPaymentMethod(m.Method), Count: m.Count, Amount: int64(m.Amount)})
	}
	return out
}

// TicketPrintToAPI maps the print data of a ticket.
func TicketPrintToAPI(pd *ticketing.PrintData) api.TicketPrintData {
	e := pd.Establishment
	return api.TicketPrintData{
		Format: api.TicketPrintFormat(pd.Format), Ticket: TicketToAPI(&pd.View), QrPayload: pd.QRPayload, Reprint: pd.Reprint,
		FooterText: invOpt(pd.FooterText), IssuedAt: pd.IssuedAt.UTC(),
		Establishment: api.TicketPrintEstablishment{
			Name: e.Name, LegalName: invOpt(e.LegalName), VatNumber: invOpt(e.VATNumber), Address: invOpt(e.Address), LogoUrl: invOpt(e.LogoURL),
		},
	}
}

// EntryTicketTypeToAPI maps an entry ticket type.
func EntryTicketTypeToAPI(t *store.EntryTicketType) api.EntryTicketType {
	return api.EntryTicketType{
		Id: t.ID, Code: t.Code, Name: catalogI18nToAPI(t.Name), Description: catalogI18nToAPI(t.Description),
		PackId: t.PackID, ZoneId: t.ZoneID, SortOrder: t.SortOrder, Active: t.Active, Version: t.Version,
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
}

// DailySaleSettingsToAPI maps the daily sale settings.
func DailySaleSettingsToAPI(s *store.DailySaleSettings) api.DailySaleSettings {
	return api.DailySaleSettings{
		PrintFormat: api.TicketPrintFormat(s.PrintFormat), AutoPrint: s.AutoPrint, ZoneFirstEnabled: s.ZoneFirstEnabled,
		FooterText: catalogI18nToAPI(s.FooterText), Version: s.Version,
	}
}

// ticketStateLabels are the Italian labels of the CSV export (source language).
var ticketStateLabels = map[ticketing.State]string{
	ticketing.StateAssigned: "assegnato", ticketing.StateUnassigned: "da assegnare", ticketing.StateIssued: "emesso",
	ticketing.StateEntered: "entrato", ticketing.StateVoided: "annullato",
}

// WriteTicketsCSV writes the daily list as CSV (UTF-8 with BOM, ';'
// separator, amounts in euro with a decimal comma), the format of the other
// exports opened in spreadsheet programs.
func WriteTicketsCSV(w io.Writer, items []ticketing.View, locale string) error {
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write([]string{"numero", "data", "ora", "tipo", "postazione_o_zona", "stato", "importo", "metodo", "codice", "nome"}); err != nil {
		return err
	}
	for i := range items {
		v := &items[i]
		t := &v.Ticket
		where := v.SpotLabel
		if where == "" {
			where = i18nPick(v.ZoneName, locale)
		}
		kind := "vendita"
		if t.Kind == store.TicketKindEntry {
			kind = "ingresso"
		}
		method := "contante"
		if t.PaymentMethod == store.TicketPaymentCard {
			method = "carta"
		}
		if err := cw.Write([]string{
			v.NumberLabel, t.TicketDate.String(), t.CreatedAt.Format("15:04"), kind, csvSafe(where), ticketStateLabels[v.State],
			euroComma(int64(t.TotalAmount)), method, v.BookingCode, csvSafe(t.HolderName),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func i18nPick(t store.I18nText, locale string) string {
	if v := t[locale]; v != "" {
		return v
	}
	if v := t["it"]; v != "" {
		return v
	}
	for _, v := range t {
		return v
	}
	return ""
}

// csvSafe neutralises spreadsheet formulas in free text (CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func euroComma(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return sign + strconv.FormatInt(c/100, 10) + "," + leftPad2(c%100)
}

func leftPad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
