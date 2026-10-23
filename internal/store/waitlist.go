package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository of the waitlist (owner: internal/domain/waitlist, migration
// 00008). Every function filters on the establishment of the scope; the
// queue order is (priority, created_at, id).

// ---- settings ---------------------------------------------------------------------

// GetWaitlistSettings returns the settings row of es (ErrNotFound = defaults).
func GetWaitlistSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*WaitlistSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(WaitlistSettings)
	err := db.NewSelect().Model(row).Where("wls.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return row, wrapRead("get waitlist settings", err)
}

// InsertWaitlistSettings inserts the first settings row of es (ErrConflict
// when a concurrent insert won).
func InsertWaitlistSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, row *WaitlistSettings) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	row.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(row).Exec(ctx)
	return wrapWrite("insert waitlist settings", err)
}

// UpdateWaitlistSettings replaces the settings of es if the stored version
// is oldVersion; it reports whether it applied.
func UpdateWaitlistSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, row *WaitlistSettings, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	row.EstablishmentID = es.ID()
	res, err := db.NewUpdate().Model(row).
		Column("enabled", "public_join", "auto_offer", "offer_hold_minutes", "version", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update waitlist settings", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---- entries ----------------------------------------------------------------------

// WaitlistFilter selects entries in ListWaitlistEntries.
type WaitlistFilter struct {
	Status   string
	ZoneID   string
	PackID   string
	From     *calendar.Date
	To       *calendar.Date
	Customer []string // customer ids (search resolved by the domain); nil = all
}

type waitlistCursor struct {
	P  int       `json:"p"`
	C  time.Time `json:"c"`
	ID string    `json:"i"`
}

func encodeWaitlistCursor(e *WaitlistEntry) string {
	b, _ := json.Marshal(waitlistCursor{P: e.Priority, C: e.CreatedAt.UTC(), ID: e.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeWaitlistCursor(s string) (*waitlistCursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) > 512 {
		return nil, ErrInvalidCursor
	}
	var c waitlistCursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}

// InsertWaitlistEntry inserts an entry of es.
func InsertWaitlistEntry(ctx context.Context, db bun.IDB, es EstablishmentScope, e *WaitlistEntry) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	e.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(e).Exec(ctx)
	return wrapWrite("insert waitlist entry", err)
}

// GetWaitlistEntry loads an entry of es.
func GetWaitlistEntry(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*WaitlistEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(WaitlistEntry)
	err := db.NewSelect().Model(e).Where("wle.id = ?", id).Where("wle.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return e, wrapRead("get waitlist entry", err)
}

// GetWaitlistEntryByToken loads the entry of es whose link token hashes to
// hash.
func GetWaitlistEntryByToken(ctx context.Context, db bun.IDB, es EstablishmentScope, hash string) (*WaitlistEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	e := new(WaitlistEntry)
	err := db.NewSelect().Model(e).Where("wle.access_token_hash = ?", hash).Where("wle.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return e, wrapRead("get waitlist entry by token", err)
}

// UpdateWaitlistEntry saves e if the stored version is oldVersion (every
// column but the identity); it reports whether it applied.
func UpdateWaitlistEntry(ctx context.Context, db bun.IDB, es EstablishmentScope, e *WaitlistEntry, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model(e).
		Column("starts_on", "ends_on", "slots", "pack_id", "zone_id", "spot_id", "quantity", "priority", "status",
			"note", "locale", "access_token_hash", "converted_booking_id", "removed_at", "version", "updated_at").
		Where("id = ?", e.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		return false, wrapWrite("update waitlist entry", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// SetWaitlistPriority sets the priority of an entry of es (reorder); it
// reports whether the entry exists.
func SetWaitlistPriority(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, priority int, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*WaitlistEntry)(nil)).
		Set("priority = ?", priority).Set("version = version + 1").Set("updated_at = ?", now).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: set waitlist priority: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MaxWaitlistPriority is the highest priority of the open entries of es (0
// when none): a new entry goes after the existing ones.
func MaxWaitlistPriority(ctx context.Context, db bun.IDB, es EstablishmentScope) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var n int
	err := db.NewSelect().Model((*WaitlistEntry)(nil)).ColumnExpr("COALESCE(MAX(wle.priority), 0)").
		Where("wle.establishment_id = ?", es.ID()).
		Where("wle.status IN (?)", bun.List([]string{WaitlistWaiting, WaitlistOffered})).
		Scan(ctx, &n)
	if err != nil {
		return 0, fmt.Errorf("store: max waitlist priority: %w", err)
	}
	return n, nil
}

// ListWaitlistEntries lists the entries of es in queue order with keyset
// pagination.
func ListWaitlistEntries(ctx context.Context, db bun.IDB, es EstablishmentScope, f WaitlistFilter, cursor string, limit int) (Page[WaitlistEntry], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[WaitlistEntry]{}, err
	}
	after, err := decodeWaitlistCursor(cursor)
	if err != nil {
		return Page[WaitlistEntry]{}, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if f.Customer != nil && len(f.Customer) == 0 {
		return Page[WaitlistEntry]{Items: []WaitlistEntry{}}, nil
	}
	var items []WaitlistEntry
	q := db.NewSelect().Model(&items).Where("wle.establishment_id = ?", es.ID())
	if f.Status != "" {
		q = q.Where("wle.status = ?", f.Status)
	}
	if f.ZoneID != "" {
		q = q.Where("wle.zone_id = ?", f.ZoneID)
	}
	if f.PackID != "" {
		q = q.Where("wle.pack_id = ?", f.PackID)
	}
	if f.From != nil {
		q = q.Where("wle.ends_on >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("wle.starts_on <= ?", *f.To)
	}
	if f.Customer != nil {
		q = q.Where("wle.customer_id IN (?)", bun.List(f.Customer))
	}
	if after != nil {
		q = q.Where("(wle.priority > ? OR (wle.priority = ? AND (wle.created_at > ? OR (wle.created_at = ? AND wle.id > ?))))",
			after.P, after.P, after.C, after.C, after.ID)
	}
	q = q.OrderExpr("wle.priority ASC, wle.created_at ASC, wle.id ASC").Limit(int64(limit + 1))
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[WaitlistEntry]{}, fmt.Errorf("store: list waitlist entries: %w", err)
	}
	page := Page[WaitlistEntry]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = encodeWaitlistCursor(&page.Items[limit-1])
	}
	if page.Items == nil {
		page.Items = []WaitlistEntry{}
	}
	return page, nil
}

// WaitingQueue returns the ids of the waiting entries of es in queue order
// (positions; bounded).
func WaitingQueue(ctx context.Context, db bun.IDB, es EstablishmentScope, limit int) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var ids []string
	err := db.NewSelect().Model((*WaitlistEntry)(nil)).Column("wle.id").
		Where("wle.establishment_id = ?", es.ID()).Where("wle.status = ?", WaitlistWaiting).
		OrderExpr("wle.priority ASC, wle.created_at ASC, wle.id ASC").Limit(int64(limit)).
		Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: waiting queue: %w", err)
	}
	return ids, nil
}

// WaitlistCandidates returns the waiting entries of es whose period
// overlaps from..to and has not ended before today, in queue order.
func WaitlistCandidates(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to, today calendar.Date, limit int) ([]WaitlistEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []WaitlistEntry
	err := db.NewSelect().Model(&items).
		Where("wle.establishment_id = ?", es.ID()).Where("wle.status = ?", WaitlistWaiting).
		Where("wle.starts_on <= ?", to).Where("wle.ends_on >= ?", from).Where("wle.ends_on >= ?", today).
		OrderExpr("wle.priority ASC, wle.created_at ASC, wle.id ASC").Limit(int64(limit)).
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: waitlist candidates: %w", err)
	}
	return items, nil
}

// ExpireEndedWaitlistEntries marks expired the waiting entries of es whose
// period ended before today and returns their ids.
func ExpireEndedWaitlistEntries(ctx context.Context, db bun.IDB, es EstablishmentScope, today calendar.Date, now time.Time) ([]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var ids []string
	err := db.NewSelect().Model((*WaitlistEntry)(nil)).Column("wle.id").
		Where("wle.establishment_id = ?", es.ID()).Where("wle.status = ?", WaitlistWaiting).Where("wle.ends_on < ?", today).
		Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: ended waitlist entries: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	_, err = db.NewUpdate().Model((*WaitlistEntry)(nil)).
		Set("status = ?", WaitlistExpired).Set("access_token_hash = NULL").Set("version = version + 1").Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("status = ?", WaitlistWaiting).Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: expire waitlist entries: %w", err)
	}
	return ids, nil
}

// EstablishmentsWithWaitlist lists the establishments having open entries
// or pending offers (jobs).
func EstablishmentsWithWaitlist(ctx context.Context, db bun.IDB) ([]string, error) {
	var ids []string
	err := db.NewSelect().TableExpr("waitlist_entries AS wle").ColumnExpr("DISTINCT wle.establishment_id").
		Where("wle.status IN (?)", bun.List([]string{WaitlistWaiting, WaitlistOffered})).
		Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: establishments with waitlist: %w", err)
	}
	return ids, nil
}

