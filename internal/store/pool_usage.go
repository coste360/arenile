package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of pool_usage and pool_allocations (spec 7.3).
// Owner: internal/domain/inventory. pool_usage is a counter changed only by
// the conditional UPDATE of ConsumePool (INV-02); every change has its
// pool_allocations detail row (INV-03).

// PoolKey identifies one counter.
type PoolKey struct {
	PoolID string
	Date   calendar.Date
	Slot   calendar.Slot
}

// EnsurePoolUsageRows creates the missing counters of keys with zero values
// (INSERT ... ON CONFLICT DO NOTHING).
func EnsurePoolUsageRows(ctx context.Context, db bun.IDB, es EstablishmentScope, keys []PoolKey, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	for start := 0; start < len(keys); start += allocationChunk {
		chunk := keys[start:min(start+allocationChunk, len(keys))]
		rows := make([]PoolUsage, len(chunk))
		for i, k := range chunk {
			rows[i] = PoolUsage{EstablishmentID: es.ID(), PoolID: k.PoolID, Date: k.Date, Slot: k.Slot, UpdatedAt: now}
		}
		if _, err := db.NewInsert().Model(&rows).On("CONFLICT (pool_id, date, slot) DO NOTHING").Exec(ctx); err != nil {
			return wrapWrite("ensure pool usage", err)
		}
	}
	return nil
}

// PoolConsumption is one conditional increment of a counter.
type PoolConsumption struct {
	Key PoolKey
	// Quantity is added to held (Held) or used.
	Quantity int
	Held     bool
	// Capacity is the effective capacity of the half-day (overrides applied).
	Capacity int
	// Online adds Quantity to online_used (quota-bound channels).
	Online bool
	// OnlineLimit, when not nil, also requires online_used + Quantity <=
	// limit (online quota, spec 7.6); it implies Online.
	OnlineLimit *int
}

// ConsumePool applies c with one conditional UPDATE:
//
//	UPDATE pool_usage SET held|used = ... + n [, online_used = online_used + n]
//	 WHERE pk AND used + held + n <= capacity [AND online_used + n <= limit]
//
// and reports whether the row was updated. The row must exist
// (EnsurePoolUsageRows). Concurrent consumers serialize on the row lock.
func ConsumePool(ctx context.Context, db bun.IDB, es EstablishmentScope, c PoolConsumption, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	col := "used"
	if c.Held {
		col = "held"
	}
	q := db.NewUpdate().TableExpr("pool_usage").
		Set(col+" = "+col+" + ?", c.Quantity).
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("pool_id = ?", c.Key.PoolID).Where("date = ?", c.Key.Date).Where("slot = ?", c.Key.Slot).
		Where("used + held + ? <= ?", c.Quantity, c.Capacity)
	if c.Online || c.OnlineLimit != nil {
		q = q.Set("online_used = online_used + ?", c.Quantity)
	}
	if c.OnlineLimit != nil {
		q = q.Where("online_used + ? <= ?", c.Quantity, *c.OnlineLimit)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return false, wrapWrite("consume pool", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PoolDelta is a signed change of a counter (releases, confirmations).
type PoolDelta struct {
	Key    PoolKey
	Used   int
	Held   int
	Online int
}

// ApplyPoolDeltas adds the deltas to the counters (CHECK constraints keep
// them non-negative).
func ApplyPoolDeltas(ctx context.Context, db bun.IDB, es EstablishmentScope, deltas []PoolDelta, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	for _, d := range deltas {
		if d.Used == 0 && d.Held == 0 && d.Online == 0 {
			continue
		}
		_, err := db.NewUpdate().TableExpr("pool_usage").
			Set("used = used + ?", d.Used).Set("held = held + ?", d.Held).
			Set("online_used = online_used + ?", d.Online).Set("updated_at = ?", now).
			Where("establishment_id = ?", es.ID()).
			Where("pool_id = ?", d.Key.PoolID).Where("date = ?", d.Key.Date).Where("slot = ?", d.Key.Slot).
			Exec(ctx)
		if err != nil {
			return wrapWrite("apply pool delta", err)
		}
	}
	return nil
}

// GetPoolUsage reads one counter (zero value when the row does not exist).
func GetPoolUsage(ctx context.Context, db bun.IDB, es EstablishmentScope, k PoolKey) (*PoolUsage, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	u := new(PoolUsage)
	err := db.NewSelect().Model(u).Where("pu.establishment_id = ?", es.ID()).
		Where("pu.pool_id = ?", k.PoolID).Where("pu.date = ?", k.Date).Where("pu.slot = ?", k.Slot).Scan(ctx)
	if isNoRows(err) {
		return &PoolUsage{EstablishmentID: es.ID(), PoolID: k.PoolID, Date: k.Date, Slot: k.Slot}, nil
	}
	return u, wrapRead("get pool usage", err)
}

// ListPoolUsage returns the counters of poolIDs between from and to.
func ListPoolUsage(ctx context.Context, db bun.IDB, es EstablishmentScope, poolIDs []string, from, to calendar.Date) ([]PoolUsage, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(poolIDs) == 0 {
		return nil, nil
	}
	var rows []PoolUsage
	err := db.NewSelect().Model(&rows).Where("pu.establishment_id = ?", es.ID()).
		Where("pu.pool_id IN (?)", bun.List(poolIDs)).
		Where("pu.date >= ?", from).Where("pu.date <= ?", to).
		OrderExpr("pu.pool_id, pu.date, pu.slot").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list pool usage: %w", err)
	}
	return rows, nil
}

// InsertPoolAllocations inserts detail rows (establishment forced to es).
func InsertPoolAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []PoolAllocation) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	for start := 0; start < len(rows); start += allocationChunk {
		chunk := rows[start:min(start+allocationChunk, len(rows))]
		for i := range chunk {
			chunk[i].EstablishmentID = es.ID()
		}
		if _, err := db.NewInsert().Model(&chunk).Exec(ctx); err != nil {
			return wrapWrite("insert pool allocations", err)
		}
	}
	return nil
}

