package setup

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
)

// Beach map kit in the templates and the demo (docs/adr/0012-kit-mappa.md):
// spots get an illustration by kind and generated maps get a lido-like set of
// kit decorations (walkway to the sea, entrance, desk, showers, bar, toilets,
// AED, palms, lifeguard tower, sea flag, JOB chair and accessible ramp).

// kitTile is the side of a modular walkway tile in map units: the kit draws
// tiles on a 120-unit viewBox, the renderer and the editor keep them at this
// scale and on this grid so that the pieces join (web/shared/src/map/kit).
const kitTile = 60.0

// kitArtScale is how much larger than its footprint (shape) a spot is drawn:
// the kit art leaves a transparent margin inside its viewBox. Keep it equal to
// SPOT_ART_SCALE in web/shared/src/map/kit/geometry.ts.
const kitArtScale = 1.3

// kitUnit is the drawn side of a template umbrella: the kit decorations of a
// generated map are sized from it, so services, furniture and spots share
// one scale instead of reading as specks next to the umbrellas.
const kitUnit = layoutUmbrellaDia * kitArtScale

// kitSpotAssets is the illustration of template spots by kind.
var kitSpotAssets = map[string]string{
	"umbrella": "ombrellone_2_lettini",
	"gazebo":   "gazebo_lettini",
}

func kitSpotAsset(kind string) *string {
	a, ok := kitSpotAssets[kind]
	if !ok {
		return nil
	}
	return &a
}

type kitRect struct{ x0, y0, x1, y1 float64 }

func (r kitRect) overlaps(o kitRect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

func kitSquare(x, y, side float64) kitRect {
	return kitRect{x - side/2, y - side/2, x + side/2, y + side/2}
}

// kitSpotRect is the drawn area of a spot (footprint enlarged by the art scale).
func kitSpotRect(x, y float64, shape store.Shape) kitRect {
	side := max(shape.W, shape.H) * kitArtScale
	return kitSquare(x, y, side)
}

// kitItem is one decoration candidate; items that do not fit the map or
// would cover a spot or an earlier item are skipped, so small custom grids
// still get a tidy map.
type kitItem struct {
	asset, kind, variant string
	x, y, size, rotation float64
}

// kitDecorations lays out the kit decorations of a map with the sea at the
// bottom. occupied are the drawn areas of the spots.
func kitDecorations(establishmentID, mapID string, width, height float64, occupied []kitRect, now time.Time) []store.MapDecoration {
	var out []store.MapDecoration
	add := func(asset, kind, variant string, x, y, w, h, rotation float64) {
		d := store.MapDecoration{
			ID: store.NewID(), EstablishmentID: establishmentID, MapID: mapID, Kind: kind, X: x, Y: y,
			Rotation: rotation, Shape: store.Shape{Type: "rect", W: w, H: h}, CreatedAt: now, UpdatedAt: now,
		}
		a := asset
		d.Asset = &a
		if variant != "" {
			v := variant
			d.Variant = &v
		}
		out = append(out, d)
	}
	placed := append([]kitRect(nil), occupied...)
	// Walkway along the left edge, from the entrance (top) down to the sea:
	// one entrance tile and one straight run drawn as repeated tiles.
	if width >= 3*kitTile && height >= 3*kitTile {
		add("passerella_ingresso", "walkway", "", kitTile/2, kitTile/2, kitTile, kitTile, 0)
		n := math.Floor((height - kitTile) / kitTile)
		run := n * kitTile
		add("passerella_verticale", "walkway", "", kitTile/2, kitTile+run/2, kitTile, run, 0)
		placed = append(placed, kitRect{0, 0, kitTile, height})
	}
	// The sign of the entrance sits beside the head of the walkway; the
	// shore row stays inside the map (items at most 56 units tall).
	top, shore := 50.0, height-28
	items := []kitItem{
		{asset: "ingresso", kind: "entrance", x: kitTile + 26, y: kitTile / 2, size: 44},
		{asset: "bar", kind: "bar", x: width - 130, y: top, size: 84},
		{asset: "doccia_doppia", kind: "shower", x: 170, y: top, size: 56},
		{asset: "torretta_bagnino", kind: "lifeguard", x: width / 2, y: shore, size: 56},
		{asset: "bandiera_mare", kind: "custom", variant: "green", x: width/2 + 56, y: shore, size: 48},
		{asset: "wc", kind: "custom", x: width - 270, y: top, size: kitUnit},
		{asset: "dae", kind: "custom", x: width - 212, y: top, size: 44},
		{asset: "rampa_accessibile", kind: "custom", x: 112, y: shore, size: kitUnit}, //nolint:misspell // kit asset id (Italian)
		{asset: "sedia_job", kind: "custom", x: 168, y: shore, size: 48},
		{asset: "pattino_salvataggio", kind: "custom", x: width/2 - 76, y: shore, size: 56, rotation: 90},
		{asset: "palma", kind: "custom", x: 240, y: top - 4, size: 64},
		{asset: "palma_piccola", kind: "custom", x: width - 40, y: top + 6, size: kitUnit},
		{asset: "fioriera_rotonda", kind: "custom", x: 300, y: top, size: 40},
		{asset: "fioriera_rotonda", kind: "custom", x: width - 320, y: top, size: 40},
		{asset: "salvagente_palo", kind: "custom", x: width - 90, y: shore, size: 44},
	}
	for _, it := range items {
		r := kitSquare(it.x, it.y, it.size)
		if r.x0 < 0 || r.y0 < 0 || r.x1 > width || r.y1 > height {
			continue
		}
		clash := false
		for _, p := range placed {
			if r.overlaps(p) {
				clash = true
				break
			}
		}
		if clash {
			continue
		}
		placed = append(placed, r)
		add(it.asset, it.kind, it.variant, it.x, it.y, it.size, it.size, it.rotation)
	}
	return out
}

// preKitTemplateDecoration reports whether d is one of the four plain
// decorations the classic template drew before the kit (entrance, bar,
// showers and lifeguard, at fixed positions of a width x height map). Only
// these are replaced by an upgrade; anything else was added by hand.
func preKitTemplateDecoration(d store.MapDecoration, width, height float64) bool {
	if d.Asset != nil || d.Shape.Type != "rect" {
		return false
	}
	type ref struct{ x, y, w, h float64 }
	refs := map[string]ref{
		"entrance":  {layoutMargin / 2, layoutMargin / 2, 40, 20},
		"bar":       {width - layoutMargin/2, layoutMargin / 2, 40, 30},
		"shower":    {layoutMargin / 2, height - layoutMargin, 20, 20},
		"lifeguard": {width / 2, height - layoutMargin/2, 30, 30},
	}
	r, ok := refs[d.Kind]
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.5 }
	return ok && near(d.X, r.x) && near(d.Y, r.y) && near(d.Shape.W, r.w) && near(d.Shape.H, r.h)
}