// CustomerWaitlistEntries returns the open entries of a customer of es.
func CustomerWaitlistEntries(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]WaitlistEntry, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []WaitlistEntry
	err := db.NewSelect().Model(&items).
		Where("wle.establishment_id = ?", es.ID()).Where("wle.customer_id = ?", customerID).
		Where("wle.status IN (?)", bun.List([]string{WaitlistWaiting, WaitlistOffered})).
		OrderExpr("wle.id").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer waitlist entries: %w", err)
	}
	return items, nil
}

// HasOpenWaitlistEntryForEmail reports whether a customer with this email
// (normalised) already has an open entry (waiting or offered) for the same
// pack and period: the public join then does nothing (no new customer, no
// entry, no email), so one address cannot be flooded through the public
// endpoint.
func HasOpenWaitlistEntryForEmail(ctx context.Context, db bun.IDB, es EstablishmentScope, email, packID string, startsOn, endsOn calendar.Date) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().Model((*WaitlistEntry)(nil)).
		Join("JOIN customers AS cu ON cu.id = wle.customer_id").
		Where("wle.establishment_id = ?", es.ID()).
		Where("wle.status IN (?)", bun.List([]string{WaitlistWaiting, WaitlistOffered})).
		Where("wle.pack_id = ?", packID).Where("wle.starts_on = ?", startsOn).Where("wle.ends_on = ?", endsOn).
		Where("LOWER(cu.email) = ?", email).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: open waitlist entry by email: %w", err)
	}
	return n > 0, nil
}

