package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Repository of bookings, booking_lines and booking_line_extras (owner:
// internal/domain/booking). Every function takes an EstablishmentScope and
// filters on establishment_id; writes go through the caller's transaction.
// Allocation rows are written only by the inventory repository: the
// functions below that read spot_allocations / pool_allocations are
// read-only views for the booking detail.

// ---- bookings ----------------------------------------------------------------

// InsertBooking inserts a booking of es. A duplicate code is ErrConflict.
func InsertBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, b *Booking) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	b.EstablishmentID = es.ID()
	normalizeBooking(b)
	_, err := db.NewInsert().Model(b).Exec(ctx)
	return wrapWrite("insert booking", err)
}

func normalizeBooking(b *Booking) {
	if len(b.PriceSnapshot) == 0 {
		b.PriceSnapshot = JSON("{}")
	}
	if len(b.PolicySnapshot) == 0 {
		b.PolicySnapshot = JSON("{}")
	}
	if b.Version == 0 {
		b.Version = 1
	}
}

// GetBooking loads a booking of es (ErrNotFound otherwise).
func GetBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	b := new(Booking)
	err := db.NewSelect().Model(b).Where("bk.id = ?", id).Where("bk.establishment_id = ?", es.ID()).Scan(ctx)
	return b, wrapRead("get booking", err)
}

// LockBooking loads a booking of es and locks its row for the rest of the
// transaction. The lock is a no-op UPDATE (portable: PostgreSQL takes the
// row lock, SQLite already holds the database write lock of the immediate
// transaction), so concurrent use cases on the same booking serialize and
// read the latest committed version.
func LockBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*Booking)(nil)).
		Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: lock booking: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetBooking(ctx, db, es, id)
}

// bookingMutableColumns are the columns rewritten by UpdateBookingVersioned.
var bookingMutableColumns = []string{
	"customer_id", "status", "starts_on", "ends_on", "slots", "formula", "season_id",
	"price_snapshot", "price_current", "total_amount", "deposit_due", "policy_snapshot", "promo_code_id", "agreement_id",
	"hold_expires_at", "hold_id", "notes_customer", "notes_internal", "confirmed_at", "checked_in_at",
	"cancelled_at", "cancel_reason", "version", "updated_at",
}

// UpdateBookingVersioned writes the mutable columns of b only when the stored
// version is still expected (optimistic concurrency) and sets b.Version to
// expected+1. paid_amount is never written here (ApplyBookingPayment owns
// it, INV-22). It reports false when the version did not match.
func UpdateBookingVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, b *Booking, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	b.Version = expected + 1
	normalizeBooking(b)
	res, err := db.NewUpdate().Model(b).Column(bookingMutableColumns...).
		Where("id = ?", b.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).
		Exec(ctx)
	if err != nil {
		b.Version = expected
		return false, wrapWrite("update booking", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		b.Version = expected
		return false, nil
	}
	return true, nil
}

// ApplyBookingPayment adds delta (negative for refunds) to paid_amount and
// bumps the version, returning the updated booking.
func ApplyBookingPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, delta money.Cents, at time.Time) (*Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*Booking)(nil)).
		Set("paid_amount = paid_amount + ?", int64(delta)).
		Set("version = version + 1").
		Set("updated_at = ?", at.UTC()).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: apply booking payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetBooking(ctx, db, es, id)
}

// LastBookingCode returns the greatest code of es starting with prefix
// ("" when none). Codes are zero padded, so the lexical maximum is the
// numeric one.
func LastBookingCode(ctx context.Context, db bun.IDB, es EstablishmentScope, prefix string) (string, error) {
	if err := es.mustBeValid(); err != nil {
		return "", err
	}
	var codes []string
	err := db.NewSelect().Model((*Booking)(nil)).Column("code").
		Where("establishment_id = ?", es.ID()).Where("code LIKE ?", escapeLike(prefix)+"%").
		OrderExpr("code DESC").Limit(1).Scan(ctx, &codes)
	if err != nil && !isNoRows(err) {
		return "", fmt.Errorf("store: last booking code: %w", err)
	}
	if len(codes) == 0 {
		return "", nil
	}
	return codes[0], nil
}

