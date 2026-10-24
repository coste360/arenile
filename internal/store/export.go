package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Batched reads of the list exports (GET /exports/{list}, ADR 0019 F):
// keyset pagination by id so that an export streams with flat memory.
// Test rows (bookings.test, payments.test) are always excluded (INV-74).
// Only the columns the export writes are read: no email or phone.

// ExportCustomer is one customer row of the customers export.
type ExportCustomer struct {
	ID               string            `bun:"id"`
	FirstName        string            `bun:"first_name"`
	LastName         string            `bun:"last_name"`
	Locale           string            `bun:"locale"`
	Tags             []string          `bun:"tags,type:json"`
	MarketingConsent *MarketingConsent `bun:"marketing_consent,type:json"`
	AnonymizedAt     *time.Time        `bun:"anonymized_at"`
	CreatedAt        time.Time         `bun:"created_at"`
}

// ExportCustomersBatch returns up to limit customers of es after afterID.
func ExportCustomersBatch(ctx context.Context, db bun.IDB, es EstablishmentScope, afterID string, limit int) ([]ExportCustomer, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []ExportCustomer
	q := db.NewSelect().TableExpr("customers AS cu").
		Column("cu.id", "cu.first_name", "cu.last_name", "cu.locale", "cu.tags", "cu.marketing_consent", "cu.anonymized_at", "cu.created_at").
		Where("cu.establishment_id = ?", es.ID())
	if afterID != "" {
		q = q.Where("cu.id > ?", afterID)
	}
	if err := q.OrderExpr("cu.id ASC").Limit(int64(limit)).Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: export customers: %w", err)
	}
	return out, nil
}

// ExportBooking is one booking row of the bookings export.
type ExportBooking struct {
	ID           string         `bun:"id"`
	Code         string         `bun:"code"`
	Status       string         `bun:"status"`
	Channel      string         `bun:"channel"`
	StartsOn     calendar.Date  `bun:"starts_on"`
	EndsOn       calendar.Date  `bun:"ends_on"`
	Slots        calendar.Slots `bun:"slots"`
	Formula      string         `bun:"formula"`
	IsSeasonPass bool           `bun:"is_season_pass"`
	TotalAmount  money.Cents    `bun:"total_amount"`
	PaidAmount   money.Cents    `bun:"paid_amount"`
	CreatedAt    time.Time      `bun:"created_at"`
	FirstName    string         `bun:"first_name"`
	LastName     string         `bun:"last_name"`
	AnonymizedAt *time.Time     `bun:"anonymized_at"`
}

// ExportBookingsBatch returns up to limit non-test bookings of es after
// afterID whose stay overlaps from..to (nil = unbounded); seasonPasses
// selects passes only (true) or every booking (false).
func ExportBookingsBatch(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to *calendar.Date, seasonPasses bool, afterID string, limit int) ([]ExportBooking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []ExportBooking
	q := db.NewSelect().TableExpr("bookings AS bk").
		Column("bk.id", "bk.code", "bk.status", "bk.channel", "bk.starts_on", "bk.ends_on", "bk.slots", "bk.formula",
			"bk.is_season_pass", "bk.total_amount", "bk.paid_amount", "bk.created_at").
		ColumnExpr("cu.first_name, cu.last_name, cu.anonymized_at").
		Join("JOIN customers AS cu ON cu.id = bk.customer_id AND cu.establishment_id = bk.establishment_id").
		Where("bk.establishment_id = ?", es.ID()).
		Where("bk.test = ?", false)
	if seasonPasses {
		q = q.Where("bk.is_season_pass = ?", true)
	}
	if from != nil {
		q = q.Where("bk.ends_on >= ?", *from)
	}
	if to != nil {
		q = q.Where("bk.starts_on <= ?", *to)
	}
	if afterID != "" {
		q = q.Where("bk.id > ?", afterID)
	}
	if err := q.OrderExpr("bk.id ASC").Limit(int64(limit)).Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: export bookings: %w", err)
	}
	return out, nil
}

// ExportPassSpots returns the spot label of the first spot line of each
// listed season pass, by booking id.
func ExportPassSpots(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(bookingIDs))
	if len(bookingIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		BookingID string `bun:"booking_id"`
		Label     string `bun:"label"`
	}
	err := db.NewSelect().TableExpr("booking_lines AS bl").
		ColumnExpr("bl.booking_id, sp.label").
		Join("JOIN spots AS sp ON sp.id = bl.spot_id").
		Where("bl.establishment_id = ?", es.ID()).
		Where("bl.booking_id IN (?)", bun.List(bookingIDs)).
		OrderExpr("bl.booking_id, bl.id").Scan(ctx, &rows)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: export pass spots: %w", err)
	}
	for _, r := range rows {
		if _, ok := out[r.BookingID]; !ok {
			out[r.BookingID] = r.Label
		}
	}
	return out, nil
}

// ExportPayment is one ledger row of the payments export.
type ExportPayment struct {
	ID           string        `bun:"id"`
	BusinessDate calendar.Date `bun:"business_date"`
	Direction    string        `bun:"direction"`
	Method       string        `bun:"method"`
	Provider     string        `bun:"provider"`
	Status       string        `bun:"status"`
	Amount       money.Cents   `bun:"amount"`
	BookingCode  *string       `bun:"booking_code"`
}

// ExportPaymentsBatch returns up to limit non-test payments of es after
// afterID with business_date in from..to (nil = unbounded).
func ExportPaymentsBatch(ctx context.Context, db bun.IDB, es EstablishmentScope, from, to *calendar.Date, afterID string, limit int) ([]ExportPayment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var out []ExportPayment
	q := db.NewSelect().TableExpr("payments AS pm").
		Column("pm.id", "pm.business_date", "pm.direction", "pm.method", "pm.provider", "pm.status", "pm.amount").
		ColumnExpr("bk.code AS booking_code").
		Join("LEFT JOIN bookings AS bk ON bk.id = pm.booking_id AND bk.establishment_id = pm.establishment_id").
		Where("pm.establishment_id = ?", es.ID()).
		Where("pm.test = ?", false)
	if from != nil {
		q = q.Where("pm.business_date >= ?", *from)
	}
	if to != nil {
		q = q.Where("pm.business_date <= ?", *to)
	}
	if afterID != "" {
		q = q.Where("pm.id > ?", afterID)
	}
	if err := q.OrderExpr("pm.id ASC").Limit(int64(limit)).Scan(ctx, &out); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: export payments: %w", err)
	}
	return out, nil
}
