package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Read models of the inventory queries (availability, operations day,
// planning, auto-assignment). Owner: internal/domain/inventory. Every
// function loads one table (or one join) for a whole range so that the
// services never issue per-spot or per-day queries (no N+1).

// InventorySpotFilter narrows InventorySpots; empty fields do not filter.
type InventorySpotFilter struct {
	MapID      string
	ZoneIDs    []string
	SpotIDs    []string
	ActiveOnly bool
}

// InventorySpots lists the spots of es ordered by map, row, number, label.
func InventorySpots(ctx context.Context, db bun.IDB, es EstablishmentScope, f InventorySpotFilter) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Spot
	q := db.NewSelect().Model(&rows).Where("sp.establishment_id = ?", es.ID()).
		OrderExpr("sp.map_id, sp.row_label, sp.number, sp.label, sp.id")
	if f.MapID != "" {
		q = q.Where("sp.map_id = ?", f.MapID)
	}
	if f.ZoneIDs != nil {
		if len(f.ZoneIDs) == 0 {
			return nil, nil
		}
		q = q.Where("sp.zone_id IN (?)", bun.List(f.ZoneIDs))
	}
	if f.SpotIDs != nil {
		if len(f.SpotIDs) == 0 {
			return nil, nil
		}
		q = q.Where("sp.id IN (?)", bun.List(f.SpotIDs))
	}
	if f.ActiveOnly {
		q = q.Where("sp.active = ?", true)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: inventory spots: %w", err)
	}
	return rows, nil
}

// InventoryZones lists the zones of es (ids nil = all) by sort order.
func InventoryZones(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Zone, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if ids != nil && len(ids) == 0 {
		return nil, nil
	}
	var rows []Zone
	q := db.NewSelect().Model(&rows).Where("z.establishment_id = ?", es.ID()).OrderExpr("z.sort_order, z.code, z.id")
	if ids != nil {
		q = q.Where("z.id IN (?)", bun.List(ids))
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: inventory zones: %w", err)
	}
	return rows, nil
}

// InventoryPools lists the pools of es: ids nil = all, by system flag and id.
func InventoryPools(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Pool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if ids != nil && len(ids) == 0 {
		return nil, nil
	}
	var rows []Pool
	q := db.NewSelect().Model(&rows).Where("pl.establishment_id = ?", es.ID()).OrderExpr("pl.system_managed DESC, pl.id ASC")
	if ids != nil {
		q = q.Where("pl.id IN (?)", bun.List(ids))
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: inventory pools: %w", err)
	}
	return rows, nil
}

// TrackedComponentPools maps the codes of tracked components bound to a
// pool to that pool id.
func TrackedComponentPools(ctx context.Context, db bun.IDB, es EstablishmentScope, codes []string) (map[string]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(codes) == 0 {
		return out, nil
	}
	var rows []struct {
		Code   string `bun:"code"`
		PoolID string `bun:"pool_id"`
	}
	err := db.NewSelect().TableExpr("components").ColumnExpr("code, pool_id").
		Where("establishment_id = ?", es.ID()).Where("tracked = ?", true).
		Where("pool_id IS NOT NULL").Where("code IN (?)", bun.List(codes)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: tracked component pools: %w", err)
	}
	for _, r := range rows {
		out[r.Code] = r.PoolID
	}
	return out, nil
}

// ClosureDaysInRange returns the closure days of es between from and to.
func ClosureDaysInRange(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ClosureDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []ClosureDay
	err := db.NewSelect().Model(&rows).Where("cd.establishment_id = ?", es.ID()).
		Where("cd.date >= ?", from).Where("cd.date <= ?", to).OrderExpr("cd.date").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: closure days in range: %w", err)
	}
	return rows, nil
}

