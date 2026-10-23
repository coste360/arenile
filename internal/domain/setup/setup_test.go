package setup

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/store"
)

func TestLayoutMap_ClassicGrid(t *testing.T) {
	tpl, ok := TemplateByCode(CodeClassicRows)
	if !ok {
		t.Fatal("classic template missing")
	}
	a := &applier{s: &Service{d: Deps{Clock: clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))}}, rows: 8, perRow: 15}
	m, byZone, decos := a.layoutMap(tpl.Maps[0], 0)
	if len(decos) == 0 {
		t.Fatal("no decorations")
	}
	wantCounts := map[string]int{"row1": 15, "row2": 15, "standard": 6 * 15, "gazebo": 4}
	labels := map[string]bool{}
	rowY := map[string]float64{}
	for zone, want := range wantCounts {
		zs := byZone[zone]
		if zs == nil || len(zs.spots) != want {
			t.Fatalf("zone %s: got %v spots, want %d", zone, zs, want)
		}
		for _, sp := range zs.spots {
			if labels[sp.Label] {
				t.Fatalf("duplicate label %s", sp.Label)
			}
			labels[sp.Label] = true
			if sp.X < 0 || sp.Y < 0 || sp.X > float64(m.Width) || sp.Y > float64(m.Height) {
				t.Fatalf("spot %s outside the %dx%d map: %v,%v", sp.Label, m.Width, m.Height, sp.X, sp.Y)
			}
			rowY[sp.RowLabel] = sp.Y
		}
	}
	if byZone["row1"].spots[0].Label != "A1" || byZone["row2"].spots[0].Label != "B1" {
		t.Fatalf("first labels: %s %s", byZone["row1"].spots[0].Label, byZone["row2"].spots[0].Label)
	}
	// The sea is at the top (spec 11 default): row A is the highest on
	// screen (smallest y), every following row moves away from the sea, the
	// gazebos are last.
	order := []string{"A", "B", "C", "D", "E", "F", "G", "H", "GZ"}
	for i := 1; i < len(order); i++ {
		if rowY[order[i]] <= rowY[order[i-1]] {
			t.Fatalf("row %s (y=%v) is not farther from the sea than %s (y=%v)", order[i], rowY[order[i]], order[i-1], rowY[order[i-1]])
		}
	}
}

func TestPeriodRanges_CoverSeasonWithoutOverlap(t *testing.T) {
	tests := []struct {
		name          string
		opens, closes string
		wantPeriods   []string
	}{
		{"default season", "2027-05-01", "2027-09-30", []string{PeriodLow, PeriodMid, PeriodHigh, PeriodPeak}},
		{"long season", "2027-04-01", "2027-10-31", []string{PeriodLow, PeriodMid, PeriodHigh, PeriodPeak}},
		{"summer only", "2027-07-01", "2027-08-31", []string{PeriodHigh, PeriodPeak}},
		{"one day", "2027-08-15", "2027-08-15", []string{PeriodPeak}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opens, closes := calendar.MustParse(tt.opens), calendar.MustParse(tt.closes)
			ranges := periodRanges(opens, closes)
			seen := map[calendar.Date]string{}
			var codes []string
			for code, rs := range ranges {
				codes = append(codes, code)
				for _, r := range rs {
					for _, d := range calendar.Range(r.From, r.To) {
						if prev, dup := seen[d]; dup {
							t.Fatalf("%s in %s and %s", d, prev, code)
						}
						seen[d] = code
					}
				}
			}
			for _, d := range calendar.Range(opens, closes) {
				if seen[d] == "" {
					t.Fatalf("%s not covered", d)
				}
			}
			if len(seen) != opens.DaysUntil(closes)+1 {
				t.Fatalf("days outside the season: %d", len(seen))
			}
			slices.Sort(codes)
			want := slices.Clone(tt.wantPeriods)
			slices.Sort(want)
			if !slices.Equal(codes, want) {
				t.Fatalf("periods = %v, want %v", codes, want)
			}
		})
	}
	r := periodRanges(calendar.MustParse("2027-05-01"), calendar.MustParse("2027-09-30"))
	if len(r[PeriodLow]) != 2 || r[PeriodLow][1].From.String() != "2027-09-16" || r[PeriodPeak][0].From.String() != "2027-08-01" {
		t.Fatalf("default ranges: %+v", r)
	}
}

func TestDefaultSeasonYear_Scenarios(t *testing.T) {
	tests := []struct {
		today string
		want  int
	}{
		{"2027-01-10", 2027},
		{"2027-09-30", 2027},
		{"2027-10-01", 2028},
		{"2026-10-22", 2027},
	}
	for _, tt := range tests {
		if got := DefaultSeasonYear(calendar.MustParse(tt.today)); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.today, got, tt.want)
		}
	}
	if o, c := defaultSeasonDates(2027); o != calendar.New(2027, time.May, 1) || c != calendar.New(2027, time.September, 30) {
		t.Fatalf("default dates %s %s", o, c)
	}
}

