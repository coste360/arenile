package mapping

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Sea side change (M5A, spec 11): Italian lidos usually draw the sea at the
// top, so "top" is the default of new maps, of the row generator and of the
// templates. When the sea side of an existing map changes, the whole layout
// can follow it (ReorientLayout, pure; ReorientMap, one versioned save) so
// that every row keeps its distance from the sea.

// Transform is the geometry change that moves the sea to another side
// (API enum MapTransform).
type Transform string

// Transforms of ReorientTransform.
const (
	// TransformNone: same side, nothing to do.
	TransformNone Transform = "none"
	// TransformMirrorY: top <-> bottom, y' = height - y, rotation r' = -r;
	// the order along the shore is unchanged.
	TransformMirrorY Transform = "mirror_y"
	// TransformMirrorX: left <-> right, x' = width - x, rotation r' = -r.
	TransformMirrorX Transform = "mirror_x"
	// TransformRotateCW: 90 degrees clockwise (top -> right -> bottom ->
	// left): (x, y) -> (height - y, x), width and height swapped, r' = r + 90.
	TransformRotateCW Transform = "rotate_cw"
	// TransformRotateCCW: 90 degrees counter-clockwise: (x, y) -> (y,
	// width - x), width and height swapped, r' = r - 90.
	TransformRotateCCW Transform = "rotate_ccw"
)

// ReorientTransform returns the transform that moves the sea from one side
// to another (pure). Opposite sides mirror, adjacent sides rotate.
func ReorientTransform(from, to string) Transform {
	if from == to {
		return TransformNone
	}
	order := map[string]int{SeaTop: 0, SeaRight: 1, SeaBottom: 2, SeaLeft: 3}
	f, okF := order[from]
	t, okT := order[to]
	if !okF || !okT {
		return TransformNone
	}
	switch (t - f + 4) % 4 {
	case 1:
		return TransformRotateCW
	case 3:
		return TransformRotateCCW
	}
	if from == SeaTop || from == SeaBottom {
		return TransformMirrorY
	}
	return TransformMirrorX
}

// Walkway tiles of the map kit (manifest "tile": true, ADR 0012): modular
// pieces drawn at KIT_TILE scale whose edges sit on a 60-unit grid. Keep in
// sync with assets/beach-map-kit/manifest.json
// (TestReorient_TileAssetsAreKitAssets checks they exist).
const (
	kitTileSide       = 60.0
	tileStraightH     = "passerella_orizzontale"
	tileStraightV     = "passerella_verticale"
	walkwayDecoration = "walkway"
)

var kitTileAssets = []string{
	"passerella_angolo", "passerella_croce", "passerella_curva", "passerella_ingresso",
	tileStraightH, "passerella_t", "passerella_terminale", tileStraightV,
}

// kitMirrorAxis is the mirror axis (degrees in screen coordinates: 0 = +x,
// 90 = +y) at rotation 0 of the kit art that is not symmetric about both
// the horizontal and the vertical axis: the corner and the curve join the
// top and right edges (axis -45), the terminal piece runs from the centre
// down (axis 90). A rotation cannot mirror a drawing, but art with a mirror
// axis α reflected across the horizontal axis equals the same art rotated
// by -2α, so the mirrored piece is drawn at r' = -r - 2α (mirror_y) or
// r' = 180 - r - 2α (mirror_x). Every other element, spots included, is
// treated as symmetric (r' = -r), like the row generator, which gives top
// and bottom rows the same rotation.
var kitMirrorAxis = map[string]float64{
	"passerella_angolo":    -45,
	"passerella_curva":     -45,
	"passerella_terminale": 90,
}

func isKitTile(asset string) bool { return slices.Contains(kitTileAssets, asset) }

// tilesSnapToGrid mirrors the editor rule (map-editor tilesSnapToGrid):
// walkway tiles keep the 60-unit tile grid only when it is a subset of the
// map grid.
func tilesSnapToGrid(gridSize int) bool {
	return gridSize > 0 && int(kitTileSide)%gridSize == 0
}