// kitUpgrade is what upgradeDemoMapKit changed.
type kitUpgrade struct {
	Maps, SpotsIllustrated, DecorationsRemoved, DecorationsAdded int
}

// upgradeDemoMapKit gives the kit illustrations to a demo establishment
// seeded before the kit existed: spots without an illustration get the one
// of their kind, and the four plain decorations of the pre-kit template
// (entrance, bar, showers, lifeguard, matched by kind, position and size)
// are replaced by the kit decorations; decorations added by hand are kept
// and never covered. Maps that already use the kit are left alone, so
// re-seeding is idempotent. Each changed map gets a new version (ETag) and
// an audit entry listing what changed.
func (s *Service) upgradeDemoMapKit(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, now time.Time) (kitUpgrade, error) {
	var up kitUpgrade
	var maps []store.Map
	if err := tx.NewSelect().Model(&maps).Where("m.establishment_id = ?", es.ID()).Scan(ctx); err != nil {
		return up, fmt.Errorf("setup: demo kit: list maps: %w", err)
	}
	for i := range maps {
		m := &maps[i]
		spots, err := store.ListMapSpots(ctx, tx, es, m.ID)
		if err != nil {
			return up, err
		}
		decos, err := store.ListMapDecorations(ctx, tx, es, m.ID)
		if err != nil {
			return up, err
		}
		width, height := float64(m.Width), float64(m.Height)
		illustrated := 0
		occupied := make([]kitRect, 0, len(spots)+len(decos))
		for j := range spots {
			sp := &spots[j]
			occupied = append(occupied, kitSpotRect(sp.X, sp.Y, sp.Shape))
			if sp.Asset != nil {
				continue
			}
			if sp.Asset = kitSpotAsset(sp.Kind); sp.Asset == nil {
				continue
			}
			sp.UpdatedAt = now
			if err := store.UpdateSpot(ctx, tx, es, sp); err != nil {
				return up, fmt.Errorf("setup: demo kit: spot %s: %w", sp.Label, err)
			}
			illustrated++
		}
		usesKit := false
		var template []string
		removed := make([]string, 0)
		for _, d := range decos {
			usesKit = usesKit || d.Asset != nil
			if preKitTemplateDecoration(d, width, height) {
				template = append(template, d.ID)
				removed = append(removed, d.Kind)
				continue
			}
			occupied = append(occupied, kitSquare(d.X, d.Y, max(d.Shape.W, d.Shape.H)))
		}
		var fresh []store.MapDecoration
		if !usesKit && (len(template) > 0 || len(decos) == 0) {
			if len(template) > 0 {
				if err := store.DeleteDecorations(ctx, tx, es, m.ID, template); err != nil {
					return up, err
				}
			}
			fresh = kitDecorations(es.ID(), m.ID, width, height, occupied, now)
			if len(fresh) > 0 {
				if err := store.InsertDecorations(ctx, tx, es, fresh); err != nil {
					return up, err
				}
			}
		}
		if illustrated == 0 && len(template) == 0 && len(fresh) == 0 {
			continue
		}
		if _, err := tx.NewUpdate().Model((*store.Map)(nil)).
			Set("version = version + 1").Set("updated_at = ?", now).
			Where("m.id = ?", m.ID).Where("m.establishment_id = ?", es.ID()).Exec(ctx); err != nil {
			return up, fmt.Errorf("setup: demo kit: bump map version: %w", err)
		}
		added := make([]string, 0, len(fresh))
		for _, d := range fresh {
			added = append(added, *d.Asset)
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: "setup.demo_map_kit_upgraded",
			EntityType: "map", EntityID: m.ID, Diff: map[string]any{
				"spots_illustrated": illustrated, "decorations_removed": removed, "decorations_added": added,
			},
		}); err != nil {
			return up, err
		}
		up.Maps++
		up.SpotsIllustrated += illustrated
		up.DecorationsRemoved += len(template)
		up.DecorationsAdded += len(fresh)
	}
	return up, nil
}
