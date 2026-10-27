package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Queries of the M8 partner area (spec 30.2, ADR 0028): partners, customer
// categories and their members, requirement verifications and partner
// payments. Every query is scoped by the establishment (INV-26). Contact
// data of partners and the notes of verifications are personal data:
// never logged here or by the callers.

// ---- partners ----------------------------------------------------------------

// PartnerFilter selects partners in ListPartners. Terms are folded
// (lowercase, accent-free) search terms matched on search_text.
type PartnerFilter struct {
	Terms  []string
	Kind   string
	Active *bool
}

// InsertPartner inserts a partner.
func InsertPartner(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Partner) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert partner", err)
}

// GetPartner loads a partner in scope.
func GetPartner(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Partner, error) {
	p := new(Partner)
	if err := pricingGetScoped(ctx, db, es, p, "pa", id, "get partner"); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdatePartner writes p if the stored version is still oldVersion.
func UpdatePartner(ctx context.Context, db bun.IDB, es EstablishmentScope, p *Partner, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, p, oldVersion, "update partner")
}

// DeletePartner deletes a partner in scope.
func DeletePartner(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*Partner)(nil), id, "delete partner")
}

// ListPartners pages the partners ordered by id (creation order).
func ListPartners(ctx context.Context, db bun.IDB, es EstablishmentScope, f PartnerFilter, cursor string, limit int) (Page[Partner], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[Partner]{}, err
	}
	var items []Partner
	q := db.NewSelect().Model(&items).Where("pa.establishment_id = ?", es.ID())
	for _, t := range f.Terms {
		if t != "" {
			q = q.Where(`pa.search_text LIKE ? ESCAPE '\'`, "%"+EscapeLike(t)+"%")
		}
	}
	if f.Kind != "" {
		q = q.Where("pa.kind = ?", f.Kind)
	}
	if f.Active != nil {
		q = q.Where("pa.active = ?", *f.Active)
	}
	return pricingKeysetPage(ctx, q, &items, "pa", cursor, limit, func(r *Partner) string { return r.ID }, "list partners")
}

// PartnersByIDs returns the partners of es among ids, keyed by id.
func PartnersByIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]Partner, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]Partner{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []Partner
	err := db.NewSelect().Model(&rows).Where("pa.establishment_id = ?", es.ID()).
		Where("pa.id IN (?)", bun.List(dedupe(ids))).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: partners by ids: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, nil
}

// PartnerInUse reports whether agreements, bookings or payments reference
// the partner.
func PartnerInUse(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	for _, t := range []string{"agreements", "bookings", "partner_payments"} {
		n, err := db.NewSelect().TableExpr(t+" AS x").
			Where("x.establishment_id = ?", es.ID()).Where("x.partner_id = ?", id).Limit(1).Count(ctx)
		if err != nil {
			return false, fmt.Errorf("store: partner in use: %w", err)
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

// PartnerAgreementCounts returns the number of agreements targeting each
// partner of ids.
func PartnerAgreementCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]int, error) {
	return countBy(ctx, db, es, "agreements", "partner_id", ids)
}

// PartnerBalance is the all-time account of a partner (INV-104).
type PartnerBalance struct {
	Charged money.Cents
	Paid    money.Cents
}

// Balance is Charged - Paid.
func (b PartnerBalance) Balance() money.Cents { return b.Charged - b.Paid }

// PartnerBalances returns the all-time charged and paid amounts of each
// partner of ids (test bookings excluded, voided payments excluded).
func PartnerBalances(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]PartnerBalance, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]PartnerBalance{}
	if len(ids) == 0 {
		return out, nil
	}
	var charged []struct {
		K string      `bun:"k"`
		N money.Cents `bun:"n"`
	}
	err := db.NewSelect().TableExpr("bookings AS bk").
		ColumnExpr("bk.partner_id AS k").ColumnExpr("COALESCE(SUM(bk.partner_amount), 0) AS n").
		Where("bk.establishment_id = ?", es.ID()).Where("bk.partner_id IN (?)", bun.List(ids)).
		Where("bk.test = ?", false).GroupExpr("bk.partner_id").Scan(ctx, &charged)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: partner balances: %w", err)
	}
	paid := charged[:0:0]
	err = db.NewSelect().TableExpr("partner_payments AS pp").
		ColumnExpr("pp.partner_id AS k").ColumnExpr("COALESCE(SUM(pp.amount), 0) AS n").
		Where("pp.establishment_id = ?", es.ID()).Where("pp.partner_id IN (?)", bun.List(ids)).
		Where("pp.voided_at IS NULL").GroupExpr("pp.partner_id").Scan(ctx, &paid)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: partner balances: %w", err)
	}
	for _, r := range charged {
		b := out[r.K]
		b.Charged = r.N
		out[r.K] = b
	}
	for _, r := range paid {
		b := out[r.K]
		b.Paid = r.N
		out[r.K] = b
	}
	return out, nil
}