// ---- offers -----------------------------------------------------------------------

// InsertWaitlistOffer inserts an offer of es (ErrConflict: the entry
// already has a pending offer, or the hold has an offer).
func InsertWaitlistOffer(ctx context.Context, db bun.IDB, es EstablishmentScope, o *WaitlistOffer) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	o.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(o).Exec(ctx)
	return wrapWrite("insert waitlist offer", err)
}

// GetWaitlistOffer loads an offer of es.
func GetWaitlistOffer(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*WaitlistOffer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	o := new(WaitlistOffer)
	err := db.NewSelect().Model(o).Where("wlo.id = ?", id).Where("wlo.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return o, wrapRead("get waitlist offer", err)
}

// GetWaitlistOfferByHold loads the offer owning a hold of es.
func GetWaitlistOfferByHold(ctx context.Context, db bun.IDB, es EstablishmentScope, holdID string) (*WaitlistOffer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	o := new(WaitlistOffer)
	err := db.NewSelect().Model(o).Where("wlo.hold_id = ?", holdID).Where("wlo.establishment_id = ?", es.ID()).Limit(1).Scan(ctx)
	return o, wrapRead("get waitlist offer by hold", err)
}

// PendingWaitlistOffer returns the pending offer of an entry (ErrNotFound).
func PendingWaitlistOffer(ctx context.Context, db bun.IDB, es EstablishmentScope, entryID string) (*WaitlistOffer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	o := new(WaitlistOffer)
	err := db.NewSelect().Model(o).Where("wlo.entry_id = ?", entryID).Where("wlo.establishment_id = ?", es.ID()).
		Where("wlo.status = ?", WaitlistOfferPending).Limit(1).Scan(ctx)
	return o, wrapRead("get pending waitlist offer", err)
}

// ListWaitlistOffers returns the offers of an entry, newest first.
func ListWaitlistOffers(ctx context.Context, db bun.IDB, es EstablishmentScope, entryID string) ([]WaitlistOffer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []WaitlistOffer
	err := db.NewSelect().Model(&items).Where("wlo.entry_id = ?", entryID).Where("wlo.establishment_id = ?", es.ID()).
		OrderExpr("wlo.created_at DESC, wlo.id DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list waitlist offers: %w", err)
	}
	if items == nil {
		items = []WaitlistOffer{}
	}
	return items, nil
}