// TestTemplates_Consistent checks the static template definitions: four
// templates with i18n texts in it/en/de/fr, zones referencing known maps,
// packs made of template components, extras of template components, policies
// that pass the policy engine validation.
func TestTemplates_Consistent(t *testing.T) {
	ts := Templates()
	if len(ts) != 4 {
		t.Fatalf("templates: %d", len(ts))
	}
	wantCodes := []Code{CodeClassicRows, CodeQuantityBeach, CodeLidoWithPool, CodeEmpty}
	locales := []string{"it", "en", "de", "fr"}
	for i, tpl := range ts {
		if tpl.Code != wantCodes[i] || !tpl.Code.Valid() {
			t.Fatalf("template %d: %s", i, tpl.Code)
		}
		if _, ok := TemplateByCode(tpl.Code); !ok {
			t.Fatalf("lookup %s", tpl.Code)
		}
		for _, l := range locales {
			if tpl.Name[l] == "" || tpl.Description[l] == "" {
				t.Fatalf("%s: missing %s text", tpl.Code, l)
			}
		}
		maps := map[string]bool{}
		for _, m := range tpl.Maps {
			maps[m.Key] = true
		}
		comps := map[string]bool{}
		for _, c := range tpl.Components {
			comps[c.Code] = true
		}
		for _, z := range tpl.Zones {
			for _, l := range locales {
				if z.Name[l] == "" {
					t.Fatalf("%s/%s: missing %s name", tpl.Code, z.Code, l)
				}
			}
			if z.AssignmentMode == "quantity" {
				if z.Map != "" || len(z.Pools) == 0 {
					t.Fatalf("%s/%s: quantity zone needs pools and no map", tpl.Code, z.Code)
				}
			} else if !maps[z.Map] {
				t.Fatalf("%s/%s: unknown map %q", tpl.Code, z.Code, z.Map)
			}
		}
		for _, p := range tpl.Packs {
			for c := range p.Composition {
				if !comps[c] {
					t.Fatalf("%s/%s: component %q not in template", tpl.Code, p.Code, c)
				}
			}
		}
		for _, x := range tpl.Extras {
			if x.ComponentCode != "" && !comps[x.ComponentCode] {
				t.Fatalf("%s/%s: component %q not in template", tpl.Code, x.Code, x.ComponentCode)
			}
		}
		for _, p := range tpl.Policies {
			if err := p.Rules.CheckConfig(); err != nil {
				t.Fatalf("%s/%s: %v", tpl.Code, p.Name, err)
			}
		}
	}
	classic, _ := TemplateByCode(CodeClassicRows)
	if classic.Rows != 8 || classic.SpotsPerRow != 15 {
		t.Fatalf("classic grid %dx%d", classic.Rows, classic.SpotsPerRow)
	}
	if Code("nope").Valid() {
		t.Fatal("unknown code must be invalid")
	}
	if err := policyAugustWeeks.Rules.CheckConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestUniqueCode_Suffix(t *testing.T) {
	taken := map[string]bool{"row1": true, "row1_2": true}
	if got := uniqueCode("row1", taken); got != "row1_3" {
		t.Fatalf("got %s", got)
	}
	if got := uniqueCode("pool", taken); got != "pool" || !taken["pool"] {
		t.Fatalf("got %s", got)
	}
}

func TestLayoutMap_KitDecorations(t *testing.T) {
	tpl, _ := TemplateByCode(CodeClassicRows)
	a := &applier{s: &Service{d: Deps{Clock: clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))}}, rows: 8, perRow: 15}
	m, byZone, decos := a.layoutMap(tpl.Maps[0], 0)
	var spots []kitRect
	for _, zs := range byZone {
		for _, sp := range zs.spots {
			spots = append(spots, kitSpotRect(sp.X, sp.Y, templateSpotShape(zs.kind)))
		}
	}
	assets := map[string]bool{}
	for _, d := range decos {
		if d.Asset == nil || !mapping.IsKitAsset(*d.Asset) {
			t.Fatalf("decoration %s without a valid kit asset", d.Kind)
		}
		assets[*d.Asset] = true
		r := kitRect{d.X - d.Shape.W/2, d.Y - d.Shape.H/2, d.X + d.Shape.W/2, d.Y + d.Shape.H/2}
		if r.x0 < 0 || r.y0 < 0 || r.x1 > float64(m.Width) || r.y1 > float64(m.Height) {
			t.Errorf("%s outside the map: %+v", *d.Asset, r)
		}
		if d.Kind == "walkway" {
			if math.Mod(d.Shape.W, kitTile) != 0 || math.Mod(d.Shape.H, kitTile) != 0 || math.Mod(r.x0, kitTile) != 0 || math.Mod(r.y0, kitTile) != 0 {
				t.Errorf("walkway %s not on the %v tile grid: %+v", *d.Asset, kitTile, r)
			}
			continue
		}
		for _, s := range spots {
			if r.overlaps(s) {
				t.Errorf("%s covers a spot", *d.Asset)
			}
		}
	}
	for _, want := range []string{"passerella_verticale", "bar", "doccia_doppia", "torretta_bagnino", "bandiera_mare", "dae", "rampa_accessibile", "sedia_job", "palma", "ingresso"} { //nolint:misspell // kit asset ids (Italian)
		if !assets[want] {
			t.Errorf("classic map without %s (got %v)", want, assets)
		}
	}
	for _, zs := range byZone {
		sp := newSpot("e", m.ID, "z", zs.kind, zs.spots[0], time.Now())
		if sp.Asset == nil || !mapping.IsKitBookableAsset(*sp.Asset) {
			t.Errorf("%s spot without a bookable kit asset", zs.kind)
		}
	}
	checkSeaTopLayout(t, m, byZone, decos)
}

