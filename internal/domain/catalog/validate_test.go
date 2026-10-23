package catalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

func ptr[T any](v T) *T { return &v }

func fieldsOf(ve *configerr.ValidationError) []string {
	out := make([]string, 0, len(ve.Fields))
	for _, f := range ve.Fields {
		out = append(out, f.Field)
	}
	return out
}

func hasField(ve *configerr.ValidationError, field string) bool {
	for _, f := range ve.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}

func TestValidateComponent_Table(t *testing.T) {
	name := store.I18nText{"it": "Lettino"}
	tests := []struct {
		name      string
		in        ComponentInput
		wantField string // empty = valid
	}{
		{"valid untracked", ComponentInput{Code: "sunbed", Name: name}, ""},
		{"valid tracked", ComponentInput{Code: "sunbed", Name: name, Tracked: true, PoolID: ptr("p1")}, ""},
		{"code trimmed", ComponentInput{Code: "  sunbed ", Name: name}, ""},
		{"missing code", ComponentInput{Name: name}, "code"},
		{"uppercase code", ComponentInput{Code: "Sunbed", Name: name}, "code"},
		{"code with space", ComponentInput{Code: "sun bed", Name: name}, "code"},
		{"code too long", ComponentInput{Code: strings.Repeat("a", MaxCodeLen+1), Name: name}, "code"},
		{"missing default locale name", ComponentInput{Code: "sunbed", Name: store.I18nText{"en": "Sunbed"}}, "name.it"},
		{"blank name", ComponentInput{Code: "sunbed", Name: store.I18nText{"it": "   "}}, "name.it"},
		{"bad locale", ComponentInput{Code: "sunbed", Name: store.I18nText{"it": "Lettino", "ITALIANO": "x"}}, "name.ITALIANO"},
		{"tracked without pool", ComponentInput{Code: "sunbed", Name: name, Tracked: true}, "pool_id"},
		{"tracked blank pool", ComponentInput{Code: "sunbed", Name: name, Tracked: true, PoolID: ptr(" ")}, "pool_id"},
		{"untracked with pool", ComponentInput{Code: "sunbed", Name: name, PoolID: ptr("p1")}, "pool_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ve := validateComponent(tt.in, "it")
			if tt.wantField == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Code != "sunbed" {
					t.Fatalf("code = %q", f.Code)
				}
				return
			}
			if !hasField(ve, tt.wantField) {
				t.Fatalf("want field %q, got %v", tt.wantField, fieldsOf(ve))
			}
		})
	}
}

func validPack() PackInput {
	return PackInput{
		Code:            "front-row",
		Name:            store.I18nText{"it": "Prima fila", "en": "Front row"},
		Composition:     map[string]int{"umbrella": 1, "sunbed": 2},
		AllowedFormulas: []string{"day", "half_day"},
		Channels:        []string{"online", "desk"},
	}
}

func TestValidatePack_Table(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(p *PackInput)
		wantField string
	}{
		{"valid", func(*PackInput) {}, ""},
		{"empty composition", func(p *PackInput) { p.Composition = nil }, "composition"},
		{"zero quantity", func(p *PackInput) { p.Composition["sunbed"] = 0 }, "composition.sunbed"},
		{"negative quantity", func(p *PackInput) { p.Composition["sunbed"] = -1 }, "composition.sunbed"},
		{"bad spot kind", func(p *PackInput) { p.RequiresSpotKind = []string{"umbrella", "yacht"} }, "requires_spot_kind[1]"},
		{"kit spot kinds", func(p *PackInput) { p.RequiresSpotKind = []string{"sunbed", "daybed", "cabana", "hammock"} }, ""},
		{"bad formula", func(p *PackInput) { p.AllowedFormulas = []string{"day", "year"} }, "allowed_formulas[1]"},
		{"no formula", func(p *PackInput) { p.AllowedFormulas = nil }, "allowed_formulas"},
		{"bad channel", func(p *PackInput) { p.Channels = []string{"phone"} }, "channels[0]"},
		{"no channel", func(p *PackInput) { p.Channels = []string{} }, "channels"},
		{"max per booking zero", func(p *PackInput) { p.MaxPerBooking = ptr(0) }, "max_per_booking"},
		{"vat negative", func(p *PackInput) { p.VATRateBP = ptr(int64(-1)) }, "vat_rate_bp"},
		{"vat over 100%", func(p *PackInput) { p.VATRateBP = ptr(int64(10001)) }, "vat_rate_bp"},
		{"blank zone id", func(p *PackInput) { p.AllowedZoneIDs = []string{""} }, "allowed_zone_ids[0]"},
		{"bad image", func(p *PackInput) { p.Image = ptr("javascript:alert(1)") }, "image"},
		{"sort order out of range", func(p *PackInput) { p.SortOrder = MaxSortOrder + 1 }, "sort_order"},
		{"description too long", func(p *PackInput) { p.Description = store.I18nText{"it": strings.Repeat("x", MaxDescriptionLen+1)} }, "description.it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validPack()
			tt.mutate(&in)
			_, ve := validatePack(in, "it")
			if tt.wantField == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !hasField(ve, tt.wantField) {
				t.Fatalf("want field %q, got %v", tt.wantField, fieldsOf(ve))
			}
		})
	}
}