// WaitlistOfferStats returns, for the given entries of es, the number of
// offers and the pending one (if any).
func WaitlistOfferStats(ctx context.Context, db bun.IDB, es EstablishmentScope, entryIDs []string) (map[string]int, map[string]WaitlistOffer, error) {
	counts, pending := map[string]int{}, map[string]WaitlistOffer{}
	if err := es.mustBeValid(); err != nil {
		return nil, nil, err
	}
	if len(entryIDs) == 0 {
		return counts, pending, nil
	}
	var items []WaitlistOffer
	err := db.NewSelect().Model(&items).Where("wlo.establishment_id = ?", es.ID()).Where("wlo.entry_id IN (?)", bun.List(entryIDs)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, nil, fmt.Errorf("store: waitlist offer stats: %w", err)
	}
	for _, o := range items {
		counts[o.EntryID]++
		if o.Status == WaitlistOfferPending {
			pending[o.EntryID] = o
		}
	}
	return counts, pending, nil
}

// CloseWaitlistOffer moves a pending offer of es to status (accepted with
// bookingID, declined, expired, revoked) and reports whether this call did
// it (concurrent closers act once).
func CloseWaitlistOffer(ctx context.Context, db bun.IDB, es EstablishmentScope, id, status string, bookingID *string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	q := db.NewUpdate().Model((*WaitlistOffer)(nil)).
		Set("status = ?", status).Set("responded_at = ?", now).Set("updated_at = ?", now).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("status = ?", WaitlistOfferPending)
	if bookingID != nil {
		q = q.Set("booking_id = ?", *bookingID)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return false, wrapWrite("close waitlist offer", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DueWaitlistOffer identifies a pending offer past its expiry (job input).
type DueWaitlistOffer struct {
	ID              string `bun:"id"`
	EstablishmentID string `bun:"establishment_id"`
	EntryID         string `bun:"entry_id"`
}

// DueWaitlistOffers lists (all establishments) the pending offers whose
// expiry is not after now, oldest first.
func DueWaitlistOffers(ctx context.Context, db bun.IDB, now time.Time, limit int) ([]DueWaitlistOffer, error) {
	var out []DueWaitlistOffer
	err := db.NewSelect().TableExpr("waitlist_offers AS wlo").Column("wlo.id", "wlo.establishment_id", "wlo.entry_id").
		Where("wlo.status = ?", WaitlistOfferPending).Where("wlo.expires_at <= ?", now).
		OrderExpr("wlo.expires_at ASC, wlo.id ASC").Limit(int64(limit)).
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: due waitlist offers: %w", err)
	}
	return out, nil
}

// WipeCustomerWaitlist clears the notes and link tokens of every entry of a
// customer of es (GDPR anonymisation, spec 23); the rows stay for the
// statistics, without personal data.
func WipeCustomerWaitlist(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*WaitlistEntry)(nil)).
		Set("note = ''").Set("access_token_hash = NULL").Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: wipe customer waitlist: %w", err)
	}
	return nil
}
