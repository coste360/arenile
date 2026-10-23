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

// Repository of the season pass renewals (owner: internal/domain/renewal):
// renewal_campaigns and renewal_proposals (migration 00008) and the reads
// of the source passes. Every function filters on the establishment of the
// scope, except the job scan RenewalProposalsToSync (system, all
// establishments, returns references only).

// ---- campaigns -----------------------------------------------------------------

// InsertRenewalCampaign inserts a campaign (ErrConflict when the same
// source and target seasons already have one).
func InsertRenewalCampaign(ctx context.Context, db bun.IDB, es EstablishmentScope, c *RenewalCampaign) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert renewal campaign", err)
}

// GetRenewalCampaign loads a campaign.
func GetRenewalCampaign(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RenewalCampaign, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	c := new(RenewalCampaign)
	err := db.NewSelect().Model(c).Where("rcp.id = ?", id).Where("rcp.establishment_id = ?", es.ID()).Scan(ctx)
	return c, wrapRead("get renewal campaign", err)
}

// LockRenewalCampaign takes a write lock on a campaign row (no change) and
// returns it.
func LockRenewalCampaign(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RenewalCampaign, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*RenewalCampaign)(nil)).Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, wrapWrite("lock renewal campaign", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetRenewalCampaign(ctx, db, es, id)
}

// UpdateRenewalCampaignVersioned writes name, notes, deadline, status and
// closed_at when the stored version is expected; c.Version becomes
// expected+1. It reports false on a version mismatch.
func UpdateRenewalCampaignVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, c *RenewalCampaign, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	c.Version = expected + 1
	res, err := db.NewUpdate().Model(c).
		Column("name", "notes", "deadline_at", "status", "closed_at", "version", "updated_at").
		Where("id = ?", c.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update renewal campaign", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RenewalCampaignFilter filters ListRenewalCampaigns (empty = any).
type RenewalCampaignFilter struct {
	TargetSeasonID string
	Status         string
}

// ListRenewalCampaigns lists campaigns, newest first.
func ListRenewalCampaigns(ctx context.Context, db bun.IDB, es EstablishmentScope, f RenewalCampaignFilter, cursor string, limit int) (Page[RenewalCampaign], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[RenewalCampaign]{}, err
	}
	after, err := DecodeCursor(cursor)
	if err != nil {
		return Page[RenewalCampaign]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []RenewalCampaign
	q := db.NewSelect().Model(&items).Where("rcp.establishment_id = ?", es.ID()).OrderExpr("rcp.id DESC").Limit(int64(limit + 1))
	if f.TargetSeasonID != "" {
		q = q.Where("rcp.target_season_id = ?", f.TargetSeasonID)
	}
	if f.Status != "" {
		q = q.Where("rcp.status = ?", f.Status)
	}
	if after != "" {
		q = q.Where("rcp.id < ?", after)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[RenewalCampaign]{}, fmt.Errorf("store: list renewal campaigns: %w", err)
	}
	page := Page[RenewalCampaign]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = EncodeCursor(page.Items[limit-1].ID)
	}
	if page.Items == nil {
		page.Items = []RenewalCampaign{}
	}
	return page, nil
}

// RenewalCampaignStats are the proposal counts and amounts of a campaign.
type RenewalCampaignStats struct {
	ByStatus        map[string]int
	Sent            int
	TotalAmount     money.Cents // proposals not declined or expired
	ConvertedAmount money.Cents
}

// RenewalCampaignStatsOf aggregates the proposals of the given campaigns.
func RenewalCampaignStatsOf(ctx context.Context, db bun.IDB, es EstablishmentScope, campaignIDs []string) (map[string]*RenewalCampaignStats, error) {
	out := map[string]*RenewalCampaignStats{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	for _, id := range campaignIDs {
		out[id] = &RenewalCampaignStats{ByStatus: map[string]int{}}
	}
	if len(campaignIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		CampaignID string `bun:"campaign_id"`
		Status     string `bun:"status"`
		N          int64  `bun:"n"`
		Sent       int64  `bun:"sent"`
		Amount     int64  `bun:"amount"`
	}
	err := db.NewSelect().Model((*RenewalProposal)(nil)).
		ColumnExpr("rnp.campaign_id AS campaign_id, rnp.status AS status, COUNT(*) AS n").
		ColumnExpr("SUM(CASE WHEN rnp.send_count > 0 THEN 1 ELSE 0 END) AS sent").
		ColumnExpr("COALESCE(SUM(rnp.total_amount), 0) AS amount").
		Where("rnp.establishment_id = ?", es.ID()).
		Where("rnp.campaign_id IN (?)", bun.List(campaignIDs)).
		GroupExpr("rnp.campaign_id, rnp.status").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal campaign stats: %w", err)
	}
	for _, r := range rows {
		st := out[r.CampaignID]
		if st == nil {
			continue
		}
		st.ByStatus[r.Status] = int(r.N)
		st.Sent += int(r.Sent)
		switch r.Status {
		case RenewalDeclined, RenewalExpired:
		default:
			st.TotalAmount += money.Cents(r.Amount)
		}
		if r.Status == RenewalConverted {
			st.ConvertedAmount += money.Cents(r.Amount)
		}
	}
	return out, nil
}

