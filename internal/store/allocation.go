package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of spot_allocations (spec 7.2). Owner:
// internal/domain/inventory. The unique index (spot_id, date, slot) is the
// only guard against double booking (INV-01): inserts use ON CONFLICT DO
// NOTHING and report how many rows the database accepted, so that a refused
// half-day never aborts the caller's transaction (PostgreSQL would otherwise
// poison it) and the conflicting rows can be read back for the error.

// allocationChunk bounds the rows of one multi-row INSERT (bind variables).
const allocationChunk = 400

// InsertSpotAllocations inserts rows (establishment forced to es) skipping
// the ones that violate the unique (spot_id, date, slot) index. It returns
// the number of rows actually inserted: fewer than len(rows) means that some
// half-days are taken and the caller must fail (and roll back).
func InsertSpotAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []SpotAllocation) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	total := 0
	for start := 0; start < len(rows); start += allocationChunk {
		chunk := rows[start:min(start+allocationChunk, len(rows))]
		for i := range chunk {
			chunk[i].EstablishmentID = es.ID()
		}
		res, err := db.NewInsert().Model(&chunk).On("CONFLICT (spot_id, date, slot) DO NOTHING").Exec(ctx)
		if err != nil {
			return total, wrapWrite("insert spot allocations", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("store: insert spot allocations: rows affected: %w", err)
		}
		total += int(n)
	}
	return total, nil
}

// SpotAllocationsInRange returns the rows of spotIDs (nil = every spot of
// the establishment) between from and to inclusive. With validOnly, hold
// rows whose expires_at <= now are left out (INV-04, INV-05).
func SpotAllocationsInRange(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string, from, to calendar.Date, validOnly bool, now time.Time) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if spotIDs != nil && len(spotIDs) == 0 {
		return nil, nil
	}
	var rows []SpotAllocation
	q := db.NewSelect().Model(&rows).
		Where("sa.establishment_id = ?", es.ID()).
		Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		OrderExpr("sa.spot_id, sa.date, sa.slot")
	if spotIDs != nil {
		q = q.Where("sa.spot_id IN (?)", bun.List(spotIDs))
	}
	if validOnly {
		q = q.Where("(sa.kind <> 'hold' OR sa.expires_at > ?)", now.UTC())
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: spot allocations in range: %w", err)
	}
	return rows, nil
}

// DeleteExpiredSpotHolds deletes the hold rows of spotIDs between from and
// to whose expires_at <= now (local cleanup of spec 7.2, INV-05). It returns
// the deleted rows.
func DeleteExpiredSpotHolds(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string, from, to calendar.Date, now time.Time) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(spotIDs) == 0 {
		return nil, nil
	}
	var rows []SpotAllocation
	err := db.NewDelete().Model(&rows).
		Where("establishment_id = ?", es.ID()).
		Where("kind = ?", AllocationHold).Where("expires_at <= ?", now.UTC()).
		Where("spot_id IN (?)", bun.List(spotIDs)).
		Where("date >= ?", from).Where("date <= ?", to).
		Returning("*").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: delete expired spot holds: %w", err)
	}
	return rows, nil
}

// AllocationOwner selects allocation rows by owner. Exactly one of
// BookingID, HoldID and BlockID is set; LineIDs and From narrow a booking
// selection; Kind (optional) restricts the kind; DateSlots (optional)
// restricts to the listed half-days.
type AllocationOwner struct {
	BookingID string
	LineIDs   []string
	HoldID    string
	BlockID   string
	SpotID    string
	Kind      string
	From      *calendar.Date
	DateSlots map[calendar.Slot][]calendar.Date
}

// validate checks that exactly one owner is selected.
func (o AllocationOwner) validate() error {
	n := 0
	for _, id := range []string{o.BookingID, o.HoldID, o.BlockID} {
		if id != "" {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("store: allocation owner: exactly one owner required")
	}
	return nil
}

// filter adds the owner conditions to q (establishment included).
func (o AllocationOwner) filter(es EstablishmentScope) func(bun.QueryBuilder) bun.QueryBuilder {
	return func(q bun.QueryBuilder) bun.QueryBuilder {
		q = q.Where("establishment_id = ?", es.ID())
		switch {
		case o.BookingID != "":
			q = q.Where("booking_id = ?", o.BookingID)
			if len(o.LineIDs) > 0 {
				q = q.Where("booking_line_id IN (?)", bun.List(o.LineIDs))
			}
		case o.HoldID != "":
			q = q.Where("hold_id = ?", o.HoldID)
		default:
			q = q.Where("block_id = ?", o.BlockID)
		}
		if o.SpotID != "" {
			q = q.Where("spot_id = ?", o.SpotID)
		}
		if o.Kind != "" {
			q = q.Where("kind = ?", o.Kind)
		}
		if o.From != nil {
			q = q.Where("date >= ?", *o.From)
		}
		if o.DateSlots != nil {
			if len(o.DateSlots) == 0 {
				return q.Where("1 = 0")
			}
			q = q.WhereGroup(" AND ", func(g bun.QueryBuilder) bun.QueryBuilder {
				for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
					if ds := o.DateSlots[slot]; len(ds) > 0 {
						g = g.WhereGroup(" OR ", func(g bun.QueryBuilder) bun.QueryBuilder {
							return g.Where("slot = ?", slot).Where("date IN (?)", bun.List(ds))
						})
					}
				}
				return g
			})
		}
		return q
	}
}