// DeletePoolAllocations deletes the detail rows of an owner (booking or
// hold; Kind, LineIDs and From narrow it) and returns them, so that the
// caller decrements exactly what was deleted.
func DeletePoolAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, o AllocationOwner) ([]PoolAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if o.BlockID != "" || o.SpotID != "" {
		return nil, nil // blocks and spot-scoped selections never own pool rows
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	var rows []PoolAllocation
	q := db.NewDelete().Model(&rows).ApplyQueryBuilder(o.filter(es))
	if err := q.Returning("*").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: delete pool allocations: %w", err)
	}
	return rows, nil
}

// DeleteExpiredPoolHolds deletes the expired hold rows of poolIDs between
// from and to and returns them (local cleanup, INV-05).
func DeleteExpiredPoolHolds(ctx context.Context, db bun.IDB, es EstablishmentScope, poolIDs []string, from, to calendar.Date, now time.Time) ([]PoolAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(poolIDs) == 0 {
		return nil, nil
	}
	var rows []PoolAllocation
	err := db.NewDelete().Model(&rows).
		Where("establishment_id = ?", es.ID()).
		Where("kind = ?", AllocationHold).Where("expires_at <= ?", now.UTC()).
		Where("pool_id IN (?)", bun.List(poolIDs)).
		Where("date >= ?", from).Where("date <= ?", to).
		Returning("*").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: delete expired pool holds: %w", err)
	}
	return rows, nil
}

// ConfirmPoolHolds turns the valid hold detail rows of a booking into
// booking rows and returns them (the caller moves held -> used).
func ConfirmPoolHolds(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, now time.Time) ([]PoolAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []PoolAllocation
	err := db.NewUpdate().Model(&rows).
		Set("kind = ?", AllocationBooking).Set("expires_at = NULL").
		Where("establishment_id = ?", es.ID()).Where("booking_id = ?", bookingID).
		Where("kind = ?", AllocationHold).Where("expires_at > ?", now.UTC()).
		Returning("*").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: confirm pool holds: %w", err)
	}
	return rows, nil
}

// PoolOccupancy is the valid consumption of one counter computed from the
// detail rows: expired holds excluded (INV-03, INV-05).
type PoolOccupancy struct {
	PoolID string        `bun:"pool_id"`
	Date   calendar.Date `bun:"date"`
	Slot   calendar.Slot `bun:"slot"`
	Used   int           `bun:"used"`
	Held   int           `bun:"held"`
	Online int           `bun:"online"`
}

// PoolOccupancyInRange aggregates the valid detail rows of poolIDs between
// from and to (one query). excludeBookingID / excludeHoldID leave out the
// rows of the owner being modified (dry runs).
func PoolOccupancyInRange(ctx context.Context, db bun.IDB, es EstablishmentScope, poolIDs []string, from, to calendar.Date, now time.Time, excludeBookingID, excludeHoldID string) ([]PoolOccupancy, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(poolIDs) == 0 {
		return nil, nil
	}
	var rows []PoolOccupancy
	q := db.NewSelect().TableExpr("pool_allocations AS pla").
		ColumnExpr("pla.pool_id, pla.date, pla.slot").
		ColumnExpr("COALESCE(SUM(CASE WHEN pla.kind = 'booking' THEN pla.quantity ELSE 0 END), 0) AS used").
		ColumnExpr("COALESCE(SUM(CASE WHEN pla.kind = 'hold' THEN pla.quantity ELSE 0 END), 0) AS held").
		ColumnExpr("COALESCE(SUM(CASE WHEN pla.channel <> 'desk' THEN pla.quantity ELSE 0 END), 0) AS online").
		Where("pla.establishment_id = ?", es.ID()).
		Where("pla.pool_id IN (?)", bun.List(poolIDs)).
		Where("pla.date >= ?", from).Where("pla.date <= ?", to).
		Where("(pla.kind <> 'hold' OR pla.expires_at > ?)", now.UTC()).
		GroupExpr("pla.pool_id, pla.date, pla.slot")
	if excludeBookingID != "" {
		q = q.Where("(pla.booking_id IS NULL OR pla.booking_id <> ?)", excludeBookingID)
	}
	if excludeHoldID != "" {
		q = q.Where("(pla.hold_id IS NULL OR pla.hold_id <> ?)", excludeHoldID)
	}
	if err := q.Scan(ctx, &rows); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pool occupancy: %w", err)
	}
	return rows, nil
}
