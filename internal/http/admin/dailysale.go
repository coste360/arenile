package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"

	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Daily sale handlers ("Vendita giornaliera", spec 14.3, ADR 0020). Owner:
// internal/domain/dailysale.

// QuoteDailySale implements POST .../daily-sales/quote.
func (h *Handlers) QuoteDailySale(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.QuoteDailySaleParams) {
	var req api.DailySaleQuoteRequest
	if !decode(w, r, &req) {
		return
	}
	in := dailysale.SaleInput{
		Target: dailysale.Target{SpotID: paymentDeref(req.SpotId), ZoneID: paymentDeref(req.ZoneId)},
		Date:   req.Date, Slots: calendar.Slots(req.Slots), PackID: req.PackId, Extras: saleExtras(req.Extras),
	}
	q, err := h.DailySale.QuoteDaily(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.DailySaleQuote{
		Breakdown: BreakdownToAPI(q.Result.Breakdown), Total: int64(q.Result.Breakdown.Total), Available: q.Available,
		ZoneFree: q.ZoneFree, Conflicts: bkConflicts(q.Conflicts),
	})
}

// CreateDailySale implements POST .../daily-sales.
func (h *Handlers) CreateDailySale(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateDailySaleParams) {
	var req api.DailySaleCreate
	if !decode(w, r, &req) {
		return
	}
	in := dailysale.SaleInput{
		Target: dailysale.Target{SpotID: paymentDeref(req.SpotId), ZoneID: paymentDeref(req.ZoneId)},
		Date:   req.Date, Slots: calendar.Slots(req.Slots), PackID: req.PackId, Extras: saleExtras(req.Extras),
		Payment:       salePayment(req.Payment),
		ExpectedTotal: centsPtr(req.ExpectedTotal), NotesInternal: paymentDeref(req.NotesInternal),
	}
	if req.Holder != nil {
		in.FirstName, in.LastName = paymentDeref(req.Holder.FirstName), paymentDeref(req.Holder.LastName)
	}
	res, err := h.DailySale.CreateDaily(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	h.writeSaleResult(w, r, establishmentID, res)
}

// writeSaleResult answers 201 with the ticket of a completed sale.
func (h *Handlers) writeSaleResult(w http.ResponseWriter, r *http.Request, establishmentID string, res *dailysale.Result) {
	v, err := h.Tickets.Get(r.Context(), principalOf(r), establishmentID, res.Ticket.ID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.DailySaleResult{Ticket: TicketToAPI(v), BookingId: res.BookingID, PaymentId: res.PaymentID}
	if res.Change != nil {
		c := int64(*res.Change)
		out.ChangeAmount = &c
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetDailySaleSettings implements GET .../daily-sale-settings.
func (h *Handlers) GetDailySaleSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	st, err := h.DailySale.Settings(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, st.Version, DailySaleSettingsToAPI(st))
}

// UpdateDailySaleSettings implements PUT .../daily-sale-settings.
func (h *Handlers) UpdateDailySaleSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateDailySaleSettingsParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.DailySaleSettingsUpdate
	if !decode(w, r, &req) {
		return
	}
	st, err := h.DailySale.UpdateSettings(r.Context(), principalOf(r), establishmentID, dailysale.SettingsInput{
		PrintFormat: string(req.PrintFormat), AutoPrint: req.AutoPrint, ZoneFirstEnabled: req.ZoneFirstEnabled,
		FooterText: store.I18nText(req.FooterText), ExpectedVersion: expected,
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, st.Version, DailySaleSettingsToAPI(st))
}

func saleExtras(xs *[]api.SimulateExtra) []booking.ExtraInput {
	if xs == nil {
		return nil
	}
	out := make([]booking.ExtraInput, 0, len(*xs))
	for _, x := range *xs {
		out = append(out, booking.ExtraInput{ExtraID: x.ExtraId, Quantity: x.Quantity})
	}
	return out
}

func salePayment(p api.DailySalePayment) dailysale.SalePayment {
	return dailysale.SalePayment{Method: string(p.Method), CashSessionID: p.CashSessionId, Tendered: centsPtr(p.TenderedAmount)}
}

func centsPtr(v *int64) *money.Cents {
	if v == nil {
		return nil
	}
	c := money.Cents(*v)
	return &c
}
