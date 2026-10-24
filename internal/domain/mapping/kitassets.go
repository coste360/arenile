package mapping

import (
	"slices"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// The beach map kit (docs/adr/0012-kit-mappa.md): every spot and decoration
// may name the illustration it is drawn with. The lists of valid ids are
// generated from assets/map-kits/*/manifest.json into kitassets_gen.go by
// `make gen`, so the backend and the renderer always agree on the catalogue.

// IsKitBookableAsset reports whether id is a kit illustration a spot may use.
func IsKitBookableAsset(id string) bool {
	_, ok := slices.BinarySearch(KitBookableAssets, id)
	return ok
}

// IsKitAsset reports whether id is any kit illustration (decorations may use
// every asset, a non-bookable lettino or umbrella included).
func IsKitAsset(id string) bool {
	if IsKitBookableAsset(id) {
		return true
	}
	_, ok := slices.BinarySearch(KitDecorationAssets, id)
	return ok
}

func validateSpotAsset(ve *configerr.ValidationError, field, asset string) {
	if asset != "" && !IsKitBookableAsset(asset) {
		ve.Add(field, "Illustrazione non valida per una postazione prenotabile.")
	}
}

func validateDecorationAsset(ve *configerr.ValidationError, field, asset, variant string) {
	if asset != "" && !IsKitAsset(asset) {
		ve.Add(field+".asset", "Illustrazione non valida.")
		return
	}
	if variant == "" {
		return
	}
	if !slices.Contains(KitAssetVariants[asset], variant) {
		ve.Add(field+".variant", "Variante non disponibile per questa illustrazione.")
	}
}
