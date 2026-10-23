package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Repository of season passes and their releases (owner:
// internal/domain/pass). A season pass is a booking with is_season_pass =
// true (ADR 0010); the functions here read that view of bookings and own
// the season_releases table.

// SeasonPassFilter selects season passes in ListSeasonPasses (empty = any).
type SeasonPassFilter struct {
	SeasonID   string
	CustomerID string
	SpotID     string
	Status     string
}

// SeasonPassView is a season pass as listed by the panel: the booking with
// its (first) spot line, the holder's display name and the release totals.
type SeasonPassView struct {
	Booking             Booking
	LineID              string
	PackID              string
	SpotID              string
	SpotLabel           string
	CustomerDisplayName string
	// ReleasedDays counts the half-days released and not revoked.
	ReleasedDays int
	// CreditAmount is the credit accrued by resold releases.
	CreditAmount money.Cents
}

// ListSeasonPasses lists the season passes of es by id (creation order).
func ListSeasonPasses(ctx context.Context, db bun.IDB, es EstablishmentScope, f SeasonPassFilter, cursor string, limit int) (Page[SeasonPassView], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[SeasonPassView]{}, err
	}
	var items []Booking
	q := db.NewSelect().Model(&items).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.is_season_pass = ?", true)
	if f.SeasonID != "" {
		q = q.Where("bk.season_id = ?", f.SeasonID)
	}
	if f.CustomerID != "" {
		q = q.Where("bk.customer_id = ?", f.CustomerID)
	}
	if f.Status != "" {
		q = q.Where("bk.status = ?", f.Status)
	}
	if f.SpotID != "" {
		q = q.Where("bk.id IN (SELECT bl.booking_id FROM booking_lines AS bl WHERE bl.establishment_id = ? AND bl.spot_id = ?)", es.ID(), f.SpotID)
	}
	page, err := catalogKeysetPage(ctx, q, &items, "bk", cursor, limit, func(b *Booking) string { return b.ID }, "list season passes")
	if err != nil {
		return Page[SeasonPassView]{}, err
	}
	views, err := seasonPassViews(ctx, db, es, page.Items)
	if err != nil {
		return Page[SeasonPassView]{}, err
	}
	return Page[SeasonPassView]{Items: views, NextCursor: page.NextCursor}, nil
}

// SeasonPassesOfCustomer returns every season pass of a customer, newest first.
func SeasonPassesOfCustomer(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]SeasonPassView, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Booking
	err := db.NewSelect().Model(&items).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.is_season_pass = ?", true).
		Where("bk.customer_id = ?", customerID).
		OrderExpr("bk.starts_on DESC, bk.id DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season passes of customer: %w", err)
	}
	return seasonPassViews(ctx, db, es, items)
}

// GetSeasonPass returns the season pass id of es (ErrNotFound for unknown
// ids and for bookings that are not season passes).
func GetSeasonPass(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*SeasonPassView, error) {
	b, err := PassGetBooking(ctx, db, es, id)
	if err != nil {
		return nil, err
	}
	if !b.IsSeasonPass {
		return nil, ErrNotFound
	}
	views, err := seasonPassViews(ctx, db, es, []Booking{*b})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// PassGetBooking returns one booking of es.
func PassGetBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Booking, error) {
	b := new(Booking)
	if err := catalogGetScoped(ctx, db, es, b, "bk", id, "get booking"); err != nil {
		return nil, err
	}
	return b, nil
}

// PassBookingLines returns the lines of a booking of es in line order.
func PassBookingLines(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]BookingLine, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var lines []BookingLine
	err := db.NewSelect().Model(&lines).
		Where("bl.establishment_id = ?", es.ID()).Where("bl.booking_id = ?", bookingID).
		OrderExpr("bl.sort_order ASC, bl.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking lines: %w", err)
	}
	return lines, nil
}

// PassGetBookingLine returns one booking line of es.
func PassGetBookingLine(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*BookingLine, error) {
	l := new(BookingLine)
	if err := catalogGetScoped(ctx, db, es, l, "bl", id, "get booking line"); err != nil {
		return nil, err
	}
	return l, nil
}

