package report

import (
	"context"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Deps are the dependencies of the report service. Every field is required
// except Log.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Log   *slog.Logger
}

// Service implements the report use cases. Every method authorises the
// principal with auth.PermReportsRead on establishmentID first, then runs
// read-only queries scoped by the returned store.EstablishmentScope.
//
// Handler mapping (internal/http/admin/reports.go):
//
//	GetReportSummary        -> Summary        -> api.ReportSummary
//	GetOccupancyReport      -> Occupancy      -> api.OccupancyReport
//	GetRevenueReport        -> Revenue        -> api.RevenueReport
//	GetBookingsReport       -> Bookings       -> api.BookingsReport
//	GetCancellationsReport  -> Cancellations  -> api.CancellationsReport
//	GetSeasonPassesReport   -> SeasonPasses   -> api.SeasonPassesReport
//	GetTopCustomersReport   -> TopCustomers   -> api.TopCustomersReport
//	GetOpenBalancesReport   -> OpenBalances   -> api.OpenBalancesReport
//	GetYearComparisonReport -> Comparison     -> api.YearComparisonReport
//
// Each result has a Write<Name>CSV(io.Writer, *<Name>) companion (to be
// added next to the query) used when format=csv or Accept: text/csv.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// Summary returns the KPI set of the period.
func (s *Service) Summary(ctx context.Context, p *auth.Principal, establishmentID string, period Period) (*Summary, error) {
	return nil, configerr.ErrNotImplemented
}

// Occupancy returns the occupancy report.
func (s *Service) Occupancy(ctx context.Context, p *auth.Principal, establishmentID string, q OccupancyQuery) (*Occupancy, error) {
	return nil, configerr.ErrNotImplemented
}

// Revenue returns the revenue report. payment_method and vat_rate require
// BasisCollected; pack and extra require BasisBooked (validation error).
func (s *Service) Revenue(ctx context.Context, p *auth.Principal, establishmentID string, q RevenueQuery) (*Revenue, error) {
	return nil, configerr.ErrNotImplemented
}

// Bookings returns the bookings report.
func (s *Service) Bookings(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy BookingsGroupBy) (*Bookings, error) {
	return nil, configerr.ErrNotImplemented
}

// Cancellations returns the cancellations and no-shows report.
func (s *Service) Cancellations(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy CancellationsGroupBy) (*Cancellations, error) {
	return nil, configerr.ErrNotImplemented
}

// SeasonPasses returns the season passes and releases report.
func (s *Service) SeasonPasses(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy SeasonPassesGroupBy) (*SeasonPasses, error) {
	return nil, configerr.ErrNotImplemented
}

// TopCustomers returns the customer ranking (limit 1..100, default 20).
func (s *Service) TopCustomers(ctx context.Context, p *auth.Principal, establishmentID string, period Period, sort TopCustomersSort, limit int) (*TopCustomers, error) {
	return nil, configerr.ErrNotImplemented
}

// OpenBalances returns the bookings with an open balance.
func (s *Service) OpenBalances(ctx context.Context, p *auth.Principal, establishmentID string, period Period) (*OpenBalances, error) {
	return nil, configerr.ErrNotImplemented
}

// Comparison returns the year-over-year comparison.
func (s *Service) Comparison(ctx context.Context, p *auth.Principal, establishmentID string, q ComparisonQuery) (*Comparison, error) {
	return nil, configerr.ErrNotImplemented
}