// ---- proposals -----------------------------------------------------------------

// InsertRenewalProposal inserts a proposal (ErrConflict on the unique keys:
// campaign + source pass, booking, one open proposal per source pass).
func InsertRenewalProposal(ctx context.Context, db bun.IDB, es EstablishmentScope, p *RenewalProposal) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert renewal proposal", err)
}

// GetRenewalProposal loads a proposal.
func GetRenewalProposal(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RenewalProposal, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(RenewalProposal)
	err := db.NewSelect().Model(p).Where("rnp.id = ?", id).Where("rnp.establishment_id = ?", es.ID()).Scan(ctx)
	return p, wrapRead("get renewal proposal", err)
}

// GetRenewalProposalByBooking loads the proposal owning a target booking.
func GetRenewalProposalByBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (*RenewalProposal, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(RenewalProposal)
	err := db.NewSelect().Model(p).Where("rnp.booking_id = ?", bookingID).Where("rnp.establishment_id = ?", es.ID()).Scan(ctx)
	return p, wrapRead("get renewal proposal by booking", err)
}

// LockRenewalProposal takes a write lock on a proposal row and returns it.
func LockRenewalProposal(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*RenewalProposal, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*RenewalProposal)(nil)).Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, wrapWrite("lock renewal proposal", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetRenewalProposal(ctx, db, es, id)
}

// UpdateRenewalProposalVersioned writes the mutable columns of p when the
// stored version is expected; p.Version becomes expected+1.
func UpdateRenewalProposalVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, p *RenewalProposal, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	p.Version = expected + 1
	res, err := db.NewUpdate().Model(p).
		Column("status", "total_amount", "deposit_due", "deadline_at", "send_count", "last_sent_at", "responded_at",
			"response_channel", "decline_reason", "version", "updated_at").
		Where("id = ?", p.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update renewal proposal", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RenewalProposalFilter filters the proposals of a campaign (empty = any).
type RenewalProposalFilter struct {
	Status string
	Sent   *bool
	ZoneID string
	Query  string
	IDs    []string
}

// RenewalProposalRow is a proposal with the fields the panel shows: the
// target booking, the customer and the spot.
type RenewalProposalRow struct {
	Proposal            RenewalProposal
	BookingCode         string
	BookingStatus       string
	StartsOn, EndsOn    calendar.Date
	Slots               calendar.Slots
	PaidAmount          money.Cents
	HoldExpiresAt       *time.Time
	CustomerDisplayName string
	CustomerHasEmail    bool
	SpotLabel           string
	ZoneID              string
}

func renewalProposalQuery(db bun.IDB, es EstablishmentScope, items *[]RenewalProposal, campaignID string, f RenewalProposalFilter) *bun.SelectQuery {
	q := db.NewSelect().Model(items).Where("rnp.establishment_id = ?", es.ID())
	if campaignID != "" {
		q = q.Where("rnp.campaign_id = ?", campaignID)
	}
	if f.Status != "" {
		q = q.Where("rnp.status = ?", f.Status)
	}
	if f.Sent != nil {
		if *f.Sent {
			q = q.Where("rnp.send_count > 0")
		} else {
			q = q.Where("rnp.send_count = 0")
		}
	}
	if len(f.IDs) > 0 {
		q = q.Where("rnp.id IN (?)", bun.List(f.IDs))
	}
	if f.ZoneID != "" {
		q = q.Where("rnp.spot_id IN (SELECT sp.id FROM spots AS sp WHERE sp.establishment_id = ? AND sp.zone_id = ?)", es.ID(), f.ZoneID)
	}
	if s := strings.ToLower(strings.TrimSpace(escapeLike(f.Query))); s != "" {
		q = q.Where("(rnp.booking_id IN (SELECT bk.id FROM bookings AS bk WHERE bk.establishment_id = ? AND LOWER(bk.code) LIKE ?)"+
			" OR rnp.customer_id IN (SELECT cu.id FROM customers AS cu WHERE cu.establishment_id = ? AND cu.search_text LIKE ?)"+
			" OR rnp.spot_id IN (SELECT sp.id FROM spots AS sp WHERE sp.establishment_id = ? AND LOWER(sp.label) LIKE ?))",
			es.ID(), s+"%", es.ID(), "%"+s+"%", es.ID(), s+"%")
	}
	return q
}

// ListRenewalProposalRows lists the proposals of a campaign by id
// (creation order) with keyset pagination.
func ListRenewalProposalRows(ctx context.Context, db bun.IDB, es EstablishmentScope, campaignID string, f RenewalProposalFilter, cursor string, limit int) (Page[RenewalProposalRow], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[RenewalProposalRow]{}, err
	}
	var items []RenewalProposal
	q := renewalProposalQuery(db, es, &items, campaignID, f)
	page, err := catalogKeysetPage(ctx, q, &items, "rnp", cursor, limit, func(p *RenewalProposal) string { return p.ID }, "list renewal proposals")
	if err != nil {
		return Page[RenewalProposalRow]{}, err
	}
	rows, err := RenewalProposalRows(ctx, db, es, page.Items)
	if err != nil {
		return Page[RenewalProposalRow]{}, err
	}
	return Page[RenewalProposalRow]{Items: rows, NextCursor: page.NextCursor}, nil
}

