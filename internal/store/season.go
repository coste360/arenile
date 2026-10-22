package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository functions of the season area: seasons, rate periods, closure
// days (spec 6.1). Every function takes an EstablishmentScope and filters on
// its establishment. Versioned updates and deletes only touch the row when
// its version is still the expected one and report false otherwise.

// ---- keyset cursors ------------------------------------------------------------

// seasonEncodeCursor makes an opaque cursor from the sort key of the last
// row of a page.
func seasonEncodeCursor(parts ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("k:" + strings.Join(parts, "|")))
}

// seasonDecodeCursor reverses seasonEncodeCursor; n is the expected number
// of parts. An empty cursor yields nil.
func seasonDecodeCursor(c string, n int) ([]string, error) {
	if c == "" {
		return nil, nil
	}
	if len(c) > 256 {
		return nil, ErrInvalidCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil || !strings.HasPrefix(string(b), "k:") {
		return nil, ErrInvalidCursor
	}
	parts := strings.Split(string(b[2:]), "|")
	if len(parts) != n {
		return nil, ErrInvalidCursor
	}
	return parts, nil
}

// ---- seasons -------------------------------------------------------------------

// ListSeasons lists the seasons of the establishment, newest year first.
func ListSeasons(ctx context.Context, db bun.IDB, es EstablishmentScope, cursor string, limit int) (Page[Season], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Season]{}, err
	}
	after, err := seasonDecodeCursor(cursor, 1)
	if err != nil {
		return Page[Season]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Season
	q := db.NewSelect().Model(&items).Where("sn.establishment_id = ?", es.ID()).
		OrderExpr("sn.year DESC").Limit(int64(limit + 1))
	if after != nil {
		year, err := strconv.Atoi(after[0])
		if err != nil {
			return Page[Season]{}, ErrInvalidCursor
		}
		q = q.Where("sn.year < ?", year)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Season]{}, fmt.Errorf("store: list seasons: %w", err)
	}
	page := Page[Season]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = seasonEncodeCursor(strconv.Itoa(page.Items[limit-1].Year))
	}
	return page, nil
}

// GetSeason loads a season of the establishment.
func GetSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(Season)
	err := db.NewSelect().Model(s).Where("sn.id = ?", id).Where("sn.establishment_id = ?", es.ID()).Scan(ctx)
	return s, wrapRead("get season", err)
}

// GetSeasonByYear loads the season of a year.
func GetSeasonByYear(ctx context.Context, db bun.IDB, es EstablishmentScope, year int) (*Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(Season)
	err := db.NewSelect().Model(s).Where("sn.establishment_id = ?", es.ID()).Where("sn.year = ?", year).Scan(ctx)
	return s, wrapRead("get season by year", err)
}

// SeasonsCovering lists the seasons whose opening period contains d.
func SeasonsCovering(ctx context.Context, db bun.IDB, es EstablishmentScope, d calendar.Date) ([]Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Season
	err := db.NewSelect().Model(&items).Where("sn.establishment_id = ?", es.ID()).
		Where("sn.opens_on <= ?", d).Where("sn.closes_on >= ?", d).OrderExpr("sn.year ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: seasons covering: %w", err)
	}
	return items, nil
}

// InsertSeason inserts a season in the establishment of es.
func InsertSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, s *Season) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s.EstablishmentID = es.ID()
	if s.Version == 0 {
		s.Version = 1
	}
	_, err := db.NewInsert().Model(s).Exec(ctx)
	return wrapWrite("insert season", err)
}

// UpdateSeasonIfVersion replaces a season when its version is still expected.
func UpdateSeasonIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, s *Season, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	s.EstablishmentID = es.ID()
	res, err := db.NewUpdate().Model(s).WherePK().
		Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("update season", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteSeasonIfVersion deletes a season (its rate periods, price rules,
// fixed prices and policies cascade) when its version is still expected.
func DeleteSeasonIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().Model((*Season)(nil)).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete season", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// LockSeason takes a write lock on a season row without changing it, so
// that concurrent writers of its rate periods are serialized (PostgreSQL row
// lock; SQLite write transactions are already serialized). It returns
// ErrNotFound for unknown seasons.
func LockSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewUpdate().Model((*Season)(nil)).Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("lock season", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- rate periods --------------------------------------------------------------

// ListRatePeriods lists the rate periods of the establishment ordered by
// season, sort order and id; seasonID filters when not empty.
func ListRatePeriods(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID, cursor string, limit int) (Page[RatePeriod], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[RatePeriod]{}, err
	}
	after, err := seasonDecodeCursor(cursor, 3)
	if err != nil {
		return Page[RatePeriod]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []RatePeriod
	q := db.NewSelect().Model(&items).Where("rp.establishment_id = ?", es.ID()).
		OrderExpr("rp.season_id ASC, rp.sort_order ASC, rp.id ASC").Limit(int64(limit + 1))
	if seasonID != "" {
		q = q.Where("rp.season_id = ?", seasonID)
	}
	if after != nil {
		so, err := strconv.Atoi(after[1])
		if err != nil {
			return Page[RatePeriod]{}, ErrInvalidCursor
		}
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("rp.season_id > ?", after[0]).
				WhereOr("rp.season_id = ? AND rp.sort_order > ?", after[0], so).
				WhereOr("rp.season_id = ? AND rp.sort_order = ? AND rp.id > ?", after[0], so, after[2])
		})
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[RatePeriod]{}, fmt.Errorf("store: list rate periods: %w", err)
	}
	page := Page[RatePeriod]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = seasonEncodeCursor(last.SeasonID, strconv.Itoa(last.SortOrder), last.ID)
	}
	return page, nil
}

