package ticketing

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Page sizes of the daily list.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// ListInput filters the tickets of a day.
type ListInput struct {
	Date   calendar.Date
	State  State  // empty: every state
	Kind   string // empty: both kinds
	ZoneID string
	// Cursor is the next_cursor of the previous page (the last number).
	Cursor string
	// Limit bounds the page (default DefaultPageSize); a negative limit
	// returns the whole day (CSV export).
	Limit int
}

// MethodTotal is the count and amount of the valid tickets of one method.
type MethodTotal struct {
	Method string
	Count  int
	Amount money.Cents
}

// Summary is the day summary of the tickets (spec 14.3): voided tickets
// are counted but never summed (their refund offsets the takings).
type Summary struct {
	Issued     int
	Voided     int
	Unassigned int
	Entered    int
	Total      money.Cents
	ByMethod   []MethodTotal
	VAT        []pricing.VATBreakdown
}

// List is a page of the tickets of a day with the summary of the whole day.
type List struct {
	Items      []View
	Summary    Summary
	NextCursor string
}

// List returns the tickets of a day by number (auth.PermBookingsRead).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, in ListInput) (*List, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if in.Date.IsZero() {
		return nil, configerr.Invalid("date", "Campo obbligatorio.")
	}
	after := int64(0)
	if in.Cursor != "" {
		n, err := strconv.ParseInt(in.Cursor, 10, 64)
		if err != nil || n < 0 {
			return nil, configerr.Invalid("cursor", "Cursore non valido.")
		}
		after = n
	}
	limit := in.Limit
	switch {
	case limit == 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}
	all, err := store.ListDayTickets(ctx, s.d.DB, es, in.Date, store.TicketFilter{})
	if err != nil {
		return nil, err
	}
	vs, err := views(ctx, s.d.DB, es, all)
	if err != nil {
		return nil, err
	}
	out := &List{Summary: Summarize(vs), Items: []View{}}
	for _, v := range vs {
		t := &v.Ticket
		if t.TicketNumber <= after || (in.State != "" && v.State != in.State) ||
			(in.Kind != "" && t.Kind != in.Kind) || (in.ZoneID != "" && t.ZoneID != in.ZoneID) {
			continue
		}
		if limit > 0 && len(out.Items) == limit {
			out.NextCursor = strconv.FormatInt(out.Items[len(out.Items)-1].Ticket.TicketNumber, 10)
			break
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// Summarize computes the day summary of views.
func Summarize(vs []View) Summary {
	sum := Summary{ByMethod: []MethodTotal{}, VAT: []pricing.VATBreakdown{}}
	methods := map[string]*MethodTotal{}
	vat := map[string]*pricing.VATBreakdown{}
	for i := range vs {
		t := &vs[i].Ticket
		if t.Status == store.TicketVoided {
			sum.Voided++
			continue
		}
		sum.Issued++
		if t.NeedsAssignment {
			sum.Unassigned++
		}
		if t.EnteredAt != nil {
			sum.Entered++
		}
		sum.Total += t.TotalAmount
		m := methods[t.PaymentMethod]
		if m == nil {
			m = &MethodTotal{Method: t.PaymentMethod}
			methods[t.PaymentMethod] = m
		}
		m.Count++
		m.Amount += t.TotalAmount
		for _, x := range vs[i].VAT {
			k := x.Rate.String()
			acc := vat[k]
			if acc == nil {
				acc = &pricing.VATBreakdown{Rate: x.Rate}
				vat[k] = acc
			}
			acc.Taxable += x.Taxable
			acc.Tax += x.Tax
		}
	}
	for _, m := range methods {
		sum.ByMethod = append(sum.ByMethod, *m)
	}
	sort.Slice(sum.ByMethod, func(a, b int) bool { return sum.ByMethod[a].Method < sum.ByMethod[b].Method })
	for _, x := range vat {
		sum.VAT = append(sum.VAT, *x)
	}
	sort.Slice(sum.VAT, func(a, b int) bool { return sum.VAT[a].Rate.Cmp(sum.VAT[b].Rate) > 0 })
	return sum
}

// Get returns one ticket (auth.PermBookingsRead).
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*View, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	t, err := store.GetTicket(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return view(ctx, s.d.DB, es, t)
}

// GetByCode returns the ticket of a scanned QR (gate token) or of a booking
// code printed under it, for the staff checks from the map
// (auth.PermBookingsRead). Reading is not admitting: the gate (CheckIn)
// accepts the booking code only as a confirmed manual entry.
func (s *Service) GetByCode(ctx context.Context, p *auth.Principal, establishmentID, code string) (*View, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	code = NormalizeCode(code)
	t, err := store.GetTicketByGateToken(ctx, s.d.DB, es, code)
	if errors.Is(err, store.ErrNotFound) {
		t, err = store.GetTicketByBookingCode(ctx, s.d.DB, es, code)
	}
	if err != nil {
		return nil, notFound(err)
	}
	return view(ctx, s.d.DB, es, t)
}

// NormalizeCode extracts the gate token or booking code from a scanned QR
// payload: the payload is the code itself, possibly with spaces or a URL
// around it (the last path segment).
func NormalizeCode(raw string) string {
	c := strings.TrimSpace(raw)
	if i := strings.LastIndex(c, "/"); i >= 0 {
		c = c[i+1:]
	}
	return strings.ToUpper(strings.TrimSpace(c))
}