// ReorientLayout moves the sea of the document in to the side `to` and
// transforms the geometry so that every element keeps its distance from the
// sea (pure; in is not modified). Spot and decoration x/y are the centre of
// their shape (web/shared/src/map/types.ts):
//
//	mirror_y   (x, y) -> (x, H - y)   r' = -r (mirror axis aware)
//	mirror_x   (x, y) -> (W - x, y)   r' = -r (mirror axis aware)
//	rotate_cw  (x, y) -> (H - y, x)   r' = r + 90, W and H swapped
//	rotate_ccw (x, y) -> (y, W - x)   r' = r - 90, W and H swapped
//
// Coordinates are rounded to 1e-6 and rotations normalised to [0, 360), so
// reorienting back is the identity for layouts with at most six decimals.
// Straight walkway tiles turned by 90 degrees swap their sides and their
// art (horizontal <-> vertical) instead of being rotated, which is how the
// editor draws and snaps them. Walkway tiles lying on the 60-unit tile grid
// are put back on it when the map size is not a multiple of 60 (round half
// up, so that going back restores them), unless that would move their
// centre out of the map. Ids, labels, rows, zones and every non-geometric
// field are kept.
func ReorientLayout(in SaveInput, to string) (SaveInput, Transform, error) {
	if !slices.Contains(SeaSides, to) {
		return in, TransformNone, configerr.Invalid("sea_side", "Valore non valido.")
	}
	if !slices.Contains(SeaSides, in.Map.SeaSide) {
		return in, TransformNone, configerr.Invalid("map.sea_side", "Lato mare della mappa non valido.")
	}
	tr := ReorientTransform(in.Map.SeaSide, to)
	out := SaveInput{Map: in.Map, Spots: slices.Clone(in.Spots), Decorations: slices.Clone(in.Decorations)}
	out.Map.SeaSide = to
	if tr == TransformNone {
		return out, tr, nil
	}
	g := geometry{tr: tr, w: float64(in.Map.Width), h: float64(in.Map.Height)}
	if g.quarterTurn() {
		out.Map.Width, out.Map.Height = in.Map.Height, in.Map.Width
	}
	for i := range out.Spots {
		sp := &out.Spots[i]
		sp.X, sp.Y = g.point(sp.X, sp.Y)
		sp.Rotation = g.rotation(sp.Rotation, "")
	}
	snap := tilesSnapToGrid(in.Map.GridSize)
	newW, newH := float64(out.Map.Width), float64(out.Map.Height)
	for i := range out.Decorations {
		d := &out.Decorations[i]
		asset := d.Asset
		if asset == "" && d.Kind == walkwayDecoration {
			asset = tileStraightH // default walkway art: a straight run
		}
		onGrid := snap && isKitTile(asset) && tileOnGrid(d.X, d.Y, d.Shape)
		d.X, d.Y = g.point(d.X, d.Y)
		if g.quarterTurn() && (asset == tileStraightH || asset == tileStraightV) {
			// The renderer lays straight runs along their long side, so
			// swapping the sides (and the explicit art) is the same quarter
			// turn and keeps the run axis-aligned for the tile snapping.
			d.Shape.W, d.Shape.H = d.Shape.H, d.Shape.W
			switch d.Asset {
			case tileStraightH:
				d.Asset = tileStraightV
			case tileStraightV:
				d.Asset = tileStraightH
			}
		} else {
			d.Rotation = g.rotation(d.Rotation, asset)
		}
		if onGrid {
			d.X = snapTileCentre(d.X, d.Shape.W, newW)
			d.Y = snapTileCentre(d.Y, shapeH(d.Shape), newH)
		}
	}
	return out, tr, nil
}

// geometry applies one transform to points and rotations.
type geometry struct {
	tr   Transform
	w, h float64 // size of the map before the transform
}

func (g geometry) quarterTurn() bool {
	return g.tr == TransformRotateCW || g.tr == TransformRotateCCW
}

func (g geometry) point(x, y float64) (float64, float64) {
	switch g.tr {
	case TransformMirrorY:
		return round6(x), round6(g.h - y)
	case TransformMirrorX:
		return round6(g.w - x), round6(y)
	case TransformRotateCW:
		return round6(g.h - y), round6(x)
	case TransformRotateCCW:
		return round6(y), round6(g.w - x)
	default:
		return x, y
	}
}