// SeasonsOverlapping returns the seasons of es overlapping [from, to].
func SeasonsOverlapping(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Season
	err := db.NewSelect().Model(&rows).Where("sn.establishment_id = ?", es.ID()).
		Where("sn.opens_on <= ?", to).Where("sn.closes_on >= ?", from).OrderExpr("sn.opens_on").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: seasons overlapping: %w", err)
	}
	return rows, nil
}

// GetPackForInventory loads the restrictions of a pack (zones, spot kinds).
func GetPackForInventory(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Pack, error) {
	return GetPack(ctx, db, es, id)
}

// BookingSummary is what the operational views show of a booking.
type BookingSummary struct {
	ID            string         `bun:"id"`
	Code          string         `bun:"code"`
	Status        string         `bun:"status"`
	Channel       string         `bun:"channel"`
	CustomerID    string         `bun:"customer_id"`
	StartsOn      calendar.Date  `bun:"starts_on"`
	EndsOn        calendar.Date  `bun:"ends_on"`
	Slots         calendar.Slots `bun:"slots"`
	IsSeasonPass  bool           `bun:"is_season_pass"`
	TotalAmount   money.Cents    `bun:"total_amount"`
	PaidAmount    money.Cents    `bun:"paid_amount"`
	NotesCustomer string         `bun:"notes_customer"`
	NotesInternal string         `bun:"notes_internal"`
	FirstName     string         `bun:"first_name"`
	LastName      string         `bun:"last_name"`
	AnonymizedAt  *time.Time     `bun:"anonymized_at"`
	Tags          []string       `bun:"tags,type:json"`
	HasExtras     bool           `bun:"has_extras"`
	Version       int64          `bun:"version"`
	// Test marks a booking created in test mode (spec 15.4).
	Test bool `bun:"test"`
}

// BalanceDue is what remains to be collected (never negative).
func (b *BookingSummary) BalanceDue() money.Cents { return max(b.TotalAmount-b.PaidAmount, 0) }

func bookingSummarySelect(db bun.IDB, es EstablishmentScope) *bun.SelectQuery {
	return db.NewSelect().TableExpr("bookings AS bk").
		Join("JOIN customers AS c ON c.id = bk.customer_id").
		ColumnExpr("bk.id, bk.code, bk.status, bk.channel, bk.customer_id, bk.starts_on, bk.ends_on, bk.slots").
		ColumnExpr("bk.is_season_pass, bk.total_amount, bk.paid_amount, bk.notes_customer, bk.notes_internal, bk.version, bk.test").
		ColumnExpr("c.first_name, c.last_name, c.anonymized_at, c.tags").
		ColumnExpr("EXISTS (SELECT 1 FROM booking_line_extras AS ble WHERE ble.booking_id = bk.id) AS has_extras").
		Where("bk.establishment_id = ?", es.ID())
}

// BookingSummariesByID loads the summaries of the given bookings.
func BookingSummariesByID(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]BookingSummary, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []BookingSummary
	err := bookingSummarySelect(db, es).Where("bk.id IN (?)", bun.List(ids)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking summaries: %w", err)
	}
	return rows, nil
}

// BookingSummariesOn loads the bookings of es in statuses covering date.
func BookingSummariesOn(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, statuses []string) ([]BookingSummary, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []BookingSummary
	err := bookingSummarySelect(db, es).
		Where("bk.starts_on <= ?", date).Where("bk.ends_on >= ?", date).
		Where("bk.status IN (?)", bun.List(statuses)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking summaries on: %w", err)
	}
	return rows, nil
}

// LinePack is a booking line with its pack code.
type LinePack struct {
	LineID    string `bun:"line_id"`
	BookingID string `bun:"booking_id"`
	PackCode  string `bun:"pack_code"`
}

