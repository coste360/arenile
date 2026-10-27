package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Report queries of M8 (spec 30.2.4-30.2.5, ADR 0028): partner statement,
// report per partner and agreements register. Basis: the booked statuses
// of the reports (ReportBookedStatuses), test bookings excluded, bookings
// whose stay overlaps the period.

// M8BookingFilter selects the bookings of the M8 reports.
type M8BookingFilter struct {
	From, To calendar.Date
	// PartnerIDs keeps the bookings of these partners; WithPartner keeps
	// every booking with a partner.
	PartnerIDs  []string
	WithPartner bool
	// AgreementIDs keeps the bookings of these agreements; WithAgreement
	// keeps every booking with an agreement.
	AgreementIDs  []string
	WithAgreement bool
}

// M8ReportBooking is one booking of the M8 reports.
type M8ReportBooking struct {
	ID            string        `bun:"id"`
	Code          string        `bun:"code"`
	Status        string        `bun:"status"`
	CustomerID    string        `bun:"customer_id"`
	FirstName     string        `bun:"first_name"`
	LastName      string        `bun:"last_name"`
	PartnerID     *string       `bun:"partner_id"`
	AgreementID   *string       `bun:"agreement_id"`
	GroupName     string        `bun:"group_name"`
	StartsOn      calendar.Date `bun:"starts_on"`
	EndsOn        calendar.Date `bun:"ends_on"`
	TotalAmount   money.Cents   `bun:"total_amount"`
	PartnerAmount money.Cents   `bun:"partner_amount"`
	PartyAdults   *int          `bun:"party_adults"`
	PartyChildren *int          `bun:"party_children"`
	PriceSnapshot JSON          `bun:"price_snapshot"`
	PriceCurrent  JSON          `bun:"price_current"`
	// Units is the number of spots or units of the booking (sum of the
	// line quantities).
	Units int `bun:"units"`
}

// Price is the breakdown pricing the booking today.
func (b *M8ReportBooking) Price() JSON {
	if len(b.PriceCurrent) > 0 {
		return b.PriceCurrent
	}
	return b.PriceSnapshot
}

// ReportM8Bookings lists the bookings of f, ordered by first day and code.
func ReportM8Bookings(ctx context.Context, db bun.IDB, es EstablishmentScope, f M8BookingFilter) ([]M8ReportBooking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []M8ReportBooking{}
	q := db.NewSelect().TableExpr("bookings AS bk").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id").
		ColumnExpr("bk.id, bk.code, bk.status, bk.customer_id, cu.first_name, cu.last_name, bk.partner_id, bk.agreement_id, "+
			"bk.group_name, bk.starts_on, bk.ends_on, bk.total_amount, bk.partner_amount, bk.party_adults, bk.party_children, "+
			"bk.price_snapshot, bk.price_current").
		ColumnExpr("(SELECT COALESCE(SUM(bl.quantity), 0) FROM booking_lines AS bl WHERE bl.booking_id = bk.id) AS units").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false).
		Where("bk.status IN (?)", bun.List(ReportBookedStatuses)).
		Where("bk.starts_on <= ?", f.To).Where("bk.ends_on >= ?", f.From)
	switch {
	case len(f.PartnerIDs) > 0:
		q = q.Where("bk.partner_id IN (?)", bun.List(dedupe(f.PartnerIDs)))
	case f.WithPartner:
		q = q.Where("bk.partner_id IS NOT NULL")
	}
	switch {
	case len(f.AgreementIDs) > 0:
		q = q.Where("bk.agreement_id IN (?)", bun.List(dedupe(f.AgreementIDs)))
	case f.WithAgreement:
		q = q.Where("bk.agreement_id IS NOT NULL")
	}
	if err := q.OrderExpr("bk.starts_on ASC, bk.code ASC").Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report m8 bookings: %w", err)
	}
	return out, nil
}

// PartnerAccountBefore returns what was charged to a partner for the
// bookings ended before day (booked statuses, test excluded) and what it
// paid before day (non voided payments): the opening balance of a
// statement starting on day.
func PartnerAccountBefore(ctx context.Context, db bun.IDB, es EstablishmentScope, partnerID string, day calendar.Date) (PartnerBalance, error) {
	var out PartnerBalance
	if err := es.mustBeValid(); err != nil {
		return out, err
	}
	err := db.NewSelect().TableExpr("bookings AS bk").ColumnExpr("COALESCE(SUM(bk.partner_amount), 0)").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.partner_id = ?", partnerID).
		Where("bk.test = ?", false).Where("bk.status IN (?)", bun.List(ReportBookedStatuses)).
		Where("bk.ends_on < ?", day).Scan(ctx, &out.Charged)
	if err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: partner account before: %w", err)
	}
	err = db.NewSelect().TableExpr("partner_payments AS pp").ColumnExpr("COALESCE(SUM(pp.amount), 0)").
		Where("pp.establishment_id = ?", es.ID()).Where("pp.partner_id = ?", partnerID).
		Where("pp.voided_at IS NULL").Where("pp.received_on < ?", day).Scan(ctx, &out.Paid)
	if err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: partner account before: %w", err)
	}
	return out, nil
}

// ListAllAgreements lists every agreement of es (register).
func ListAllAgreements(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Agreement, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Agreement
	if err := db.NewSelect().Model(&rows).Where("ag.establishment_id = ?", es.ID()).
		OrderExpr("ag.name ASC, ag.id ASC").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list all agreements: %w", err)
	}
	return rows, nil
}

// ListAllPartners lists every partner of es (reports).
func ListAllPartners(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Partner, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Partner
	if err := db.NewSelect().Model(&rows).Where("pa.establishment_id = ?", es.ID()).
		OrderExpr("pa.name ASC, pa.id ASC").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list all partners: %w", err)
	}
	return rows, nil
}
