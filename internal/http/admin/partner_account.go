package admin

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/printout"
	"github.com/coste360/arenile/internal/store"
)

// Partner account and M8 reports (spec 30.2.4-30.2.5, ADR 0028).
//
//	GetPartnerStatement   -> Reports.PartnerStatement + Partners; json | xlsx | html
//	ListPartnerPayments / RecordPartnerPayment / VoidPartnerPayment -> Partners
//	GetPartnersReport     -> Reports.Partners; json | csv | xlsx
//	GetAgreementsRegister -> Reports.AgreementsRegister; json | xlsx | html

// Printable formats (api.PrintableFormat).
const (
	printableJSON = "json"
	printableXLSX = "xlsx"
	printableHTML = "html"
)

func printableFormat(f *api.PrintableFormat) (string, error) {
	if f == nil || *f == "" {
		return printableJSON, nil
	}
	switch *f {
	case printableJSON, printableXLSX, printableHTML:
		return *f, nil
	}
	return "", configerr.Invalid("format", "Valori ammessi: json, xlsx, html.")
}

// GetPartnerStatement implements GET /partners/{partnerId}/statement.
func (h *Handlers) GetPartnerStatement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, params api.GetPartnerStatementParams) {
	if !h.partnersReady(w, r) {
		return
	}
	format, err := printableFormat(params.Format)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	st, err := h.Reports.PartnerStatement(r.Context(), principalOf(r), establishmentID, id, p)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	pv, err := h.Partners.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	file := "estratto-conto_" + fileSlug(pv.Name) + "_" + p.From.String() + "_" + p.To.String()
	switch format {
	case printableXLSX:
		h.writeExport(w, r, exportXLSX, file, func(dst io.Writer) error { return report.WriteStatementCSV(dst, st) })
		return
	case printableHTML:
		h.writePartnerStatementHTML(w, r, establishmentID, pv, st, file)
		return
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, PartnerStatementToAPI(pv, st))
}

