package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Reports of spec 15.3 and the year comparison (M5A). Owner: reports
// backend builder. Every handler: build report.Period with
// report.NewPeriod(params.From, params.To), call the service, then answer
// JSON or, when params.Format == api.ReportFormatCSV or the Accept header
// contains text/csv, the CSV export written by report.Write<Name>CSV with
// Content-Type "text/csv; charset=utf-8", Content-Disposition attachment
// filename "<report>_<from>_<to>.csv" and Cache-Control no-store (same as
// GetCorrispettivi in fiscal.go).
//
//	GetReportSummary        -> Reports.Summary       -> api.ReportSummary
//	GetOccupancyReport      -> Reports.Occupancy     -> api.OccupancyReport
//	GetRevenueReport        -> Reports.Revenue       -> api.RevenueReport
//	GetBookingsReport       -> Reports.Bookings      -> api.BookingsReport
//	GetCancellationsReport  -> Reports.Cancellations -> api.CancellationsReport
//	GetSeasonPassesReport   -> Reports.SeasonPasses  -> api.SeasonPassesReport
//	GetTopCustomersReport   -> Reports.TopCustomers  -> api.TopCustomersReport
//	GetOpenBalancesReport   -> Reports.OpenBalances  -> api.OpenBalancesReport
//	GetYearComparisonReport -> Reports.Comparison    -> api.YearComparisonReport

// GetReportSummary implements GET /api/admin/v1/establishments/{establishmentId}/reports/summary.
func (h *Handlers) GetReportSummary(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetReportSummaryParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetOccupancyReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/occupancy.
func (h *Handlers) GetOccupancyReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetOccupancyReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetRevenueReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/revenue.
func (h *Handlers) GetRevenueReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetRevenueReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetBookingsReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/bookings.
func (h *Handlers) GetBookingsReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetBookingsReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetCancellationsReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/cancellations.
func (h *Handlers) GetCancellationsReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetCancellationsReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetSeasonPassesReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/season-passes.
func (h *Handlers) GetSeasonPassesReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetSeasonPassesReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetTopCustomersReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/top-customers.
func (h *Handlers) GetTopCustomersReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetTopCustomersReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetOpenBalancesReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/open-balances.
func (h *Handlers) GetOpenBalancesReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetOpenBalancesReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetYearComparisonReport implements GET /api/admin/v1/establishments/{establishmentId}/reports/comparison.
func (h *Handlers) GetYearComparisonReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetYearComparisonReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}
