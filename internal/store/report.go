package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Read-only queries of the manager reports (spec 15.3, owner:
// internal/domain/report). Every function takes an EstablishmentScope and
// filters on establishment_id; nothing here writes. The queries use only
// the SQL subset shared by SQLite and PostgreSQL and aggregate in SQL
// wherever the grouping does not need calendar arithmetic or JSON (the VAT
// shares of payments and the price snapshots are decoded in Go). Instants
// are compared as UTC half-open windows computed by the caller from the
// establishment timezone; calendar dates are compared as dates.
//
// Customer data: only first_name, last_name and anonymized_at are ever
// selected (spec 23); never email, phone, notes or address.
//
// Test mode (spec 15.4, migration 00009): rows with bookings.test or
// payments.test are excluded from every query of this file; allocations
// of test bookings count as free. Guarded by
// TestReportQueries_TestBookingsExcluded.

// Booking statuses as stored in bookings.status (the state machine lives in
// internal/domain/booking; the reports only read them).
const (
	reportStatusDraft     = "draft"
	reportStatusHeld      = "held"
	reportStatusExpired   = "expired"
	reportStatusRejected  = "rejected"
	reportStatusCancelled = "cancelled"
)

// ReportNotSoldStatuses are never counted as sales: the booking never
// existed for the customer (draft, held) or never became one (expired,
// rejected).
var ReportNotSoldStatuses = []string{reportStatusDraft, reportStatusHeld, reportStatusExpired, reportStatusRejected}

// ReportBookedStatuses are the statuses whose value counts as booked
// revenue (confirmed and everything that follows a confirmation except a
// cancellation).
var ReportBookedStatuses = []string{"confirmed", "checked_in", "completed", "no_show"}

// ReportSpotFilter narrows the spot based queries; zero fields do not filter.
type ReportSpotFilter struct {
	ZoneID string
	MapID  string
}

func (f ReportSpotFilter) apply(q *bun.SelectQuery, alias string) *bun.SelectQuery {
	if f.ZoneID != "" {
		q = q.Where("?.zone_id = ?", bun.Ident(alias), f.ZoneID)
	}
	if f.MapID != "" {
		q = q.Where("?.map_id = ?", bun.Ident(alias), f.MapID)
	}
	return q
}

// ReportZone is the label data of a zone.
type ReportZone struct {
	ID        string   `bun:"id"`
	Code      string   `bun:"code"`
	Name      I18nText `bun:"name,type:json"`
	SortOrder int      `bun:"sort_order"`
}

// ReportZones lists the zones of es ordered for display.
func ReportZones(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]ReportZone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportZone{}
	err := db.NewSelect().TableExpr("zones AS z").
		ColumnExpr("z.id, z.code, z.name, z.sort_order").
		Where("z.establishment_id = ?", es.ID()).
		OrderExpr("z.sort_order ASC, z.code ASC").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report zones: %w", err)
	}
	return out, nil
}

// ReportSpotCount is the number of active spots of one zone row.
type ReportSpotCount struct {
	ZoneID   string `bun:"zone_id"`
	RowLabel string `bun:"row_label"`
	Spots    int64  `bun:"spots"`
}

// ReportSpotCounts counts the active spots of es by zone and row.
func ReportSpotCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, f ReportSpotFilter) ([]ReportSpotCount, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportSpotCount{}
	q := db.NewSelect().TableExpr("spots AS s").
		ColumnExpr("s.zone_id, s.row_label, COUNT(*) AS spots").
		Where("s.establishment_id = ?", es.ID()).
		Where("s.active = ?", true).
		GroupExpr("s.zone_id, s.row_label")
	q = f.apply(q, "s")
	if err := q.Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report spot counts: %w", err)
	}
	return out, nil
}

// ReportUnitCount is a number of (spot, date, slot) units of one kind on
// one zone row.
type ReportUnitCount struct {
	Date     calendar.Date `bun:"date"`
	Slot     calendar.Slot `bun:"slot"`
	Kind     string        `bun:"kind"`
	ZoneID   string        `bun:"zone_id"`
	RowLabel string        `bun:"row_label"`
	Units    int64         `bun:"units"`
}