func (h *Handlers) writePartnerStatementHTML(w http.ResponseWriter, r *http.Request, establishmentID string, pv *partner.View, st *report.Statement, file string) {
	pc, err := h.Reports.PrintContext(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	doc := printout.PartnerStatement{
		Locale: printLocale(r, pc), Currency: pc.Establishment.Currency, Location: pc.Location, Header: printHeader(pc),
		Partner: printout.StatementPartner{
			Name: pv.Name, Address: pv.Address, Municipality: pv.Municipality, VATNumber: pv.VATNumber, TaxCode: pv.TaxCode,
		},
		From: st.Period.From, To: st.Period.To, GeneratedAt: st.GeneratedAt, SpotDays: st.Totals.SpotDays, People: st.Totals.People,
		Total: st.Totals.Total, Discount: st.Totals.Discount, Opening: st.OpeningBalance, Charged: st.Charged, Paid: st.Paid,
		Closing: st.ClosingBalance,
	}
	for _, b := range st.Bookings {
		doc.Bookings = append(doc.Bookings, printout.StatementBooking{
			Code: b.Code, Guest: b.CustomerDisplayName, Group: b.GroupName, Agreement: b.AgreementName, StartsOn: b.StartsOn,
			EndsOn: b.EndsOn, SpotDays: b.SpotDays, People: b.People, Total: b.Total, Discount: b.Discount, Partner: b.PartnerAmount,
		})
	}
	for _, pay := range st.Payments {
		doc.Payments = append(doc.Payments, printout.StatementPayment{
			ReceivedOn: pay.ReceivedOn, Method: pay.Method, Reference: pay.Reference, Amount: pay.Amount, Voided: pay.VoidedAt != nil,
		})
	}
	var buf bytes.Buffer
	if err := printout.RenderPartnerStatement(&buf, doc); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writePrintable(w, file+".html", buf.Bytes())
}

// ListPartnerPayments implements GET /partners/{partnerId}/payments.
func (h *Handlers) ListPartnerPayments(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, params api.ListPartnerPaymentsParams) {
	if !h.partnersReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Partners.ListPayments(r.Context(), principalOf(r), establishmentID, id, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PartnerPaymentList{Items: make([]api.PartnerPayment, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, PartnerPaymentToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// RecordPartnerPayment implements POST /partners/{partnerId}/payments.
func (h *Handlers) RecordPartnerPayment(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, _ api.RecordPartnerPaymentParams) {
	if !h.partnersReady(w, r) {
		return
	}
	var req api.PartnerPaymentInput
	if !decode(w, r, &req) {
		return
	}
	pay, err := h.Partners.RecordPayment(r.Context(), principalOf(r), establishmentID, id, partner.PaymentInput{
		Amount: money.Cents(req.Amount), ReceivedOn: req.ReceivedOn, Method: string(req.Method),
		Reference: customerStr(req.Reference), Note: customerStr(req.Note),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, PartnerPaymentToAPI(pay))
}

// VoidPartnerPayment implements POST /partners/{partnerId}/payments/{partnerPaymentId}/void.
func (h *Handlers) VoidPartnerPayment(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PartnerID, paymentID api.PartnerPaymentID, _ api.VoidPartnerPaymentParams) {
	if !h.partnersReady(w, r) {
		return
	}
	var req api.PartnerPaymentVoid
	if !decode(w, r, &req) {
		return
	}
	pay, err := h.Partners.VoidPayment(r.Context(), principalOf(r), establishmentID, id, paymentID, req.Reason)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, PartnerPaymentToAPI(pay))
}

// GetPartnersReport implements GET /reports/partners.
func (h *Handlers) GetPartnersReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetPartnersReportParams) {
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	if err := uuidParams(map[string]*string{"partner_id": params.PartnerId}); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res, err := h.Reports.Partners(r.Context(), principalOf(r), establishmentID, p, customerStr(params.PartnerId))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeReport(h, w, r, params.Format, "partner", p, res, func(x *report.PartnersReport) any { return PartnersReportToAPI(x) }, report.WritePartnersCSV)
}

// GetAgreementsRegister implements GET /reports/agreements-register.
func (h *Handlers) GetAgreementsRegister(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetAgreementsRegisterParams) {
	format, err := printableFormat(params.Format)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	p, ok := h.reportPeriod(w, r, params.From, params.To)
	if !ok {
		return
	}
	if err := uuidParams(map[string]*string{"season_id": params.SeasonId}); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	reg, err := h.Reports.AgreementsRegister(r.Context(), principalOf(r), establishmentID, p, customerStr(params.SeasonId))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	file := "registro-convenzioni_" + p.From.String() + "_" + p.To.String()
	switch format {
	case printableXLSX:
		h.writeExport(w, r, exportXLSX, file, func(dst io.Writer) error { return report.WriteAgreementsRegisterCSV(dst, reg) })
		return
	case printableHTML:
		h.writeRegisterHTML(w, r, establishmentID, reg, file)
		return
	}
	writeJSON(w, http.StatusOK, AgreementsRegisterToAPI(reg))
}

func (h *Handlers) writeRegisterHTML(w http.ResponseWriter, r *http.Request, establishmentID string, reg *report.AgreementsRegister, file string) {
	pc, err := h.Reports.PrintContext(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	doc := printout.AgreementsRegister{
		Locale: printLocale(r, pc), Currency: pc.Establishment.Currency, Location: pc.Location, Header: printHeader(pc),
		From: reg.Period.From, To: reg.Period.To, SeasonYear: reg.SeasonYear, GeneratedAt: reg.GeneratedAt,
		Total: printCounters(reg.Totals),
	}
	byCat := map[string]int{}
	for _, t := range reg.ByCategory {
		byCat[t.RegisterCategory] = len(doc.Categories)
		doc.Categories = append(doc.Categories, printout.RegisterCategory{Category: t.RegisterCategory, Total: printCounters(t.RegisterCounters)})
	}
	for _, e := range reg.Entries {
		i, ok := byCat[e.RegisterCategory]
		if !ok {
			continue
		}
		doc.Categories[i].Entries = append(doc.Categories[i].Entries, printout.RegisterEntry{
			Name: e.Name, Code: e.Code, Recipients: report.TargetLabel(e), Conditions: e.ConditionsText,
			ValidFrom: e.ValidFrom, ValidTo: e.ValidTo, RegisterCounters: printCounters(e.RegisterCounters),
		})
	}
	var buf bytes.Buffer
	if err := printout.RenderAgreementsRegister(&buf, doc); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writePrintable(w, file+".html", buf.Bytes())
}

// ---- helpers and mapping -------------------------------------------------------------

func printCounters(c report.RegisterCounters) printout.RegisterCounters {
	return printout.RegisterCounters{
		Agreements: c.Agreements, Bookings: c.Bookings, SpotDays: c.SpotDays, People: c.People,
		Revenue: c.Revenue, Discount: c.Discount,
	}
}

func printHeader(pc *report.PrintContext) printout.Header {
	est := &pc.Establishment
	hd := printout.Header{Name: est.Name, AddressLines: addressLines(est.Address)}
	if est.ConcessionRef != nil {
		hd.ConcessionRef = *est.ConcessionRef
	}
	if o := pc.Organization; o != nil {
		if o.Name != est.Name {
			hd.LegalName = o.Name
		}
		if o.VATNumber != nil {
			hd.VATNumber = *o.VATNumber
		}
		if o.FiscalCode != nil {
			hd.FiscalCode = *o.FiscalCode
		}
	}
	return hd
}

func printLocale(r *http.Request, pc *report.PrintContext) string {
	if p := principalOf(r); p != nil && p.Locale != "" {
		return p.Locale
	}
	return pc.Establishment.DefaultLocale
}

// writePrintable answers a printable HTML document (ADR 0019).
func writePrintable(w http.ResponseWriter, file string, body []byte) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", printout.ContentSecurityPolicy)
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Disposition", `inline; filename="`+file+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// fileSlug makes a name safe for a file name ("Hotel Miramare" ->
// "hotel-miramare").
func fileSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "partner"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// PartnerPaymentToAPI maps a partner payment.
func PartnerPaymentToAPI(p *store.PartnerPayment) api.PartnerPayment {
	return api.PartnerPayment{
		Id: p.ID, PartnerId: p.PartnerID, Amount: int64(p.Amount), ReceivedOn: p.ReceivedOn,
		Method: api.PartnerPaymentMethod(p.Method), Reference: strPtr(p.Reference), Note: strPtr(p.Note),
		RecordedByUserId: p.RecordedByUserID, VoidedAt: utcPtr(p.VoidedAt), VoidReason: p.VoidReason, CreatedAt: p.CreatedAt.UTC(),
	}
}

// PartnerStatementToAPI maps a statement with its partner.
func PartnerStatementToAPI(pv *partner.View, st *report.Statement) api.PartnerStatement {
	out := api.PartnerStatement{
		Partner: PartnerToAPI(pv), Period: reportPeriodToAPI(st.Period),
		Bookings: make([]api.PartnerStatementBooking, 0, len(st.Bookings)),
		Payments: make([]api.PartnerPayment, 0, len(st.Payments)),
		Totals: api.PartnerStatementTotals{
			Bookings: st.Totals.Bookings, People: st.Totals.People, SpotDays: st.Totals.SpotDays,
			TotalAmount: int64(st.Totals.Total), DiscountAmount: int64(st.Totals.Discount), Charged: int64(st.Charged),
			Paid: int64(st.Paid), OpeningBalance: int64(st.OpeningBalance), ClosingBalance: int64(st.ClosingBalance),
		},
	}
	for _, b := range st.Bookings {
		out.Bookings = append(out.Bookings, api.PartnerStatementBooking{
			BookingId: b.BookingID, Code: b.Code, Status: api.BookingStatus(b.Status), CustomerDisplayName: b.CustomerDisplayName,
			GroupName: strPtr(b.GroupName), AgreementName: strPtr(b.AgreementName), StartsOn: b.StartsOn, EndsOn: b.EndsOn,
			SpotDays: b.SpotDays, People: b.People, TotalAmount: int64(b.Total), DiscountAmount: int64(b.Discount),
			PartnerAmount: int64(b.PartnerAmount),
		})
	}
	for i := range st.Payments {
		out.Payments = append(out.Payments, PartnerPaymentToAPI(&st.Payments[i]))
	}
	return out
}

// PartnersReportToAPI maps the report per partner.
func PartnersReportToAPI(x *report.PartnersReport) api.PartnersReport {
	out := api.PartnersReport{Period: reportPeriodToAPI(x.Period), Rows: make([]api.PartnersReportRow, 0, len(x.Rows))}
	for _, r := range x.Rows {
		out.Rows = append(out.Rows, api.PartnersReportRow{
			PartnerId: r.PartnerID, PartnerName: r.PartnerName, Kind: api.PartnerKind(r.Kind), Bookings: r.Bookings,
			People: r.People, SpotDays: r.SpotDays, Revenue: int64(r.Revenue), Discount: int64(r.Discount),
			Charged: int64(r.Charged), Paid: int64(r.Paid),
		})
	}
	t := x.Totals
	out.Totals = api.PartnersReportTotals{
		Bookings: t.Bookings, People: t.People, SpotDays: t.SpotDays, Revenue: int64(t.Revenue), Discount: int64(t.Discount),
		Charged: int64(t.Charged), Paid: int64(t.Paid),
	}
	return out
}

func registerTotalToAPI(cat string, c report.RegisterCounters) api.AgreementsRegisterCategoryTotal {
	out := api.AgreementsRegisterCategoryTotal{
		Agreements: c.Agreements, Bookings: c.Bookings, SpotDays: c.SpotDays, People: c.People,
		Revenue: int64(c.Revenue), Discount: int64(c.Discount),
	}
	if cat != "" {
		rc := api.RegisterCategory(cat)
		out.RegisterCategory = &rc
	}
	return out
}

// AgreementsRegisterToAPI maps the register.
func AgreementsRegisterToAPI(x *report.AgreementsRegister) api.AgreementsRegister {
	out := api.AgreementsRegister{
		EstablishmentName: x.EstablishmentName, EstablishmentLegalName: strPtr(x.EstablishmentLegalName),
		EstablishmentVatNumber: strPtr(x.EstablishmentVATNumber), SeasonId: strPtr(x.SeasonID),
		Period: reportPeriodToAPI(x.Period), GeneratedAt: x.GeneratedAt,
		Entries:    make([]api.AgreementsRegisterEntry, 0, len(x.Entries)),
		ByCategory: make([]api.AgreementsRegisterCategoryTotal, 0, len(x.ByCategory)),
		Totals:     registerTotalToAPI("", x.Totals),
	}
	if x.SeasonYear != 0 {
		y := x.SeasonYear
		out.SeasonYear = &y
	}
	for _, e := range x.Entries {
		out.Entries = append(out.Entries, api.AgreementsRegisterEntry{
			AgreementId: e.AgreementID, Name: e.Name, Code: strPtr(e.Code), PartnerName: strPtr(e.PartnerName),
			CustomerCategoryName: strPtr(e.CustomerCategoryName), Target: api.AgreementTarget(e.Target),
			RegisterCategory: api.RegisterCategory(e.RegisterCategory), ConditionsText: e.ConditionsText,
			ValidFrom: e.ValidFrom, ValidTo: e.ValidTo, Bookings: e.Bookings, SpotDays: e.SpotDays, People: e.People,
			Revenue: int64(e.Revenue), Discount: int64(e.Discount),
		})
	}
	for _, c := range x.ByCategory {
		out.ByCategory = append(out.ByCategory, registerTotalToAPI(c.RegisterCategory, c.RegisterCounters))
	}
	return out
}
