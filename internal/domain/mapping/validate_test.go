package mapping

import (
	"math"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

func validDoc() SaveInput {
	return SaveInput{
		Map: MapInput{Name: store.I18nText{"it": "Spiaggia"}, Width: 1000, Height: 600, GridSize: 10, SeaSide: SeaBottom},
		Spots: []SpotInput{
			{ZoneID: "z", Label: "A1", Kind: "umbrella", X: 10, Y: 10, Shape: store.Shape{Type: ShapeCircle, W: 8, H: 8}, Active: true},
			{ID: "s2", ZoneID: "z", Label: "A2", Kind: "gazebo", X: 1000, Y: 600, Shape: store.Shape{Type: ShapeRect, W: 20, H: 10}},
		},
		Decorations: []DecorationInput{{Kind: "shower", X: 0, Y: 0, Shape: store.Shape{Type: ShapeRect, W: 5, H: 5}}},
	}
}

func TestValidateDocument_Cases(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(*SaveInput)
		field string // empty: valid
	}{
		{"valid", func(*SaveInput) {}, ""},
		{"missing name", func(in *SaveInput) { in.Map.Name = store.I18nText{"it": "  "} }, "map.name"},
		{"bad locale", func(in *SaveInput) { in.Map.Name = store.I18nText{"Italiano": "x"} }, "map.name.Italiano"},
		{"zero width", func(in *SaveInput) { in.Map.Width = 0 }, "map.width"},
		{"grid larger than map", func(in *SaveInput) { in.Map.GridSize = 700 }, "map.grid_size"},
		{"bad sea side", func(in *SaveInput) { in.Map.SeaSide = "north" }, "map.sea_side"},
		{"image without url", func(in *SaveInput) { in.Map.Background = &Background{Type: "image"} }, "map.background.url"},
		{"javascript url", func(in *SaveInput) { in.Map.Background = &Background{Type: "image", URL: "javascript:alert(1)"} }, "map.background.url"},
		{"opacity", func(in *SaveInput) {
			in.Map.Background = &Background{Type: "image", URL: "https://x.it/a.png", Opacity: f64p(2)}
		}, "map.background.opacity"},
		{"duplicate label", func(in *SaveInput) { in.Spots[1].Label = "A1" }, "spots[1].label"},
		{"label trimmed duplicate", func(in *SaveInput) { in.Spots[1].Label = " A1 " }, "spots[1].label"},
		{"empty label", func(in *SaveInput) { in.Spots[0].Label = "" }, "spots[0].label"},
		{"long label", func(in *SaveInput) { in.Spots[0].Label = "ABCDEFGHIJKLMNOPQ" }, "spots[0].label"},
		{"x out of bounds", func(in *SaveInput) { in.Spots[0].X = 1000.5 }, "spots[0].x"},
		{"negative y", func(in *SaveInput) { in.Spots[0].Y = -1 }, "spots[0].y"},
		{"NaN x", func(in *SaveInput) { in.Spots[0].X = math.NaN() }, "spots[0].x"},
		{"Inf rotation", func(in *SaveInput) { in.Spots[0].Rotation = math.Inf(1) }, "spots[0].rotation"},
		{"bad kind", func(in *SaveInput) { in.Spots[0].Kind = "boat" }, "spots[0].kind"},
		{"bad shape type", func(in *SaveInput) { in.Spots[0].Shape.Type = "star" }, "spots[0].shape.type"},
		{"zero shape", func(in *SaveInput) { in.Spots[0].Shape.W = 0 }, "spots[0].shape.w"},
		{"negative capacity", func(in *SaveInput) { in.Spots[0].Capacity = map[string]int{"sunbed": -1} }, "spots[0].capacity.sunbed"},
		{"bad capacity code", func(in *SaveInput) { in.Spots[0].Capacity = map[string]int{"Sun Bed": 1} }, "spots[0].capacity.Sun Bed"},
		{"negative max extra", func(in *SaveInput) { in.Spots[0].MaxExtra = map[string]int{"sunbed": -2} }, "spots[0].max_extra.sunbed"},
		{"bad price category", func(in *SaveInput) { in.Spots[0].PriceCategoryOverride = strp2("Row 1") }, "spots[0].price_category_override"},
		{"missing zone", func(in *SaveInput) { in.Spots[0].ZoneID = "" }, "spots[0].zone_id"},
		{"negative number", func(in *SaveInput) { in.Spots[0].Number = -1 }, "spots[0].number"},
		{"repeated id", func(in *SaveInput) { in.Spots[0].ID = "s2" }, "spots[1].id"},
		{"decoration kind", func(in *SaveInput) { in.Decorations[0].Kind = "pier" }, "decorations[0].kind"},
		{"decoration out of bounds", func(in *SaveInput) { in.Decorations[0].Y = 601 }, "decorations[0].y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validDoc()
			tt.mod(&in)
			ve := validateDocument(&in)
			if tt.field == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if !hasField(ve, tt.field) {
				t.Fatalf("fields %+v, want %s", ve.Fields, tt.field)
			}
		})
	}
}

