package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Read models of the M8 operational map additions (spec 30.4, 30.5,
// 30.10, ADR 0028): setup from the extras shown on the map, agreement,
// partner, group and party badges, the days a split-stay line spends on
// each spot and the spot season calendar. Owner: internal/domain/inventory
// (operational map builder). One query per concern for the whole day or
// range, never per spot (no N+1).

// SetupExtra is an extra with show_on_map booked by a booking, on a line
// (BookingLineID set) or on the whole booking (BookingLineID nil).
type SetupExtra struct {
	BookingID     string  `bun:"booking_id"`
	BookingLineID *string `bun:"booking_line_id"`
	ExtraID       string  `bun:"extra_id"`
	Code          string  `bun:"code"`
	MapLabel      string  `bun:"map_label"`
	Quantity      int     `bun:"quantity"`
}

// SetupExtrasByBooking returns the extras with show_on_map of the bookings,
// quantities summed per booking, line and extra.
func SetupExtrasByBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) ([]SetupExtra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	var rows []SetupExtra
	err := db.NewSelect().TableExpr("booking_line_extras AS ble").
		Join("JOIN extras AS ex ON ex.id = ble.extra_id").
		ColumnExpr("ble.booking_id, ble.booking_line_id, ble.extra_id, ex.code, ex.map_label").
		ColumnExpr("SUM(ble.quantity) AS quantity").
		Where("ble.establishment_id = ?", es.ID()).Where("ex.establishment_id = ?", es.ID()).
		Where("ex.show_on_map = ?", true).Where("ble.booking_id IN (?)", bun.List(bookingIDs)).
		GroupExpr("ble.booking_id, ble.booking_line_id, ble.extra_id, ex.code, ex.map_label").
		OrderExpr("ble.booking_id, ex.code, ble.extra_id").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: setup extras: %w", err)
	}
	return rows, nil
}

// BookingBadge carries the M8 badges of a booking on the operational map:
// agreement (with its partner), partner of provenance, group and party.
type BookingBadge struct {
	ID                   string  `bun:"id"`
	AgreementID          *string `bun:"agreement_id"`
	AgreementName        *string `bun:"agreement_name"`
	AgreementCode        *string `bun:"agreement_code"`
	AgreementRequirement *string `bun:"agreement_requirement"`
	AgreementPartnerID   *string `bun:"agreement_partner_id"`
	AgreementPartnerName *string `bun:"agreement_partner_name"`
	PartnerID            *string `bun:"partner_id"`
	PartnerName          *string `bun:"partner_name"`
	PartnerKind          *string `bun:"partner_kind"`
	GroupName            string  `bun:"group_name"`
	PartyAdults          *int    `bun:"party_adults"`
	PartyChildren        *int    `bun:"party_children"`
}

// BookingBadges loads the badges of the bookings (agreement and partner
// names joined in the same establishment).
func BookingBadges(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) ([]BookingBadge, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	var rows []BookingBadge
	err := db.NewSelect().TableExpr("bookings AS bk").
		Join("LEFT JOIN agreements AS ag ON ag.id = bk.agreement_id AND ag.establishment_id = bk.establishment_id").
		Join("LEFT JOIN partners AS agp ON agp.id = ag.partner_id AND agp.establishment_id = bk.establishment_id").
		Join("LEFT JOIN partners AS pa ON pa.id = bk.partner_id AND pa.establishment_id = bk.establishment_id").
		ColumnExpr("bk.id, bk.agreement_id, ag.name AS agreement_name, ag.code AS agreement_code").
		ColumnExpr("ag.requirement AS agreement_requirement, ag.partner_id AS agreement_partner_id").
		ColumnExpr("COALESCE(agp.name, NULLIF(ag.partner_name, '')) AS agreement_partner_name").
		ColumnExpr("bk.partner_id, pa.name AS partner_name, pa.kind AS partner_kind").
		ColumnExpr("bk.group_name, bk.party_adults, bk.party_children").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.id IN (?)", bun.List(bookingIDs)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking badges: %w", err)
	}
	return rows, nil
}

