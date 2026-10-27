package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Agreement versions and the agreement quota (M8, spec 30.2.2, ADR 0028,
// INV-103). The cap of agreement sales is the counter
// agreement_quota_usage.used, changed only by conditional UPDATEs (and
// protected by CHECK used <= max_units); agreement_quota_allocations is
// its detail per booking line and half-day, deleted (and the counter
// decremented) when the allocations of the line are released. The
// capacity reserved to an agreement is enforced by internal/domain/inventory
// after the insert, with the agreements locked (LockAgreements).

// ---- versions -----------------------------------------------------------------

// InsertAgreementVersion stores a version snapshot (unique per version).
func InsertAgreementVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, v *AgreementVersion) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	v.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(v).Exec(ctx)
	return wrapWrite("insert agreement version", err)
}

// ListAgreementVersions lists the versions of an agreement, newest first.
func ListAgreementVersions(ctx context.Context, db bun.IDB, es EstablishmentScope, agreementID string) ([]AgreementVersion, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []AgreementVersion
	err := db.NewSelect().Model(&rows).Where("agv.establishment_id = ?", es.ID()).
		Where("agv.agreement_id = ?", agreementID).OrderExpr("agv.version DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list agreement versions: %w", err)
	}
	return rows, nil
}

// ---- quota configuration ---------------------------------------------------------

// ListQuotaAgreements returns the active agreements with a quota
// configuration (few per establishment).
func ListQuotaAgreements(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]Agreement, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Agreement
	err := db.NewSelect().Model(&rows).Where("ag.establishment_id = ?", es.ID()).
		Where("ag.active = ?", true).Where("ag.quota IS NOT NULL").OrderExpr("ag.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list quota agreements: %w", err)
	}
	return rows, nil
}

// LockAgreements takes the row lock of the agreements ids, in id order, so
// that the sales touching their reserved capacity serialize.
func LockAgreements(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	sorted := slices.Sorted(slices.Values(dedupe(ids)))
	for _, id := range sorted { // one statement per row: a deterministic lock order
		if _, err := db.NewUpdate().Model((*Agreement)(nil)).Set("version = version").
			Where("establishment_id = ?", es.ID()).Where("id = ?", id).Exec(ctx); err != nil {
			return wrapWrite("lock agreements", err)
		}
	}
	return nil
}

// ---- usage counter ----------------------------------------------------------------

// QuotaKey identifies one half-day of an agreement quota.
type QuotaKey struct {
	AgreementID string
	Date        calendar.Date
	Slot        calendar.Slot
}

// EnsureAgreementQuotaRows inserts the missing counters (used 0).
func EnsureAgreementQuotaRows(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []AgreementQuotaUsage) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	for start := 0; start < len(rows); start += allocationChunk {
		chunk := rows[start:min(start+allocationChunk, len(rows))]
		for i := range chunk {
			chunk[i].EstablishmentID = es.ID()
		}
		if _, err := db.NewInsert().Model(&chunk).On("CONFLICT (agreement_id, date, slot) DO NOTHING").Exec(ctx); err != nil {
			return wrapWrite("ensure agreement quota", err)
		}
	}
	return nil
}

// QuotaLimits is the current quota configuration of an agreement for one
// half-day, as the loaded agreement says it now (not as the counter row
// recorded it at the first sale).
type QuotaLimits struct {
	MaxUnits  *int
	Reserved  int
	ReleaseOn calendar.Date
}

// ConsumeAgreementQuota adds n to the counter of k with one conditional
// UPDATE (used + n <= lim.MaxUnits when a cap is set) and reports whether
// the row was updated; the row is refreshed with the current limits, so a
// cap lowered, raised or added after the first sale of the half-day is
// enforced from the next sale on. The row must exist
// (EnsureAgreementQuotaRows). Concurrent consumers serialize on the row
// lock (INV-103).
func ConsumeAgreementQuota(ctx context.Context, db bun.IDB, es EstablishmentScope, k QuotaKey, n int, lim QuotaLimits, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	q := db.NewUpdate().TableExpr("agreement_quota_usage").
		Set("used = used + ?", n).Set("max_units = ?", lim.MaxUnits).Set("reserved = ?", lim.Reserved).
		Set("release_on = ?", lim.ReleaseOn).Set("updated_at = ?", now.UTC()).
		Where("establishment_id = ?", es.ID()).
		Where("agreement_id = ?", k.AgreementID).Where("date = ?", k.Date).Where("slot = ?", k.Slot)
	if lim.MaxUnits != nil {
		q = q.Where("used + ? <= ?", n, *lim.MaxUnits)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return false, wrapWrite("consume agreement quota", err)
	}
	m, _ := res.RowsAffected()
	return m == 1, nil
}

// InsertAgreementQuotaAllocations inserts the detail rows of a sale.
func InsertAgreementQuotaAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []AgreementQuotaAllocation) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	for start := 0; start < len(rows); start += allocationChunk {
		chunk := rows[start:min(start+allocationChunk, len(rows))]
		for i := range chunk {
			chunk[i].EstablishmentID = es.ID()
		}
		if _, err := db.NewInsert().Model(&chunk).Exec(ctx); err != nil {
			return wrapWrite("insert agreement quota allocations", err)
		}
	}
	return nil
}

// LineHalfDay identifies a released half-day of a booking line.
type LineHalfDay struct {
	LineID string
	Date   calendar.Date
	Slot   calendar.Slot
}