// checkSeaTopLayout: the classic map has the sea at the top (spec 11
// default), row A nearest to it, the shore furniture along the top edge,
// the walkway starting at the shore and the entrance away from the sea.
func checkSeaTopLayout(t *testing.T, m *store.Map, byZone map[string]*zoneSpots, decos []store.MapDecoration) {
	t.Helper()
	if m.SeaSide != mapping.SeaTop {
		t.Fatalf("classic map sea side %s", m.SeaSide)
	}
	rowY := map[string]float64{}
	for _, zs := range byZone {
		for _, sp := range zs.spots {
			rowY[sp.RowLabel] = sp.Y
		}
	}
	if !(rowY["A"] < rowY["B"] && rowY["B"] < rowY["H"] && rowY["H"] < rowY["GZ"]) {
		t.Fatalf("rows are not ordered from the sea at the top: %v", rowY)
	}
	if rowY["A"] != layoutMargin+layoutSeaBand {
		t.Fatalf("row A at y %v", rowY["A"])
	}
	for _, d := range decos {
		switch *d.Asset {
		case "torretta_bagnino":
			if d.Y > float64(m.Height)/4 {
				t.Errorf("lifeguard tower far from the sea: y %v", d.Y)
			}
		case "passerella_verticale":
			if d.Y-d.Shape.H/2 != 0 {
				t.Errorf("walkway does not start at the shore: %+v", d)
			}
		case "passerella_ingresso", "ingresso":
			if d.Y < float64(m.Height)/2 {
				t.Errorf("%s on the sea side: y %v", *d.Asset, d.Y)
			}
		}
	}
}

func TestLayoutMap_SeaBottomStillSupported(t *testing.T) {
	tpl, _ := TemplateByCode(CodeClassicRows)
	ms := tpl.Maps[0]
	ms.SeaSide = mapping.SeaBottom
	a := &applier{s: &Service{d: Deps{Clock: clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))}}, rows: 2, perRow: 4}
	m, byZone, _ := a.layoutMap(ms, 0)
	if m.SeaSide != mapping.SeaBottom {
		t.Fatalf("sea side %s", m.SeaSide)
	}
	for _, zs := range byZone {
		for _, sp := range zs.spots {
			if sp.RowLabel == "A" && sp.Y != float64(m.Height)-layoutMargin-layoutSeaBand {
				t.Fatalf("row A at y %v of %d", sp.Y, m.Height)
			}
		}
	}
}

func TestKitDecorations_SmallGridSkipsWhatDoesNotFit(t *testing.T) {
	for _, side := range []string{mapping.SeaTop, mapping.SeaBottom} {
		t.Run(side, func(t *testing.T) { checkSmallGridKit(t, kitDecorations("e", "m", 220, 290, side, nil, time.Now())) })
	}
}

func checkSmallGridKit(t *testing.T, decos []store.MapDecoration) {
	t.Helper()
	for i, a := range decos {
		ra := kitRect{a.X - a.Shape.W/2, a.Y - a.Shape.H/2, a.X + a.Shape.W/2, a.Y + a.Shape.H/2}
		for _, b := range decos[i+1:] {
			rb := kitRect{b.X - b.Shape.W/2, b.Y - b.Shape.H/2, b.X + b.Shape.W/2, b.Y + b.Shape.H/2}
			if a.Kind != "walkway" && b.Kind != "walkway" && ra.overlaps(rb) {
				t.Errorf("%s overlaps %s", *a.Asset, *b.Asset)
			}
		}
		if ra.x1 > 220 || ra.y1 > 290 || ra.x0 < 0 || ra.y0 < 0 {
			t.Errorf("%s outside the map", *a.Asset)
		}
	}
}