// ReportAllocationCounts counts the booking, season and block allocations
// of the active spots of es with date in [from, to] by date, slot, kind,
// zone and row. Holds never count.
func ReportAllocationCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, f ReportSpotFilter) ([]ReportUnitCount, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportUnitCount{}
	q := db.NewSelect().TableExpr("spot_allocations AS sa").
		Join("JOIN spots AS s ON s.id = sa.spot_id").
		ColumnExpr("sa.date, sa.slot, sa.kind, s.zone_id, s.row_label, COUNT(*) AS units").
		Where("sa.establishment_id = ?", es.ID()).
		Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		Where("sa.kind IN (?)", bun.List([]string{AllocationBooking, AllocationSeason, AllocationBlock})).
		Where("s.active = ?", true).
		Where("NOT EXISTS (SELECT 1 FROM bookings AS tb WHERE tb.id = sa.booking_id AND tb.test = ?)", true).
		GroupExpr("sa.date, sa.slot, sa.kind, s.zone_id, s.row_label")
	q = f.apply(q, "s")
	if err := q.Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report allocation counts: %w", err)
	}
	return out, nil
}

// ReportFreeReleaseCounts counts the active season releases of the active
// spots of es with date in [from, to] whose unit is currently free (no
// allocation of any kind: not resold, not held), by date, slot, zone and
// row. Kind is always "release".
func ReportFreeReleaseCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, f ReportSpotFilter) ([]ReportUnitCount, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportUnitCount{}
	q := db.NewSelect().TableExpr("season_releases AS sr").
		Join("JOIN spots AS s ON s.id = sr.spot_id").
		Join("LEFT JOIN spot_allocations AS sa ON sa.spot_id = sr.spot_id AND sa.date = sr.date AND sa.slot = sr.slot").
		ColumnExpr("sr.date, sr.slot, 'release' AS kind, s.zone_id, s.row_label, COUNT(*) AS units").
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.date >= ?", from).Where("sr.date <= ?", to).
		Where("sr.revoked_at IS NULL").
		Where("sa.id IS NULL").
		Where("s.active = ?", true).
		GroupExpr("sr.date, sr.slot, s.zone_id, s.row_label")
	q = f.apply(q, "s")
	if err := q.Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report free releases: %w", err)
	}
	return out, nil
}

// ReportOpenCalendar is what decides the open half-days of a span: the
// seasons overlapping it and its closure days.
type ReportOpenCalendar struct {
	Seasons  []ReportSeasonSpan
	Closures []ClosureDay
}

// ReportSeasonSpan is the opening window of a season.
type ReportSeasonSpan struct {
	OpensOn  calendar.Date `bun:"opens_on"`
	ClosesOn calendar.Date `bun:"closes_on"`
}

// ReportCalendar loads the seasons of es overlapping [from, to] (any
// status: a season defines when the establishment is open) and the closure
// days in [from, to].
func ReportCalendar(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) (*ReportOpenCalendar, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := &ReportOpenCalendar{Seasons: []ReportSeasonSpan{}, Closures: []ClosureDay{}}
	err := db.NewSelect().TableExpr("seasons AS sn").
		ColumnExpr("sn.opens_on, sn.closes_on").
		Where("sn.establishment_id = ?", es.ID()).
		Where("sn.opens_on <= ?", to).Where("sn.closes_on >= ?", from).
		OrderExpr("sn.opens_on ASC").
		Scan(ctx, &out.Seasons)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report seasons: %w", err)
	}
	err = db.NewSelect().Model(&out.Closures).
		Where("cd.establishment_id = ?", es.ID()).
		Where("cd.date >= ?", from).Where("cd.date <= ?", to).
		OrderExpr("cd.date ASC").
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report closures: %w", err)
	}
	return out, nil
}

// ReportPayment is a succeeded movement of the payments ledger with the
// channel of its booking (nil for counter sales without a booking).
type ReportPayment struct {
	BusinessDate calendar.Date `bun:"business_date"`
	Method       string        `bun:"method"`
	Direction    string        `bun:"direction"`
	Amount       money.Cents   `bun:"amount"`
	VATBreakdown []VATShare    `bun:"vat_breakdown,type:json"`
	BookingID    *string       `bun:"booking_id"`
	Channel      *string       `bun:"channel"`
}

// ReportPayments lists the succeeded payments and refunds of es with
// business date in [from, to] (the same rows as the corrispettivi export).
func ReportPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ReportPayment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportPayment{}
	err := db.NewSelect().TableExpr("payments AS pm").
		Join("LEFT JOIN bookings AS bk ON bk.id = pm.booking_id").
		ColumnExpr("pm.business_date, pm.method, pm.direction, pm.amount, pm.vat_breakdown, pm.booking_id, bk.channel").
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.status = ?", PaymentSucceeded).
		Where("pm.test = ?", false).
		Where("pm.business_date >= ?", from).Where("pm.business_date <= ?", to).
		OrderExpr("pm.business_date ASC, pm.id ASC").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report payments: %w", err)
	}
	return out, nil
}