// ListSeasonRatePeriods returns every rate period of a season.
func ListSeasonRatePeriods(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string) ([]RatePeriod, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []RatePeriod
	err := db.NewSelect().Model(&items).Where("rp.establishment_id = ?", es.ID()).
		Where("rp.season_id = ?", seasonID).OrderExpr("rp.sort_order ASC, rp.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list season rate periods: %w", err)
	}
	return items, nil
}

// GetRatePeriod loads a rate period of the establishment.
func GetRatePeriod(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RatePeriod, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(RatePeriod)
	err := db.NewSelect().Model(p).Where("rp.id = ?", id).Where("rp.establishment_id = ?", es.ID()).Scan(ctx)
	return p, wrapRead("get rate period", err)
}

// InsertRatePeriod inserts a rate period in the establishment of es.
func InsertRatePeriod(ctx context.Context, db bun.IDB, es EstablishmentScope, p *RatePeriod) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	normalizeRatePeriod(p)
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert rate period", err)
}

// UpdateRatePeriodIfVersion replaces a rate period when its version is still
// expected.
func UpdateRatePeriodIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, p *RatePeriod, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	p.EstablishmentID = es.ID()
	normalizeRatePeriod(p)
	res, err := db.NewUpdate().Model(p).WherePK().
		Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("update rate period", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteRatePeriodIfVersion deletes a rate period when its version is still
// expected.
func DeleteRatePeriodIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().Model((*RatePeriod)(nil)).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete rate period", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func normalizeRatePeriod(p *RatePeriod) {
	if p.Name == nil {
		p.Name = I18nText{}
	}
	if p.Ranges == nil {
		p.Ranges = []DateRange{}
	}
	if p.Version == 0 {
		p.Version = 1
	}
}

// ---- closure days --------------------------------------------------------------

// ClosureDayFilter restricts ListClosureDays to an inclusive date interval;
// zero dates are open bounds.
type ClosureDayFilter struct {
	From calendar.Date
	To   calendar.Date
}

// ListClosureDays lists the closure days of the establishment by date.
func ListClosureDays(ctx context.Context, db bun.IDB, es EstablishmentScope, f ClosureDayFilter, cursor string, limit int) (Page[ClosureDay], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[ClosureDay]{}, err
	}
	after, err := seasonDecodeCursor(cursor, 1)
	if err != nil {
		return Page[ClosureDay]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []ClosureDay
	q := db.NewSelect().Model(&items).Where("cd.establishment_id = ?", es.ID()).
		OrderExpr("cd.date ASC").Limit(int64(limit + 1))
	if !f.From.IsZero() {
		q = q.Where("cd.date >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("cd.date <= ?", f.To)
	}
	if after != nil {
		d, err := calendar.Parse(after[0])
		if err != nil {
			return Page[ClosureDay]{}, ErrInvalidCursor
		}
		q = q.Where("cd.date > ?", d)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[ClosureDay]{}, fmt.Errorf("store: list closure days: %w", err)
	}
	page := Page[ClosureDay]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = seasonEncodeCursor(page.Items[limit-1].Date.String())
	}
	return page, nil
}

// GetClosureDay loads a closure day of the establishment.
func GetClosureDay(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*ClosureDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	c := new(ClosureDay)
	err := db.NewSelect().Model(c).Where("cd.id = ?", id).Where("cd.establishment_id = ?", es.ID()).Scan(ctx)
	return c, wrapRead("get closure day", err)
}

// InsertClosureDay inserts a closure day; a second closure on the same date
// is ErrConflict.
func InsertClosureDay(ctx context.Context, db bun.IDB, es EstablishmentScope, c *ClosureDay) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	if c.Version == 0 {
		c.Version = 1
	}
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert closure day", err)
}

// UpdateClosureDayIfVersion replaces a closure day when its version is still
// expected.
func UpdateClosureDayIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, c *ClosureDay, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	c.EstablishmentID = es.ID()
	res, err := db.NewUpdate().Model(c).WherePK().
		Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("update closure day", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteClosureDayIfVersion deletes a closure day when its version is still
// expected.
func DeleteClosureDayIfVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewDelete().Model((*ClosureDay)(nil)).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		return false, wrapWrite("delete closure day", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