// LinePacksByBooking returns the lines (with pack code) of the bookings.
func LinePacksByBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) ([]LinePack, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	var rows []LinePack
	err := db.NewSelect().TableExpr("booking_lines AS bl").Join("JOIN packs AS pa ON pa.id = bl.pack_id").
		ColumnExpr("bl.id AS line_id, bl.booking_id, pa.code AS pack_code").
		Where("bl.establishment_id = ?", es.ID()).Where("bl.booking_id IN (?)", bun.List(bookingIDs)).
		OrderExpr("bl.booking_id, bl.sort_order, bl.id").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: line packs: %w", err)
	}
	return rows, nil
}

// InventoryPendingLine is a zone-mode booking line waiting for a spot.
type InventoryPendingLine struct {
	LineID         string         `bun:"line_id"`
	BookingID      string         `bun:"booking_id"`
	ZoneID         string         `bun:"zone_id"`
	PackID         string         `bun:"pack_id"`
	Code           string         `bun:"code"`
	Status         string         `bun:"status"`
	CustomerID     string         `bun:"customer_id"`
	FirstName      string         `bun:"first_name"`
	LastName       string         `bun:"last_name"`
	StartsOn       calendar.Date  `bun:"starts_on"`
	EndsOn         calendar.Date  `bun:"ends_on"`
	Slots          calendar.Slots `bun:"slots"`
	PolicySnapshot JSON           `bun:"policy_snapshot"`
	SpotKinds      []string       `bun:"requires_spot_kind,type:json"`
	IsSeasonPass   bool           `bun:"is_season_pass"`
	HoldExpiresAt  *time.Time     `bun:"hold_expires_at"`
}

// PendingLinesInRange returns the pending lines of bookings in statuses
// overlapping [from, to], optionally of zoneIDs or lineIDs.
func PendingLinesInRange(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, statuses, zoneIDs, lineIDs []string) ([]InventoryPendingLine, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []InventoryPendingLine
	q := db.NewSelect().TableExpr("booking_lines AS bl").
		Join("JOIN bookings AS bk ON bk.id = bl.booking_id").
		Join("JOIN customers AS c ON c.id = bk.customer_id").
		Join("JOIN packs AS pa ON pa.id = bl.pack_id").
		ColumnExpr("bl.id AS line_id, bl.booking_id, bl.zone_id, bl.pack_id, bk.code, bk.status, bk.customer_id").
		ColumnExpr("c.first_name, c.last_name, bk.starts_on, bk.ends_on, bk.slots, bk.policy_snapshot, pa.requires_spot_kind").
		ColumnExpr("bk.is_season_pass, bk.hold_expires_at").
		Where("bl.establishment_id = ?", es.ID()).
		Where("bl.assignment_status = ?", AssignmentPending).
		Where("bk.starts_on <= ?", to).Where("bk.ends_on >= ?", from).
		Where("bk.status IN (?)", bun.List(statuses)).
		OrderExpr("bk.starts_on, bk.id, bl.sort_order, bl.id")
	if len(zoneIDs) > 0 {
		q = q.Where("bl.zone_id IN (?)", bun.List(zoneIDs))
	}
	if len(lineIDs) > 0 {
		q = q.Where("bl.id IN (?)", bun.List(lineIDs))
	}
	if err := q.Scan(ctx, &rows); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pending lines: %w", err)
	}
	return rows, nil
}

// PreviousSpots returns, per customer, the spot of their most recent
// assigned line in another booking that ended before before (continuity of
// the auto-assignment, spec 7.5).
func PreviousSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, customerIDs []string, before calendar.Date) (map[string]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(customerIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		CustomerID string        `bun:"customer_id"`
		SpotID     string        `bun:"spot_id"`
		EndsOn     calendar.Date `bun:"ends_on"`
	}
	err := db.NewSelect().TableExpr("booking_lines AS bl").Join("JOIN bookings AS bk ON bk.id = bl.booking_id").
		ColumnExpr("bk.customer_id, bl.spot_id, bk.ends_on").
		Where("bl.establishment_id = ?", es.ID()).Where("bl.spot_id IS NOT NULL").
		Where("bk.customer_id IN (?)", bun.List(customerIDs)).
		Where("bk.ends_on < ?", before).
		Where("bk.status IN (?)", bun.List([]string{"confirmed", "checked_in", "completed"})).
		OrderExpr("bk.ends_on DESC, bl.id DESC").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: previous spots: %w", err)
	}
	for _, r := range rows {
		if _, ok := out[r.CustomerID]; !ok {
			out[r.CustomerID] = r.SpotID
		}
	}
	return out, nil
}

