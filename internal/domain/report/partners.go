package report

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// M8 reports (spec 30.2.4-30.2.5, ADR 0028): partner statement, report per
// partner and the agreements register. Stay basis like the occupancy
// report; test bookings excluded; discount = list price - applied price,
// read from price_current (or price_snapshot) so the register stays
// coherent with the cash reports. Exports: internal/xlsx and an HTML
// printout (internal/printout, ADR 0019: PDF via the browser print).
// Owner: the partner account and reports builder.

// PartnerTotals are the counters of one partner (or of all of them).
type PartnerTotals struct {
	Bookings int
	People   int
	SpotDays int
	Revenue  money.Cents
	Discount money.Cents
	Charged  money.Cents
	Paid     money.Cents
}

// PartnerRow is one partner of the report.
type PartnerRow struct {
	PartnerID   string
	PartnerName string
	Kind        string
	PartnerTotals
}

// PartnersReport is api.PartnersReport.
type PartnersReport struct {
	Period Period
	Rows   []PartnerRow
	Totals PartnerTotals
}

// StatementBooking is one booking of a partner statement.
type StatementBooking struct {
	BookingID           string
	Code                string
	Status              string
	CustomerDisplayName string
	GroupName           string
	AgreementName       string
	StartsOn, EndsOn    calendar.Date
	SpotDays            int
	People              int
	Total               money.Cents
	Discount            money.Cents
	PartnerAmount       money.Cents
}

// Statement is api.PartnerStatement (without the partner and payments,
// added by the handler from internal/domain/partner). INV-104:
// ClosingBalance = OpeningBalance + Charged - Paid.
type Statement struct {
	Period         Period
	Bookings       []StatementBooking
	Charged        money.Cents
	Paid           money.Cents
	OpeningBalance money.Cents
	ClosingBalance money.Cents
}

// Partners returns the report per partner (partnerID empty = all).
// Requires auth.PermReportsRead.
func (s *Service) Partners(ctx context.Context, p *auth.Principal, establishmentID string, period Period, partnerID string) (*PartnersReport, error) {
	_, _, _, _, _ = ctx, p, establishmentID, period, partnerID
	return nil, configerr.ErrNotImplemented
}

// PartnerStatement returns the statement of a partner for the period.
// Requires auth.PermReportsRead.
func (s *Service) PartnerStatement(ctx context.Context, p *auth.Principal, establishmentID, partnerID string, period Period) (*Statement, error) {
	_, _, _, _, _ = ctx, p, establishmentID, partnerID, period
	return nil, configerr.ErrNotImplemented
}
