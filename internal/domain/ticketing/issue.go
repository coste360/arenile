package ticketing

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// IssueInput is the ticket of a completed sale (daily sale or entry sale).
type IssueInput struct {
	BookingID         string
	Date              calendar.Date
	Kind              string // store.TicketKindSale or store.TicketKindEntry
	ZoneID            string
	NeedsAssignment   bool
	ProvisionalSpotID *string
	HolderName        string
	TotalAmount       money.Cents
	PaymentMethod     string // store.TicketPaymentCash or store.TicketPaymentCard
	PaymentID         *string
	CashSessionID     *string
}

// Issue writes the numbered ticket of a sale in tx (the sale transaction):
// the next number of the day comes from store.NextTicketNumber, one atomic
// upsert on daily_ticket_counters, so concurrent sales on any register get
// distinct numbers and a rolled back sale gives its number back. Call it
// last in the sale transaction: on PostgreSQL the counter row stays locked
// until commit. UNIQUE (establishment_id, ticket_date, ticket_number) is
// the final guard (ErrNumberConflict, never expected). The caller has
// authorized the sale.
func (s *Service) Issue(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in IssueInput) (*store.Ticket, error) {
	switch {
	case in.BookingID == "" || in.ZoneID == "" || in.Date.IsZero():
		return nil, fmt.Errorf("ticketing: issue: booking, zone and date are required")
	case in.Kind != store.TicketKindSale && in.Kind != store.TicketKindEntry:
		return nil, fmt.Errorf("ticketing: issue: unknown kind %q", in.Kind)
	case in.PaymentMethod != store.TicketPaymentCash && in.PaymentMethod != store.TicketPaymentCard:
		return nil, fmt.Errorf("ticketing: issue: unknown payment method %q", in.PaymentMethod)
	case in.Kind == store.TicketKindEntry && in.NeedsAssignment:
		return nil, fmt.Errorf("ticketing: issue: entry tickets are never assigned")
	}
	now := s.now()
	token, err := NewGateToken()
	if err != nil {
		return nil, err
	}
	n, err := store.NextTicketNumber(ctx, tx, es, in.Date, now)
	if err != nil {
		return nil, err
	}
	t := &store.Ticket{
		ID: store.NewID(), EstablishmentID: es.ID(), BookingID: in.BookingID, TicketDate: in.Date, TicketNumber: n,
		Kind: in.Kind, Status: store.TicketValid, ZoneID: in.ZoneID, NeedsAssignment: in.NeedsAssignment,
		ProvisionalSpotID: in.ProvisionalSpotID, HolderName: strings.TrimSpace(in.HolderName), TotalAmount: in.TotalAmount,
		PaymentMethod: in.PaymentMethod, PaymentID: in.PaymentID, CashSessionID: in.CashSessionID,
		GateToken: token, CreatedByUserID: userIDOf(p), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertTicket(ctx, tx, es, t); err != nil {
		if store.IsUniqueViolation(err) {
			return nil, ErrNumberConflict
		}
		return nil, err
	}
	diff := map[string]any{
		"kind": t.Kind, "zone_id": t.ZoneID, "needs_assignment": t.NeedsAssignment,
		"total_amount": int64(t.TotalAmount), "payment_method": t.PaymentMethod,
	}
	if t.ProvisionalSpotID != nil {
		diff["provisional_spot_id"] = *t.ProvisionalSpotID
	}
	if err := s.record(ctx, tx, p, es, "ticket.issued", t, diff); err != nil {
		return nil, err
	}
	return t, nil
}
