package ticketing

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/store"
)

// LoadViews loads the views of tickets of one establishment, for the
// packages that change tickets outside this service (internal/domain/
// ticketassign) and must answer with the same Ticket payload.
func LoadViews(ctx context.Context, db bun.IDB, es store.EstablishmentScope, tickets []store.Ticket) ([]View, error) {
	return views(ctx, db, es, tickets)
}