// escapeLike neutralizes the LIKE wildcards of s (no ESCAPE clause needed:
// booking prefixes and searches never contain them after this).
func escapeLike(s string) string {
	return strings.NewReplacer("%", "", "_", "").Replace(s)
}

// ---- lines and extras ------------------------------------------------------------

// InsertBookingLines inserts the lines of a booking of es.
func InsertBookingLines(ctx context.Context, db bun.IDB, es EstablishmentScope, lines []BookingLine) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	for i := range lines {
		lines[i].EstablishmentID = es.ID()
	}
	_, err := db.NewInsert().Model(&lines).Exec(ctx)
	return wrapWrite("insert booking lines", err)
}

// UpdateBookingLine rewrites the spot, assignment, price category and total of a line.
func UpdateBookingLine(ctx context.Context, db bun.IDB, es EstablishmentScope, l *BookingLine) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model(l).
		Column("spot_id", "assignment_status", "zone_id", "quantity", "price_category", "line_total", "updated_at").
		Where("id = ?", l.ID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("update booking line", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBookingLines deletes every line and extra of a booking of es. The
// caller must have released the allocations first (the cascade on
// spot/pool allocations would otherwise bypass the pool counters).
func DeleteBookingLines(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if _, err := db.NewDelete().Model((*BookingLineExtra)(nil)).
		Where("booking_id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx); err != nil {
		return fmt.Errorf("store: delete booking extras: %w", err)
	}
	if _, err := db.NewDelete().Model((*BookingLine)(nil)).
		Where("booking_id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx); err != nil {
		return fmt.Errorf("store: delete booking lines: %w", err)
	}
	return nil
}

// DeleteBookingExtras deletes the extras of a booking of es.
func DeleteBookingExtras(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewDelete().Model((*BookingLineExtra)(nil)).
		Where("booking_id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: delete booking extras: %w", err)
	}
	return nil
}

// ListBookingLines returns the lines of the given bookings of es, by booking
// then sort order.
func ListBookingLines(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs ...string) ([]BookingLine, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []BookingLine{}
	if len(bookingIDs) == 0 {
		return out, nil
	}
	err := db.NewSelect().Model(&out).
		Where("bl.establishment_id = ?", es.ID()).Where("bl.booking_id IN (?)", bun.List(bookingIDs)).
		OrderExpr("bl.booking_id ASC, bl.sort_order ASC, bl.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking lines: %w", err)
	}
	return out, nil
}

// InsertBookingLineExtras inserts extras of a booking of es.
func InsertBookingLineExtras(ctx context.Context, db bun.IDB, es EstablishmentScope, extras []BookingLineExtra) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(extras) == 0 {
		return nil
	}
	for i := range extras {
		extras[i].EstablishmentID = es.ID()
	}
	_, err := db.NewInsert().Model(&extras).Exec(ctx)
	return wrapWrite("insert booking extras", err)
}

// ListBookingLineExtras returns the extras of a booking of es.
func ListBookingLineExtras(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]BookingLineExtra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []BookingLineExtra{}
	err := db.NewSelect().Model(&out).
		Where("ble.establishment_id = ?", es.ID()).Where("ble.booking_id = ?", bookingID).
		OrderExpr("ble.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking extras: %w", err)
	}
	return out, nil
}

// ---- read-only views of allocations and history ---------------------------------

// ListBookingSpotAllocations returns the spot allocations of a booking of es.
func ListBookingSpotAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []SpotAllocation{}
	err := db.NewSelect().Model(&out).
		Where("sa.establishment_id = ?", es.ID()).Where("sa.booking_id = ?", bookingID).
		OrderExpr("sa.date ASC, sa.slot ASC, sa.spot_id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking spot allocations: %w", err)
	}
	return out, nil
}

