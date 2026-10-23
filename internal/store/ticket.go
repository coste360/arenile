package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// NextTicketNumber advances the daily counter of the establishment for date
// and returns the new number (1 for the first ticket of the day). It is one
// atomic upsert: run it inside the sale transaction, so a rollback gives
// the number back and the UNIQUE key on tickets never sees a duplicate.
// SQLite serialises writers; PostgreSQL locks the conflicting counter row
// until commit, so concurrent sales on any register get distinct numbers.
func NextTicketNumber(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, now time.Time) (int64, error) {
	if err := es.mustBeValid(); err != nil {
		return 0, err
	}
	var n int64
	err := db.NewRaw(
		`INSERT INTO daily_ticket_counters (establishment_id, ticket_date, last_number, updated_at)
		 VALUES (?, ?, 1, ?)
		 ON CONFLICT (establishment_id, ticket_date)
		 DO UPDATE SET last_number = daily_ticket_counters.last_number + 1, updated_at = excluded.updated_at
		 RETURNING last_number`,
		es.ID(), date, now,
	).Scan(ctx, &n)
	if err != nil {
		return 0, fmt.Errorf("store: next ticket number: %w", err)
	}
	return n, nil
}
