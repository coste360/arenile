package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the cash desk (internal/domain/cash, spec 14.1): sessions with
// float, movements, closing with counted totals, history. Permissions are
// checked by the service.

// ListCashSessions implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions.
func (h *Handlers) ListCashSessions(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListCashSessionsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := cash.ListFilter{
		Register: paymentDeref(params.Register), RegisterID: paymentDeref(params.RegisterId), From: params.From, To: params.To,
	}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Cash.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.CashSessionList{Items: make([]api.CashSession, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, CashSessionToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// OpenCashSession implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions.
func (h *Handlers) OpenCashSession(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.OpenCashSessionParams) {
	var req api.CashSessionOpen
	if !decode(w, r, &req) {
		return
	}
	in := cash.OpenInput{
		RegisterID: paymentDeref(req.RegisterId), Register: paymentDeref(req.Register), OpeningFloat: money.Cents(req.OpeningFloat), Notes: paymentDeref(req.Notes),
	}
	cs, err := h.Cash.Open(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, cs.Version, CashSessionToAPI(cs))
}

// GetCurrentCashSession implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions/current.
func (h *Handlers) GetCurrentCashSession(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetCurrentCashSessionParams) {
	var (
		d   *cash.Detail
		err error
	)
	if id := paymentDeref(params.RegisterId); id != "" {
		d, err = h.Cash.CurrentOfRegister(r.Context(), principalOf(r), establishmentID, id)
	} else {
		d, err = h.Cash.Current(r.Context(), principalOf(r), establishmentID, paymentDeref(params.Register))
	}
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Session.Version, CashSessionDetailToAPI(d))
}

// GetCashSession implements GET /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}.
func (h *Handlers) GetCashSession(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashSessionID) {
	d, err := h.Cash.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Session.Version, CashSessionDetailToAPI(d))
}

// AddCashMovement implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}/movements.
func (h *Handlers) AddCashMovement(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashSessionID, _ api.AddCashMovementParams) {
	var req api.CashMovementInput
	if !decode(w, r, &req) {
		return
	}
	in := cash.MovementInput{Kind: string(req.Kind), Amount: money.Cents(req.Amount), Reason: req.Reason}
	m, err := h.Cash.AddMovement(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, CashMovementToAPI(m))
}

// CloseCashSession implements POST /api/admin/v1/establishments/{establishmentId}/cash-sessions/{cashSessionId}/close.
func (h *Handlers) CloseCashSession(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CashSessionID, params api.CloseCashSessionParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.CashSessionClose
	if !decode(w, r, &req) {
		return
	}
	counted := store.MethodTotals{}
	for k, v := range req.CountedTotals {
		counted[k] = money.Cents(v)
	}
	d, err := h.Cash.Close(r.Context(), principalOf(r), paymentClientIP(r), establishmentID, id, ifMatch,
		cash.CloseInput{Counted: counted, Notes: paymentDeref(req.Notes)})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Session.Version, CashSessionToAPI(&d.Session))
}

// CashSessionToAPI maps a session.
func CashSessionToAPI(cs *store.CashSession) api.CashSession {
	out := api.CashSession{
		Id: cs.ID, Register: cs.Register, OpenedAt: cs.OpenedAt.UTC(), ClosedAt: utcPtr(cs.ClosedAt),
		ClosedBy: cs.ClosedBy, OpeningFloat: int64(cs.OpeningFloat), ExpectedTotals: methodTotalsToAPI(cs.ExpectedTotals),
		Notes: cs.Notes, Status: api.CashSessionStatus(cash.StatusOf(cs)), Version: max(cs.Version, 1),
		RegisterId: cs.RegisterID,
	}
	if cs.RegisterName != "" {
		name := cs.RegisterName
		out.RegisterName = &name
	}
	if cs.OpenedBy != nil {
		out.OpenedBy = *cs.OpenedBy
	}
	if cs.CountedTotals != nil {
		ct := methodTotalsToAPI(cs.CountedTotals)
		out.CountedTotals = &ct
	}
	if cs.Difference != nil {
		d := int64(*cs.Difference)
		out.Difference = &d
	}
	return out
}

// CashSessionDetailToAPI maps a session with movements, payments and the
// takings by method.
func CashSessionDetailToAPI(d *cash.Detail) api.CashSessionDetail {
	out := api.CashSessionDetail{
		Session:        CashSessionToAPI(&d.Session),
		Movements:      make([]api.CashMovement, 0, len(d.Movements)),
		Payments:       make([]api.Payment, 0, len(d.Payments)),
		TotalsByMethod: methodTotalsToAPI(d.Summary.Takings),
	}
	for i := range d.Movements {
		out.Movements = append(out.Movements, CashMovementToAPI(&d.Movements[i]))
	}
	for i := range d.Payments {
		out.Payments = append(out.Payments, PaymentToAPI(&d.Payments[i], nil))
	}
	return out
}

// CashMovementToAPI maps a movement.
func CashMovementToAPI(m *store.CashMovement) api.CashMovement {
	return api.CashMovement{
		Id: m.ID, CashSessionId: m.CashSessionID, Kind: api.CashMovementKind(m.Kind), Amount: int64(m.Amount),
		Reason: m.Reason, CreatedBy: m.CreatedByUserID, CreatedAt: m.CreatedAt.UTC(),
	}
}

func methodTotalsToAPI(t store.MethodTotals) api.MethodTotals {
	out := api.MethodTotals{}
	for k, v := range t {
		out[k] = int64(v)
	}
	return out
}