// ListBookingPoolAllocations returns the pool allocations of a booking of es.
func ListBookingPoolAllocations(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]PoolAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []PoolAllocation{}
	err := db.NewSelect().Model(&out).
		Where("pla.establishment_id = ?", es.ID()).Where("pla.booking_id = ?", bookingID).
		OrderExpr("pla.date ASC, pla.slot ASC, pla.pool_id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking pool allocations: %w", err)
	}
	return out, nil
}

// ListBookingHistory returns the audit entries of a booking of es (entity
// type "booking") and those of its payments and refunds (entity type
// "payment", ids in paymentIDs), oldest first: the timeline of the booking.
func ListBookingHistory(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string, paymentIDs []string) ([]AuditEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []AuditEntry{}
	err := db.NewSelect().Model(&out).
		Where("establishment_id = ?", es.ID()).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereGroup(" OR ", func(g *bun.SelectQuery) *bun.SelectQuery {
				return g.Where("entity_type = ?", "booking").Where("entity_id = ?", bookingID)
			})
			if len(paymentIDs) > 0 {
				q = q.WhereGroup(" OR ", func(g *bun.SelectQuery) *bun.SelectQuery {
					return g.Where("entity_type = ?", "payment").Where("entity_id IN (?)", bun.List(paymentIDs))
				})
			}
			return q
		}).
		OrderExpr("created_at ASC, id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list booking history: %w", err)
	}
	return out, nil
}

// ---- lists ----------------------------------------------------------------------

// BookingFilter selects bookings for ListBookings. Zero fields do not filter.
type BookingFilter struct {
	// From keeps bookings ending on or after From; To those starting on or before To.
	From, To   *calendar.Date
	Statuses   []string
	CustomerID string
	SpotID     string
	Channel    string
	SeasonPass *bool
	BalanceDue *bool
	// Q matches the code prefix or the customer search text (name, phone, email).
	Q string
}

// BookingRow is a booking with the fields the list shows.
type BookingRow struct {
	Booking           `bun:",extend"`
	CustomerFirstName string   `bun:"customer_first_name"`
	CustomerLastName  string   `bun:"customer_last_name"`
	SpotLabels        []string `bun:"-"`
}

// ListBookings lists the bookings of es matching f, ordered by starts_on then
// id, with keyset pagination (the cursor encodes both).
func ListBookings(ctx context.Context, db bun.IDB, es EstablishmentScope, f BookingFilter, cursor string, limit int) (Page[BookingRow], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[BookingRow]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[BookingRow]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []BookingRow
	q := db.NewSelect().Model(&items).
		ColumnExpr("bk.*").
		ColumnExpr("cu.first_name AS customer_first_name, cu.last_name AS customer_last_name").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id").
		Where("bk.establishment_id = ?", es.ID()).
		OrderExpr("bk.starts_on ASC, bk.id ASC").
		Limit(int64(limit + 1))
	if after != "" {
		start, id, ok := strings.Cut(after, "|")
		d, perr := calendar.Parse(start)
		if !ok || perr != nil || id == "" {
			return Page[BookingRow]{}, ErrInvalidCursor
		}
		q = q.Where("(bk.starts_on > ? OR (bk.starts_on = ? AND bk.id > ?))", d, d, id)
	}
	if f.From != nil {
		q = q.Where("bk.ends_on >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("bk.starts_on <= ?", *f.To)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("bk.status IN (?)", bun.List(f.Statuses))
	}
	if f.CustomerID != "" {
		q = q.Where("bk.customer_id = ?", f.CustomerID)
	}
	if f.SpotID != "" {
		q = q.Where("EXISTS (SELECT 1 FROM booking_lines AS l WHERE l.booking_id = bk.id AND l.spot_id = ?)", f.SpotID)
	}
	if f.Channel != "" {
		q = q.Where("bk.channel = ?", f.Channel)
	}
	if f.SeasonPass != nil {
		q = q.Where("bk.is_season_pass = ?", *f.SeasonPass)
	}
	if f.BalanceDue != nil {
		if *f.BalanceDue {
			q = q.Where("bk.total_amount > bk.paid_amount")
		} else {
			q = q.Where("bk.total_amount <= bk.paid_amount")
		}
	}
	if s := strings.ToLower(strings.TrimSpace(escapeLike(f.Q))); s != "" {
		q = q.Where("(LOWER(bk.code) LIKE ? OR cu.search_text LIKE ?)", s+"%", "%"+s+"%")
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[BookingRow]{}, fmt.Errorf("store: list bookings: %w", err)
	}
	page := Page[BookingRow]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = EncodeCursor(last.StartsOn.String() + "|" + last.ID)
	}
	if page.Items == nil {
		page.Items = []BookingRow{}
	}
	if err := fillSpotLabels(ctx, db, es, page.Items); err != nil {
		return Page[BookingRow]{}, err
	}
	return page, nil
}