// DeleteSpotAllocations deletes the spot rows of an owner and returns them.
func DeleteSpotAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, o AllocationOwner) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	var rows []SpotAllocation
	q := db.NewDelete().Model(&rows).ApplyQueryBuilder(o.filter(es))
	if err := q.Returning("*").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: delete spot allocations: %w", err)
	}
	return rows, nil
}

// ListSpotAllocations returns the spot rows of an owner.
func ListSpotAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, o AllocationOwner) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	var rows []SpotAllocation
	q := db.NewSelect().Model(&rows).ApplyQueryBuilder(o.filter(es)).OrderExpr("spot_id, date, slot")
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list spot allocations: %w", err)
	}
	return rows, nil
}

// CountExpiredHoldRows counts the hold rows (spot and pool) of a booking
// whose expires_at <= now.
func CountExpiredHoldRows(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	total := 0
	for _, table := range []string{"spot_allocations", "pool_allocations"} {
		n, err := db.NewSelect().TableExpr(table).
			Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).
			Where("kind = ?", AllocationHold).Where("expires_at <= ?", now.UTC()).Count(ctx)
		if err != nil {
			return 0, fmt.Errorf("store: count expired holds: %w", err)
		}
		total += int(n)
	}
	return total, nil
}

// ConfirmSpotHolds turns the valid hold rows of a booking into kind
// (booking or season) without expiry. It returns the rows changed.
func ConfirmSpotHolds(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID, kind string, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	res, err := db.NewUpdate().TableExpr("spot_allocations").
		Set("kind = ?", kind).Set("expires_at = NULL").
		Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).
		Where("kind = ?", AllocationHold).Where("expires_at > ?", now.UTC()).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("confirm spot holds", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ExtendHoldRows moves expires_at of the valid hold rows (spot and pool) of
// a booking. It returns the rows changed.
func ExtendHoldRows(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, expiresAt, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	total := 0
	for _, table := range []string{"spot_allocations", "pool_allocations"} {
		res, err := db.NewUpdate().TableExpr(table).
			Set("expires_at = ?", expiresAt.UTC()).
			Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).
			Where("kind = ?", AllocationHold).Where("expires_at > ?", now.UTC()).Exec(ctx)
		if err != nil {
			return 0, wrapWrite("extend holds", err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// BookingHoldState is the subset of a booking the inventory needs to
// confirm its holds.
type BookingHoldState struct {
	ID            string     `bun:"id"`
	Status        string     `bun:"status"`
	IsSeasonPass  bool       `bun:"is_season_pass"`
	HoldExpiresAt *time.Time `bun:"hold_expires_at"`
}

// GetBookingHoldState reads the hold state of a booking of es.
func GetBookingHoldState(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (*BookingHoldState, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := new(BookingHoldState)
	err := db.NewSelect().TableExpr("bookings").
		ColumnExpr("id, status, is_season_pass, hold_expires_at").
		Where("id = ?", bookingID).Where("establishment_id = ?", es.ID()).Scan(ctx, out)
	return out, wrapRead("get booking hold state", err)
}

// OnlineSpotCount is the number of valid spot rows of a zone sold through a
// quota-bound channel (anything but desk) on one half-day.
type OnlineSpotCount struct {
	ZoneID string        `bun:"zone_id"`
	Date   calendar.Date `bun:"date"`
	Slot   calendar.Slot `bun:"slot"`
	N      int           `bun:"n"`
}

// OnlineSpotCounts counts, per zone and half-day, the valid spot rows of
// zoneIDs owned by bookings or panel holds of a channel other than desk
// (online quota of seat zones, spec 7.6).
func OnlineSpotCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string, from, to calendar.Date, now time.Time) ([]OnlineSpotCount, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(zoneIDs) == 0 {
		return nil, nil
	}
	var rows []OnlineSpotCount
	err := db.NewSelect().TableExpr("spot_allocations AS sa").
		Join("JOIN spots AS sp ON sp.id = sa.spot_id").
		Join("LEFT JOIN bookings AS bk ON bk.id = sa.booking_id").
		Join("LEFT JOIN holds AS ho ON ho.id = sa.hold_id").
		ColumnExpr("sp.zone_id, sa.date, sa.slot, COUNT(*) AS n").
		Where("sa.establishment_id = ?", es.ID()).
		Where("sp.zone_id IN (?)", bun.List(zoneIDs)).
		Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		Where("(sa.kind <> 'hold' OR sa.expires_at > ?)", now.UTC()).
		Where("(bk.channel <> 'desk' OR ho.channel <> 'desk')").
		GroupExpr("sp.zone_id, sa.date, sa.slot").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: online spot counts: %w", err)
	}
	return rows, nil
}
