package mapping

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/store"
)

func TestReorientTransform_Table(t *testing.T) {
	tests := []struct {
		from, to string
		want     Transform
	}{
		{"top", "top", TransformNone},
		{"bottom", "top", TransformMirrorY},
		{"top", "bottom", TransformMirrorY},
		{"left", "right", TransformMirrorX},
		{"top", "right", TransformRotateCW},
		{"right", "bottom", TransformRotateCW},
		{"left", "top", TransformRotateCW},
		{"top", "left", TransformRotateCCW},
		{"bottom", "right", TransformRotateCCW},
		{"north", "top", TransformNone},
	}
	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.to, func(t *testing.T) {
			if got := ReorientTransform(tt.from, tt.to); got != tt.want {
				t.Fatalf("ReorientTransform(%s, %s) = %s, want %s", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// seaDistance is the distance of a point from the sea side of a w x h map.
func seaDistance(side string, x, y, w, h float64) float64 {
	switch side {
	case SeaTop:
		return y
	case SeaBottom:
		return h - y
	case SeaLeft:
		return x
	default:
		return w - x
	}
}

// shoreCoord is the coordinate of a point along the shore of side.
func shoreCoord(side string, x, y float64) float64 {
	if side == SeaTop || side == SeaBottom {
		return x
	}
	return y
}

func reorientFixture() SaveInput {
	return SaveInput{
		Map: MapInput{Name: store.I18nText{"it": "Spiaggia"}, Width: 1200, Height: 600, GridSize: 10, SeaSide: SeaTop},
		Spots: []SpotInput{
			{ID: "a1", ZoneID: "z", Label: "A1", RowLabel: "A", Number: 1, Kind: "umbrella", X: 100, Y: 80, Shape: store.Shape{Type: ShapeCircle, W: 40, H: 40}, Active: true},
			{ID: "a2", ZoneID: "z", Label: "A2", RowLabel: "A", Number: 2, Kind: "umbrella", X: 160, Y: 80, Shape: store.Shape{Type: ShapeCircle, W: 40, H: 40}, Active: true},
			{ID: "b1", ZoneID: "z", Label: "B1", RowLabel: "B", Number: 1, Kind: "gazebo", X: 100, Y: 150, Rotation: 15, Shape: store.Shape{Type: ShapeRect, W: 60, H: 40}, Active: true},
		},
		Decorations: []DecorationInput{
			{ID: "w1", Kind: "walkway", Asset: tileStraightV, X: 30, Y: 300, Shape: store.Shape{Type: ShapeRect, W: 60, H: 600}},
			{ID: "c1", Kind: "walkway", Asset: "passerella_angolo", X: 90, Y: 570, Shape: store.Shape{Type: ShapeRect, W: 60, H: 60}},
			{ID: "bar", Kind: "bar", Asset: "bar", X: 1000, Y: 520, Rotation: 30, Shape: store.Shape{Type: ShapeRect, W: 84, H: 84}},
		},
	}
}

func TestReorientLayout_AllSideCombinations(t *testing.T) {
	for _, from := range SeaSides {
		for _, to := range SeaSides {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				base, _, err := ReorientLayout(reorientFixture(), from) // fixture moved to `from`
				if err != nil {
					t.Fatal(err)
				}
				got, tr, err := ReorientLayout(base, to)
				if err != nil {
					t.Fatal(err)
				}
				if tr != ReorientTransform(from, to) {
					t.Fatalf("transform %s, want %s", tr, ReorientTransform(from, to))
				}
				if got.Map.SeaSide != to {
					t.Fatalf("sea side %s", got.Map.SeaSide)
				}
				quarter := tr == TransformRotateCW || tr == TransformRotateCCW
				if quarter != (got.Map.Width != base.Map.Width) {
					t.Fatalf("size %dx%d from %dx%d with %s", got.Map.Width, got.Map.Height, base.Map.Width, base.Map.Height, tr)
				}
				bw, bh := float64(base.Map.Width), float64(base.Map.Height)
				gw, gh := float64(got.Map.Width), float64(got.Map.Height)
				for i, sp := range got.Spots {
					old := base.Spots[i]
					if sp.ID != old.ID || sp.Label != old.Label || sp.RowLabel != old.RowLabel || sp.ZoneID != old.ZoneID {
						t.Fatalf("identity changed: %+v -> %+v", old, sp)
					}
					if d0, d1 := seaDistance(from, old.X, old.Y, bw, bh), seaDistance(to, sp.X, sp.Y, gw, gh); math.Abs(d0-d1) > 1e-6 {
						t.Fatalf("%s: distance from the sea %v -> %v", sp.Label, d0, d1)
					}
					if sp.X < 0 || sp.X > gw || sp.Y < 0 || sp.Y > gh {
						t.Fatalf("%s outside the map: (%v, %v) in %vx%v", sp.Label, sp.X, sp.Y, gw, gh)
					}
				}
				if err := validateDocument(&got).OrNil(); err != nil {
					t.Fatalf("invalid document: %v", err)
				}
			})
		}
	}
}

func TestReorientLayout_MirrorKeepsShoreOrder(t *testing.T) {
	got, _, err := ReorientLayout(reorientFixture(), SeaBottom)
	if err != nil {
		t.Fatal(err)
	}
	a1, a2 := got.Spots[0], got.Spots[1]
	if a1.X >= a2.X || a1.Y != 600-80 {
		t.Fatalf("mirror_y: A1 %+v A2 %+v", a1, a2)
	}
	right, _, _ := ReorientLayout(reorientFixture(), SeaLeft) // rotate_ccw from top
	mirrored, tr, _ := ReorientLayout(right, SeaRight)
	if tr != TransformMirrorX {
		t.Fatalf("transform %s", tr)
	}
	if shoreCoord(SeaLeft, right.Spots[0].X, right.Spots[0].Y) != shoreCoord(SeaRight, mirrored.Spots[0].X, mirrored.Spots[0].Y) {
		t.Fatal("mirror_x changed the order along the shore")
	}
}

// TestReorientLayout_PathsDoNotCompose pins the documented product decision:
// top -> bottom is a mirror (shore order kept) while top -> right -> bottom is
// two quarter turns (shore order reversed), so the two paths differ.
func TestReorientLayout_PathsDoNotCompose(t *testing.T) {
	direct, tr, err := ReorientLayout(reorientFixture(), SeaBottom)
	if err != nil || tr != TransformMirrorY {
		t.Fatalf("direct: %s %v", tr, err)
	}
	viaRight, _, err := ReorientLayout(reorientFixture(), SeaRight)
	if err != nil {
		t.Fatal(err)
	}
	twoTurns, tr2, err := ReorientLayout(viaRight, SeaBottom)
	if err != nil || tr2 != TransformRotateCW {
		t.Fatalf("second turn: %s %v", tr2, err)
	}
	if twoTurns.Map.SeaSide != direct.Map.SeaSide || twoTurns.Map.Width != direct.Map.Width {
		t.Fatalf("same side and size expected: %+v vs %+v", twoTurns.Map, direct.Map)
	}
	d1, d2 := direct.Spots[0], direct.Spots[1]     // A1, A2 after the mirror
	r1, r2 := twoTurns.Spots[0], twoTurns.Spots[1] // A1, A2 after two turns
	if !(d1.X < d2.X) {
		t.Fatalf("mirror must keep A1 left of A2: %v %v", d1.X, d2.X)
	}
	if !(r1.X > r2.X) {
		t.Fatalf("two quarter turns must reverse the shore order: %v %v", r1.X, r2.X)
	}
	for _, sp := range []struct{ a, b SpotInput }{{d1, r1}, {d2, r2}} {
		if math.Abs(seaDistance(SeaBottom, sp.a.X, sp.a.Y, float64(direct.Map.Width), float64(direct.Map.Height))-
			seaDistance(SeaBottom, sp.b.X, sp.b.Y, float64(twoTurns.Map.Width), float64(twoTurns.Map.Height))) > 1e-6 {
			t.Fatalf("both paths keep the distance from the sea: %+v %+v", sp.a, sp.b)
		}
	}
}

func TestReorientLayout_RotationsAndTiles(t *testing.T) {
	in := reorientFixture()
	tests := []struct {
		to   string
		spot float64 // rotation of b1 (15 degrees)
		// corner (passerella_angolo, rotation 0, joins top and right)
		corner float64
		bar    float64
	}{
		{SeaBottom, 345, 90, 330},
		{SeaRight, 105, 90, 120},
		{SeaLeft, 285, 270, 300},
	}
	for _, tt := range tests {
		t.Run(tt.to, func(t *testing.T) {
			got, _, err := ReorientLayout(in, tt.to)
			if err != nil {
				t.Fatal(err)
			}
			if r := got.Spots[2].Rotation; r != tt.spot {
				t.Errorf("spot rotation %v, want %v", r, tt.spot)
			}
			if r := got.Decorations[1].Rotation; r != tt.corner {
				t.Errorf("corner rotation %v, want %v", r, tt.corner)
			}
			if r := got.Decorations[2].Rotation; r != tt.bar {
				t.Errorf("bar rotation %v, want %v", r, tt.bar)
			}
		})
	}

	// A quarter turn swaps the sides and the art of a straight run.
	got, _, _ := ReorientLayout(in, SeaRight)
	w := got.Decorations[0]
	if w.Asset != tileStraightH || w.Shape.W != 600 || w.Shape.H != 60 || w.Rotation != 0 {
		t.Fatalf("straight run after rotate_cw: %+v", w)
	}
	if w.X != 300 || w.Y != 30 {
		t.Fatalf("straight run position (%v, %v)", w.X, w.Y)
	}
	// The terminal piece mirrors around its own axis.
	term := SaveInput{Map: in.Map, Decorations: []DecorationInput{{ID: "t", Kind: "walkway", Asset: "passerella_terminale", X: 30, Y: 30, Shape: store.Shape{Type: ShapeRect, W: 60, H: 60}}}}
	if got, _, _ := ReorientLayout(term, SeaBottom); got.Decorations[0].Rotation != 180 {
		t.Fatalf("terminal mirror_y rotation %v", got.Decorations[0].Rotation)
	}
	term.Map.SeaSide = SeaLeft
	if got, _, _ := ReorientLayout(term, SeaRight); got.Decorations[0].Rotation != 0 {
		t.Fatalf("terminal mirror_x rotation %v", got.Decorations[0].Rotation)
	}
}

func TestReorientLayout_TilesKeepTheTileGrid(t *testing.T) {
	in := reorientFixture()
	in.Map.Height = 610 // not a multiple of 60
	got, _, err := ReorientLayout(in, SeaBottom)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got.Decorations[:2] {
		if !tileOnGrid(d.X, d.Y, d.Shape) {
			t.Fatalf("%s off the tile grid: (%v, %v) %vx%v", d.Asset, d.X, d.Y, d.Shape.W, d.Shape.H)
		}
	}
	// The bar is not a tile: mirrored exactly.
	if got.Decorations[2].Y != 610-520 {
		t.Fatalf("bar y %v", got.Decorations[2].Y)
	}
	back, _, _ := ReorientLayout(got, SeaTop)
	if !reflect.DeepEqual(back.Decorations, in.Decorations) {
		t.Fatalf("tiles not restored:\n%+v\n%+v", back.Decorations, in.Decorations)
	}
	// Grid not compatible with the tile grid: tiles are mirrored exactly.
	in.Map.GridSize = 7
	got, _, _ = ReorientLayout(in, SeaBottom)
	if got.Decorations[0].Y != 610-300 {
		t.Fatalf("tile snapped on an incompatible grid: %v", got.Decorations[0].Y)
	}
}

func TestReorientLayout_PureAndValidated(t *testing.T) {
	in := reorientFixture()
	before := reorientFixture()
	if _, _, err := ReorientLayout(in, SeaRight); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, before) {
		t.Fatal("ReorientLayout modified its input")
	}
	if _, _, err := ReorientLayout(in, "north"); err == nil {
		t.Fatal("invalid side accepted")
	}
	in.Map.SeaSide = "north"
	if _, _, err := ReorientLayout(in, SeaTop); err == nil {
		t.Fatal("invalid current side accepted")
	}
	same, tr, err := ReorientLayout(before, SeaTop)
	if err != nil || tr != TransformNone || !reflect.DeepEqual(same, before) {
		t.Fatalf("same side: %s %v", tr, err)
	}
}

func TestReorient_TileAssetsAreKitAssets(t *testing.T) {
	for _, a := range kitTileAssets {
		if !IsKitAsset(a) {
			t.Errorf("%s is not a kit asset", a)
		}
	}
	for a := range kitMirrorAxis {
		if !isKitTile(a) {
			t.Errorf("%s has a mirror axis but is not a tile", a)
		}
	}
}

// genLayout draws a map with spots (any position, rotation, shape) and
// decorations, walkway tiles included (on the tile grid, inside the map).
func genLayout(t *rapid.T) SaveInput {
	w := rapid.IntRange(120, 3000).Draw(t, "width")
	h := rapid.IntRange(120, 3000).Draw(t, "height")
	in := SaveInput{Map: MapInput{
		Name: store.I18nText{"it": "M"}, Width: w, Height: h,
		GridSize: rapid.SampledFrom([]int{1, 5, 7, 10, 20, 30, 60}).Draw(t, "grid"),
		SeaSide:  rapid.SampledFrom(SeaSides).Draw(t, "from"),
	}}
	coord := func(limit int, label string) float64 {
		return float64(rapid.IntRange(0, limit*100).Draw(t, label)) / 100
	}
	n := rapid.IntRange(0, 12).Draw(t, "spots")
	for i := range n {
		shape := store.Shape{Type: ShapeCircle, W: 40, H: 40}
		if rapid.Bool().Draw(t, "rect") {
			shape = store.Shape{Type: ShapeRect, W: 60, H: 40}
		}
		in.Spots = append(in.Spots, SpotInput{
			ID: fmt.Sprintf("s%d", i), ZoneID: "z", Label: fmt.Sprintf("S%d", i), RowLabel: "S", Number: i, Kind: "umbrella",
			X: coord(w, "x"), Y: coord(h, "y"), Rotation: float64(rapid.IntRange(0, 35999).Draw(t, "rotation")) / 100,
			Shape: shape, Active: true,
		})
	}
	m := rapid.IntRange(0, 6).Draw(t, "decorations")
	for i := range m {
		asset := rapid.SampledFrom(append([]string{"bar", "doccia_doppia", ""}, kitTileAssets...)).Draw(t, "asset")
		d := DecorationInput{ID: fmt.Sprintf("d%d", i), Kind: "custom", Asset: asset, Shape: store.Shape{Type: ShapeRect, W: 50, H: 30}}
		if asset == "" || isKitTile(asset) {
			d.Kind = walkwayDecoration
			cols, rows := w/60, h/60
			tw, th := 1, 1
			switch asset {
			case tileStraightH, "":
				tw = rapid.IntRange(1, cols).Draw(t, "run")
			case tileStraightV:
				th = rapid.IntRange(1, rows).Draw(t, "run")
			}
			cx := rapid.IntRange(0, cols-tw).Draw(t, "col")
			cy := rapid.IntRange(0, rows-th).Draw(t, "row")
			d.Shape = store.Shape{Type: ShapeRect, W: float64(tw * 60), H: float64(th * 60)}
			d.X, d.Y = float64(cx*60)+d.Shape.W/2, float64(cy*60)+d.Shape.H/2
			d.Rotation = float64(90 * rapid.IntRange(0, 3).Draw(t, "quarter"))
		} else {
			d.X, d.Y = coord(w, "dx"), coord(h, "dy")
			d.Rotation = float64(rapid.IntRange(0, 359).Draw(t, "drot"))
		}
		in.Decorations = append(in.Decorations, d)
	}
	return in
}

func TestReorientLayout_PropertyRoundTripIsIdentity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genLayout(t)
		to := rapid.SampledFrom(SeaSides).Draw(t, "to")
		there, _, err := ReorientLayout(in, to)
		if err != nil {
			t.Fatal(err)
		}
		back, _, err := ReorientLayout(there, in.Map.SeaSide)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(back, in) {
			t.Fatalf("round trip %s -> %s -> %s changed the layout:\n got %+v\nwant %+v", in.Map.SeaSide, to, in.Map.SeaSide, back, in)
		}
	})
}

