package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Repository of the daily tickets, entry ticket types and daily sale
// settings (migration 00010, ADR 0020). Owned by internal/domain/ticketing
// and internal/domain/dailysale. Every function takes an EstablishmentScope.

// TicketFilter narrows ListDayTickets; zero fields do not filter.
type TicketFilter struct {
	Kind   string
	ZoneID string
	// UnassignedOnly keeps the valid zone-first tickets still to assign.
	UnassignedOnly bool
}

// InsertTicket inserts a ticket. A duplicate (establishment, date, number)
// or booking is a unique violation (returned unwrapped by the caller check
// IsUniqueViolation, also through the ErrConflict wrap).
func InsertTicket(ctx context.Context, db bun.IDB, es EstablishmentScope, t *Ticket) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	t.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(t).Exec(ctx)
	if err != nil && IsUniqueViolation(err) {
		return fmt.Errorf("store: insert ticket: %w: %w", ErrConflict, err)
	}
	return wrapWrite("insert ticket", err)
}

// GetTicket returns a ticket of es.
func GetTicket(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).Where("tk.id = ?", id).Where("tk.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get ticket", err)
	}
	return t, nil
}

// LockTicket locks a ticket row for the rest of the transaction (no-op
// UPDATE: a row lock on PostgreSQL, the write lock on SQLite) and returns it.
func LockTicket(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*Ticket)(nil)).Set("version = version").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: lock ticket: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetTicket(ctx, db, es, id)
}

// GetTicketByBooking returns the ticket owning a booking.
func GetTicketByBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).Where("tk.booking_id = ?", bookingID).Where("tk.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get ticket by booking", err)
	}
	return t, nil
}

// GetTicketByGateToken returns the ticket whose QR carries token (the
// random gate token, ADR 0020).
func GetTicketByGateToken(ctx context.Context, db bun.IDB, es EstablishmentScope, token string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).Where("tk.gate_token = ?", token).Where("tk.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get ticket by gate token", err)
	}
	return t, nil
}

// GetTicketByNumber returns the ticket of es with the daily number on date
// (the number printed large on the ticket).
func GetTicketByNumber(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, number int64) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).Where("tk.establishment_id = ?", es.ID()).
		Where("tk.ticket_date = ?", date).Where("tk.ticket_number = ?", number).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get ticket by number", err)
	}
	return t, nil
}

// GetTicketByBookingCode returns the ticket whose booking has code (printed
// under the QR of the ticket).
func GetTicketByBookingCode(ctx context.Context, db bun.IDB, es EstablishmentScope, code string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).
		Join("JOIN bookings AS bk ON bk.id = tk.booking_id").
		Where("bk.code = ?", code).Where("bk.establishment_id = ?", es.ID()).Where("tk.establishment_id = ?", es.ID()).
		Scan(ctx)
	if err != nil {
		return nil, wrapRead("get ticket by code", err)
	}
	return t, nil
}

// ListDayTickets returns the tickets of es for date, by number.
func ListDayTickets(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, f TicketFilter) ([]Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []Ticket
	q := db.NewSelect().Model(&rows).Where("tk.establishment_id = ?", es.ID()).Where("tk.ticket_date = ?", date).
		OrderExpr("tk.ticket_number ASC")
	if f.Kind != "" {
		q = q.Where("tk.kind = ?", f.Kind)
	}
	if f.ZoneID != "" {
		q = q.Where("tk.zone_id = ?", f.ZoneID)
	}
	if f.UnassignedOnly {
		q = q.Where("tk.needs_assignment = ?", true).Where("tk.status = ?", TicketValid)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list day tickets: %w", err)
	}
	return rows, nil
}

// ticketMutableColumns are the columns rewritten by UpdateTicketVersioned.
var ticketMutableColumns = []string{
	"status", "needs_assignment", "provisional_spot_id", "holder_name", "print_count",
	"voided_at", "voided_by_user_id", "void_reason", "refund_payment_id",
	"entered_at", "entered_by_user_id", "version", "updated_at",
}

// UpdateTicketVersioned rewrites the mutable columns of t when its version
// is still expected; false when another writer got there first.
func UpdateTicketVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, t *Ticket, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	t.Version = expected + 1
	res, err := db.NewUpdate().Model(t).Column(ticketMutableColumns...).
		Where("id = ?", t.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		t.Version = expected
		return false, wrapWrite("update ticket", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Version = expected
		return false, nil
	}
	return true, nil
}

// MarkTicketEntered records the gate entry of a valid ticket with one
// conditional UPDATE (entered_at IS NULL): of two concurrent scans exactly
// one changes the row. It reports whether this call recorded the entry.
func MarkTicketEntered(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, by *string, at time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*Ticket)(nil)).
		Set("entered_at = ?", at).Set("entered_by_user_id = ?", by).Set("updated_at = ?", at).Set("version = version + 1").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("entered_at IS NULL").Where("status = ?", TicketValid).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: mark ticket entered: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// IncrementTicketPrints adds one print to the ticket and returns the new count.
func IncrementTicketPrints(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) (int, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	res, err := db.NewUpdate().Model((*Ticket)(nil)).
		Set("print_count = print_count + 1").Set("updated_at = ?", at).Set("version = version + 1").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: ticket prints: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	t, err := GetTicket(ctx, db, es, id)
	if err != nil {
		return 0, err
	}
	return t.PrintCount, nil
}

// PaymentsByIDs returns the payments of es with the given ids.
func PaymentsByIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Payment, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Payment
	err := db.NewSelect().Model(&rows).Where("pm.establishment_id = ?", es.ID()).Where("pm.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: payments by ids: %w", err)
	}
	return rows, nil
}

