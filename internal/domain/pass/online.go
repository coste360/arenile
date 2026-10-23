package pass

import (
	"context"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ReleaseAsCustomer releases days of a season pass from the manage area
// (spec 7.7, 12.4): allowed only when the policy snapshot has
// customer_can_release_season_days; credit per season_release_credit.
// actor is audit.ActorCustomer (magic link); es is resolved by the caller,
// which authenticated the magic link of passID. Ownership: the "checkout"
// builder; implements checkout.SeasonPasses (asserted in internal/app/m4.go).
func (s *Service) ReleaseAsCustomer(ctx context.Context, actor audit.Actor, es store.EstablishmentScope, passID string, in ReleaseInput) ([]store.SeasonRelease, error) {
	return nil, configerr.ErrNotImplemented
}
