package season

import (
	"context"
	"errors"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// RatePeriodInput creates or replaces a rate period.
type RatePeriodInput struct {
	SeasonID string
	Code     string
	Name     map[string]string
	Color    string
	Ranges   []store.DateRange
	// SortOrder nil appends the period after the existing ones.
	SortOrder *int
	IfMatch   *int64
	IP        string
}

// ListRatePeriods lists the rate periods of an establishment, optionally of
// one season (view).
func (s *Service) ListRatePeriods(ctx context.Context, p *auth.Principal, establishmentID, seasonID, cursor string, limit int) (store.Page[store.RatePeriod], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.RatePeriod]{}, err
	}
	page, err := store.ListRatePeriods(ctx, s.d.DB, es, seasonID, cursor, clampLimit(limit))
	return page, wrap("list rate periods", err)
}

// GetRatePeriod returns one rate period (view).
func (s *Service) GetRatePeriod(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.RatePeriod, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	rp, err := store.GetRatePeriod(ctx, s.d.DB, es, id)
	return rp, wrap("get rate period", err)
}

// CreateRatePeriod creates a rate period (configuration). Its ranges must lie
// inside the season and must not overlap any range of the other periods of
// the season; codes are unique per season. Writers of the same season are
// serialized by a lock on the season row, so concurrent creations cannot
// produce overlapping periods.
func (s *Service) CreateRatePeriod(ctx context.Context, p *auth.Principal, establishmentID string, in RatePeriodInput) (*store.RatePeriod, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.RatePeriod
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		sn, others, locale, err := s.loadPeriodContext(ctx, tx, es, in.SeasonID)
		if err != nil {
			return err
		}
		ve := &configerr.ValidationError{}
		d := ValidateRatePeriod(RatePeriodDraft{Code: in.Code, Name: in.Name, Color: in.Color, Ranges: in.Ranges}, *sn, others, locale, ve)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if codeTaken(others, d.Code, "") {
			return ErrCodeTaken
		}
		sortOrder := len(others)
		if in.SortOrder != nil {
			sortOrder = *in.SortOrder
		}
		now := s.d.Clock.Now()
		rp := &store.RatePeriod{
			ID: store.NewID(), SeasonID: sn.ID, Code: d.Code, Name: d.Name, Color: d.Color, Ranges: d.Ranges,
			SortOrder: sortOrder, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.insertRatePeriod(ctx, tx, p, es, rp, in.IP); err != nil {
			return err
		}
		out = rp
		return nil
	})
	if err != nil {
		return nil, wrap("create rate period", err)
	}
	return out, nil
}

func (s *Service) insertRatePeriod(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, rp *store.RatePeriod, ip string) error {
	if err := store.InsertRatePeriod(ctx, tx, es, rp); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrCodeTaken
		}
		return err
	}
	if err := s.record(ctx, tx, p, es, ip, ActionRatePeriodCreated, "rate_period", rp.ID, map[string]any{
		"season_id": rp.SeasonID, "code": rp.Code, "ranges": rp.Ranges,
	}); err != nil {
		return err
	}
	return s.pricingChanged(ctx, tx, es, "rate_period", rp.ID, rp.SeasonID, "created")
}

