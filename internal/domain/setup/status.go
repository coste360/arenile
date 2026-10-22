package setup

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// Status is the progress of the guided setup (api.SetupStatus).
type Status struct {
	HasSeason      bool
	HasRatePeriods bool
	HasMap         bool
	SpotCount      int
	HasCatalog     bool
	HasPrices      bool
	BookingEnabled bool
	// PriceCategories are the categories the quick price list must price:
	// those of the zones and of the spot overrides, sorted.
	PriceCategories []string
}

// ListTemplates returns the setup templates (auth.PermBookingsRead on the
// establishment: every role can see them).
func (s *Service) ListTemplates(ctx context.Context, p *auth.Principal, establishmentID string) ([]Template, error) {
	if _, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead); err != nil {
		return nil, err
	}
	return Templates(), nil
}

// Status returns the guided setup progress (auth.PermBookingsRead).
func (s *Service) Status(ctx context.Context, p *auth.Principal, establishmentID string) (*Status, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, s.d.DB, es)
}

func (s *Service) status(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (*Status, error) {
	e, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("setup: status: %w", err)
	}
	count := func(model any, alias, extra string, args ...any) (int, error) {
		q := db.NewSelect().Model(model).Where(alias+".establishment_id = ?", es.ID())
		if extra != "" {
			q = q.Where(extra, args...)
		}
		n, err := q.Count(ctx)
		if err != nil {
			return 0, fmt.Errorf("setup: status: %w", err)
		}
		return int(n), nil
	}
	st := &Status{BookingEnabled: e.BookingEnabled}
	checks := []struct {
		dst   *int
		model any
		alias string
		extra string
		args  []any
	}{
		{nil, (*store.Season)(nil), "sn", "", nil},
		{nil, (*store.RatePeriod)(nil), "rp", "", nil},
		{nil, (*store.Map)(nil), "m", "", nil},
		{&st.SpotCount, (*store.Spot)(nil), "sp", "sp.active = ?", []any{true}},
		{nil, (*store.Pack)(nil), "pa", "pa.active = ?", []any{true}},
		{nil, (*store.PriceRule)(nil), "pr", "pr.stage = ? AND pr.active = ?", []any{"base", true}},
	}
	results := make([]int, len(checks))
	for i, c := range checks {
		n, err := count(c.model, c.alias, c.extra, c.args...)
		if err != nil {
			return nil, err
		}
		results[i] = n
		if c.dst != nil {
			*c.dst = n
		}
	}
	st.HasSeason = results[0] > 0
	st.HasRatePeriods = results[1] > 0
	st.HasMap = results[2] > 0
	st.HasCatalog = results[4] > 0
	st.HasPrices = results[5] > 0
	if st.PriceCategories, err = categoriesInUse(ctx, db, es); err != nil {
		return nil, err
	}
	return st, nil
}