// PassGetSpot returns one spot of es.
func PassGetSpot(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Spot, error) {
	s := new(Spot)
	if err := catalogGetScoped(ctx, db, es, s, "sp", id, "get spot"); err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateSeasonPassNotes replaces the notes of the season pass id when its
// version is still oldVersion, bumping the version. It reports false when
// no row matched (concurrent update).
func UpdateSeasonPassNotes(ctx context.Context, db bun.IDB, es EstablishmentScope, id, notesCustomer, notesInternal string, oldVersion int64, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*Booking)(nil)).
		Set("notes_customer = ?", notesCustomer).
		Set("notes_internal = ?", notesInternal).
		Set("version = version + 1").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("is_season_pass = ?", true).
		Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: update season pass notes: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// seasonPassViews enriches bookings with their spot line, holder name and
// release totals (four queries whatever the page size).
func seasonPassViews(ctx context.Context, db bun.IDB, es EstablishmentScope, bookings []Booking) ([]SeasonPassView, error) {
	out := make([]SeasonPassView, len(bookings))
	if len(bookings) == 0 {
		return out, nil
	}
	ids := make([]string, len(bookings))
	custIDs := make([]string, len(bookings))
	for i := range bookings {
		ids[i] = bookings[i].ID
		custIDs[i] = bookings[i].CustomerID
		out[i].Booking = bookings[i]
	}
	var lines []struct {
		ID        string  `bun:"id"`
		BookingID string  `bun:"booking_id"`
		PackID    string  `bun:"pack_id"`
		SpotID    *string `bun:"spot_id"`
		Label     *string `bun:"label"`
	}
	err := db.NewSelect().TableExpr("booking_lines AS bl").
		ColumnExpr("bl.id AS id, bl.booking_id AS booking_id, bl.pack_id AS pack_id, bl.spot_id AS spot_id, sp.label AS label").
		Join("LEFT JOIN spots AS sp ON sp.id = bl.spot_id").
		Where("bl.establishment_id = ?", es.ID()).
		Where("bl.booking_id IN (?)", bun.List(ids)).
		OrderExpr("bl.sort_order DESC, bl.id DESC"). // the first line wins below
		Scan(ctx, &lines)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season pass lines: %w", err)
	}
	type lineInfo struct{ id, pack, spot, label string }
	first := map[string]lineInfo{}
	for _, l := range lines {
		li := lineInfo{id: l.ID, pack: l.PackID}
		if l.SpotID != nil {
			li.spot = *l.SpotID
		}
		if l.Label != nil {
			li.label = *l.Label
		}
		first[l.BookingID] = li
	}
	var rel []struct {
		BookingID string `bun:"booking_id"`
		Days      int64  `bun:"days"`
		Credit    int64  `bun:"credit"`
	}
	err = db.NewSelect().Model((*SeasonRelease)(nil)).
		ColumnExpr("sr.booking_id AS booking_id, COUNT(*) AS days, COALESCE(SUM(sr.credit_amount), 0) AS credit").
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.booking_id IN (?)", bun.List(ids)).
		Where("sr.revoked_at IS NULL").
		GroupExpr("sr.booking_id").
		Scan(ctx, &rel)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season pass releases: %w", err)
	}
	type relInfo struct {
		days   int
		credit money.Cents
	}
	rels := map[string]relInfo{}
	for _, r := range rel {
		rels[r.BookingID] = relInfo{days: int(r.Days), credit: money.Cents(r.Credit)}
	}
	names, err := CustomerDisplayNames(ctx, db, es, custIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		li := first[out[i].Booking.ID]
		out[i].LineID, out[i].PackID, out[i].SpotID, out[i].SpotLabel = li.id, li.pack, li.spot, li.label
		r := rels[out[i].Booking.ID]
		out[i].ReleasedDays, out[i].CreditAmount = r.days, r.credit
		out[i].CustomerDisplayName = names[out[i].Booking.CustomerID]
	}
	return out, nil
}

// ---- season releases ---------------------------------------------------------

// ListSeasonReleases returns every release (active, resold and revoked) of
// a season pass, by date and slot.
func ListSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) ([]SeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []SeasonRelease
	err := db.NewSelect().Model(&items).
		Where("sr.establishment_id = ?", es.ID()).Where("sr.booking_id = ?", bookingID).
		OrderExpr("sr.date ASC, sr.slot ASC, sr.created_at ASC, sr.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list season releases: %w", err)
	}
	if items == nil {
		items = []SeasonRelease{}
	}
	return items, nil
}