// UpdateRatePeriod replaces a rate period (configuration), with the same
// rules as CreateRatePeriod. Moving it to another season is allowed.
func (s *Service) UpdateRatePeriod(ctx context.Context, p *auth.Principal, establishmentID, id string, in RatePeriodInput) (*store.RatePeriod, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.RatePeriod
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetRatePeriod(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.IfMatch, cur.Version); err != nil {
			return err
		}
		if in.SeasonID != cur.SeasonID {
			// Lock the old season too, in a stable order.
			if err := lockSeasons(ctx, tx, es, cur.SeasonID, in.SeasonID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		sn, others, locale, err := s.loadPeriodContext(ctx, tx, es, in.SeasonID)
		if err != nil {
			return err
		}
		ve := &configerr.ValidationError{}
		d := ValidateRatePeriod(RatePeriodDraft{ID: id, Code: in.Code, Name: in.Name, Color: in.Color, Ranges: in.Ranges}, *sn, others, locale, ve)
		if err := ve.OrNil(); err != nil {
			return err
		}
		if codeTaken(others, d.Code, id) {
			return ErrCodeTaken
		}
		next := *cur
		next.SeasonID, next.Code, next.Name, next.Color, next.Ranges = sn.ID, d.Code, d.Name, d.Color, d.Ranges
		if in.SortOrder != nil {
			next.SortOrder = *in.SortOrder
		} else if in.SeasonID != cur.SeasonID {
			next.SortOrder = len(others)
		}
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateRatePeriodIfVersion(ctx, tx, es, &next, cur.Version)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrCodeTaken
			}
			return err
		}
		if !ok {
			return s.ratePeriodConflict(ctx, tx, es, id)
		}
		if err := s.record(ctx, tx, p, es, in.IP, ActionRatePeriodUpdated, "rate_period", id, map[string]any{
			"from":    map[string]any{"season_id": cur.SeasonID, "code": cur.Code, "ranges": cur.Ranges},
			"to":      map[string]any{"season_id": next.SeasonID, "code": next.Code, "ranges": next.Ranges},
			"version": next.Version,
		}); err != nil {
			return err
		}
		if err := s.pricingChanged(ctx, tx, es, "rate_period", id, next.SeasonID, "updated"); err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, wrap("update rate period", err)
	}
	return out, nil
}

// DeleteRatePeriod deletes a rate period (configuration). Price rules that
// reference its code simply stop matching; the coverage check (spec 8.7)
// reports the resulting gaps.
func (s *Service) DeleteRatePeriod(ctx context.Context, p *auth.Principal, establishmentID, id string, ifMatch *int64, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetRatePeriod(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		ok, err := store.DeleteRatePeriodIfVersion(ctx, tx, es, id, cur.Version)
		if err != nil {
			return err
		}
		if !ok {
			return s.ratePeriodConflict(ctx, tx, es, id)
		}
		if err := s.record(ctx, tx, p, es, ip, ActionRatePeriodDeleted, "rate_period", id, map[string]any{
			"season_id": cur.SeasonID, "code": cur.Code,
		}); err != nil {
			return err
		}
		return s.pricingChanged(ctx, tx, es, "rate_period", id, cur.SeasonID, "deleted")
	})
	return wrap("delete rate period", err)
}

// loadPeriodContext locks the season and loads it with its other periods
// and the establishment default locale. An unknown season is a validation
// error on season_id (the URL itself exists).
func (s *Service) loadPeriodContext(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, seasonID string) (*store.Season, []store.RatePeriod, string, error) {
	if seasonID == "" {
		return nil, nil, "", configerr.Invalid("season_id", "Obbligatorio.")
	}
	if err := store.LockSeason(ctx, tx, es, seasonID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, "", configerr.Invalid("season_id", "Stagione sconosciuta.")
		}
		return nil, nil, "", err
	}
	sn, err := store.GetSeason(ctx, tx, es, seasonID)
	if err != nil {
		return nil, nil, "", err
	}
	others, err := store.ListSeasonRatePeriods(ctx, tx, es, seasonID)
	if err != nil {
		return nil, nil, "", err
	}
	e, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, nil, "", err
	}
	return sn, others, e.DefaultLocale, nil
}

func lockSeasons(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, ids ...string) error {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		if id == "" {
			continue
		}
		if err := store.LockSeason(ctx, tx, es, id); err != nil {
			return err
		}
	}
	return nil
}

func codeTaken(periods []store.RatePeriod, code, exceptID string) bool {
	for _, p := range periods {
		if p.Code == code && p.ID != exceptID {
			return true
		}
	}
	return false
}

func (s *Service) ratePeriodConflict(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) error {
	cur, err := store.GetRatePeriod(ctx, db, es, id)
	if err != nil {
		return err
	}
	return &configerr.VersionConflictError{Current: cur.Version}
}
