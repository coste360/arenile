package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Repository of the customer area (owner: internal/domain/customer). Every
// function takes the EstablishmentScope proven by the domain service and
// filters on establishment_id; nothing here logs or formats personal data.

// CustomerFilter selects customers in ListCustomers. Terms are already
// normalised by the domain (lowercase, accent-free) and are matched as
// substrings of search_text, all of them (AND). Tag is matched exactly
// against the "|tag|" markers that the domain appends to search_text.
type CustomerFilter struct {
	Terms             []string
	Tag               string
	IncludeAnonymized bool
}

// customerCursor is the keyset position of the customer list, ordered by
// (search_text, id): alphabetical by last name, then first name.
type customerCursor struct {
	S  string `json:"s"`
	ID string `json:"i"`
}

func encodeCustomerCursor(c *Customer) string {
	b, _ := json.Marshal(customerCursor{S: c.SearchText, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCustomerCursor(s string) (*customerCursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) > 2048 {
		return nil, ErrInvalidCursor
	}
	var c customerCursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}

// EscapeLike escapes the LIKE wildcards of s for a pattern used with
// ESCAPE '\' (same syntax on SQLite and PostgreSQL).
func EscapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// InsertCustomer inserts a customer of es.
func InsertCustomer(ctx context.Context, db bun.IDB, es EstablishmentScope, c *Customer) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	if c.Tags == nil {
		c.Tags = []string{}
	}
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert customer", err)
}

// GetCustomer returns one customer of es.
func GetCustomer(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Customer, error) {
	c := new(Customer)
	if err := catalogGetScoped(ctx, db, es, c, "cu", id, "get customer"); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateCustomer writes every column of c (whose Version must already be
// oldVersion+1) when the stored row still has oldVersion. It reports false
// when no row matched (concurrent update or unknown id).
func UpdateCustomer(ctx context.Context, db bun.IDB, es EstablishmentScope, c *Customer, oldVersion int64) (bool, error) {
	if c.Tags == nil {
		c.Tags = []string{}
	}
	return catalogUpdateVersioned(ctx, db, es, c, oldVersion, "update customer")
}

// ListCustomers lists the customers of es ordered by search_text (last
// name, first name) and id, with keyset pagination.
func ListCustomers(ctx context.Context, db bun.IDB, es EstablishmentScope, f CustomerFilter, cursor string, limit int) (Page[Customer], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Customer]{}, err
	}
	after, err := decodeCustomerCursor(cursor)
	if err != nil {
		return Page[Customer]{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Customer
	q := db.NewSelect().Model(&items).Where("cu.establishment_id = ?", es.ID())
	if !f.IncludeAnonymized {
		q = q.Where("cu.anonymized_at IS NULL")
	}
	for _, t := range f.Terms {
		if t == "" {
			continue
		}
		q = q.Where(`cu.search_text LIKE ? ESCAPE '\'`, "%"+EscapeLike(t)+"%")
	}
	if f.Tag != "" {
		q = q.Where(`cu.search_text LIKE ? ESCAPE '\'`, "%|"+EscapeLike(f.Tag)+"|%")
	}
	if after != nil {
		q = q.Where("(cu.search_text > ? OR (cu.search_text = ? AND cu.id > ?))", after.S, after.S, after.ID)
	}
	q = q.OrderExpr("cu.search_text ASC, cu.id ASC").Limit(int64(limit + 1))
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return Page[Customer]{}, fmt.Errorf("store: list customers: %w", err)
	}
	page := Page[Customer]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = encodeCustomerCursor(&page.Items[limit-1])
	}
	if page.Items == nil {
		page.Items = []Customer{}
	}
	return page, nil
}

// FindCustomerDuplicates returns the non-anonymized customers of es with the
// given email or phone (normalised values; empty ones are ignored), oldest
// first, excluding excludeID.
func FindCustomerDuplicates(ctx context.Context, db bun.IDB, es EstablishmentScope, email, phone, excludeID string) ([]Customer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if email == "" && phone == "" {
		return nil, nil
	}
	var items []Customer
	q := db.NewSelect().Model(&items).
		Where("cu.establishment_id = ?", es.ID()).
		Where("cu.anonymized_at IS NULL").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if email != "" {
				q = q.WhereOr("cu.email = ?", email)
			}
			if phone != "" {
				q = q.WhereOr("cu.phone = ?", phone)
			}
			return q
		}).
		OrderExpr("cu.id ASC").Limit(int64(10))
	if excludeID != "" {
		q = q.Where("cu.id <> ?", excludeID)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: find customer duplicates: %w", err)
	}
	return items, nil
}

// CustomerAgreementExists reports whether agreementID is an agreement of es.
func CustomerAgreementExists(ctx context.Context, db bun.IDB, es EstablishmentScope, agreementID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	n, err := db.NewSelect().Model((*Agreement)(nil)).
		Where("ag.id = ?", agreementID).Where("ag.establishment_id = ?", es.ID()).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("store: customer agreement exists: %w", err)
	}
	return n > 0, nil
}