// GetSeasonRelease returns the release id of the season pass bookingID.
func GetSeasonRelease(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID, id string) (*SeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	r := new(SeasonRelease)
	err := db.NewSelect().Model(r).
		Where("sr.id = ?", id).Where("sr.booking_id = ?", bookingID).Where("sr.establishment_id = ?", es.ID()).
		Scan(ctx)
	return r, wrapRead("get season release", err)
}

// ActiveSeasonReleases returns the active (not revoked) releases of a line
// on the given dates, any slot.
func ActiveSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingLineID string, dates []calendar.Date) ([]SeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(dates) == 0 {
		return nil, nil
	}
	var items []SeasonRelease
	err := db.NewSelect().Model(&items).
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.booking_line_id = ?", bookingLineID).
		Where("sr.date IN (?)", bun.List(dates)).
		Where("sr.revoked_at IS NULL").
		OrderExpr("sr.date ASC, sr.slot ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: active season releases: %w", err)
	}
	return items, nil
}

// InsertSeasonReleases inserts releases of es. A second active release of
// the same line, date and slot violates season_releases_active_uidx and
// returns ErrConflict.
func InsertSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, rows []SeasonRelease) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].EstablishmentID = es.ID()
	}
	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return wrapWrite("insert season releases", err)
}

// RevokeSeasonRelease sets revoked_at on an active, not resold release. It
// reports false when the release was revoked or resold meanwhile (the
// conditional UPDATE is the arbiter against a concurrent resale).
func RevokeSeasonRelease(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*SeasonRelease)(nil)).
		Set("revoked_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("revoked_at IS NULL").
		Where("resold_booking_line_id IS NULL").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: revoke season release: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ResellableSeasonReleases returns the active, not resold releases of a
// spot on the given half-days.
func ResellableSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, spotID string, dates []calendar.Date, slots []calendar.Slot) ([]SeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(dates) == 0 || len(slots) == 0 {
		return nil, nil
	}
	var items []SeasonRelease
	err := db.NewSelect().Model(&items).
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.spot_id = ?", spotID).
		Where("sr.date IN (?)", bun.List(dates)).
		Where("sr.slot IN (?)", bun.List(slots)).
		Where("sr.revoked_at IS NULL").
		Where("sr.resold_booking_line_id IS NULL").
		OrderExpr("sr.date ASC, sr.slot ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: resellable season releases: %w", err)
	}
	return items, nil
}

// MarkSeasonReleaseResold sets resold_booking_line_id and credit_amount on
// an active, not yet resold release. It reports false when the release was
// revoked or resold meanwhile.
func MarkSeasonReleaseResold(ctx context.Context, db bun.IDB, es EstablishmentScope, id, lineID string, credit money.Cents, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*SeasonRelease)(nil)).
		Set("resold_booking_line_id = ?", lineID).
		Set("credit_amount = ?", credit).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("revoked_at IS NULL").
		Where("resold_booking_line_id IS NULL").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: mark season release resold: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// SeasonReleasesResoldBy returns the releases resold to booking line lineID.
func SeasonReleasesResoldBy(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID string) ([]SeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []SeasonRelease
	err := db.NewSelect().Model(&items).
		Where("sr.establishment_id = ?", es.ID()).
		Where("sr.resold_booking_line_id = ?", lineID).
		OrderExpr("sr.date ASC, sr.slot ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: season releases resold by line: %w", err)
	}
	return items, nil
}

// UnmarkSeasonReleaseResold clears resold_booking_line_id and credit_amount
// of a release resold to lineID. It reports false when nothing matched.
func UnmarkSeasonReleaseResold(ctx context.Context, db bun.IDB, es EstablishmentScope, id, lineID string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*SeasonRelease)(nil)).
		Set("resold_booking_line_id = NULL").
		Set("credit_amount = 0").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("establishment_id = ?", es.ID()).
		Where("resold_booking_line_id = ?", lineID).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: unmark season release resold: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PassLineBookingIDs maps booking line ids of es to their booking ids (the
// resale bookings of releases).
func PassLineBookingIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, lineIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	lineIDs = dedupe(lineIDs)
	if len(lineIDs) == 0 {
		return out, nil
	}
	var rows []BookingLine
	err := db.NewSelect().Model(&rows).Column("bl.id", "bl.booking_id").
		Where("bl.establishment_id = ?", es.ID()).Where("bl.id IN (?)", bun.List(lineIDs)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: line booking ids: %w", err)
	}
	for i := range rows {
		out[rows[i].ID] = rows[i].BookingID
	}
	return out, nil
}