// ReportBookingWindow selects the bookings of the booking facts: a booking
// is returned when it matches any of the three windows. Instants are UTC,
// half-open [From, To).
type ReportBookingWindow struct {
	CreatedFrom, CreatedTo     time.Time
	ConfirmedFrom, ConfirmedTo time.Time
	StartsFrom, StartsTo       calendar.Date
	// IncludePrice loads the price snapshots (for the VAT split of the
	// booked revenue); they are the heaviest columns, so only on demand.
	IncludePrice bool
}

// ReportBooking is the slim projection of a booking used by the reports.
type ReportBooking struct {
	ID              string        `bun:"id"`
	CustomerID      string        `bun:"customer_id"`
	Channel         string        `bun:"channel"`
	Status          string        `bun:"status"`
	Formula         string        `bun:"formula"`
	StartsOn        calendar.Date `bun:"starts_on"`
	EndsOn          calendar.Date `bun:"ends_on"`
	IsSeasonPass    bool          `bun:"is_season_pass"`
	RenewalOf       *string       `bun:"renewal_of_booking_id"`
	TotalAmount     money.Cents   `bun:"total_amount"`
	PaidAmount      money.Cents   `bun:"paid_amount"`
	CreatedAt       time.Time     `bun:"created_at"`
	ConfirmedAt     *time.Time    `bun:"confirmed_at"`
	Refunded        money.Cents   `bun:"refunded"`
	PriceSnapshot   JSON          `bun:"price_snapshot"`
	PriceCurrentRaw JSON          `bun:"price_current"`
}

// CurrentPrice is the breakdown pricing the booking today (see
// Booking.CurrentPrice); empty unless IncludePrice was set.
func (b *ReportBooking) CurrentPrice() JSON {
	if len(b.PriceCurrentRaw) > 0 {
		return b.PriceCurrentRaw
	}
	return b.PriceSnapshot
}

// ReportBookings lists the sold bookings of es (status not in
// ReportNotSoldStatuses) created, confirmed or starting in the windows of w,
// in one query. Refunded is the total of the succeeded refunds of cancelled
// bookings (0 for the others).
func ReportBookings(ctx context.Context, db bun.IDB, es EstablishmentScope, w ReportBookingWindow) ([]ReportBooking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportBooking{}
	cols := "bk.id, bk.customer_id, bk.channel, bk.status, bk.formula, bk.starts_on, bk.ends_on, bk.is_season_pass, " +
		"bk.renewal_of_booking_id, bk.total_amount, bk.paid_amount, bk.created_at, bk.confirmed_at"
	if w.IncludePrice {
		cols += ", bk.price_snapshot, bk.price_current"
	}
	err := db.NewSelect().TableExpr("bookings AS bk").
		ColumnExpr(cols).
		ColumnExpr("CASE WHEN bk.status = ? THEN (SELECT COALESCE(SUM(p.amount), 0) FROM payments AS p "+
			"WHERE p.booking_id = bk.id AND p.direction = ? AND p.status = ?) ELSE 0 END AS refunded",
			reportStatusCancelled, PaymentDirectionRefund, PaymentSucceeded).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false).
		Where("bk.status NOT IN (?)", bun.List(ReportNotSoldStatuses)).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				WhereOr("(bk.created_at >= ? AND bk.created_at < ?)", w.CreatedFrom, w.CreatedTo).
				WhereOr("(bk.confirmed_at >= ? AND bk.confirmed_at < ?)", w.ConfirmedFrom, w.ConfirmedTo).
				WhereOr("(bk.starts_on >= ? AND bk.starts_on <= ?)", w.StartsFrom, w.StartsTo)
		}).
		OrderExpr("bk.id ASC").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report bookings: %w", err)
	}
	return out, nil
}

// ReportCatalogRevenue is the booked value of one pack or extra.
type ReportCatalogRevenue struct {
	ID        string      `bun:"id"`
	Code      string      `bun:"code"`
	Name      I18nText    `bun:"name,type:json"`
	VATRateBP *int64      `bun:"vat_rate_bp"`
	Count     int         `bun:"items"`
	Gross     money.Cents `bun:"gross"`
}

// bookedWindow restricts a select joined with bookings (alias bk) to the
// booked statuses confirmed in [from, to) (UTC).
func bookedWindow(q *bun.SelectQuery, es EstablishmentScope, from, to time.Time) *bun.SelectQuery {
	return q.Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false).
		Where("bk.status IN (?)", bun.List(ReportBookedStatuses)).
		Where("bk.confirmed_at >= ?", from).Where("bk.confirmed_at < ?", to)
}