// CustomerWipeBookingNotes clears the free texts of every booking of a
// customer being anonymized (notes, cancel reason: they may contain personal
// data) and the manage token, bumping their version. Amounts, dates, lines
// and payments are untouched (accounting, spec 23).
func CustomerWipeBookingNotes(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	res, err := db.NewUpdate().Model((*Booking)(nil)).
		Set("notes_customer = ''").
		Set("notes_internal = ''").
		Set("cancel_reason = NULL").
		Set("manage_token_hash = NULL").
		Set("version = version + 1").
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("customer_id = ?", customerID).
		Where("(notes_customer <> '' OR notes_internal <> '' OR cancel_reason IS NOT NULL OR manage_token_hash IS NOT NULL)").
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: wipe booking notes: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// WipedFreeText counts the rows cleared by CustomerWipeFreeText per table.
type WipedFreeText struct {
	Bookings int
	Payments int
	Credits  int
}

// CustomerWipeFreeText clears every free text linked to a customer being
// anonymized: booking notes and cancel reasons (CustomerWipeBookingNotes),
// the notes of its payments and refunds (directly or through its bookings)
// and the notes of its credit movements. Amounts, dates, VAT splits and
// links are untouched (spec 23: the accounts stay, the person disappears).
func CustomerWipeFreeText(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) (WipedFreeText, error) {
	var out WipedFreeText
	var err error
	if out.Bookings, err = CustomerWipeBookingNotes(ctx, db, es, customerID, now); err != nil {
		return out, err
	}
	res, err := db.NewUpdate().Model((*Payment)(nil)).
		Set("note = ''").
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).
		Where("note <> ''").
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.Where("customer_id = ?", customerID).
				WhereOr("booking_id IN (SELECT b2.id FROM bookings AS b2 WHERE b2.establishment_id = ? AND b2.customer_id = ?)", es.ID(), customerID)
		}).
		Exec(ctx)
	if err != nil {
		return out, fmt.Errorf("store: wipe payment notes: %w", err)
	}
	n, _ := res.RowsAffected()
	out.Payments = int(n)
	res, err = db.NewUpdate().Model((*CustomerCredit)(nil)).
		Set("note = ''").
		Where("establishment_id = ?", es.ID()).
		Where("customer_id = ?", customerID).
		Where("note <> ''").
		Exec(ctx)
	if err != nil {
		return out, fmt.Errorf("store: wipe credit notes: %w", err)
	}
	n, _ = res.RowsAffected()
	out.Credits = int(n)
	return out, nil
}

// ---- credits -------------------------------------------------------------------

// InsertCustomerCredit appends a movement to the credit ledger of es.
func InsertCustomerCredit(ctx context.Context, db bun.IDB, es EstablishmentScope, c *CustomerCredit) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert customer credit", err)
}

// ListCustomerCredits returns the credit movements of a customer, newest first.
func ListCustomerCredits(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]CustomerCredit, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []CustomerCredit
	err := db.NewSelect().Model(&items).
		Where("cc.establishment_id = ?", es.ID()).Where("cc.customer_id = ?", customerID).
		OrderExpr("cc.created_at DESC, cc.id DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list customer credits: %w", err)
	}
	if items == nil {
		items = []CustomerCredit{}
	}
	return items, nil
}

// SumCustomerCredits is the sum of the movements of a customer that have not
// expired on today (expires_on NULL or >= today).
func SumCustomerCredits(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, today calendar.Date) (money.Cents, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var sum int64
	err := db.NewSelect().Model((*CustomerCredit)(nil)).
		ColumnExpr("COALESCE(SUM(cc.amount), 0)").
		Where("cc.establishment_id = ?", es.ID()).Where("cc.customer_id = ?", customerID).
		Where("(cc.expires_on IS NULL OR cc.expires_on >= ?)", today).
		Scan(ctx, &sum)
	if err != nil {
		return 0, fmt.Errorf("store: sum customer credits: %w", err)
	}
	return money.Cents(sum), nil
}

// ---- customer views on bookings and payments ------------------------------

// CustomerBookings returns the bookings of a customer, most recent stay
// first; limit <= 0 returns all of them (GDPR export).
func CustomerBookings(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, limit int) ([]Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Booking
	q := db.NewSelect().Model(&items).
		Where("bk.establishment_id = ?", es.ID()).Where("bk.customer_id = ?", customerID).
		OrderExpr("bk.starts_on DESC, bk.id DESC")
	if limit > 0 {
		q = q.Limit(int64(limit))
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer bookings: %w", err)
	}
	if items == nil {
		items = []Booking{}
	}
	return items, nil
}

