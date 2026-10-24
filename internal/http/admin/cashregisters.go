package admin

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/printout"
	"github.com/coste360/arenile/internal/store"
)

// D. Cash registers and the printable Z closing (ADR 0019): register CRUD
// (permissions checked by internal/domain/cash) and the Z summary as a
// print-ready HTML document rendered by internal/printout.

// ListCashRegisters implements GET /api/admin/v1/establishments/{establishmentId}/cash-registers.
func (h *Handlers) ListCashRegisters(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListCashRegistersParams) {
	inactive := params.IncludeInactive != nil && *params.IncludeInactive
	regs, err := h.Cash.ListRegisters(r.Context(), principalOf(r), establishmentID, inactive)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.CashRegisterList{Items: make([]api.CashRegister, 0, len(regs))}
	for i := range regs {
		out.Items = append(out.Items, CashRegisterToAPI(&regs[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateCashRegister implements POST /api/admin/v1/establishments/{establishmentId}/cash-registers.
func (h *Handlers) CreateCashRegister(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateCashRegisterParams) {
	var req api.CashRegisterInput
	if !decode(w, r, &req) {
		return
	}
	v, err := h.Cash.CreateRegister(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, registerInput(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, v.Version, CashRegisterToAPI(v))
}

// GetCashRegister implements GET /api/admin/v1/establishments/{establishmentId}/cash-registers/{registerId}.
func (h *Handlers) GetCashRegister(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashRegisterID) {
	v, err := h.Cash.GetRegister(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, CashRegisterToAPI(v))
}

// UpdateCashRegister implements PUT /api/admin/v1/establishments/{establishmentId}/cash-registers/{registerId}.
func (h *Handlers) UpdateCashRegister(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashRegisterID, params api.UpdateCashRegisterParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.CashRegisterInput
	if !decode(w, r, &req) {
		return
	}
	in := registerInput(req)
	in.Code = nil // immutable
	v, err := h.Cash.UpdateRegister(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, id, ifMatch, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Version, CashRegisterToAPI(v))
}

// DeleteCashRegister implements DELETE /api/admin/v1/establishments/{establishmentId}/cash-registers/{registerId}.
func (h *Handlers) DeleteCashRegister(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashRegisterID, params api.DeleteCashRegisterParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Cash.DeleteRegister(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, id, ifMatch); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetCashSessionZReport implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}/z-report:
// a self-contained HTML document (print styles for A4 or 80 mm paper); the
// PDF is the browser's print (ADR 0019).
func (h *Handlers) GetCashSessionZReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashSessionID, params api.GetCashSessionZReportParams) {
	paper := printout.PaperA4
	if params.Paper != nil {
		if !params.Paper.Valid() {
			problem.Error(w, r, h.Log, configerr.Invalid("paper", "Valori ammessi: a4, receipt80."))
			return
		}
		paper = string(*params.Paper)
	}
	p := principalOf(r)
	data, err := h.Cash.ZReport(r.Context(), p, establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	locale := data.Establishment.DefaultLocale
	if p != nil && p.Locale != "" {
		locale = p.Locale
	}
	var buf bytes.Buffer
	if err := printout.RenderZReport(&buf, ZReportInput(data, paper, locale)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	s := &data.Detail.Session
	name := "chiusura-z-" + s.Register + "-" + s.OpenedAt.In(data.Location).Format("2006-01-02") + ".html"
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", printout.ContentSecurityPolicy)
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Disposition", `inline; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// ZReportInput maps the cash data of a session to the printout input.
func ZReportInput(d *cash.ZReportData, paper, locale string) printout.ZReport {
	s := &d.Detail.Session
	sum := d.Detail.Summary
	est := d.Establishment
	z := printout.ZReport{
		Locale: locale, Paper: paper, Location: d.Location, Currency: est.Currency,
		SessionID: s.ID, RegisterName: s.RegisterName, RegisterCode: s.Register, Closed: s.ClosedAt != nil,
		OpenedAt: s.OpenedAt, ClosedAt: s.ClosedAt, OpenedBy: d.OpenedByName, ClosedBy: d.ClosedByName,
		GeneratedAt: d.GeneratedAt, OpeningFloat: s.OpeningFloat, MovementsIn: sum.MovementsIn,
		MovementsOut: sum.MovementsOut, Payments: sum.Payments, Refunds: sum.Refunds, Difference: sum.Difference(),
		Notes: s.Notes,
	}
	if s.Difference != nil {
		z.Difference = *s.Difference
	}
	z.Header = printout.Header{Name: est.Name, AddressLines: addressLines(est.Address)}
	if est.ConcessionRef != nil {
		z.Header.ConcessionRef = *est.ConcessionRef
	}
	if o := d.Organization; o != nil {
		if o.Name != est.Name {
			z.Header.LegalName = o.Name
		}
		if o.VATNumber != nil {
			z.Header.VATNumber = *o.VATNumber
		}
		if o.FiscalCode != nil {
			z.Header.FiscalCode = *o.FiscalCode
		}
	}
	methods := map[string]bool{}
	for m := range sum.Takings {
		methods[m] = true
	}
	expected := sum.Expected
	if s.ClosedAt != nil && len(s.ExpectedTotals) > 0 {
		expected = s.ExpectedTotals // frozen on close (INV-25)
	}
	for m := range expected {
		methods[m] = true
	}
	for m := range sum.Counted {
		methods[m] = true
	}
	for m := range methods {
		row := printout.ZMethodRow{Method: m, Takings: sum.Takings[m], Expected: expected[m]}
		if sum.Counted != nil {
			row.Counted = sum.Counted[m]
			row.Difference = sum.Counted[m] - expected[m]
		}
		z.Methods = append(z.Methods, row)
	}
	printout.SortMethods(z.Methods)
	for _, v := range sum.ByVAT {
		z.VAT = append(z.VAT, printout.ZVATRow{RateBP: v.RateBP, Taxable: v.Taxable, Tax: v.Tax, Gross: v.Gross})
	}
	for _, m := range d.Detail.Movements {
		z.Movements = append(z.Movements, printout.ZMovement{At: m.CreatedAt, Kind: m.Kind, Amount: m.Amount, Reason: m.Reason})
	}
	return z
}

func addressLines(a store.Address) []string {
	var out []string
	if a.Street != "" {
		out = append(out, a.Street)
	}
	city := strings.TrimSpace(strings.Join(nonEmpty(a.PostalCode, a.City), " "))
	if a.Province != "" {
		city = strings.TrimSpace(city + " (" + a.Province + ")")
	}
	if city != "" {
		out = append(out, city)
	}
	return out
}

func nonEmpty(ss ...string) []string {
	out := ss[:0:0]
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func registerInput(req api.CashRegisterInput) cash.RegisterInput {
	return cash.RegisterInput{Code: req.Code, Name: req.Name, Active: req.Active, SortOrder: req.SortOrder}
}

// CashRegisterToAPI maps a register with its open session.
func CashRegisterToAPI(v *cash.RegisterView) api.CashRegister {
	return api.CashRegister{
		Id: v.ID, Code: v.Code, Name: v.Name, Active: v.Active, IsDefault: v.IsDefault, SortOrder: v.SortOrder,
		OpenSessionId: v.OpenSessionID, CreatedAt: v.CreatedAt.UTC(), UpdatedAt: v.UpdatedAt.UTC(), Version: max(v.Version, 1),
	}
}