// LineSpotDay is one half-day of a booking line on a spot.
type LineSpotDay struct {
	BookingLineID string        `bun:"booking_line_id"`
	SpotID        string        `bun:"spot_id"`
	Date          calendar.Date `bun:"date"`
}

// SplitLineSpotDays returns the allocated dates (one row per date and
// spot) of the lines among lineIDs that occupy more than one spot (split
// stays, scheduled moves, swaps: spec 30.6, 30.7). Lines that stay on a
// single spot are left out, so season passes cost one aggregate row each.
func SplitLineSpotDays(ctx context.Context, db bun.IDB, es EstablishmentScope, lineIDs []string) ([]LineSpotDay, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(lineIDs) == 0 {
		return nil, nil
	}
	var split []string
	err := db.NewSelect().TableExpr("spot_allocations AS sa").ColumnExpr("sa.booking_line_id").
		Where("sa.establishment_id = ?", es.ID()).Where("sa.booking_line_id IN (?)", bun.List(lineIDs)).
		GroupExpr("sa.booking_line_id").Having("COUNT(DISTINCT sa.spot_id) > 1").Scan(ctx, &split)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: split lines: %w", err)
	}
	if len(split) == 0 {
		return nil, nil
	}
	var rows []LineSpotDay
	err = db.NewSelect().TableExpr("spot_allocations AS sa").
		ColumnExpr("DISTINCT sa.booking_line_id, sa.spot_id, sa.date").
		Where("sa.establishment_id = ?", es.ID()).Where("sa.booking_line_id IN (?)", bun.List(split)).
		OrderExpr("sa.booking_line_id, sa.date, sa.spot_id").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: split line days: %w", err)
	}
	return rows, nil
}

// SpotSeasonReleases returns the active (not revoked) season releases of a
// spot between from and to, resold ones included.
func SpotSeasonReleases(ctx context.Context, db bun.IDB, es EstablishmentScope, spotID string, from, to calendar.Date) ([]ActiveSeasonRelease, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []ActiveSeasonRelease
	err := db.NewSelect().TableExpr("season_releases AS sr").
		ColumnExpr("sr.id, sr.booking_id, sr.spot_id, sr.date, sr.slot, sr.resold_booking_line_id").
		Where("sr.establishment_id = ?", es.ID()).Where("sr.spot_id = ?", spotID).Where("sr.revoked_at IS NULL").
		Where("sr.date >= ?", from).Where("sr.date <= ?", to).OrderExpr("sr.date, sr.slot").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: spot season releases: %w", err)
	}
	return rows, nil
}

// CalendarSeason returns the season of the spot calendar when none is
// given (spec 30.5): the one containing today, else the next one to open,
// else the last one closed. ErrNotFound when the establishment has none.
func CalendarSeason(ctx context.Context, db bun.IDB, es EstablishmentScope, today calendar.Date) (*Season, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	pick := func(q func(*bun.SelectQuery) *bun.SelectQuery) (*Season, error) {
		s := new(Season)
		err := q(db.NewSelect().Model(s).Where("sn.establishment_id = ?", es.ID())).Limit(1).Scan(ctx)
		return s, wrapRead("calendar season", err)
	}
	s, err := pick(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("sn.opens_on <= ?", today).Where("sn.closes_on >= ?", today).OrderExpr("sn.opens_on ASC")
	})
	if !errors.Is(err, ErrNotFound) {
		return s, err
	}
	s, err = pick(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("sn.opens_on > ?", today).OrderExpr("sn.opens_on ASC")
	})
	if !errors.Is(err, ErrNotFound) {
		return s, err
	}
	return pick(func(q *bun.SelectQuery) *bun.SelectQuery { return q.OrderExpr("sn.closes_on DESC") })
}