// countBy counts the rows of table grouped by col for the values ids.
func countBy(ctx context.Context, db bun.IDB, es EstablishmentScope, table, col string, ids []string) (map[string]int, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		K string `bun:"k"`
		N int    `bun:"n"`
	}
	err := db.NewSelect().TableExpr(table+" AS x").
		ColumnExpr("x."+col+" AS k").ColumnExpr("COUNT(*) AS n").
		Where("x.establishment_id = ?", es.ID()).Where("x."+col+" IN (?)", bun.List(dedupe(ids))).
		GroupExpr("x."+col).Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: count %s by %s: %w", table, col, err)
	}
	for _, r := range rows {
		out[r.K] = r.N
	}
	return out, nil
}

// ---- customer categories ------------------------------------------------------

// InsertCustomerCategory inserts a category; a duplicate code is ErrConflict.
func InsertCustomerCategory(ctx context.Context, db bun.IDB, es EstablishmentScope, c *CustomerCategory) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert customer category", err)
}

// GetCustomerCategory loads a category in scope.
func GetCustomerCategory(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*CustomerCategory, error) {
	c := new(CustomerCategory)
	if err := pricingGetScoped(ctx, db, es, c, "cc", id, "get customer category"); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateCustomerCategory writes c if the stored version is still oldVersion.
func UpdateCustomerCategory(ctx context.Context, db bun.IDB, es EstablishmentScope, c *CustomerCategory, oldVersion int64) (bool, error) {
	return pricingUpdateVersioned(ctx, db, es, c, oldVersion, "update customer category")
}

// DeleteCustomerCategory deletes a category (its members go with it).
func DeleteCustomerCategory(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	return pricingDeleteScoped(ctx, db, es, (*CustomerCategory)(nil), id, "delete customer category")
}

// ListCustomerCategories lists the categories ordered by name.
func ListCustomerCategories(ctx context.Context, db bun.IDB, es EstablishmentScope, active *bool) ([]CustomerCategory, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []CustomerCategory
	q := db.NewSelect().Model(&rows).Where("cc.establishment_id = ?", es.ID())
	if active != nil {
		q = q.Where("cc.active = ?", *active)
	}
	if err := q.OrderExpr("cc.name ASC, cc.id ASC").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list customer categories: %w", err)
	}
	if rows == nil {
		rows = []CustomerCategory{}
	}
	return rows, nil
}

// CustomerCategoryInUse reports whether agreements target the category.
func CustomerCategoryInUse(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (bool, error) {
	n, err := countBy(ctx, db, es, "agreements", "customer_category_id", []string{id})
	if err != nil {
		return false, err
	}
	return n[id] > 0, nil
}

// CustomerCategoryMemberCounts returns the number of members of each category.
func CustomerCategoryMemberCounts(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (map[string]int, error) {
	return countBy(ctx, db, es, "customer_category_members", "category_id", ids)
}

// CountCustomerCategories returns how many of ids are categories of es.
func CountCustomerCategories(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := db.NewSelect().Model((*CustomerCategory)(nil)).Where("cc.establishment_id = ?", es.ID()).
		Where("cc.id IN (?)", bun.List(dedupe(ids))).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count customer categories: %w", err)
	}
	return int(n), nil
}

// SetCustomerCategories replaces the categories of a customer and reports
// whether the set changed. The caller validates ids (CountCustomerCategories).
func SetCustomerCategories(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, ids []string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	cur, err := CustomerCategoryIDs(ctx, db, es, []string{customerID})
	if err != nil {
		return false, err
	}
	want := dedupe(ids)
	if sameSet(cur[customerID], want) {
		return false, nil
	}
	if _, err := db.NewDelete().Model((*CustomerCategoryMember)(nil)).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).Exec(ctx); err != nil {
		return false, wrapWrite("set customer categories", err)
	}
	if len(want) == 0 {
		return true, nil
	}
	rows := make([]CustomerCategoryMember, len(want))
	for i, id := range want {
		rows[i] = CustomerCategoryMember{EstablishmentID: es.ID(), CustomerID: customerID, CategoryID: id, CreatedAt: now}
	}
	if _, err := db.NewInsert().Model(&rows).Exec(ctx); err != nil {
		return false, wrapWrite("set customer categories", err)
	}
	return true, nil
}

