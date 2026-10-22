package season

import (
	"context"
	"errors"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// SeasonInput creates or replaces a season.
type SeasonInput struct {
	Year     int
	OpensOn  calendar.Date
	ClosesOn calendar.Date
	// Status: empty means draft on create and "unchanged" on update.
	Status Status
	// IfMatch is the expected version on update (nil = no precondition).
	IfMatch *int64
	// IP is recorded in the audit log.
	IP string
}

// ListSeasons lists the seasons of an establishment, newest first (view).
func (s *Service) ListSeasons(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[store.Season], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.Season]{}, err
	}
	page, err := store.ListSeasons(ctx, s.d.DB, es, cursor, clampLimit(limit))
	return page, wrap("list seasons", err)
}

// GetSeason returns one season (view).
func (s *Service) GetSeason(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.Season, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	sn, err := store.GetSeason(ctx, s.d.DB, es, id)
	return sn, wrap("get season", err)
}

// CreateSeason creates a season (configuration). Seasons are unique per
// (establishment, year); a new season starts as draft or active.
func (s *Service) CreateSeason(ctx context.Context, p *auth.Principal, establishmentID string, in SeasonInput) (*store.Season, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	ValidateSeasonDates(in.Year, in.OpensOn, in.ClosesOn, ve)
	status := in.Status
	if status == "" {
		status = StatusDraft
	}
	if !status.Valid() {
		ve.Add("status", "Valori ammessi: draft, active, closed.")
	} else if status == StatusClosed {
		ve.Add("status", "Una nuova stagione non può essere chiusa.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	sn := &store.Season{
		ID: store.NewID(), Year: in.Year, OpensOn: in.OpensOn, ClosesOn: in.ClosesOn, Status: string(status),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		return s.insertSeason(ctx, tx, p, es, sn, in.IP, ActionSeasonCreated, nil)
	})
	if err != nil {
		return nil, wrap("create season", err)
	}
	return sn, nil
}

// insertSeason inserts sn, mapping the unique (establishment, year)
// violation, and writes the audit entry and the event.
func (s *Service) insertSeason(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, sn *store.Season, ip, action string, extra map[string]any) error {
	if _, err := store.GetSeasonByYear(ctx, tx, es, sn.Year); err == nil {
		return ErrSeasonYearTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err := store.InsertSeason(ctx, tx, es, sn); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrSeasonYearTaken
		}
		return err
	}
	diff := map[string]any{"year": sn.Year, "opens_on": sn.OpensOn, "closes_on": sn.ClosesOn, "status": sn.Status}
	for k, v := range extra {
		diff[k] = v
	}
	if err := s.record(ctx, tx, p, es, ip, action, "season", sn.ID, diff); err != nil {
		return err
	}
	evAction := "created"
	if action == ActionSeasonCopied {
		evAction = "copied"
	}
	return s.pricingChanged(ctx, tx, es, "season", sn.ID, sn.ID, evAction)
}

// UpdateSeason replaces a season (configuration). Status changes follow
// CanTransition; new dates must still contain every rate period range.
func (s *Service) UpdateSeason(ctx context.Context, p *auth.Principal, establishmentID, id string, in SeasonInput) (*store.Season, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.Season
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetSeason(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.IfMatch, cur.Version); err != nil {
			return err
		}
		ve := &configerr.ValidationError{}
		ValidateSeasonDates(in.Year, in.OpensOn, in.ClosesOn, ve)
		status := Status(cur.Status)
		if in.Status != "" {
			if !in.Status.Valid() {
				ve.Add("status", "Valori ammessi: draft, active, closed.")
			} else {
				status = in.Status
			}
		}
		if err := ve.OrNil(); err != nil {
			return err
		}
		if !CanTransition(Status(cur.Status), status) {
			return ErrInvalidStatus
		}
		if in.Year != cur.Year {
			if _, err := store.GetSeasonByYear(ctx, tx, es, in.Year); err == nil {
				return ErrSeasonYearTaken
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		if !in.OpensOn.Equal(cur.OpensOn) || !in.ClosesOn.Equal(cur.ClosesOn) {
			periods, err := store.ListSeasonRatePeriods(ctx, tx, es, cur.ID)
			if err != nil {
				return err
			}
			if out := RangesOutside(periods, in.OpensOn, in.ClosesOn); len(out) > 0 {
				return configerr.Invalid("opens_on", "Alcuni periodi tariffari uscirebbero dalla stagione: "+strings.Join(out, ", ")+".")
			}
		}
		next := *cur
		next.Year, next.OpensOn, next.ClosesOn, next.Status = in.Year, in.OpensOn, in.ClosesOn, string(status)
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateSeasonIfVersion(ctx, tx, es, &next, cur.Version)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrSeasonYearTaken
			}
			return err
		}
		if !ok {
			return s.seasonConflict(ctx, tx, es, id)
		}
		if err := s.record(ctx, tx, p, es, in.IP, ActionSeasonUpdated, "season", id, map[string]any{
			"from":    map[string]any{"year": cur.Year, "opens_on": cur.OpensOn, "closes_on": cur.ClosesOn, "status": cur.Status},
			"to":      map[string]any{"year": next.Year, "opens_on": next.OpensOn, "closes_on": next.ClosesOn, "status": next.Status},
			"version": next.Version,
		}); err != nil {
			return err
		}
		if err := s.pricingChanged(ctx, tx, es, "season", id, id, "updated"); err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, wrap("update season", err)
	}
	return out, nil
}

// DeleteSeason deletes a draft season together with its rate periods and
// price configuration (cascade). Active and closed seasons cannot be deleted.
func (s *Service) DeleteSeason(ctx context.Context, p *auth.Principal, establishmentID, id string, ifMatch *int64, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetSeason(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		if Status(cur.Status) != StatusDraft {
			return ErrSeasonNotDraft
		}
		ok, err := store.DeleteSeasonIfVersion(ctx, tx, es, id, cur.Version)
		if err != nil {
			return err
		}
		if !ok {
			return s.seasonConflict(ctx, tx, es, id)
		}
		if err := s.record(ctx, tx, p, es, ip, ActionSeasonDeleted, "season", id, map[string]any{"year": cur.Year}); err != nil {
			return err
		}
		return s.pricingChanged(ctx, tx, es, "season", id, id, "deleted")
	})
	return wrap("delete season", err)
}

func (s *Service) seasonConflict(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) error {
	cur, err := store.GetSeason(ctx, db, es, id)
	if err != nil {
		return err
	}
	return &configerr.VersionConflictError{Current: cur.Version}
}