// fillSpotLabels sets SpotLabels from the assigned lines of each row.
func fillSpotLabels(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []BookingRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
		rows[i].SpotLabels = []string{}
	}
	var labels []struct {
		BookingID string `bun:"booking_id"`
		Label     string `bun:"label"`
	}
	err := db.NewSelect().TableExpr("booking_lines AS l").
		ColumnExpr("l.booking_id, sp.label").
		Join("JOIN spots AS sp ON sp.id = l.spot_id").
		Where("l.establishment_id = ?", es.ID()).Where("l.booking_id IN (?)", bun.List(ids)).
		OrderExpr("l.booking_id ASC, l.sort_order ASC").Scan(ctx, &labels)
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("store: booking spot labels: %w", err)
	}
	byID := make(map[string]int, len(rows))
	for i := range rows {
		byID[rows[i].ID] = i
	}
	for _, l := range labels {
		if i, ok := byID[l.BookingID]; ok {
			rows[i].SpotLabels = append(rows[i].SpotLabels, l.Label)
		}
	}
	return nil
}

// SpotLabels returns the labels of the given spots of es, by id.
func SpotLabels(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string) (map[string]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(spotIDs) == 0 {
		return out, nil
	}
	var rows []Spot
	err := db.NewSelect().Model(&rows).Column("id", "label").
		Where("establishment_id = ?", es.ID()).Where("id IN (?)", bun.List(spotIDs)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: spot labels: %w", err)
	}
	for _, s := range rows {
		out[s.ID] = s.Label
	}
	return out, nil
}

// BookingSpotsByIDs returns the spots of es with the given ids.
func BookingSpotsByIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []Spot{}
	if len(ids) == 0 {
		return out, nil
	}
	err := db.NewSelect().Model(&out).
		Where("sp.establishment_id = ?", es.ID()).Where("sp.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking spots: %w", err)
	}
	return out, nil
}

// ---- jobs -------------------------------------------------------------------------

// BookingRef identifies a booking with its establishment (system jobs).
type BookingRef struct {
	EstablishmentID string `bun:"establishment_id"`
	ID              string `bun:"id"`
}

// ListExpiredBookingHolds returns up to limit bookings, of any
// establishment, whose hold expired at now (system job: the caller builds a
// SystemEstablishmentScope for each).
func ListExpiredBookingHolds(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]BookingRef, error) {
	var out []BookingRef
	err := db.NewSelect().Model((*Booking)(nil)).Column("establishment_id", "id").
		Where("hold_expires_at IS NOT NULL").Where("hold_expires_at <= ?", now.UTC()).
		Where("status IN (?)", bun.List([]string{"held", "pending_payment", "awaiting_approval"})).
		OrderExpr("hold_expires_at ASC").Limit(int64(limit)).Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: expired booking holds: %w", err)
	}
	return out, nil
}

// ListBookingsEndedBefore returns up to limit bookings of es in one of
// statuses whose last day is before day (completion job).
func ListBookingsEndedBefore(ctx context.Context, db bun.IDB, es EstablishmentScope, day calendar.Date, statuses []string, limit int) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []string
	err := db.NewSelect().Model((*Booking)(nil)).Column("id").
		Where("establishment_id = ?", es.ID()).Where("ends_on < ?", day).
		Where("status IN (?)", bun.List(statuses)).
		OrderExpr("ends_on ASC, id ASC").Limit(int64(limit)).Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: bookings ended before: %w", err)
	}
	return out, nil
}

