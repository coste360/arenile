package report

import (
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Group-by values of each report (API enums of the same name).
type (
	// OccupancyGroupBy: day, zone, row, slot.
	OccupancyGroupBy string
	// RevenueGroupBy: day, week, month, channel, pack, extra, payment_method, vat_rate.
	RevenueGroupBy string
	// RevenueBasis: collected, booked.
	RevenueBasis string
	// BookingsGroupBy: channel, lead_time, formula, day.
	BookingsGroupBy string
	// CancellationsGroupBy: day, month, channel.
	CancellationsGroupBy string
	// SeasonPassesGroupBy: day, month, zone.
	SeasonPassesGroupBy string
	// TopCustomersSort: revenue, bookings.
	TopCustomersSort string
	// Granularity of the comparison series: day, week, month.
	Granularity string
)

// Group-by constants.
const (
	OccupancyByDay  OccupancyGroupBy = "day"
	OccupancyByZone OccupancyGroupBy = "zone"
	OccupancyByRow  OccupancyGroupBy = "row"
	OccupancyBySlot OccupancyGroupBy = "slot"

	RevenueByDay           RevenueGroupBy = "day"
	RevenueByWeek          RevenueGroupBy = "week"
	RevenueByMonth         RevenueGroupBy = "month"
	RevenueByChannel       RevenueGroupBy = "channel"
	RevenueByPack          RevenueGroupBy = "pack"
	RevenueByExtra         RevenueGroupBy = "extra"
	RevenueByPaymentMethod RevenueGroupBy = "payment_method"
	RevenueByVATRate       RevenueGroupBy = "vat_rate"

	BasisCollected RevenueBasis = "collected"
	BasisBooked    RevenueBasis = "booked"

	BookingsByChannel  BookingsGroupBy = "channel"
	BookingsByLeadTime BookingsGroupBy = "lead_time"
	BookingsByFormula  BookingsGroupBy = "formula"
	BookingsByDay      BookingsGroupBy = "day"

	CancellationsByDay     CancellationsGroupBy = "day"
	CancellationsByMonth   CancellationsGroupBy = "month"
	CancellationsByChannel CancellationsGroupBy = "channel"

	SeasonPassesByDay   SeasonPassesGroupBy = "day"
	SeasonPassesByMonth SeasonPassesGroupBy = "month"
	SeasonPassesByZone  SeasonPassesGroupBy = "zone"

	SortByRevenue  TopCustomersSort = "revenue"
	SortByBookings TopCustomersSort = "bookings"

	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

// ChannelCounter is the revenue channel of counter sales without a booking.
const ChannelCounter = "counter"

// AnonymizedName is the display name of anonymised customers (spec 23).
const AnonymizedName = "Cliente anonimizzato"

// Summary is the KPI set of the report dashboard.
type Summary struct {
	Period              Period
	OccupancyBP         int64
	OccupiedUnits       int64
	CapacityUnits       int64
	RevenueCollected    money.Cents
	RevenueBooked       money.Cents
	Bookings            int
	AverageBookingValue money.Cents
	Cancellations       int
	NoShows             int
	SeasonPassesSold    int
	OpenBalanceTotal    money.Cents
	OpenBalanceBookings int
	GeneratedAt         time.Time
}

// OccupancyRow is one group of the occupancy report (also the totals row).
type OccupancyRow struct {
	Key, Label    string
	ZoneID        string
	CapacityUnits int64
	OccupiedUnits int64
	SeasonUnits   int64
	ReleasedUnits int64
	BlockedUnits  int64
	OccupancyBP   int64
}

// OccupancyQuery filters the occupancy report.
type OccupancyQuery struct {
	Period  Period
	GroupBy OccupancyGroupBy
	ZoneID  string
	MapID   string
}

// Occupancy is the occupancy report.
type Occupancy struct {
	Period  Period
	GroupBy OccupancyGroupBy
	Rows    []OccupancyRow
	Totals  OccupancyRow
}

// RevenueRow is one group of the revenue report (also the totals row).
type RevenueRow struct {
	Key, Label string
	Count      int
	Gross      money.Cents
	Taxable    money.Cents
	Tax        money.Cents
	Refunds    money.Cents
}

// RevenueQuery selects the revenue report.
type RevenueQuery struct {
	Period  Period
	GroupBy RevenueGroupBy
	Basis   RevenueBasis
}

// Revenue is the revenue report.
type Revenue struct {
	Period  Period
	GroupBy RevenueGroupBy
	Basis   RevenueBasis
	Rows    []RevenueRow
	Totals  RevenueRow
}

// BookingsRow is one group of the bookings report.
type BookingsRow struct {
	Key, Label      string
	Bookings        int
	Value           money.Cents
	AverageValue    money.Cents
	ShareBP         int64
	AverageLeadDays int
}

// Bookings is the bookings report.
type Bookings struct {
	Period  Period
	GroupBy BookingsGroupBy
	Rows    []BookingsRow
	Totals  BookingsRow
}

// CancellationsRow is one group of the cancellations report.
type CancellationsRow struct {
	Key, Label  string
	Bookings    int
	Cancelled   int
	NoShows     int
	CancelledBP int64
	NoShowBP    int64
	LostValue   money.Cents
	Refunded    money.Cents
}

// Cancellations is the cancellations and no-shows report.
type Cancellations struct {
	Period  Period
	GroupBy CancellationsGroupBy
	Rows    []CancellationsRow
	Totals  CancellationsRow
}

// SeasonPassesRow is one group of the releases section.
type SeasonPassesRow struct {
	Key, Label    string
	ReleasedUnits int64
	ResoldUnits   int64
	ResaleBP      int64
	ResaleRevenue money.Cents
	CreditsIssued money.Cents
}

// SeasonPasses is the season passes and releases report.
type SeasonPasses struct {
	Period      Period
	GroupBy     SeasonPassesGroupBy
	PassesSold  int
	PassesValue money.Cents
	Renewals    int
	Rows        []SeasonPassesRow
	Totals      SeasonPassesRow
}

// TopCustomerRow is one customer of the ranking (display name only).
type TopCustomerRow struct {
	Rank          int
	CustomerID    string
	DisplayName   string
	Bookings      int
	SpotDays      int
	Revenue       money.Cents
	LastBookingOn *calendar.Date
}

// TopCustomers is the top customers report.
type TopCustomers struct {
	Period Period
	Sort   TopCustomersSort
	Rows   []TopCustomerRow
}

// OpenBalanceRow is a booking with an open balance.
type OpenBalanceRow struct {
	BookingID, Code string
	CustomerID      string
	DisplayName     string
	Status          string
	StartsOn        calendar.Date
	EndsOn          calendar.Date
	Total           money.Cents
	Paid            money.Cents
	Balance         money.Cents
}

// OpenBalances is the open balances report.
type OpenBalances struct {
	Period       Period
	Rows         []OpenBalanceRow
	TotalBalance money.Cents
}

// Metric keys of the comparison (API enum ComparisonMetricKey).
const (
	MetricOccupancyBP      = "occupancy_bp"
	MetricRevenueCollected = "revenue_collected"
	MetricRevenueBooked    = "revenue_booked"
	MetricBookings         = "bookings"
	MetricAverageValue     = "average_booking_value"
	MetricCancellations    = "cancellations"
	MetricNoShows          = "no_shows"
	MetricSeasonPassesSold = "season_passes_sold"
	MetricReleasedUnits    = "released_units"
	MetricResoldUnits      = "resold_units"
)

// Units of comparison metrics (API enum ComparisonUnit).
const (
	UnitCents       = "cents"
	UnitCount       = "count"
	UnitBasisPoints = "basis_points"
)

// ComparisonMetric compares one metric across the two spans.
type ComparisonMetric struct {
	Metric   string
	Unit     string
	Current  int64
	Previous int64
	Delta    int64
	DeltaBP  *int64
}

// ComparisonPoint is one bucket of the aligned series.
type ComparisonPoint struct {
	Key, Label                              string
	CurrentFrom, PreviousFrom               calendar.Date
	CurrentRevenue, PreviousRevenue         money.Cents
	CurrentOccupancyBP, PreviousOccupancyBP int64
	CurrentBookings, PreviousBookings       int
}

// ComparisonQuery selects the year comparison.
type ComparisonQuery struct {
	Period      Period
	CompareYear int // 0 = previous year
	Granularity Granularity
}

// Comparison is the year-over-year comparison report.
type Comparison struct {
	Current, Previous Period
	Granularity       Granularity
	Metrics           []ComparisonMetric
	Series            []ComparisonPoint
}
