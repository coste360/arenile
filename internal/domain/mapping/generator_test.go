package mapping

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func intp(v int) *int         { return &v }
func f64p(v float64) *float64 { return &v }
func labels(s []SpotInput) []string {
	out := make([]string, len(s))
	for i, sp := range s {
		out[i] = sp.Label
	}
	return out
}

func TestGenerate_Layouts(t *testing.T) {
	base := func() GenerateParams {
		return GenerateParams{ZoneID: "z", Rows: 2, SpotsPerRow: 3, OriginX: 10, OriginY: 100, SpacingX: 5, SpacingY: 8}
	}
	type pos struct{ x, y float64 }
	tests := []struct {
		name     string
		mod      func(*GenerateParams)
		labels   []string
		pos      []pos
		rowLabel []string
		rotation float64
	}{
		{
			name:     "defaults sea top: rows go down, A is closest to the sea",
			mod:      func(*GenerateParams) {},
			labels:   []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			pos:      []pos{{10, 100}, {15, 100}, {20, 100}, {10, 108}, {15, 108}, {20, 108}},
			rowLabel: []string{"A", "A", "A", "B", "B", "B"},
		},
		{
			name:   "sea bottom: rows go up",
			mod:    func(p *GenerateParams) { p.SeaSide = SeaBottom },
			labels: []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			pos:    []pos{{10, 100}, {15, 100}, {20, 100}, {10, 92}, {15, 92}, {20, 92}},
		},
		{
			name:     "sea left: rows are columns moving right, rotated 90",
			mod:      func(p *GenerateParams) { p.SeaSide = SeaLeft },
			labels:   []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			pos:      []pos{{10, 100}, {10, 105}, {10, 110}, {18, 100}, {18, 105}, {18, 110}},
			rotation: 90,
		},
		{
			name:     "sea right: rows move left",
			mod:      func(p *GenerateParams) { p.SeaSide = SeaRight },
			labels:   []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			pos:      []pos{{10, 100}, {10, 105}, {10, 110}, {2, 100}, {2, 105}, {2, 110}},
			rotation: 90,
		},
		{
			name:   "rtl direction",
			mod:    func(p *GenerateParams) { p.Direction = DirectionRTL; p.OriginX = 50 },
			labels: []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			pos:    []pos{{50, 100}, {45, 100}, {40, 100}, {50, 108}, {45, 108}, {40, 108}},
		},
		{
			name: "numbers, continuous numbering, custom format and start",
			mod: func(p *GenerateParams) {
				p.RowLabelStyle = RowLabelsNumbers
				p.Numbering = NumberingContinuous
				p.LabelFormat = "{row}-{number}"
				p.NumberStart = intp(10)
			},
			labels:   []string{"1-10", "1-11", "1-12", "2-13", "2-14", "2-15"},
			rowLabel: []string{"1", "1", "1", "2", "2", "2"},
		},
		{
			name:   "first row label C",
			mod:    func(p *GenerateParams) { p.FirstRowLabel = "c" },
			labels: []string{"C1", "C2", "C3", "D1", "D2", "D3"},
		},
		{
			name:   "continuous numbering without row in the label",
			mod:    func(p *GenerateParams) { p.Numbering = NumberingContinuous; p.LabelFormat = "{number}" },
			labels: []string{"1", "2", "3", "4", "5", "6"},
		},
		{
			name:     "explicit rotation is normalized",
			mod:      func(p *GenerateParams) { p.Rotation = f64p(-90) },
			labels:   []string{"A1", "A2", "A3", "B1", "B2", "B3"},
			rotation: 270,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			tt.mod(&p)
			got, err := Generate(p)
			if err != nil {
				t.Fatal(err)
			}
			if g := labels(got); !equalStrings(g, tt.labels) {
				t.Fatalf("labels %v, want %v", g, tt.labels)
			}
			for i, want := range tt.pos {
				if got[i].X != want.x || got[i].Y != want.y {
					t.Errorf("spot %d at (%v,%v), want (%v,%v)", i, got[i].X, got[i].Y, want.x, want.y)
				}
			}
			for i, want := range tt.rowLabel {
				if got[i].RowLabel != want {
					t.Errorf("spot %d row %q, want %q", i, got[i].RowLabel, want)
				}
			}
			for _, sp := range got {
				if sp.Rotation != tt.rotation || !sp.Active || sp.ID != "" || sp.ZoneID != "z" || sp.Kind != "umbrella" {
					t.Fatalf("unexpected spot %+v", sp)
				}
			}
		})
	}
}