// ReportBookedPacks aggregates the pack value (line total minus the line
// extras, before booking-level discounts) of the bookings of es confirmed
// in [from, to) by pack. Count is the number of lines.
func ReportBookedPacks(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to time.Time) ([]ReportCatalogRevenue, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportCatalogRevenue{}
	q := db.NewSelect().TableExpr("booking_lines AS bl").
		Join("JOIN bookings AS bk ON bk.id = bl.booking_id").
		Join("JOIN packs AS pa ON pa.id = bl.pack_id").
		ColumnExpr("pa.id, pa.code, pa.name, pa.vat_rate_bp, COUNT(*) AS items").
		ColumnExpr("SUM(bl.line_total - (SELECT COALESCE(SUM(x.total), 0) FROM booking_line_extras AS x WHERE x.booking_line_id = bl.id)) AS gross").
		GroupExpr("pa.id, pa.code, pa.name, pa.vat_rate_bp").
		OrderExpr("pa.code ASC")
	if err := bookedWindow(q, es, from, to).Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report booked packs: %w", err)
	}
	return out, nil
}

// ReportBookedExtras aggregates the extras (of lines and of the booking) of
// the bookings of es confirmed in [from, to) by extra. Count is the
// quantity sold.
func ReportBookedExtras(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to time.Time) ([]ReportCatalogRevenue, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportCatalogRevenue{}
	q := db.NewSelect().TableExpr("booking_line_extras AS bx").
		Join("JOIN bookings AS bk ON bk.id = bx.booking_id").
		Join("JOIN extras AS ex ON ex.id = bx.extra_id").
		ColumnExpr("ex.id, ex.code, ex.name, ex.vat_rate_bp, SUM(bx.quantity) AS items, SUM(bx.total) AS gross").
		GroupExpr("ex.id, ex.code, ex.name, ex.vat_rate_bp").
		OrderExpr("ex.code ASC")
	if err := bookedWindow(q, es, from, to).Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report booked extras: %w", err)
	}
	return out, nil
}

// ReportRelease is an active season release with its resale, if any.
type ReportRelease struct {
	Date         calendar.Date `bun:"date"`
	Slot         calendar.Slot `bun:"slot"`
	ZoneID       string        `bun:"zone_id"`
	CreditAmount money.Cents   `bun:"credit_amount"`
	// ResoldLineID is the booking line now allocated on the released unit
	// (nil when the unit is free or held).
	ResoldLineID *string     `bun:"resold_line_id"`
	LineTotal    money.Cents `bun:"line_total"`
	// LineUnits is the number of units allocated to ResoldLineID (to
	// prorate its total per unit).
	LineUnits int64 `bun:"line_units"`
}

// ReportReleases lists the active season releases of es with date in
// [from, to], with the booking line that resold each unit.
func ReportReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ReportRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportRelease{}
	err := db.NewSelect().TableExpr("season_releases AS sr").
		Join("JOIN spots AS s ON s.id = sr.spot_id").
		Join("LEFT JOIN spot_allocations AS sa ON sa.spot_id = sr.spot_id AND sa.date = sr.date AND sa.slot = sr.slot AND sa.kind = ? "+
			"AND NOT EXISTS (SELECT 1 FROM bookings AS tb WHERE tb.id = sa.booking_id AND tb.test = ?)", AllocationBooking, true).
		Join("LEFT JOIN booking_lines AS bl ON bl.id = sa.booking_line_id").
		ColumnExpr("sr.date, sr.slot, s.zone_id, sr.credit_amount, sa.booking_line_id AS resold_line_id").
		ColumnExpr("COALESCE(bl.line_total, 0) AS line_total").
		ColumnExpr("CASE WHEN sa.booking_line_id IS NULL THEN 0 ELSE (SELECT COUNT(*) FROM spot_allocations AS x WHERE x.booking_line_id = sa.booking_line_id) END AS line_units").
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.date >= ?", from).Where("sr.date <= ?", to).
		Where("sr.revoked_at IS NULL").
		OrderExpr("sr.date ASC, sr.slot ASC, sr.id ASC").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report releases: %w", err)
	}
	return out, nil
}

// ReportCustomer is one customer of the top customers ranking (display
// name parts only).
type ReportCustomer struct {
	CustomerID    string         `bun:"customer_id"`
	FirstName     string         `bun:"first_name"`
	LastName      string         `bun:"last_name"`
	AnonymizedAt  *time.Time     `bun:"anonymized_at"`
	Bookings      int            `bun:"bookings"`
	Revenue       money.Cents    `bun:"revenue"`
	LastBookingOn *calendar.Date `bun:"last_booking_on"`
	SpotDays      int            `bun:"-"`
}

