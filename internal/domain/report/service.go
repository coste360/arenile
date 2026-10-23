package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/store"
)

// ErrEstablishmentNotFound is returned for unknown or out-of-scope
// establishments (404).
var ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)

// DefaultTopCustomers is the default size of the top customers ranking.
const DefaultTopCustomers = 20

// Deps are the dependencies of the report service. Every field is required
// except Log.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Log   *slog.Logger
}

// Service implements the report use cases (imperative shell): every method
// authorises the principal with auth.PermReportsRead on establishmentID
// first, validates the query, runs a few read-only queries scoped by the
// returned store.EstablishmentScope and hands the rows to the pure builders
// of this package.
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
// Each result has a Write<Name>CSV(io.Writer, *<Name>) companion in
// export.go, used when format=csv or Accept: text/csv.
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

// env is what every report needs from the establishment.
type env struct {
	es         store.EstablishmentScope
	loc        *time.Location
	defaultVAT int64
}

func (s *Service) open(ctx context.Context, p *auth.Principal, establishmentID string) (*env, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermReportsRead)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrEstablishmentNotFound
	}
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("report: load establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("report: timezone %q: %w", est.Timezone, err)
	}
	settings, _ := establishment.ParseSettings(est.Settings, est.Currency) // malformed settings fall back to the defaults
	return &env{es: es, loc: loc, defaultVAT: settings.Pricing.DefaultVATRateBP}, nil
}

// validPeriod re-checks a Period not built with NewPeriod (e.g. the zero
// value), so the service never runs an unbounded query.
func validPeriod(p Period) error {
	_, err := NewPeriod(p.From, p.To)
	return err
}