func TestReorientLayout_PropertyDistanceFromTheSea(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genLayout(t)
		to := rapid.SampledFrom(SeaSides).Draw(t, "to")
		got, _, err := ReorientLayout(in, to)
		if err != nil {
			t.Fatal(err)
		}
		w0, h0 := float64(in.Map.Width), float64(in.Map.Height)
		w1, h1 := float64(got.Map.Width), float64(got.Map.Height)
		for i, sp := range got.Spots {
			old := in.Spots[i]
			if d0, d1 := seaDistance(in.Map.SeaSide, old.X, old.Y, w0, h0), seaDistance(to, sp.X, sp.Y, w1, h1); math.Abs(d0-d1) > 1e-6 {
				t.Fatalf("spot %s: distance %v -> %v", sp.Label, d0, d1)
			}
			if sp.X < 0 || sp.X > w1 || sp.Y < 0 || sp.Y > h1 || sp.Rotation < 0 || sp.Rotation >= 360 {
				t.Fatalf("spot %s out of range: %+v", sp.Label, sp)
			}
		}
		// Spots of the same row (same distance) stay in the same row.
		for i := range got.Spots {
			for j := range got.Spots {
				a0 := seaDistance(in.Map.SeaSide, in.Spots[i].X, in.Spots[i].Y, w0, h0)
				b0 := seaDistance(in.Map.SeaSide, in.Spots[j].X, in.Spots[j].Y, w0, h0)
				a1 := seaDistance(to, got.Spots[i].X, got.Spots[i].Y, w1, h1)
				b1 := seaDistance(to, got.Spots[j].X, got.Spots[j].Y, w1, h1)
				if (a0 < b0) != (a1 < b1-1e-6) && math.Abs(a0-b0) > 1e-6 {
					t.Fatalf("row order changed between %d and %d", i, j)
				}
			}
		}
		for _, d := range got.Decorations {
			if d.X < 0 || d.X > w1 || d.Y < 0 || d.Y > h1 {
				t.Fatalf("decoration %s outside the map: %+v", d.ID, d)
			}
			asset := d.Asset
			if asset == "" {
				asset = tileStraightH
			}
			if isKitTile(asset) && tilesSnapToGrid(in.Map.GridSize) && !tileOnGrid(d.X, d.Y, d.Shape) {
				t.Fatalf("tile %s off the tile grid: %+v", d.ID, d)
			}
		}
	})
}