// AssignLineSpot sets the spot of a pending line only if it is still
// pending; it reports whether the line changed (concurrent assigners).
func AssignLineSpot(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID, spotID string, at time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().TableExpr("booking_lines").
		Set("spot_id = ?", spotID).Set("assignment_status = ?", AssignmentAssigned).Set("updated_at = ?", at).
		Where("id = ?", lineID).Where("establishment_id = ?", es.ID()).
		Where("assignment_status = ?", AssignmentPending).Exec(ctx)
	if err != nil {
		return false, wrapWrite("assign line spot", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ActiveSeasonRelease is a released, not revoked half-day of a pass.
type ActiveSeasonRelease struct {
	ID                  string        `bun:"id"`
	BookingID           string        `bun:"booking_id"`
	SpotID              string        `bun:"spot_id"`
	Date                calendar.Date `bun:"date"`
	Slot                calendar.Slot `bun:"slot"`
	ResoldBookingLineID *string       `bun:"resold_booking_line_id"`
}

// ActiveSeasonReleasesInRange returns the active releases between from and to.
func ActiveSeasonReleasesInRange(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date) ([]ActiveSeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []ActiveSeasonRelease
	err := db.NewSelect().TableExpr("season_releases AS sr").
		ColumnExpr("sr.id, sr.booking_id, sr.spot_id, sr.date, sr.slot, sr.resold_booking_line_id").
		Where("sr.establishment_id = ?", es.ID()).Where("sr.revoked_at IS NULL").
		Where("sr.date >= ?", from).Where("sr.date <= ?", to).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season releases in range: %w", err)
	}
	return rows, nil
}

// DayMovements counts arrivals, departures and check-ins of a day.
type DayMovements struct {
	Arrivals   int `bun:"arrivals"`
	Departures int `bun:"departures"`
	CheckedIn  int `bun:"checked_in"`
}

// DayMovementsOn counts the bookings of es in statuses starting / ending on
// date and the checked-in ones covering it (season passes excluded from
// arrivals and departures).
func DayMovementsOn(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, statuses []string) (DayMovements, error) {
	var out DayMovements
	if err := es.mustBeValid(); err != nil {
		return out, err
	}
	err := db.NewSelect().TableExpr("bookings AS bk").
		ColumnExpr("COALESCE(SUM(CASE WHEN bk.starts_on = ? AND bk.is_season_pass = ? THEN 1 ELSE 0 END), 0) AS arrivals", date, false).
		ColumnExpr("COALESCE(SUM(CASE WHEN bk.ends_on = ? AND bk.is_season_pass = ? THEN 1 ELSE 0 END), 0) AS departures", date, false).
		ColumnExpr("COALESCE(SUM(CASE WHEN bk.status = 'checked_in' THEN 1 ELSE 0 END), 0) AS checked_in").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.starts_on <= ?", date).Where("bk.ends_on >= ?", date).
		Where("bk.status IN (?)", bun.List(statuses)).Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return out, fmt.Errorf("store: day movements: %w", err)
	}
	return out, nil
}

// UnassignLineSpot puts a line back to pending (undo of a lost
// auto-assignment race inside the same transaction).
func UnassignLineSpot(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID string, at time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().TableExpr("booking_lines").
		Set("spot_id = NULL").Set("assignment_status = ?", AssignmentPending).Set("updated_at = ?", at).
		Where("id = ?", lineID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return wrapWrite("unassign line spot", err)
}