// CustomerBookingStats aggregates the bookings of a customer.
type CustomerBookingStats struct {
	// BookingsCount counts the bookings not cancelled, expired or rejected.
	BookingsCount int
	// TotalSpent is the net amount collected (paid_amount) on every booking.
	TotalSpent money.Cents
	// BalanceDue is what remains to collect on the live bookings.
	BalanceDue money.Cents
	// LastBookingOn is the latest start date among the counted bookings.
	LastBookingOn *calendar.Date
}

// Booking statuses that do not count as a stay (stats, balance).
var customerDeadStatuses = []string{"cancelled", "expired", "rejected", "draft"}

// GetCustomerBookingStats computes CustomerBookingStats in one query.
func GetCustomerBookingStats(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) (CustomerBookingStats, error) {
	if err := es.mustBeValid(); err != nil {
		return CustomerBookingStats{}, err
	}
	var row struct {
		Count   int64
		Spent   int64
		Balance int64
		Last    *calendar.Date
	}
	err := db.NewSelect().Model((*Booking)(nil)).
		ColumnExpr("COALESCE(SUM(CASE WHEN bk.status IN (?) THEN 0 ELSE 1 END), 0) AS count", bun.List(customerDeadStatuses)).
		ColumnExpr("COALESCE(SUM(bk.paid_amount), 0) AS spent").
		ColumnExpr("COALESCE(SUM(CASE WHEN bk.status IN (?) OR bk.total_amount <= bk.paid_amount THEN 0 ELSE bk.total_amount - bk.paid_amount END), 0) AS balance",
			bun.List(append([]string{"no_show", "completed"}, customerDeadStatuses...))).
		ColumnExpr("MAX(CASE WHEN bk.status IN (?) THEN NULL ELSE bk.starts_on END) AS last", bun.List(customerDeadStatuses)).
		Where("bk.establishment_id = ?", es.ID()).Where("bk.customer_id = ?", customerID).
		Scan(ctx, &row.Count, &row.Spent, &row.Balance, &row.Last)
	if err != nil && !isNoRows(err) {
		return CustomerBookingStats{}, fmt.Errorf("store: customer booking stats: %w", err)
	}
	return CustomerBookingStats{
		BookingsCount: int(row.Count), TotalSpent: money.Cents(row.Spent),
		BalanceDue: money.Cents(row.Balance), LastBookingOn: row.Last,
	}, nil
}

// CustomerPayments returns every payment and refund linked to a customer,
// directly (customer_id) or through one of its bookings, oldest first.
func CustomerPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var items []Payment
	err := db.NewSelect().Model(&items).
		Where("pm.establishment_id = ?", es.ID()).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("pm.customer_id = ?", customerID).
				WhereOr("pm.booking_id IN (SELECT b2.id FROM bookings AS b2 WHERE b2.establishment_id = ? AND b2.customer_id = ?)", es.ID(), customerID)
		}).
		OrderExpr("pm.created_at ASC, pm.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer payments: %w", err)
	}
	if items == nil {
		items = []Payment{}
	}
	return items, nil
}

// CustomerBookingCodes maps booking ids of es to their codes.
func CustomerBookingCodes(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	ids = dedupe(ids)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   string `bun:"id"`
		Code string `bun:"code"`
	}
	err := db.NewSelect().Model((*Booking)(nil)).Column("bk.id", "bk.code").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.id IN (?)", bun.List(ids)).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking codes: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Code
	}
	return out, nil
}

// CustomerBookingSpotLabels maps booking ids of es to the labels of the
// spots assigned to their lines (line order).
func CustomerBookingSpotLabels(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	bookingIDs = dedupe(bookingIDs)
	if len(bookingIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		BookingID string `bun:"booking_id"`
		Label     string `bun:"label"`
	}
	err := db.NewSelect().TableExpr("booking_lines AS bl").
		ColumnExpr("bl.booking_id AS booking_id, sp.label AS label").
		Join("JOIN spots AS sp ON sp.id = bl.spot_id").
		Where("bl.establishment_id = ?", es.ID()).
		Where("bl.booking_id IN (?)", bun.List(bookingIDs)).
		OrderExpr("bl.booking_id, bl.sort_order, bl.id").
		Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking spot labels: %w", err)
	}
	for _, r := range rows {
		out[r.BookingID] = append(out[r.BookingID], r.Label)
	}
	return out, nil
}

// CustomerDisplayNames maps customer ids of es to "Last First".
func CustomerDisplayNames(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	ids = dedupe(ids)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []Customer
	err := db.NewSelect().Model(&rows).Column("cu.id", "cu.first_name", "cu.last_name").
		Where("cu.establishment_id = ?", es.ID()).Where("cu.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer display names: %w", err)
	}
	for i := range rows {
		out[rows[i].ID] = CustomerDisplayName(&rows[i])
	}
	return out, nil
}

// CustomerDisplayName is "Last First" (trimmed), as shown in lists and on the map.
func CustomerDisplayName(c *Customer) string {
	return strings.TrimSpace(c.LastName + " " + c.FirstName)
}
