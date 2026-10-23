package setup

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/store"
)

// Demo upgrade to the sea at the top (M5A, spec 11): the classic template
// now draws the sea at the top with row A nearest to it. A demo seeded
// before keeps working, but on the next `seed demo` its template maps are
// reoriented with the same pure transform as the panel
// (mapping.ReorientLayout, a mirror for top <-> bottom).
//
// Safety rules:
//   - only maps of the demo establishment with the sea at the bottom that
//     carry the template walkway entrance (the kit tile in the top-left
//     corner, laid out by the classic template or by upgradeDemoMapKit) are
//     touched: maps drawn by hand are left alone;
//   - geometry only: spot ids, labels, rows, zones and allocations are
//     unchanged, so bookings and season passes keep their spots;
//   - each map gets one versioned update (version + 1), an audit entry
//     (setup.demo_map_reoriented) and config.map_changed with change
//     "reoriented", in the seed transaction;
//   - idempotent: a reoriented map has the sea at the top and is skipped.

// isTemplateEntrance reports whether d is the walkway entrance tile the
// classic template lays out on the left edge, away from the sea: in the
// top-left corner with the sea at the bottom, at the bottom (above the last
// whole tile) with the sea at the top.
func isTemplateEntrance(d store.MapDecoration, height float64, seaSide string) bool {
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.5 }
	if d.Asset == nil || *d.Asset != "passerella_ingresso" || !near(d.X, kitTile/2) || !near(d.Shape.W, kitTile) || !near(d.Shape.H, kitTile) {
		return false
	}
	if seaSide == mapping.SeaTop {
		return d.Y > height/2
	}
	return near(d.Y, kitTile/2)
}

// upgradeDemoSeaTop moves the sea of the demo template maps to the top and
// returns how many maps changed.
func (s *Service) upgradeDemoSeaTop(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, now time.Time) (int, error) {
	var maps []store.Map
	if err := tx.NewSelect().Model(&maps).Where("m.establishment_id = ?", es.ID()).
		Where("m.sea_side = ?", mapping.SeaBottom).OrderExpr("m.sort_order, m.id").Scan(ctx); err != nil {
		return 0, fmt.Errorf("setup: demo sea top: list maps: %w", err)
	}
	changed := 0
	for i := range maps {
		ok, err := s.reorientDemoMap(ctx, tx, es, &maps[i], now)
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	return changed, nil
}

func (s *Service) reorientDemoMap(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, m *store.Map, now time.Time) (bool, error) {
	return s.reorientDemoMapTo(ctx, tx, es, m, mapping.SeaTop, now)
}

// reorientDemoMapTo moves the sea of a template map to side `to` (the
// upgrade uses the top; the tests use it to rebuild a pre-M5 demo).
func (s *Service) reorientDemoMapTo(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, m *store.Map, to string, now time.Time) (bool, error) {
	spots, err := store.ListMapSpots(ctx, tx, es, m.ID)
	if err != nil {
		return false, err
	}
	decos, err := store.ListMapDecorations(ctx, tx, es, m.ID)
	if err != nil {
		return false, err
	}
	template := false
	for _, d := range decos {
		template = template || isTemplateEntrance(d, float64(m.Height), m.SeaSide)
	}
	if !template {
		return false, nil
	}
	in := mapping.SaveInput{Map: mapping.MapInput{Width: m.Width, Height: m.Height, GridSize: m.GridSize, SeaSide: m.SeaSide}}
	for _, sp := range spots {
		in.Spots = append(in.Spots, mapping.SpotInput{ID: sp.ID, X: sp.X, Y: sp.Y, Rotation: sp.Rotation, Shape: sp.Shape})
	}
	for _, d := range decos {
		in.Decorations = append(in.Decorations, mapping.DecorationInput{ID: d.ID, Kind: d.Kind, Asset: deref(d.Asset), X: d.X, Y: d.Y, Rotation: d.Rotation, Shape: d.Shape})
	}
	out, tr, err := mapping.ReorientLayout(in, to)
	if err != nil {
		return false, err
	}
	for i := range spots {
		sp, g := &spots[i], out.Spots[i]
		sp.X, sp.Y, sp.Rotation, sp.UpdatedAt = g.X, g.Y, g.Rotation, now
		if err := store.UpdateSpot(ctx, tx, es, sp); err != nil {
			return false, fmt.Errorf("setup: demo sea top: spot %s: %w", sp.Label, err)
		}
	}
	for i := range decos {
		d, g := &decos[i], out.Decorations[i]
		d.X, d.Y, d.Rotation, d.Shape, d.UpdatedAt = g.X, g.Y, g.Rotation, g.Shape, now
		if g.Asset != "" {
			a := g.Asset
			d.Asset = &a
		}
		if err := store.UpdateDecoration(ctx, tx, es, d); err != nil {
			return false, fmt.Errorf("setup: demo sea top: decoration %s: %w", d.ID, err)
		}
	}
	from := m.Version
	next := *m
	next.SeaSide, next.Width, next.Height, next.UpdatedAt = out.Map.SeaSide, out.Map.Width, out.Map.Height, now
	ok, err := store.UpdateMapVersioned(ctx, tx, es, &next, from)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("setup: demo sea top: map %s changed meanwhile", m.ID)
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: "setup.demo_map_reoriented",
		EntityType: "map", EntityID: m.ID, Diff: map[string]any{
			"from_sea_side": m.SeaSide, "to_sea_side": to, "transform": tr,
			"from_version": from, "to_version": next.Version, "spots": len(spots), "decorations": len(decos),
		},
	}); err != nil {
		return false, err
	}
	data := event.ConfigChange{Entity: "map", EntityID: m.ID, Change: event.ChangeReoriented, MapID: m.ID, Version: next.Version}
	if _, err := s.d.Outbox.Write(ctx, tx, event.ConfigMapChanged, es.ID(), 1, data); err != nil {
		return false, err
	}
	return true, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
