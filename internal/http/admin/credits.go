package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// M5B customer credit ledger (spec 13.4): owned by the builder of
// internal/domain/credit (Handlers.Credits).

// GetCustomerCredits implements GET /customers/{customerId}/credits.
func (h *Handlers) GetCustomerCredits(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, customerID api.CustomerID, params api.GetCustomerCreditsParams) {
	if h.Credits == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	l, err := h.Credits.Ledger(r.Context(), principalOf(r), establishmentID, customerID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.CustomerCreditLedger{
		CustomerId: l.CustomerID, Balance: int64(l.Balance), Available: int64(l.Available),
		Expiring: CreditExpiringToAPI(l.Expiring), Items: make([]api.CustomerCredit, 0, len(l.Items.Items)),
		NextCursor: nextCursor(l.Items.NextCursor),
	}
	for i := range l.Items.Items {
		out.Items = append(out.Items, CreditToAPI(&l.Items.Items[i]))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// CreateCustomerCredit implements POST /customers/{customerId}/credits.
func (h *Handlers) CreateCustomerCredit(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, customerID api.CustomerID, _ api.CreateCustomerCreditParams) {
	if h.Credits == nil {
		notImplemented(w, r)
		return
	}
	var req api.CustomerCreditCreate
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Credits.Grant(r.Context(), principalOf(r), establishmentID, customerID, credit.GrantInput{
		Amount: money.Cents(req.Amount), Source: string(req.Source), ExpiresOn: req.ExpiresOn,
		BookingID: customerStr(req.BookingId), Note: customerStr(req.Note), IP: customerClientIP(r),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeJSON(w, http.StatusCreated, CreditToAPI(c))
}

// CreditToAPI maps a ledger movement (also used by the public handlers).
func CreditToAPI(c *store.CustomerCredit) api.CustomerCredit {
	return customerCreditToAPI(c)
}

// CreditExpiringToAPI maps the expiring credit by date.
func CreditExpiringToAPI(xs []credit.Expiring) []api.CustomerCreditExpiring {
	out := make([]api.CustomerCreditExpiring, 0, len(xs))
	for _, x := range xs {
		out = append(out, api.CustomerCreditExpiring{ExpiresOn: x.ExpiresOn, Amount: int64(x.Amount)})
	}
	return out
}