// BookingsByIDs returns the bookings of es with the given ids.
func BookingsByIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, ids []string) ([]Booking, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Booking
	err := db.NewSelect().Model(&rows).Where("bk.establishment_id = ?", es.ID()).Where("bk.id IN (?)", bun.List(ids)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: bookings by ids: %w", err)
	}
	return rows, nil
}

// BookingLineExtrasOf returns the extras of the given bookings.
func BookingLineExtrasOf(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) ([]BookingLineExtra, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	var rows []BookingLineExtra
	err := db.NewSelect().Model(&rows).Where("ble.establishment_id = ?", es.ID()).Where("ble.booking_id IN (?)", bun.List(bookingIDs)).
		OrderExpr("ble.created_at, ble.id").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: booking line extras: %w", err)
	}
	return rows, nil
}

// ---- entry ticket types ------------------------------------------------------

// ListEntryTicketTypes returns the entry ticket types of es by sort order.
func ListEntryTicketTypes(ctx context.Context, db bun.IDB, es EstablishmentScope) ([]EntryTicketType, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	var rows []EntryTicketType
	err := db.NewSelect().Model(&rows).Where("ett.establishment_id = ?", es.ID()).OrderExpr("ett.sort_order, ett.code, ett.id").Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list entry ticket types: %w", err)
	}
	return rows, nil
}

// GetEntryTicketType returns an entry ticket type of es.
func GetEntryTicketType(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*EntryTicketType, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(EntryTicketType)
	err := db.NewSelect().Model(t).Where("ett.id = ?", id).Where("ett.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get entry ticket type", err)
	}
	return t, nil
}

// InsertEntryTicketType inserts a type; a duplicate code or pack is ErrConflict.
func InsertEntryTicketType(ctx context.Context, db bun.IDB, es EstablishmentScope, t *EntryTicketType) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	t.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(t).Exec(ctx)
	return wrapWrite("insert entry ticket type", err)
}

// UpdateEntryTicketTypeVersioned rewrites t when its version is expected.
func UpdateEntryTicketTypeVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, t *EntryTicketType, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	t.Version = expected + 1
	res, err := db.NewUpdate().Model(t).
		Column("name", "description", "pack_id", "zone_id", "sort_order", "active", "version", "updated_at").
		Where("id = ?", t.ID).Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		t.Version = expected
		return false, wrapWrite("update entry ticket type", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Version = expected
		return false, nil
	}
	return true, nil
}

// DeleteEntryTicketType deletes a type of es.
func DeleteEntryTicketType(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	res, err := db.NewDelete().Model((*EntryTicketType)(nil)).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return wrapWrite("delete entry ticket type", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PackSold reports whether a booking line of es uses the pack.
func PackSold(ctx context.Context, db bun.IDB, es EstablishmentScope, packID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	ok, err := db.NewSelect().Model((*BookingLine)(nil)).Where("bl.establishment_id = ?", es.ID()).Where("bl.pack_id = ?", packID).Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("store: pack sold: %w", err)
	}
	return ok, nil
}

// ---- daily sale settings -----------------------------------------------------

// GetDailySaleSettings returns the settings row of es (ErrNotFound when the
// establishment never saved one).
func GetDailySaleSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*DailySaleSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	s := new(DailySaleSettings)
	err := db.NewSelect().Model(s).Where("dss.establishment_id = ?", es.ID()).Scan(ctx)
	if err != nil {
		return nil, wrapRead("get daily sale settings", err)
	}
	return s, nil
}

// EnsureDailySaleSettings inserts the default settings row of es when it is
// missing (ON CONFLICT DO NOTHING, safe under concurrency).
func EnsureDailySaleSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	s := &DailySaleSettings{
		EstablishmentID: es.ID(), PrintFormat: PrintThermal80, AutoPrint: true, ZoneFirstEnabled: true,
		FooterText: I18nText{}, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(s).On("CONFLICT (establishment_id) DO NOTHING").Exec(ctx)
	return wrapWrite("ensure daily sale settings", err)
}

// LockDailySaleSettings locks the settings row of es (it must exist, see
// EnsureDailySaleSettings) and returns it.
func LockDailySaleSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*DailySaleSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	res, err := db.NewUpdate().Model((*DailySaleSettings)(nil)).Set("version = version").
		Where("establishment_id = ?", es.ID()).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: lock daily sale settings: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return GetDailySaleSettings(ctx, db, es)
}

// UpdateDailySaleSettingsVersioned rewrites the settings when the version
// is expected (nil expected: unconditional, version still bumped).
func UpdateDailySaleSettingsVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, s *DailySaleSettings, expected int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	s.Version = expected + 1
	res, err := db.NewUpdate().Model(s).
		Column("walk_in_customer_id", "print_format", "auto_print", "zone_first_enabled", "footer_text", "version", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("version = ?", expected).Exec(ctx)
	if err != nil {
		s.Version = expected
		return false, wrapWrite("update daily sale settings", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.Version = expected
		return false, nil
	}
	return true, nil
}

// SetDailySaleWalkIn records the walk-in customer of es (internal
// bookkeeping: the version, the ETag of the settings, is not bumped).
func SetDailySaleWalkIn(ctx context.Context, db bun.IDB, es EstablishmentScope, customerID string, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*DailySaleSettings)(nil)).
		Set("walk_in_customer_id = ?", customerID).Set("updated_at = ?", now).
		Where("establishment_id = ?", es.ID()).Exec(ctx)
	return wrapWrite("set walk-in customer", err)
}
