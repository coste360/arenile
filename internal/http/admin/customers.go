package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the customer domain (internal/domain/customer): decode, call
// the service (which checks permissions), encode. Responses carrying
// personal data are marked Cache-Control: no-store.

// ListCustomers implements GET /api/admin/v1/establishments/{establishmentId}/customers.
func (h *Handlers) ListCustomers(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListCustomersParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := customer.ListFilter{Q: customerStr(params.Q), Tag: customerStr(params.Tag), CategoryID: customerStr(params.CategoryId)}
	if err := uuidParams(map[string]*string{"category_id": params.CategoryId}); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if params.IncludeAnonymized != nil {
		f.IncludeAnonymized = *params.IncludeAnonymized
	}
	page, err := h.Customers.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.CustomerList{Items: make([]api.Customer, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, CustomerToAPI(&page.Items[i]))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// CreateCustomer implements POST /api/admin/v1/establishments/{establishmentId}/customers.
// A customer with the same email or phone answers 409 conflict with
// existing_customer_id; the query parameter force=true creates it anyway.
func (h *Handlers) CreateCustomer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreateCustomerParams) {
	var req api.CustomerInput
	if !decode(w, r, &req) {
		return
	}
	force := params.Force != nil && *params.Force
	c, err := h.Customers.Create(r.Context(), principalOf(r), customerClientIP(r), establishmentID, customerInputFromAPI(&req), force)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusCreated, c.Version, CustomerToAPI(c))
}

// GetCustomer implements GET /api/admin/v1/establishments/{establishmentId}/customers/{customerId}.
func (h *Handlers) GetCustomer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID) {
	d, err := h.Customers.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	name := store.CustomerDisplayName(&d.Customer)
	out := api.CustomerDetail{
		Customer:       CustomerToAPI(&d.Customer),
		CreditBalance:  int64(d.CreditBalance),
		Credits:        make([]api.CustomerCredit, 0, len(d.Credits)),
		RecentBookings: make([]api.BookingSummary, 0, len(d.RecentBookings)),
		SeasonPasses:   make([]api.SeasonPass, 0, len(d.SeasonPasses)),
		Stats: api.CustomerStats{
			BookingsCount: d.Stats.BookingsCount, TotalSpent: int64(d.Stats.TotalSpent),
			BalanceDue: int64(d.Stats.BalanceDue), LastBookingOn: d.Stats.LastBookingOn,
		},
	}
	for i := range d.Credits {
		out.Credits = append(out.Credits, customerCreditToAPI(&d.Credits[i]))
	}
	for i := range d.RecentBookings {
		b := &d.RecentBookings[i]
		out.RecentBookings = append(out.RecentBookings, customerBookingSummary(b, name, d.SpotLabels[b.ID]))
	}
	for i := range d.SeasonPasses {
		out.SeasonPasses = append(out.SeasonPasses, SeasonPassToAPI(&d.SeasonPasses[i]))
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, d.Customer.Version, out)
}

// UpdateCustomer implements PUT /api/admin/v1/establishments/{establishmentId}/customers/{customerId}.
func (h *Handlers) UpdateCustomer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID, params api.UpdateCustomerParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.CustomerInput
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Customers.Update(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch, customerInputFromAPI(&req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, c.Version, CustomerToAPI(c))
}

// ExportCustomer implements GET /api/admin/v1/establishments/{establishmentId}/customers/{customerId}/export.
func (h *Handlers) ExportCustomer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID) {
	x, err := h.Customers.Export(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	name := store.CustomerDisplayName(&x.Customer)
	out := api.CustomerExport{
		Customer:   CustomerToAPI(&x.Customer),
		ExportedAt: x.ExportedAt,
		Bookings:   make([]api.BookingSummary, 0, len(x.Bookings)),
		Payments:   make([]api.Payment, 0, len(x.Payments)),
		Credits:    make([]api.CustomerCredit, 0, len(x.Credits)),
	}
	for i := range x.Bookings {
		b := &x.Bookings[i]
		out.Bookings = append(out.Bookings, customerBookingSummary(b, name, x.SpotLabels[b.ID]))
	}
	for i := range x.Payments {
		out.Payments = append(out.Payments, customerPaymentToAPI(&x.Payments[i], x.BookingCodes))
	}
	for i := range x.Credits {
		out.Credits = append(out.Credits, customerCreditToAPI(&x.Credits[i]))
	}
	customerExportM8(&out, x)
	customerNoStore(w)
	w.Header().Set("Content-Disposition", `attachment; filename="cliente-`+x.Customer.ID+`.json"`)
	writeJSON(w, http.StatusOK, out)
}

// AnonymizeCustomer implements POST /api/admin/v1/establishments/{establishmentId}/customers/{customerId}/anonymize.
func (h *Handlers) AnonymizeCustomer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.CustomerID, params api.AnonymizeCustomerParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	c, err := h.Customers.Anonymize(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, CustomerToAPI(c))
}

