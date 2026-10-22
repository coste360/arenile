package season

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ClosureDayInput creates or replaces an extraordinary closure.
type ClosureDayInput struct {
	Date   calendar.Date
	Slots  []calendar.Slot
	Reason string
	// IfMatch is the expected version on update (nil = no precondition).
	IfMatch *int64
	IP      string
}

// ListClosureDays lists closure days by date, optionally within [from, to]
// (zero dates are open bounds) (view).
func (s *Service) ListClosureDays(ctx context.Context, p *auth.Principal, establishmentID string, from, to calendar.Date, cursor string, limit int) (store.Page[store.ClosureDay], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.ClosureDay]{}, err
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return store.Page[store.ClosureDay]{}, configerr.Invalid("to", "La data finale non può precedere quella iniziale.")
	}
	page, err := store.ListClosureDays(ctx, s.d.DB, es, store.ClosureDayFilter{From: from, To: to}, cursor, clampLimit(limit))
	return page, wrap("list closure days", err)
}

// GetClosureDay returns one closure day (view).
func (s *Service) GetClosureDay(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.ClosureDay, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	c, err := store.GetClosureDay(ctx, s.d.DB, es, id)
	return c, wrap("get closure day", err)
}

// CreateClosureDay records an extraordinary closure (configuration). The
// date must fall inside a season of the establishment; slots are a non-empty
// subset of AM/PM; one closure per date.
func (s *Service) CreateClosureDay(ctx context.Context, p *auth.Principal, establishmentID string, in ClosureDayInput) (*store.ClosureDay, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.ClosureDay
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		slots, reason, err := validateClosure(ctx, tx, es, in)
		if err != nil {
			return err
		}
		now := s.d.Clock.Now()
		c := &store.ClosureDay{ID: store.NewID(), Date: in.Date, Slots: slots, Reason: reason, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := store.InsertClosureDay(ctx, tx, es, c); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrClosureDateTaken
			}
			return err
		}
		if err := s.record(ctx, tx, p, es, in.IP, ActionClosureCreated, "closure_day", c.ID, map[string]any{
			"date": c.Date, "slots": c.Slots, "reason": c.Reason,
		}); err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		return nil, wrap("create closure day", err)
	}
	return out, nil
}

// UpdateClosureDay replaces a closure day (configuration).
func (s *Service) UpdateClosureDay(ctx context.Context, p *auth.Principal, establishmentID, id string, in ClosureDayInput) (*store.ClosureDay, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.ClosureDay
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetClosureDay(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.IfMatch, cur.Version); err != nil {
			return err
		}
		slots, reason, err := validateClosure(ctx, tx, es, in)
		if err != nil {
			return err
		}
		next := *cur
		next.Date, next.Slots, next.Reason = in.Date, slots, reason
		next.Version = cur.Version + 1
		next.UpdatedAt = s.d.Clock.Now()
		ok, err := store.UpdateClosureDayIfVersion(ctx, tx, es, &next, cur.Version)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrClosureDateTaken
			}
			return err
		}
		if !ok {
			return s.closureConflict(ctx, tx, es, id)
		}
		if err := s.record(ctx, tx, p, es, in.IP, ActionClosureUpdated, "closure_day", id, map[string]any{
			"from":    map[string]any{"date": cur.Date, "slots": cur.Slots, "reason": cur.Reason},
			"to":      map[string]any{"date": next.Date, "slots": next.Slots, "reason": next.Reason},
			"version": next.Version,
		}); err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, wrap("update closure day", err)
	}
	return out, nil
}

// DeleteClosureDay removes a closure day (configuration).
func (s *Service) DeleteClosureDay(ctx context.Context, p *auth.Principal, establishmentID, id string, ifMatch *int64, ip string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetClosureDay(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(ifMatch, cur.Version); err != nil {
			return err
		}
		ok, err := store.DeleteClosureDayIfVersion(ctx, tx, es, id, cur.Version)
		if err != nil {
			return err
		}
		if !ok {
			return s.closureConflict(ctx, tx, es, id)
		}
		return s.record(ctx, tx, p, es, ip, ActionClosureDeleted, "closure_day", id, map[string]any{"date": cur.Date})
	})
	return wrap("delete closure day", err)
}

// validateClosure checks the input and that the date falls in a season.
func validateClosure(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in ClosureDayInput) ([]calendar.Slot, string, error) {
	ve := &configerr.ValidationError{}
	if in.Date.IsZero() {
		ve.Add("date", "Obbligatoria.")
	}
	slots := NormalizeSlots(in.Slots, ve)
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > MaxReasonLength {
		ve.Add("reason", fmt.Sprintf("Al massimo %d caratteri.", MaxReasonLength))
	}
	if err := ve.OrNil(); err != nil {
		return nil, "", err
	}
	seasons, err := store.SeasonsCovering(ctx, tx, es, in.Date)
	if err != nil {
		return nil, "", err
	}
	if len(seasons) == 0 {
		return nil, "", configerr.Invalid("date", "La data non rientra in nessuna stagione dello stabilimento.")
	}
	return slots, reason, nil
}

func (s *Service) closureConflict(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) error {
	cur, err := store.GetClosureDay(ctx, db, es, id)
	if err != nil {
		return err
	}
	return &configerr.VersionConflictError{Current: cur.Version}
}
