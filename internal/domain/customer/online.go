package customer

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// TagOnlinePlaceholder marks the per-establishment placeholder customer of
// held online bookings (M4, see internal/domain/booking/online.go): it is
// hidden from lists, searches, exports and retention, and never receives
// messages. Ownership: the "checkout" builder.
const TagOnlinePlaceholder = "_online_placeholder"

// OnlinePlaceholder returns the placeholder customer of es, creating it in
// tx on first use (idempotent under concurrency: unique lookup by tag and
// retry on conflict).
func (s *Service) OnlinePlaceholder(ctx context.Context, tx bun.IDB, es store.EstablishmentScope) (*store.Customer, error) {
	return nil, configerr.ErrNotImplemented
}