func TestGenerate_LettersBeyondZ(t *testing.T) {
	got, err := Generate(GenerateParams{ZoneID: "z", Rows: 3, SpotsPerRow: 1, SpacingX: 1, SpacingY: 1, OriginY: 10, FirstRowLabel: "Y"})
	if err != nil {
		t.Fatal(err)
	}
	if g := labels(got); !equalStrings(g, []string{"Y1", "Z1", "AA1"}) {
		t.Fatalf("labels %v", g)
	}
	for i, want := range []string{"A", "Z", "AA", "AZ", "BA", "ZZ", "AAA"} {
		idx := []int{0, 25, 26, 51, 52, 701, 702}[i]
		if got := lettersLabel(idx); got != want {
			t.Errorf("lettersLabel(%d) = %q, want %q", idx, got, want)
		}
	}
}

func TestGenerate_EightRowsOfFifteen(t *testing.T) {
	got, err := Generate(GenerateParams{
		ZoneID: "z", Rows: 8, SpotsPerRow: 15, OriginX: 20, OriginY: 60, SpacingX: 40, SpacingY: 45,
		Kind: "umbrella", Capacity: map[string]int{"umbrella": 1, "sunbed": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 120 {
		t.Fatalf("got %d spots", len(got))
	}
	last := got[len(got)-1]
	if last.Label != "H15" || last.X != 20+14*40 || last.Y != 60+7*45 || last.Number != 15 {
		t.Fatalf("last spot %+v", last)
	}
	if last.Shape != (store.Shape{Type: ShapeCircle, W: 32, H: 32}) {
		t.Fatalf("default shape %+v", last.Shape)
	}
	// Capacity maps are not shared between spots.
	got[0].Capacity["sunbed"] = 9
	if got[1].Capacity["sunbed"] != 2 {
		t.Fatal("capacity map shared between spots")
	}
}

func TestGenerate_InvalidParams(t *testing.T) {
	ok := GenerateParams{ZoneID: "z", Rows: 2, SpotsPerRow: 2, SpacingX: 1, SpacingY: 1}
	tests := []struct {
		name  string
		mod   func(*GenerateParams)
		field string
	}{
		{"zero rows", func(p *GenerateParams) { p.Rows = 0 }, "rows"},
		{"too many rows", func(p *GenerateParams) { p.Rows = 53 }, "rows"},
		{"too many spots", func(p *GenerateParams) { p.SpotsPerRow = 201 }, "spots_per_row"},
		{"zero spacing", func(p *GenerateParams) { p.SpacingX = 0 }, "spacing_x"},
		{"negative spacing", func(p *GenerateParams) { p.SpacingY = -1 }, "spacing_y"},
		{"missing zone", func(p *GenerateParams) { p.ZoneID = "" }, "zone_id"},
		{"bad kind", func(p *GenerateParams) { p.Kind = "boat" }, "kind"},
		{"bad first letter", func(p *GenerateParams) { p.FirstRowLabel = "A1" }, "first_row_label"},
		{"bad first number", func(p *GenerateParams) { p.RowLabelStyle = RowLabelsNumbers; p.FirstRowLabel = "x" }, "first_row_label"},
		{"bad style", func(p *GenerateParams) { p.RowLabelStyle = "roman" }, "row_label_style"},
		{"format without number", func(p *GenerateParams) { p.LabelFormat = "{row}" }, "label_format"},
		{"per row format without row", func(p *GenerateParams) { p.LabelFormat = "{number}" }, "label_format"},
		{"label too long", func(p *GenerateParams) { p.LabelFormat = "Ombrellone-fila-{row}-{number}" }, "label_format"},
		{"negative start", func(p *GenerateParams) { p.NumberStart = intp(-1) }, "number_start"},
		{"bad shape", func(p *GenerateParams) { p.Shape = &store.Shape{Type: "star", W: 1, H: 1} }, "shape.type"},
		{"bad capacity", func(p *GenerateParams) { p.Capacity = map[string]int{"sunbed": -1} }, "capacity.sunbed"},
		{"bad direction", func(p *GenerateParams) { p.Direction = "up" }, "direction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ok
			tt.mod(&p)
			_, err := Generate(p)
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want validation error, got %v", err)
			}
			if !hasField(ve, tt.field) {
				t.Fatalf("fields %+v, want %s", ve.Fields, tt.field)
			}
		})
	}
}

func hasField(ve *configerr.ValidationError, field string) bool {
	for _, f := range ve.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
