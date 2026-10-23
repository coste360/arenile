package rollover

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// SeasonCopier copies season and price list inside the caller's
// transaction. Implemented by *pricingconfig.Service: the rollover builder
// adds CopySeasonInTx in a new file (internal/domain/pricingconfig/
// pricing_copy_tx.go) that runs the body of CopySeason on tx without
// opening its own transaction (CopySeason then delegates to it).
type SeasonCopier interface {
	CopySeasonInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, sourceSeasonID string, in pricingconfig.CopySeasonInput) (*pricingconfig.CopySeasonResult, error)
}