// CustomerCategoryIDs returns the category ids of each customer of ids
// (sorted by category id).
func CustomerCategoryIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, customerIDs []string) (map[string][]string, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	if len(customerIDs) == 0 {
		return out, nil
	}
	var rows []CustomerCategoryMember
	err := db.NewSelect().Model(&rows).Where("ccm.establishment_id = ?", es.ID()).
		Where("ccm.customer_id IN (?)", bun.List(dedupe(customerIDs))).
		OrderExpr("ccm.customer_id ASC, ccm.category_id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer category ids: %w", err)
	}
	for _, r := range rows {
		out[r.CustomerID] = append(out[r.CustomerID], r.CategoryID)
	}
	return out, nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool, len(a))
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			return false
		}
	}
	return true
}

// ---- requirement verifications -------------------------------------------------

// InsertRequirementVerification inserts a verification.
func InsertRequirementVerification(ctx context.Context, db bun.IDB, es EstablishmentScope, v *RequirementVerification) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	v.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(v).Exec(ctx)
	return wrapWrite("insert requirement verification", err)
}

// CustomerRequirementVerifications lists the verifications recorded on a
// customer (subject customer), newest first.
func CustomerRequirementVerifications(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) ([]RequirementVerification, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []RequirementVerification
	err := db.NewSelect().Model(&rows).Where("rv.establishment_id = ?", es.ID()).
		Where("rv.customer_id = ?", customerID).OrderExpr("rv.verified_at DESC, rv.id DESC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer requirement verifications: %w", err)
	}
	if rows == nil {
		rows = []RequirementVerification{}
	}
	return rows, nil
}

// LatestBookingVerifications returns the latest verification of each booking
// of ids (subject booking).
func LatestBookingVerifications(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) (map[string]RequirementVerification, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]RequirementVerification{}
	if len(bookingIDs) == 0 {
		return out, nil
	}
	var rows []RequirementVerification
	err := db.NewSelect().Model(&rows).Where("rv.establishment_id = ?", es.ID()).
		Where("rv.booking_id IN (?)", bun.List(dedupe(bookingIDs))).
		OrderExpr("rv.verified_at ASC, rv.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking requirement verifications: %w", err)
	}
	for _, r := range rows {
		out[*r.BookingID] = r // ascending: the last one wins
	}
	return out, nil
}

// ---- partner payments ---------------------------------------------------------

// InsertPartnerPayment inserts a payment received from a partner.
func InsertPartnerPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, p *PartnerPayment) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	p.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(p).Exec(ctx)
	return wrapWrite("insert partner payment", err)
}

// GetPartnerPayment loads a payment of the partner in scope.
func GetPartnerPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, partnerID, id string) (*PartnerPayment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	p := new(PartnerPayment)
	err := db.NewSelect().Model(p).Where("pp.establishment_id = ?", es.ID()).
		Where("pp.partner_id = ?", partnerID).Where("pp.id = ?", id).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get partner payment", err)
	}
	return p, nil
}