// ReleaseAgreementQuota deletes the quota detail of the released half-days
// of the bookings (all of them when keys is nil) and decrements the
// counters by exactly what was deleted. It returns the rows deleted.
func ReleaseAgreementQuota(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string, keys map[LineHalfDay]bool, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	if len(bookingIDs) == 0 {
		return 0, nil
	}
	var rows []AgreementQuotaAllocation
	err := db.NewSelect().Model(&rows).Where("aqa.establishment_id = ?", es.ID()).
		Where("aqa.booking_id IN (?)", bun.List(dedupe(bookingIDs))).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return 0, fmt.Errorf("store: release agreement quota: %w", err)
	}
	var ids []string
	dec := map[QuotaKey]int{}
	for _, r := range rows {
		if keys != nil && !keys[LineHalfDay{LineID: r.BookingLineID, Date: r.Date, Slot: r.Slot}] {
			continue
		}
		ids = append(ids, r.ID)
		dec[QuotaKey{AgreementID: r.AgreementID, Date: r.Date, Slot: r.Slot}] += r.Quantity
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for start := 0; start < len(ids); start += allocationChunk {
		chunk := ids[start:min(start+allocationChunk, len(ids))]
		if _, err := db.NewDelete().Model((*AgreementQuotaAllocation)(nil)).
			Where("establishment_id = ?", es.ID()).Where("id IN (?)", bun.List(chunk)).Exec(ctx); err != nil {
			return 0, wrapWrite("release agreement quota", err)
		}
	}
	keysSorted := make([]QuotaKey, 0, len(dec))
	for k := range dec {
		keysSorted = append(keysSorted, k)
	}
	slices.SortFunc(keysSorted, func(a, b QuotaKey) int {
		switch {
		case a.AgreementID != b.AgreementID:
			if a.AgreementID < b.AgreementID {
				return -1
			}
			return 1
		case !a.Date.Equal(b.Date):
			if a.Date.Before(b.Date) {
				return -1
			}
			return 1
		case a.Slot < b.Slot:
			return -1
		case a.Slot > b.Slot:
			return 1
		}
		return 0
	})
	for _, k := range keysSorted {
		if _, err := db.NewUpdate().TableExpr("agreement_quota_usage").
			Set("used = CASE WHEN used >= ? THEN used - ? ELSE 0 END", dec[k], dec[k]).Set("updated_at = ?", now.UTC()).
			Where("establishment_id = ?", es.ID()).
			Where("agreement_id = ?", k.AgreementID).Where("date = ?", k.Date).Where("slot = ?", k.Slot).
			Exec(ctx); err != nil {
			return 0, wrapWrite("release agreement quota", err)
		}
	}
	return len(ids), nil
}

// AgreementQuotaUsageRange returns the counters of an agreement between
// from and to (inclusive), ordered by date and slot.
func AgreementQuotaUsageRange(ctx context.Context, db bun.IDB, es EstablishmentScope, agreementID string, from, to calendar.Date) ([]AgreementQuotaUsage, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []AgreementQuotaUsage
	err := db.NewSelect().Model(&rows).Where("aqu.establishment_id = ?", es.ID()).
		Where("aqu.agreement_id = ?", agreementID).Where("aqu.date >= ?", from).Where("aqu.date <= ?", to).
		OrderExpr("aqu.date ASC, aqu.slot ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: agreement quota usage: %w", err)
	}
	return rows, nil
}

// AgreementUsedOn returns the used counters of the agreements ids on the
// half-days between from and to, keyed by QuotaKey.
func AgreementUsedOn(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string, from, to calendar.Date) (map[QuotaKey]int, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[QuotaKey]int{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []AgreementQuotaUsage
	err := db.NewSelect().Model(&rows).Where("aqu.establishment_id = ?", es.ID()).
		Where("aqu.agreement_id IN (?)", bun.List(dedupe(ids))).Where("aqu.date >= ?", from).Where("aqu.date <= ?", to).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: agreement used: %w", err)
	}
	for _, r := range rows {
		out[QuotaKey{AgreementID: r.AgreementID, Date: r.Date, Slot: r.Slot}] = r.Used
	}
	return out, nil
}

// ---- occupancy for the reserved capacity ----------------------------------------

// ZoneHalfDay is a count per zone and half-day.
type ZoneHalfDay struct {
	ZoneID string        `bun:"zone_id"`
	Date   calendar.Date `bun:"date"`
	Slot   calendar.Slot `bun:"slot"`
	N      int           `bun:"n"`
}

// SeatOccupancy counts the valid spot allocations (expired holds excluded)
// of the spots of zoneIDs per zone and half-day between from and to.
func SeatOccupancy(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string, from, to calendar.Date, now time.Time) ([]ZoneHalfDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(zoneIDs) == 0 {
		return nil, nil
	}
	var rows []ZoneHalfDay
	err := db.NewSelect().TableExpr("spot_allocations AS sa").
		Join("JOIN spots AS sp ON sp.id = sa.spot_id").
		ColumnExpr("sp.zone_id, sa.date, sa.slot, COUNT(*) AS n").
		Where("sa.establishment_id = ?", es.ID()).
		Where("sp.zone_id IN (?)", bun.List(dedupe(zoneIDs))).
		Where("sp.active = ?", true).
		Where("sa.date >= ?", from).Where("sa.date <= ?", to).
		Where("(sa.kind <> 'hold' OR sa.expires_at > ?)", now.UTC()).
		GroupExpr("sp.zone_id, sa.date, sa.slot").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: seat occupancy: %w", err)
	}
	return rows, nil
}

// GetAgreementByCode loads an agreement by code, ignoring case.
func GetAgreementByCode(ctx context.Context, db bun.IDB, es EstablishmentScope, code string) (*Agreement, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	a := new(Agreement)
	err := db.NewSelect().Model(a).Where("ag.establishment_id = ?", es.ID()).
		Where("lower(ag.code) = ?", strings.ToLower(strings.TrimSpace(code))).Limit(1).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get agreement by code", err)
	}
	return a, nil
}