func TestValidatePack_Normalization(t *testing.T) {
	in := validPack()
	in.Channels = []string{"online", "desk", "online"}
	in.AllowedFormulas = []string{"season", "day"}
	in.RequiresSpotKind = []string{}
	in.AllowedZoneIDs = []string{"z2", "z1", "z2"}
	in.Description = store.I18nText{"it": "  ", "en": " Nice "}
	in.Image = ptr("  ")
	f, ve := validatePack(in, "it")
	if err := ve.OrNil(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Channels, []string{"desk", "online"}) {
		t.Errorf("channels = %v", f.Channels)
	}
	if !reflect.DeepEqual(f.AllowedFormulas, []string{"day", "season"}) {
		t.Errorf("formulas = %v", f.AllowedFormulas)
	}
	if f.RequiresSpotKind != nil {
		t.Errorf("empty spot kinds must become nil, got %v", f.RequiresSpotKind)
	}
	if !reflect.DeepEqual(f.AllowedZoneIDs, []string{"z2", "z1"}) {
		t.Errorf("zones = %v", f.AllowedZoneIDs)
	}
	if !reflect.DeepEqual(f.Description, store.I18nText{"en": "Nice"}) {
		t.Errorf("description = %v", f.Description)
	}
	if f.Image != nil || f.MaxPerBooking != 1 || f.VATRateBP != nil {
		t.Errorf("defaults: image %v max %d vat %v", f.Image, f.MaxPerBooking, f.VATRateBP)
	}
}

func validExtra() ExtraInput {
	return ExtraInput{
		Code: "towel", Name: store.I18nText{"it": "Telo"}, PricingUnit: "per_day", AttachTo: "line",
		Channels: []string{"desk"},
	}
}

func TestValidateExtra_Table(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(x *ExtraInput)
		wantField string
	}{
		{"valid", func(*ExtraInput) {}, ""},
		{"attach defaults to line", func(x *ExtraInput) { x.AttachTo = "" }, ""},
		{"booking extra", func(x *ExtraInput) { x.AttachTo = "booking"; x.PricingUnit = "per_booking" }, ""},
		{"bad pricing unit", func(x *ExtraInput) { x.PricingUnit = "per_hour" }, "pricing_unit"},
		{"bad attach", func(x *ExtraInput) { x.AttachTo = "spot" }, "attach_to"},
		{"max quantity zero", func(x *ExtraInput) { x.MaxQuantity = ptr(0) }, "max_quantity"},
		{"no channel", func(x *ExtraInput) { x.Channels = nil }, "channels"},
		{"booking extra restricted to packs", func(x *ExtraInput) { x.AttachTo = "booking"; x.AllowedPackIDs = []string{"p1"} }, "allowed_pack_ids"},
		{"missing name", func(x *ExtraInput) { x.Name = nil }, "name.it"},
		{"vat too high", func(x *ExtraInput) { x.VATRateBP = ptr(int64(20000)) }, "vat_rate_bp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validExtra()
			tt.mutate(&in)
			f, ve := validateExtra(in, "it")
			if tt.wantField == "" {
				if err := ve.OrNil(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.AttachTo == "" || f.MaxQuantity != 1 {
					t.Fatalf("defaults: %+v", f)
				}
				return
			}
			if !hasField(ve, tt.wantField) {
				t.Fatalf("want field %q, got %v", tt.wantField, fieldsOf(ve))
			}
		})
	}
}

func TestBuildPricingCatalog_Mapping(t *testing.T) {
	vat := int64(1000)
	over := "premium"
	cat := BuildPricingCatalog(
		pricing.Settings{Currency: "EUR", DefaultVATRateBP: 2200},
		[]store.Pack{{ID: "p1", Code: "std", Active: true}, {ID: "p2", Code: "old", VATRateBP: &vat}},
		[]store.Extra{{ID: "x1", Code: "towel", PricingUnit: "per_day", Active: true}},
		[]store.Zone{{ID: "z1", Code: "a", PriceCategory: "first"}},
		[]store.Spot{{ID: "s1", Label: "A1", ZoneID: "z1", PriceCategoryOverride: &over, Attributes: []string{"shade"}}},
	)
	if len(cat.Packs) != 2 || cat.Packs[1].VATRateBP == nil || *cat.Packs[1].VATRateBP != 1000 || cat.Packs[1].Active {
		t.Fatalf("packs = %+v", cat.Packs)
	}
	if cat.Packs[0].VATRateBP != nil {
		t.Fatal("nil VAT must stay nil (establishment default)")
	}
	if cat.Extras[0].PricingUnit != pricing.PerDayUnit {
		t.Fatalf("extras = %+v", cat.Extras)
	}
	if cat.Zones[0].PriceCategory != "first" || cat.Spots[0].PriceCategoryOverride != "premium" || cat.Spots[0].Attributes[0] != "shade" {
		t.Fatalf("zones/spots = %+v %+v", cat.Zones, cat.Spots)
	}
	if cat.Settings.DefaultVATRateBP != 2200 {
		t.Fatal("settings not carried")
	}
	vat = 5
	if *cat.Packs[1].VATRateBP != 1000 {
		t.Fatal("VAT pointer must be copied")
	}
}