// VoidPartnerPayment marks a payment voided if it is not already; it
// reports whether the row changed.
func VoidPartnerPayment(ctx context.Context, db bun.IDB, es EstablishmentScope, id, reason string, by *string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*PartnerPayment)(nil)).
		Set("voided_at = ?", now).Set("void_reason = ?", reason).Set("voided_by_user_id = ?", by).
		Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("id = ?", id).Where("voided_at IS NULL").Exec(ctx)
	if err != nil {
		return false, wrapWrite("void partner payment", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ListPartnerPayments pages the payments of a partner ordered by id.
func ListPartnerPayments(ctx context.Context, db bun.IDB, es EstablishmentScope, partnerID, cursor string, limit int) (Page[PartnerPayment], error) {
	if err := es.mustBeValid(); err != nil {
		return Page[PartnerPayment]{}, err
	}
	var items []PartnerPayment
	q := db.NewSelect().Model(&items).Where("pp.establishment_id = ?", es.ID()).Where("pp.partner_id = ?", partnerID)
	return pricingKeysetPage(ctx, q, &items, "pp", cursor, limit, func(r *PartnerPayment) string { return r.ID }, "list partner payments")
}

// PartnerPaymentsBetween lists the payments of a partner (voided included)
// received between from and to (inclusive; zero bounds are open), oldest
// first.
func PartnerPaymentsBetween(ctx context.Context, db bun.IDB, es EstablishmentScope, partnerIDs []string, from, to calendar.Date) ([]PartnerPayment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []PartnerPayment
	q := db.NewSelect().Model(&rows).Where("pp.establishment_id = ?", es.ID())
	if len(partnerIDs) > 0 {
		q = q.Where("pp.partner_id IN (?)", bun.List(dedupe(partnerIDs)))
	}
	if !from.IsZero() {
		q = q.Where("pp.received_on >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("pp.received_on <= ?", to)
	}
	if err := q.OrderExpr("pp.received_on ASC, pp.id ASC").Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: partner payments between: %w", err)
	}
	return rows, nil
}

// ---- GDPR (spec 23) -------------------------------------------------------------

// CustomerM8Records are the M8 personal records linked to a customer.
type CustomerM8Records struct {
	CategoryIDs   []string
	Verifications []RequirementVerification // on the customer and on its bookings
}

// CustomerM8Data returns the M8 records of a customer for the GDPR export.
func CustomerM8Data(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string) (*CustomerM8Records, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	cats, err := CustomerCategoryIDs(ctx, db, es, []string{customerID})
	if err != nil {
		return nil, err
	}
	out := &CustomerM8Records{CategoryIDs: cats[customerID]}
	err = db.NewSelect().Model(&out.Verifications).Where("rv.establishment_id = ?", es.ID()).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("rv.customer_id = ?", customerID).
				WhereOr("rv.booking_id IN (SELECT bk.id FROM bookings AS bk WHERE bk.establishment_id = ? AND bk.customer_id = ?)", es.ID(), customerID)
		}).OrderExpr("rv.verified_at ASC, rv.id ASC").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: customer m8 data: %w", err)
	}
	return out, nil
}

// CustomerM8Wiped counts what WipeCustomerM8Data removed.
type CustomerM8Wiped struct {
	Categories    int
	Verifications int
	Bookings      int
}

// WipeCustomerM8Data removes the M8 personal data of a customer being
// anonymized: category memberships and the verifications recorded on the
// customer are deleted, the notes of the verifications on its bookings
// are cleared, and the guest names and group name of its bookings are
// wiped (the party size stays for the attendance statistics).
func WipeCustomerM8Data(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) (CustomerM8Wiped, error) {
	var out CustomerM8Wiped
	if err := es.mustBeValid(); err != nil {
		return out, err
	}
	res, err := db.NewDelete().Model((*CustomerCategoryMember)(nil)).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).Exec(ctx)
	if err != nil {
		return out, wrapWrite("wipe customer categories", err)
	}
	n, _ := res.RowsAffected()
	out.Categories = int(n)
	res, err = db.NewDelete().Model((*RequirementVerification)(nil)).
		Where("establishment_id = ?", es.ID()).Where("subject = ?", "customer").Where("customer_id = ?", customerID).Exec(ctx)
	if err != nil {
		return out, wrapWrite("wipe customer verifications", err)
	}
	n, _ = res.RowsAffected()
	out.Verifications = int(n)
	bookings := "SELECT bk.id FROM bookings AS bk WHERE bk.establishment_id = ? AND bk.customer_id = ?"
	if _, err := db.NewUpdate().Model((*RequirementVerification)(nil)).Set("note = ''").
		Where("establishment_id = ?", es.ID()).Where("booking_id IN ("+bookings+")", es.ID(), customerID).Exec(ctx); err != nil {
		return out, wrapWrite("wipe booking verification notes", err)
	}
	res, err = db.NewUpdate().Model((*Booking)(nil)).
		Set("guest_names = NULL").Set("group_name = ''").Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Where("customer_id = ?", customerID).
		Where("(guest_names IS NOT NULL OR group_name <> '')").Exec(ctx)
	if err != nil {
		return out, wrapWrite("wipe booking guests", err)
	}
	n, _ = res.RowsAffected()
	out.Bookings = int(n)
	return out, nil
}

// ExistingUser returns &id when id is a user row, nil otherwise (empty,
// API keys, deleted users): the operator columns of the M8 tables
// reference users.
func ExistingUser(ctx context.Context, db bun.IDB, id *string) *string {
	if id == nil || *id == "" {
		return nil
	}
	n, err := db.NewSelect().Model((*User)(nil)).Where("id = ?", *id).Count(ctx)
	if err != nil || n == 0 {
		return nil
	}
	v := *id
	return &v
}
