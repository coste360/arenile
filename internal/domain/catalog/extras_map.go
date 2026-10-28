package catalog

import (
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// MaxMapLabelLen bounds the map label of an extra ("L", "S", "Max").
const MaxMapLabelLen = 3

// validateMapSetup checks the map setup fields of an extra (spec 30.4): a
// label of 1-3 characters, required when the extra is shown on the map.
func validateMapSetup(ve *configerr.ValidationError, f *extraFields, in ExtraInput) {
	f.ShowOnMap = in.ShowOnMap
	if in.MapLabel != nil {
		l := strings.TrimSpace(*in.MapLabel)
		if n := utf8.RuneCountInString(l); n > MaxMapLabelLen {
			ve.Add("map_label", "Al massimo 3 caratteri.")
		}
		f.MapLabel = &l
	}
	if in.ShowOnMap != nil && *in.ShowOnMap && (f.MapLabel == nil || *f.MapLabel == "") {
		ve.Add("map_label", "Indica la sigla da mostrare sulla mappa.")
	}
}

func applyMapSetup(dst *store.Extra, f extraFields) {
	if f.ShowOnMap != nil {
		dst.ShowOnMap = *f.ShowOnMap
	}
	if f.MapLabel != nil {
		dst.MapLabel = *f.MapLabel
	}
}