func (g geometry) rotation(r float64, asset string) float64 {
	alpha, asym := kitMirrorAxis[asset]
	switch g.tr {
	case TransformMirrorY:
		if asym {
			return normalizeRotation6(-r - 2*alpha)
		}
		return normalizeRotation6(-r)
	case TransformMirrorX:
		if asym {
			return normalizeRotation6(180 - r - 2*alpha)
		}
		return normalizeRotation6(-r)
	case TransformRotateCW:
		return normalizeRotation6(r + 90)
	case TransformRotateCCW:
		return normalizeRotation6(r - 90)
	default:
		return r
	}
}

func shapeH(s store.Shape) float64 {
	if s.Type == ShapeCircle {
		return s.W
	}
	return s.H
}

// tileOnGrid reports whether both near edges of a tile lie on the tile grid.
func tileOnGrid(x, y float64, s store.Shape) bool {
	on := func(edge float64) bool {
		q := edge / kitTileSide
		return math.Abs(q-math.Round(q)) < 1e-6
	}
	return on(x-s.W/2) && on(y-shapeH(s)/2)
}

// snapTileCentre puts the near edge of a tile run of length `length` on the
// tile grid (round half up, like the editor's snapTileCentre), keeping the
// centre inside [0, limit]; when that is impossible the centre is kept.
func snapTileCentre(c, length, limit float64) float64 {
	half := length / 2
	edge := math.Floor((c-half)/kitTileSide+0.5) * kitTileSide
	out := round6(edge + half)
	if out < 0 || out > limit {
		return c
	}
	return out
}

func round6(f float64) float64 {
	r := math.Round(f*1e6) / 1e6
	if r == 0 {
		return 0 // no -0
	}
	return r
}

func normalizeRotation6(r float64) float64 {
	r = round6(normalizeRotation(r))
	if r >= 360 {
		return 0
	}
	return r
}

// ReorientInput asks to move the sea side of a map.
type ReorientInput struct {
	SeaSide string
	// DryRun computes the preview without saving (no 409, counts only).
	DryRun bool
	// AcknowledgeFutureAllocations confirms the change when spots of the
	// map have allocations from today on (otherwise
	// configerr.SpotsAllocatedError -> 409 spot_has_future_allocations).
	AcknowledgeFutureAllocations bool
	IP                           string
}

// ReorientResult is the transformed document and what was done.
type ReorientResult struct {
	// Document is the saved document (or the preview with DryRun).
	Document  *Document
	From, To  string
	Transform Transform
	Applied   bool
	// FutureAllocations counts booking, season and block allocations from
	// today on (establishment timezone) on the spots of the map.
	FutureAllocations          int
	SpotsWithFutureAllocations int
}

// reorientSummary is the audit diff of a reorientation.
type reorientSummary struct {
	FromSeaSide                  string      `json:"from_sea_side"`
	ToSeaSide                    string      `json:"to_sea_side"`
	Transform                    Transform   `json:"transform"`
	FutureAllocations            int         `json:"future_allocations"`
	AcknowledgeFutureAllocations bool        `json:"acknowledge_future_allocations"`
	Save                         saveSummary `json:"save"`
}