func TestValidateDocument_Normalizes(t *testing.T) {
	in := validDoc()
	in.Spots[0].Rotation = -45
	in.Spots[0].Attributes = []string{" accessible ", "accessible", "", "shade"}
	in.Spots[0].PriceCategoryOverride = strp2(" ")
	if err := validateDocument(&in).OrNil(); err != nil {
		t.Fatal(err)
	}
	s := in.Spots[0]
	if s.Rotation != 315 || !equalStrings(s.Attributes, []string{"accessible", "shade"}) || s.PriceCategoryOverride != nil {
		t.Fatalf("not normalized: %+v", s)
	}
}

func TestValidateZoneInput_Cases(t *testing.T) {
	mapID := "m"
	ok := func() ZoneInput {
		return ZoneInput{MapID: &mapID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: ModeSeat, PriceCategory: "row1"}
	}
	tests := []struct {
		name  string
		mod   func(*ZoneInput)
		field string
	}{
		{"valid", func(*ZoneInput) {}, ""},
		{"quantity without map", func(z *ZoneInput) { z.AssignmentMode = ModeQuantity; z.MapID = nil }, ""},
		{"percent quota", func(z *ZoneInput) { z.OnlineQuota = &store.OnlineQuota{Type: QuotaPercent, Value: 100} }, ""},
		{"zone mode auto assign", func(z *ZoneInput) { z.AssignmentMode = ModeZone; z.AutoAssign = true }, ""},
		{"bad code", func(z *ZoneInput) { z.Code = "Row 1" }, "code"},
		{"bad mode", func(z *ZoneInput) { z.AssignmentMode = "free" }, "assignment_mode"},
		{"seat without map", func(z *ZoneInput) { z.MapID = nil }, "map_id"},
		{"empty map id", func(z *ZoneInput) { e := ""; z.MapID = &e }, "map_id"},
		{"percent over 100", func(z *ZoneInput) { z.OnlineQuota = &store.OnlineQuota{Type: QuotaPercent, Value: 101} }, "online_quota.value"},
		{"negative count", func(z *ZoneInput) { z.OnlineQuota = &store.OnlineQuota{Type: QuotaCount, Value: -1} }, "online_quota.value"},
		{"bad quota type", func(z *ZoneInput) { z.OnlineQuota = &store.OnlineQuota{Type: "ratio", Value: 1} }, "online_quota.type"},
		{"bad price category", func(z *ZoneInput) { z.PriceCategory = "" }, "price_category"},
		{"auto assign seat", func(z *ZoneInput) { z.AutoAssign = true }, "auto_assign"},
		{"missing name", func(z *ZoneInput) { z.Name = nil }, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok()
			tt.mod(&in)
			ve := validateZoneInput(&in)
			if tt.field == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if !hasField(ve, tt.field) {
				t.Fatalf("fields %+v, want %s", ve.Fields, tt.field)
			}
		})
	}
}

func TestValidatePoolInput_Overrides(t *testing.T) {
	d := calendar.MustParse
	ov := func(from, to string, c int) store.CapacityOverride {
		return store.CapacityOverride{From: d(from), To: d(to), Capacity: c}
	}
	tests := []struct {
		name      string
		overrides []store.CapacityOverride
		field     string
	}{
		{"none", nil, ""},
		{"disjoint unsorted", []store.CapacityOverride{ov("2027-08-01", "2027-08-31", 10), ov("2027-06-01", "2027-07-31", 5)}, ""},
		{"adjacent", []store.CapacityOverride{ov("2027-06-01", "2027-06-30", 5), ov("2027-07-01", "2027-07-31", 5)}, ""},
		{"overlap", []store.CapacityOverride{ov("2027-06-01", "2027-07-01", 5), ov("2027-07-01", "2027-07-31", 5)}, "capacity_overrides"},
		{"contained", []store.CapacityOverride{ov("2027-06-01", "2027-09-01", 5), ov("2027-07-01", "2027-07-31", 5)}, "capacity_overrides"},
		{"reversed", []store.CapacityOverride{ov("2027-07-01", "2027-06-01", 5)}, "capacity_overrides[0].to"},
		{"negative", []store.CapacityOverride{ov("2027-07-01", "2027-07-02", -1)}, "capacity_overrides[0].capacity"},
		{"missing dates", []store.CapacityOverride{{Capacity: 1}}, "capacity_overrides[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := PoolInput{ComponentCode: "sunbed", Name: store.I18nText{"it": "Lettini"}, Capacity: 40, CapacityOverrides: tt.overrides}
			ve := validatePoolInput(&in)
			if tt.field == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				for i := 1; i < len(in.CapacityOverrides); i++ {
					if in.CapacityOverrides[i].From.Before(in.CapacityOverrides[i-1].From) {
						t.Fatal("overrides not sorted")
					}
				}
				return
			}
			if !hasField(ve, tt.field) {
				t.Fatalf("fields %+v, want %s", ve.Fields, tt.field)
			}
		})
	}
	bad := PoolInput{ComponentCode: "Sun", Name: nil, Capacity: -1}
	ve := validatePoolInput(&bad)
	for _, f := range []string{"component_code", "name", "capacity"} {
		if !hasField(ve, f) {
			t.Errorf("missing %s in %+v", f, ve.Fields)
		}
	}
}

func strp2(v string) *string { return &v }