// ListRenewalProposalIDs returns the ids of every proposal of a campaign
// matching f, by id (bulk actions).
func ListRenewalProposalIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, campaignID string, f RenewalProposalFilter) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []RenewalProposal
	q := renewalProposalQuery(db, es, &items, campaignID, f).Column("rnp.id").OrderExpr("rnp.id ASC")
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal proposal ids: %w", err)
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	return ids, nil
}

// RenewalProposalRows enriches proposals with booking, customer and spot.
func RenewalProposalRows(ctx context.Context, db bun.IDB, es EstablishmentScope, ps []RenewalProposal) ([]RenewalProposalRow, error) {
	out := make([]RenewalProposalRow, len(ps))
	if len(ps) == 0 {
		return out, nil
	}
	bookingIDs, customerIDs, spotIDs := make([]string, len(ps)), make([]string, len(ps)), make([]string, len(ps))
	for i := range ps {
		out[i].Proposal = ps[i]
		bookingIDs[i], customerIDs[i], spotIDs[i] = ps[i].BookingID, ps[i].CustomerID, ps[i].SpotID
	}
	var bookings []Booking
	if err := db.NewSelect().Model(&bookings).
		Column("bk.id", "bk.code", "bk.status", "bk.starts_on", "bk.ends_on", "bk.slots", "bk.paid_amount", "bk.hold_expires_at").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.id IN (?)", bun.List(dedupe(bookingIDs))).Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal bookings: %w", err)
	}
	byBooking := make(map[string]*Booking, len(bookings))
	for i := range bookings {
		byBooking[bookings[i].ID] = &bookings[i]
	}
	var customers []Customer
	if err := db.NewSelect().Model(&customers).Column("cu.id", "cu.first_name", "cu.last_name", "cu.email", "cu.anonymized_at").
		Where("cu.establishment_id = ?", es.ID()).Where("cu.id IN (?)", bun.List(dedupe(customerIDs))).Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal customers: %w", err)
	}
	byCustomer := make(map[string]*Customer, len(customers))
	for i := range customers {
		byCustomer[customers[i].ID] = &customers[i]
	}
	var spots []Spot
	if err := db.NewSelect().Model(&spots).Column("sp.id", "sp.label", "sp.zone_id").
		Where("sp.establishment_id = ?", es.ID()).Where("sp.id IN (?)", bun.List(dedupe(spotIDs))).Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal spots: %w", err)
	}
	bySpot := make(map[string]*Spot, len(spots))
	for i := range spots {
		bySpot[spots[i].ID] = &spots[i]
	}
	for i := range out {
		p := &out[i].Proposal
		if b := byBooking[p.BookingID]; b != nil {
			out[i].BookingCode, out[i].BookingStatus, out[i].StartsOn, out[i].EndsOn = b.Code, b.Status, b.StartsOn, b.EndsOn
			out[i].Slots, out[i].PaidAmount, out[i].HoldExpiresAt = b.Slots, b.PaidAmount, b.HoldExpiresAt
		}
		if c := byCustomer[p.CustomerID]; c != nil {
			out[i].CustomerDisplayName = CustomerDisplayName(c)
			out[i].CustomerHasEmail = c.AnonymizedAt == nil && c.Email != nil && strings.TrimSpace(*c.Email) != ""
		}
		if s := bySpot[p.SpotID]; s != nil {
			out[i].SpotLabel, out[i].ZoneID = s.Label, s.ZoneID
		}
	}
	return out, nil
}