// ReorientMap moves the sea side of a map and transforms the whole layout
// (spots, decorations, walkway tiles) with ReorientLayout so that every row
// keeps its distance from the sea. Geometry only: ids, labels, rows, zones
// and allocations are unchanged. One versioned save through the same path
// as SaveMap (validation, diff, pool sync): expected must match the map
// version (configerr.VersionConflictError otherwise); the version is bumped
// once, the audit log records "map.reoriented" and the outbox gets
// config.map_changed with Change event.ChangeReoriented, in one
// transaction. When spots of the map have booking, season or block
// allocations from today on, the change needs AcknowledgeFutureAllocations
// (configerr.SpotsAllocatedError otherwise). DryRun returns the preview and
// the counts without writing, without the version check and without the
// 409. Moving the sea to the side it already has saves nothing (Applied
// false, TransformNone). Permission auth.PermEstablishmentConfigure.
func (s *Service) ReorientMap(ctx context.Context, p *auth.Principal, establishmentID, mapID string, expected int64, in ReorientInput) (*ReorientResult, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(SeaSides, in.SeaSide) {
		return nil, configerr.Invalid("sea_side", "Valore non valido.")
	}
	var res *ReorientResult
	run := func(ctx context.Context, db bun.IDB, tx *bun.Tx) error {
		cur, err := store.GetMap(ctx, db, es, mapID)
		if err != nil {
			return notFound(err, ErrMapNotFound)
		}
		if tx != nil {
			if err := configerr.CheckVersion(&expected, cur.Version); err != nil {
				return err
			}
		}
		doc, err := s.loadDocument(ctx, db, es, cur.ID)
		if err != nil {
			return err
		}
		res = &ReorientResult{Document: doc, From: cur.SeaSide, To: in.SeaSide, Transform: TransformNone}
		spotIDs := make([]string, len(doc.Spots))
		for i, sp := range doc.Spots {
			spotIDs[i] = sp.ID
		}
		today, err := s.today(ctx, db, es)
		if err != nil {
			return err
		}
		fa, err := store.FutureAllocationsOf(ctx, db, es, spotIDs, today)
		if err != nil {
			return err
		}
		res.FutureAllocations, res.SpotsWithFutureAllocations = fa.Allocations, len(fa.SpotIDs)
		if cur.SeaSide == in.SeaSide {
			return nil // already there: nothing to transform or save
		}
		cin, err := s.currentInput(ctx, db, es, cur)
		if err != nil {
			return err
		}
		next, tr, err := ReorientLayout(cin, in.SeaSide)
		if err != nil {
			return err
		}
		res.Transform = tr
		if err := validateDocument(&next).OrNil(); err != nil {
			return err
		}
		if tx == nil {
			res.Document = previewDocument(doc, next)
			return nil
		}
		if fa.Allocations > 0 && !in.AcknowledgeFutureAllocations {
			return &configerr.SpotsAllocatedError{SpotIDs: fa.SpotIDs}
		}
		sum := reorientSummary{
			FromSeaSide: cur.SeaSide, ToSeaSide: in.SeaSide, Transform: tr,
			FutureAllocations: fa.Allocations, AcknowledgeFutureAllocations: in.AcknowledgeFutureAllocations,
		}
		saved, err := s.applyDocumentAs(ctx, *tx, p, es, cur, next, event.ChangeReoriented, func(d saveSummary) any {
			sum.Save = d
			return sum
		})
		if err != nil {
			return err
		}
		res.Document, res.Applied = saved, true
		return nil
	}
	if in.DryRun {
		err = run(ctx, s.d.DB, nil)
	} else {
		err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error { return run(ctx, tx, &tx) })
	}
	if err != nil {
		return nil, fmt.Errorf("mapping: reorient map: %w", err)
	}
	return res, nil
}

// previewDocument is doc with the geometry of next (dry run): same ids and
// version, the transformed map size and sea side, positions, rotations,
// shapes and walkway art.
func previewDocument(doc *Document, next SaveInput) *Document {
	out := &Document{MapView: doc.MapView}
	out.Map.Width, out.Map.Height, out.Map.SeaSide = next.Map.Width, next.Map.Height, next.Map.SeaSide
	spots := make(map[string]SpotInput, len(next.Spots))
	for _, sp := range next.Spots {
		spots[sp.ID] = sp
	}
	out.Spots = make([]store.Spot, len(doc.Spots))
	for i, sp := range doc.Spots {
		n := spots[sp.ID]
		sp.X, sp.Y, sp.Rotation = n.X, n.Y, n.Rotation
		out.Spots[i] = sp
	}
	decs := make(map[string]DecorationInput, len(next.Decorations))
	for _, d := range next.Decorations {
		decs[d.ID] = d
	}
	out.Decorations = make([]store.MapDecoration, len(doc.Decorations))
	for i, d := range doc.Decorations {
		n := decs[d.ID]
		d.X, d.Y, d.Rotation, d.Shape, d.Asset = n.X, n.Y, n.Rotation, n.Shape, optional(n.Asset)
		out.Decorations[i] = d
	}
	return out
}