// ListEstablishmentTimezones returns the id and timezone of every
// non-archived establishment (system jobs).
func ListEstablishmentTimezones(ctx context.Context, db bun.IDB) (map[string]string, error) {
	var rows []Establishment
	err := db.NewSelect().Model(&rows).Column("id", "timezone").Where("archived_at IS NULL").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: establishment timezones: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, e := range rows {
		out[e.ID] = e.Timezone
	}
	return out, nil
}

// ---- promo codes and auto-assignment ---------------------------------------------

// ConsumePromoCode increments the uses of a promo code of es unless its
// global limit is reached (conditional UPDATE: concurrent bookings can
// never exceed max_uses). It reports false when the code is exhausted.
func ConsumePromoCode(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*PromoCode)(nil)).
		Set("uses = uses + 1").Set("updated_at = ?", at.UTC()).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("(max_uses IS NULL OR uses < max_uses)").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: consume promo code: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PendingLine is a line waiting for a spot with the dates of its booking.
type PendingLine struct {
	BookingLine `bun:",extend"`
	CustomerID  string         `bun:"customer_id"`
	StartsOn    calendar.Date  `bun:"starts_on"`
	EndsOn      calendar.Date  `bun:"ends_on"`
	Slots       calendar.Slots `bun:"slots"`
}

// ListPendingLines returns the pending lines of es (zone-mode zones) of
// bookings in one of statuses overlapping [from, to], optionally of one zone.
func ListPendingLines(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to calendar.Date, zoneID string, statuses []string) ([]PendingLine, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []PendingLine
	q := db.NewSelect().Model(&out).
		ColumnExpr("bl.*").
		ColumnExpr("bk.customer_id AS customer_id, bk.starts_on AS starts_on, bk.ends_on AS ends_on, bk.slots AS slots").
		Join("JOIN bookings AS bk ON bk.id = bl.booking_id").
		Where("bl.establishment_id = ?", es.ID()).Where("bl.assignment_status = ?", AssignmentPending).
		Where("bk.status IN (?)", bun.List(statuses)).
		Where("bk.ends_on >= ?", from).Where("bk.starts_on <= ?", to).
		OrderExpr("bk.starts_on ASC, bl.id ASC")
	if zoneID != "" {
		q = q.Where("bl.zone_id = ?", zoneID)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: pending lines: %w", err)
	}
	return out, nil
}

// ListZoneSpots returns the active spots of the given zones of es.
func ListZoneSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, zoneIDs []string) ([]Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []Spot{}
	if len(zoneIDs) == 0 {
		return out, nil
	}
	err := db.NewSelect().Model(&out).
		Where("sp.establishment_id = ?", es.ID()).Where("sp.zone_id IN (?)", bun.List(zoneIDs)).
		Where("sp.active = ?", true).OrderExpr("sp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: zone spots: %w", err)
	}
	return out, nil
}

// OccupiedHalfDays returns the (spot, date, slot) rows of spotIDs between
// from and to that block a sale at now: every allocation except holds
// already expired (INV-04, INV-05).
func OccupiedHalfDays(ctx context.Context, db bun.IDB, es EstablishmentScope, spotIDs []string, from, to calendar.Date, now time.Time) ([]SpotAllocation, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []SpotAllocation{}
	if len(spotIDs) == 0 {
		return out, nil
	}
	err := db.NewSelect().Model(&out).Column("spot_id", "date", "slot", "kind", "booking_id").
		Where("establishment_id = ?", es.ID()).Where("spot_id IN (?)", bun.List(spotIDs)).
		Where("date >= ?", from).Where("date <= ?", to).
		Where("(expires_at IS NULL OR expires_at > ?)", now.UTC()).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: occupied half-days: %w", err)
	}
	return out, nil
}