// OpenRenewalSources returns, among sourceIDs, the source passes that
// already have an open proposal (proposed or accepted, any campaign) or a
// renewal booking that is not terminal (a pass created from the panel with
// renewal_of_booking_id).
func OpenRenewalSources(ctx context.Context, db bun.IDB, es EstablishmentScope, sourceIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(sourceIDs) == 0 {
		return out, nil
	}
	var props []RenewalProposal
	if err := db.NewSelect().Model(&props).Column("rnp.source_booking_id").
		Where("rnp.establishment_id = ?", es.ID()).Where("rnp.source_booking_id IN (?)", bun.List(sourceIDs)).
		Where("rnp.status IN (?)", bun.List([]string{RenewalProposed, RenewalAccepted, RenewalConverted})).
		Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: open renewal proposals: %w", err)
	}
	for _, p := range props {
		out[p.SourceBookingID] = true
	}
	var bks []Booking
	if err := db.NewSelect().Model(&bks).Column("bk.renewal_of_booking_id").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.renewal_of_booking_id IN (?)", bun.List(sourceIDs)).
		Where("bk.status NOT IN (?)", bun.List([]string{"cancelled", "expired", "rejected", "draft"})).
		Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal bookings: %w", err)
	}
	for _, b := range bks {
		if b.RenewalOfBookingID != nil {
			out[*b.RenewalOfBookingID] = true
		}
	}
	return out, nil
}

// RenewalCandidates returns the season passes of a season to propose:
// passIDs (any status, the caller classifies them) or, when empty, every
// pass in RenewableBookingStatuses. Ordered by id.
func RenewalCandidates(ctx context.Context, db bun.IDB, es EstablishmentScope, seasonID string, passIDs []string) ([]SeasonPassView, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Booking
	q := db.NewSelect().Model(&items).
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.is_season_pass = ?", true).
		Where("bk.season_id = ?", seasonID).
		OrderExpr("bk.id ASC")
	if len(passIDs) > 0 {
		q = q.Where("bk.id IN (?)", bun.List(dedupe(passIDs)))
	} else {
		q = q.Where("bk.status IN (?)", bun.List(RenewableBookingStatuses))
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal candidates: %w", err)
	}
	return seasonPassViews(ctx, db, es, items)
}

// LineExtrasOf returns the extras of a booking line.
func LineExtrasOf(ctx context.Context, db bun.IDB, es EstablishmentScope, lineID string) ([]BookingLineExtra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	items := []BookingLineExtra{}
	err := db.NewSelect().Model(&items).Where("ble.establishment_id = ?", es.ID()).Where("ble.booking_line_id = ?", lineID).
		OrderExpr("ble.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: line extras: %w", err)
	}
	return items, nil
}

// RenewalProposalRef identifies a proposal for the sync job.
type RenewalProposalRef struct {
	ID              string `bun:"id"`
	EstablishmentID string `bun:"establishment_id"`
}

// RenewalProposalsToSync returns open proposals (proposed or accepted)
// whose pre-emption deadline passed or whose booking is no longer held or
// pending payment, across establishments (system job), oldest first.
func RenewalProposalsToSync(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]RenewalProposalRef, error) {
	var refs []RenewalProposalRef
	err := db.NewSelect().TableExpr("renewal_proposals AS rnp").
		ColumnExpr("rnp.id AS id, rnp.establishment_id AS establishment_id").
		Join("JOIN bookings AS bk ON bk.id = rnp.booking_id").
		Where("rnp.status IN (?)", bun.List([]string{RenewalProposed, RenewalAccepted})).
		Where("(rnp.deadline_at <= ? OR bk.status NOT IN (?))", now.UTC(), bun.List([]string{"held", "pending_payment"})).
		OrderExpr("rnp.deadline_at ASC, rnp.id ASC").Limit(int64(limit)).
		Scan(ctx, &refs)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: renewal proposals to sync: %w", err)
	}
	return refs, nil
}

// FindRenewalCampaign loads the campaign of a source and target season
// (ErrNotFound when none).
func FindRenewalCampaign(ctx context.Context, db bun.IDB, es EstablishmentScope, sourceSeasonID, targetSeasonID string) (*RenewalCampaign, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	c := new(RenewalCampaign)
	err := db.NewSelect().Model(c).Where("rcp.establishment_id = ?", es.ID()).
		Where("rcp.source_season_id = ?", sourceSeasonID).Where("rcp.target_season_id = ?", targetSeasonID).Scan(ctx)
	return c, wrapRead("find renewal campaign", err)
}