// ReportTopCustomers ranks the customers of es by the sold bookings (status
// not in ReportNotSoldStatuses) starting in [from, to]: Bookings excludes
// cancelled ones, Revenue is the net paid amount of all of them (penalties
// retained on cancellations included). byBookings sorts by Bookings first,
// otherwise by Revenue first; ties by the other metric, then customer id.
func ReportTopCustomers(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, byBookings bool, limit int) ([]ReportCustomer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportCustomer{}
	order := "revenue DESC, bookings DESC, bk.customer_id ASC"
	if byBookings {
		order = "bookings DESC, revenue DESC, bk.customer_id ASC"
	}
	err := db.NewSelect().TableExpr("bookings AS bk").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id").
		ColumnExpr("bk.customer_id, cu.first_name, cu.last_name, cu.anonymized_at").
		ColumnExpr("SUM(CASE WHEN bk.status = ? THEN 0 ELSE 1 END) AS bookings", reportStatusCancelled).
		ColumnExpr("SUM(bk.paid_amount) AS revenue, MAX(bk.starts_on) AS last_booking_on").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false).
		Where("bk.status NOT IN (?)", bun.List(ReportNotSoldStatuses)).
		Where("bk.starts_on >= ?", from).Where("bk.starts_on <= ?", to).
		GroupExpr("bk.customer_id, cu.first_name, cu.last_name, cu.anonymized_at").
		Having("SUM(CASE WHEN bk.status = ? THEN 0 ELSE 1 END) > 0 OR SUM(bk.paid_amount) <> 0", reportStatusCancelled).
		OrderExpr(order).
		Limit(int64(limit)).
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report top customers: %w", err)
	}
	return out, nil
}

// ReportCustomerSpotDays counts, for each customer of ids, the distinct
// (spot, date) pairs in [from, to] allocated to their bookings (booking or
// season allocations).
func ReportCustomerSpotDays(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, ids []string) (map[string]int, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	inner := db.NewSelect().TableExpr("spot_allocations AS sa").
		Join("JOIN bookings AS bk ON bk.id = sa.booking_id").
		ColumnExpr("bk.customer_id, sa.spot_id, sa.date").Distinct().
		Where("sa.establishment_id = ?", es.ID()).
		Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		Where("sa.kind IN (?)", bun.List([]string{AllocationBooking, AllocationSeason})).
		Where("bk.customer_id IN (?)", bun.List(ids)).
		Where("bk.test = ?", false)
	var rows []struct {
		CustomerID string `bun:"customer_id"`
		Days       int    `bun:"days"`
	}
	err := db.NewSelect().TableExpr("(?) AS d", inner).
		ColumnExpr("d.customer_id, COUNT(*) AS days").
		GroupExpr("d.customer_id").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report spot days: %w", err)
	}
	for _, r := range rows {
		out[r.CustomerID] = r.Days
	}
	return out, nil
}

// ReportOpenBalance is a booking with an amount still to collect.
type ReportOpenBalance struct {
	BookingID    string        `bun:"id"`
	Code         string        `bun:"code"`
	CustomerID   string        `bun:"customer_id"`
	FirstName    string        `bun:"first_name"`
	LastName     string        `bun:"last_name"`
	AnonymizedAt *time.Time    `bun:"anonymized_at"`
	Status       string        `bun:"status"`
	StartsOn     calendar.Date `bun:"starts_on"`
	EndsOn       calendar.Date `bun:"ends_on"`
	TotalAmount  money.Cents   `bun:"total_amount"`
	PaidAmount   money.Cents   `bun:"paid_amount"`
}

// ReportOpenBalances lists the bookings of es in a booked status with
// total_amount > paid_amount and starts_on in [from, to], by first day.
func ReportOpenBalances(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ReportOpenBalance, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []ReportOpenBalance{}
	err := db.NewSelect().TableExpr("bookings AS bk").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id").
		ColumnExpr("bk.id, bk.code, bk.customer_id, cu.first_name, cu.last_name, cu.anonymized_at").
		ColumnExpr("bk.status, bk.starts_on, bk.ends_on, bk.total_amount, bk.paid_amount").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false).
		Where("bk.status IN (?)", bun.List(ReportBookedStatuses)).
		Where("bk.total_amount > bk.paid_amount").
		Where("bk.starts_on >= ?", from).Where("bk.starts_on <= ?", to).
		OrderExpr("bk.starts_on ASC, bk.code ASC").
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: report open balances: %w", err)
	}
	return out, nil
}