// Summary returns the KPI set of the period.
func (s *Service) Summary(ctx context.Context, p *auth.Principal, establishmentID string, period Period) (*Summary, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	sf, err := s.facts(ctx, e, period)
	if err != nil {
		return nil, err
	}
	rows, err := store.ReportOpenBalances(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildSummary(sf, buildOpenBalances(period, rows), s.d.Clock.Now()), nil
}

// Occupancy returns the occupancy report.
func (s *Service) Occupancy(ctx context.Context, p *auth.Principal, establishmentID string, q OccupancyQuery) (*Occupancy, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if q.GroupBy == "" {
		q.GroupBy = OccupancyByDay
	}
	if err := validPeriod(q.Period); err != nil {
		return nil, err
	}
	switch q.GroupBy {
	case OccupancyByDay, OccupancyByZone, OccupancyByRow, OccupancyBySlot:
	default:
		return nil, configerr.Invalid("group_by", "Valori ammessi: day, zone, row, slot.")
	}
	in, err := s.occupancyInput(ctx, e, q.Period, store.ReportSpotFilter{ZoneID: q.ZoneID, MapID: q.MapID})
	if err != nil {
		return nil, err
	}
	return buildOccupancy(*in, q.GroupBy), nil
}

// Revenue returns the revenue report. payment_method requires
// BasisCollected; pack and extra require BasisBooked (validation error).
func (s *Service) Revenue(ctx context.Context, p *auth.Principal, establishmentID string, q RevenueQuery) (*Revenue, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if q.GroupBy == "" {
		q.GroupBy = RevenueByDay
	}
	if q.Basis == "" {
		q.Basis = BasisCollected
	}
	if err := validPeriod(q.Period); err != nil {
		return nil, err
	}
	if err := validateRevenue(q); err != nil {
		return nil, err
	}
	if q.Basis == BasisCollected {
		pays, err := store.ReportPayments(ctx, s.d.DB, e.es, q.Period.From, q.Period.To)
		if err != nil {
			return nil, fmt.Errorf("report: %w", err)
		}
		return buildRevenueCollected(q.Period, pays, q.GroupBy, e.defaultVAT), nil
	}
	from, to := dayWindow(q.Period, e.loc)
	switch q.GroupBy {
	case RevenueByPack, RevenueByExtra:
		load := store.ReportBookedPacks
		if q.GroupBy == RevenueByExtra {
			load = store.ReportBookedExtras
		}
		items, err := load(ctx, s.d.DB, e.es, from, to)
		if err != nil {
			return nil, fmt.Errorf("report: %w", err)
		}
		return buildRevenueCatalog(q.Period, q.GroupBy, items, e.defaultVAT), nil
	default:
		bks, err := store.ReportBookings(ctx, s.d.DB, e.es, store.ReportBookingWindow{ConfirmedFrom: from, ConfirmedTo: to, IncludePrice: true})
		if err != nil {
			return nil, fmt.Errorf("report: %w", err)
		}
		return buildRevenueBooked(q.Period, bks, q.GroupBy, e.loc, e.defaultVAT), nil
	}
}

// validateRevenue checks the group_by / basis combination.
func validateRevenue(q RevenueQuery) error {
	ve := &configerr.ValidationError{}
	switch q.Basis {
	case BasisCollected, BasisBooked:
	default:
		ve.Add("basis", "Valori ammessi: collected, booked.")
	}
	switch q.GroupBy {
	case RevenueByDay, RevenueByWeek, RevenueByMonth, RevenueByChannel, RevenueByVATRate:
	case RevenueByPaymentMethod:
		if q.Basis == BasisBooked {
			ve.Add("group_by", "Il metodo di pagamento è disponibile solo sull'incassato (basis=collected).")
		}
	case RevenueByPack, RevenueByExtra:
		if q.Basis == BasisCollected {
			ve.Add("group_by", "Pack ed extra sono disponibili solo sul venduto (basis=booked).")
		}
	default:
		ve.Add("group_by", "Valori ammessi: day, week, month, channel, pack, extra, payment_method, vat_rate.")
	}
	return ve.OrNil()
}

// Bookings returns the bookings report.
func (s *Service) Bookings(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy BookingsGroupBy) (*Bookings, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if groupBy == "" {
		groupBy = BookingsByChannel
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	switch groupBy {
	case BookingsByChannel, BookingsByLeadTime, BookingsByFormula, BookingsByDay:
	default:
		return nil, configerr.Invalid("group_by", "Valori ammessi: channel, lead_time, formula, day.")
	}
	from, to := dayWindow(period, e.loc)
	bks, err := store.ReportBookings(ctx, s.d.DB, e.es, store.ReportBookingWindow{CreatedFrom: from, CreatedTo: to})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildBookings(period, bks, groupBy, e.loc), nil
}

// Cancellations returns the cancellations and no-shows report.
func (s *Service) Cancellations(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy CancellationsGroupBy) (*Cancellations, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if groupBy == "" {
		groupBy = CancellationsByMonth
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	switch groupBy {
	case CancellationsByDay, CancellationsByMonth, CancellationsByChannel:
	default:
		return nil, configerr.Invalid("group_by", "Valori ammessi: day, month, channel.")
	}
	bks, err := store.ReportBookings(ctx, s.d.DB, e.es, store.ReportBookingWindow{StartsFrom: period.From, StartsTo: period.To})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildCancellations(period, bks, groupBy), nil
}

// SeasonPasses returns the season passes and releases report.
func (s *Service) SeasonPasses(ctx context.Context, p *auth.Principal, establishmentID string, period Period, groupBy SeasonPassesGroupBy) (*SeasonPasses, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if groupBy == "" {
		groupBy = SeasonPassesByMonth
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	switch groupBy {
	case SeasonPassesByDay, SeasonPassesByMonth, SeasonPassesByZone:
	default:
		return nil, configerr.Invalid("group_by", "Valori ammessi: day, month, zone.")
	}
	from, to := dayWindow(period, e.loc)
	bks, err := store.ReportBookings(ctx, s.d.DB, e.es, store.ReportBookingWindow{ConfirmedFrom: from, ConfirmedTo: to})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	rels, err := store.ReportReleases(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	zones, err := store.ReportZones(ctx, s.d.DB, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildSeasonPasses(period, bks, rels, zones, groupBy, e.loc), nil
}

// TopCustomers returns the customer ranking (limit 1..100, default 20).
func (s *Service) TopCustomers(ctx context.Context, p *auth.Principal, establishmentID string, period Period, sort TopCustomersSort, limit int) (*TopCustomers, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if sort == "" {
		sort = SortByRevenue
	}
	if limit == 0 {
		limit = DefaultTopCustomers
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	if sort != SortByRevenue && sort != SortByBookings {
		ve.Add("sort", "Valori ammessi: revenue, bookings.")
	}
	if limit < 1 || limit > 100 {
		ve.Add("limit", "Tra 1 e 100.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	rows, err := store.ReportTopCustomers(ctx, s.d.DB, e.es, period.From, period.To, sort == SortByBookings, limit)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.CustomerID)
	}
	days, err := store.ReportCustomerSpotDays(ctx, s.d.DB, e.es, period.From, period.To, ids)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildTopCustomers(period, sort, rows, days), nil
}

// OpenBalances returns the bookings with an open balance.
func (s *Service) OpenBalances(ctx context.Context, p *auth.Principal, establishmentID string, period Period) (*OpenBalances, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	rows, err := store.ReportOpenBalances(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildOpenBalances(period, rows), nil
}

// Comparison returns the year-over-year comparison.
func (s *Service) Comparison(ctx context.Context, p *auth.Principal, establishmentID string, q ComparisonQuery) (*Comparison, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if q.Granularity == "" {
		q.Granularity = GranularityWeek
	}
	if q.Align == "" {
		q.Align = AlignCalendar
	}
	if err := validPeriod(q.Period); err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	switch q.Granularity {
	case GranularityDay, GranularityWeek, GranularityMonth:
	default:
		ve.Add("granularity", "Valori ammessi: day, week, month.")
	}
	if q.Align != AlignCalendar && q.Align != AlignWeekday {
		ve.Add("align", "Valori ammessi: calendar, weekday.")
	}
	if q.CompareYear != 0 && (q.CompareYear < 2000 || q.CompareYear > 2999) {
		ve.Add("compare_year", "Anno non valido.")
	} else if q.CompareYear != 0 && q.CompareYear >= q.Period.From.Year() {
		ve.Add("compare_year", "Deve precedere l'anno di from.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	prevPeriod := ComparisonSpan(q.Period, q.CompareYear, q.Align)
	cur, err := s.facts(ctx, e, q.Period)
	if err != nil {
		return nil, err
	}
	prev, err := s.facts(ctx, e, prevPeriod)
	if err != nil {
		return nil, err
	}
	return buildComparison(cur, prev, q.Granularity, q.Align), nil
}

// occupancyInput loads the occupancy rows of period (5 queries).
func (s *Service) occupancyInput(ctx context.Context, e *env, period Period, f store.ReportSpotFilter) (*occupancyInput, error) {
	cal, err := store.ReportCalendar(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	in := &occupancyInput{period: period, open: openUnits(period, cal)}
	if in.zones, err = store.ReportZones(ctx, s.d.DB, e.es); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	if in.spots, err = store.ReportSpotCounts(ctx, s.d.DB, e.es, f); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	if in.allocs, err = store.ReportAllocationCounts(ctx, s.d.DB, e.es, period.From, period.To, f); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	if in.free, err = store.ReportFreeReleaseCounts(ctx, s.d.DB, e.es, period.From, period.To, f); err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return in, nil
}

// facts loads the per-day facts of period: the occupancy queries, the
// payments, the bookings of the three windows (one query) and the releases.
func (s *Service) facts(ctx context.Context, e *env, period Period) (*spanFacts, error) {
	occ, err := s.occupancyInput(ctx, e, period, store.ReportSpotFilter{})
	if err != nil {
		return nil, err
	}
	pays, err := store.ReportPayments(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	from, to := dayWindow(period, e.loc)
	bks, err := store.ReportBookings(ctx, s.d.DB, e.es, store.ReportBookingWindow{
		CreatedFrom: from, CreatedTo: to, ConfirmedFrom: from, ConfirmedTo: to, StartsFrom: period.From, StartsTo: period.To,
	})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	rels, err := store.ReportReleases(ctx, s.d.DB, e.es, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	return buildFacts(factsInput{
		period: period, occupancy: buildOccupancy(*occ, OccupancyByDay), payments: pays, bookings: bks, releases: rels, loc: e.loc,
	}), nil
}
