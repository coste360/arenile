package catalog

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func TestValidateMapSetup_Extras(t *testing.T) {
	yes, no := true, false
	str := func(s string) *string { return &s }
	tests := []struct {
		name     string
		show     *bool
		label    *string
		errField string
		want     string
	}{
		{"untouched", nil, nil, "", ""},
		{"shown with label", &yes, str(" L "), "", "L"},
		{"shown without label", &yes, nil, "map_label", ""},
		{"shown with blank label", &yes, str("  "), "map_label", ""},
		{"label too long", &no, str("Lett"), "map_label", ""},
		{"hidden keeps label", &no, str("S"), "", "S"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			var f extraFields
			validateMapSetup(ve, &f, ExtraInput{ShowOnMap: tt.show, MapLabel: tt.label})
			if tt.errField != "" {
				if len(ve.Fields) == 0 || ve.Fields[0].Field != tt.errField {
					t.Fatalf("want error on %s, got %+v", tt.errField, ve.Fields)
				}
				return
			}
			if len(ve.Fields) > 0 {
				t.Fatalf("unexpected errors %+v", ve.Fields)
			}
			x := &store.Extra{MapLabel: "old"}
			applyMapSetup(x, f)
			if tt.label == nil && x.MapLabel != "old" || tt.label != nil && x.MapLabel != tt.want {
				t.Fatalf("map label %q", x.MapLabel)
			}
			if tt.show != nil && x.ShowOnMap != *tt.show {
				t.Fatalf("show on map %v", x.ShowOnMap)
			}
		})
	}
}
