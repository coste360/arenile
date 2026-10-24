package cash

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// ZReportData is everything the printable Z closing needs (ADR 0019): the
// session detail with its summary, the establishment header and the names
// of the operators. Rendering lives in internal/printout.
type ZReportData struct {
	Detail        *Detail
	Establishment *store.Establishment
	// Organization is nil when it cannot be read (the header then shows the
	// establishment only).
	Organization *store.Organization
	OpenedByName string
	ClosedByName string
	Location     *time.Location
	GeneratedAt  time.Time
}

// ZReport loads the data of the printable Z closing of a session (open
// sessions give a provisional summary). Requires auth.PermCashOperate or
// auth.PermReportsRead.
func (s *Service) ZReport(ctx context.Context, p *auth.Principal, establishmentID, sessionID string) (*ZReportData, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermCashOperate, auth.PermReportsRead)
	if err != nil {
		return nil, err
	}
	cs, err := store.GetCashSession(ctx, s.d.DB, es, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("cash: %w", err)
	}
	d, err := s.detail(ctx, s.d.DB, es, cs)
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("cash: load establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("cash: timezone %q: %w", est.Timezone, err)
	}
	out := &ZReportData{Detail: d, Establishment: est, Location: loc, GeneratedAt: s.now()}
	if org, err := store.GetOrganization(ctx, s.d.DB, est.OrganizationID); err == nil {
		out.Organization = org
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("cash: load organization: %w", err)
	}
	var ids []string
	if cs.OpenedBy != nil {
		ids = append(ids, *cs.OpenedBy)
	}
	if cs.ClosedBy != nil {
		ids = append(ids, *cs.ClosedBy)
	}
	if len(ids) > 0 {
		names, err := store.UserNames(ctx, s.d.DB, ids)
		if err != nil {
			return nil, fmt.Errorf("cash: operator names: %w", err)
		}
		if cs.OpenedBy != nil {
			out.OpenedByName = names[*cs.OpenedBy]
		}
		if cs.ClosedBy != nil {
			out.ClosedByName = names[*cs.ClosedBy]
		}
	}
	return out, nil
}