// ---- mapping ----------------------------------------------------------------

// CustomerToAPI maps a customer to its API representation.
func CustomerToAPI(c *store.Customer) api.Customer {
	out := api.Customer{
		Id: c.ID, FirstName: c.FirstName, LastName: c.LastName, DisplayName: store.CustomerDisplayName(c),
		Email: c.Email, Phone: c.Phone, Locale: c.Locale, Notes: c.Notes, Tags: c.Tags, AgreementId: c.AgreementID,
		AnonymizedAt: c.AnonymizedAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: c.Version,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if c.CategoryIDs != nil {
		ids := append([]string{}, c.CategoryIDs...)
		out.CategoryIds = &ids
	}
	if mc := c.MarketingConsent; mc != nil {
		out.MarketingConsent = &api.MarketingConsent{GivenAt: mc.GivenAt, Source: mc.Source, TextVersion: mc.TextVersion}
	}
	return out
}

func customerInputFromAPI(in *api.CustomerInput) customer.Input {
	out := customer.Input{
		FirstName: customerStr(in.FirstName), LastName: in.LastName, Phone: customerStr(in.Phone),
		Notes: customerStr(in.Notes), AgreementID: customerStr(in.AgreementId),
	}
	if in.Email != nil {
		out.Email = string(*in.Email)
	}
	if in.Locale != nil {
		out.Locale = string(*in.Locale)
	}
	if in.Tags != nil {
		out.Tags = *in.Tags
	}
	if in.CategoryIds != nil {
		ids := append([]string{}, *in.CategoryIds...)
		out.CategoryIDs = &ids
	}
	if mc := in.MarketingConsent; mc != nil {
		out.MarketingConsent = &store.MarketingConsent{GivenAt: mc.GivenAt, Source: mc.Source, TextVersion: mc.TextVersion}
	}
	return out
}

func customerCreditToAPI(c *store.CustomerCredit) api.CustomerCredit {
	out := api.CustomerCredit{
		Id: c.ID, Amount: int64(c.Amount), Source: api.CustomerCreditSource(c.Source),
		BookingId: c.BookingID, ExpiresOn: c.ExpiresOn, CreatedAt: c.CreatedAt, ReferenceCreditId: c.ReferenceCreditID,
	}
	if c.Note != "" {
		note := c.Note
		out.Note = &note
	}
	return out
}

// customerBookingSummary maps a booking of the customer page or export.
func customerBookingSummary(b *store.Booking, customerName string, labels []string) api.BookingSummary {
	if labels == nil {
		labels = []string{}
	}
	return api.BookingSummary{
		Id: b.ID, Code: b.Code, Status: api.BookingStatus(b.Status), Channel: api.Channel(b.Channel),
		CustomerId: b.CustomerID, CustomerDisplayName: customerName, StartsOn: b.StartsOn, EndsOn: b.EndsOn,
		Slots: api.Slots(b.Slots), Formula: api.Formula(b.Formula), IsSeasonPass: b.IsSeasonPass,
		TotalAmount: int64(b.TotalAmount), PaidAmount: int64(b.PaidAmount), BalanceDue: int64(b.BalanceDue()),
		DepositDue: int64(b.DepositDue), HoldExpiresAt: b.HoldExpiresAt, SpotLabels: labels,
		CreatedAt: b.CreatedAt, Version: b.Version,
	}
}

// customerPaymentToAPI maps a payment of the GDPR export.
func customerPaymentToAPI(p *store.Payment, codes map[string]string) api.Payment {
	out := api.Payment{
		Id: p.ID, Amount: int64(p.Amount), Direction: api.PaymentDirection(p.Direction), Method: api.PaymentMethod(p.Method),
		Status: api.PaymentStatus(p.Status), Provider: p.Provider, ProviderRef: p.ProviderRef, BookingId: p.BookingID,
		CashSessionId: p.CashSessionID, RefundOfPaymentId: p.RefundOfPaymentID, ReceivedByUserId: p.ReceivedByUserID,
		CreatedAt: p.CreatedAt,
	}
	if p.BookingID != nil {
		if code, ok := codes[*p.BookingID]; ok {
			out.BookingCode = &code
		}
	}
	if p.Description != "" {
		s := p.Description
		out.Description = &s
	}
	if p.Note != "" {
		s := p.Note
		out.Note = &s
	}
	return out
}

// customerClientIP is the caller's IP for the audit log.
func customerClientIP(r *http.Request) string { return reqctx.From(r.Context()).ClientIP }

// customerNoStore keeps responses with personal data out of shared caches.
func customerNoStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func customerStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
