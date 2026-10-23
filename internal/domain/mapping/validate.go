package mapping

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Validation of the mapping inputs. Messages are Italian (source language);
// field paths follow the JSON request body.

var (
	codeRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
)

const (
	maxMapSide     = 100000
	maxNameLen     = 200
	maxLabelLen    = 16
	maxRowLabelLen = 8
	maxAttrLen     = 32
	maxURLLen      = 2048
)

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// validateI18n checks a localized text: locale keys, bounded values and, when
// required, at least one non-empty value.
func validateI18n(ve *configerr.ValidationError, field string, t store.I18nText, required bool) {
	nonEmpty := false
	for k, v := range t {
		if !localeRe.MatchString(k) {
			ve.Add(field+"."+k, "Codice lingua non valido.")
			continue
		}
		if utf8.RuneCountInString(v) > maxNameLen {
			ve.Add(field+"."+k, fmt.Sprintf("Massimo %d caratteri.", maxNameLen))
		}
		if strings.TrimSpace(v) != "" {
			nonEmpty = true
		}
	}
	if required && !nonEmpty {
		ve.Add(field, "Obbligatorio in almeno una lingua.")
	}
}

// cleanI18n trims values and drops empty translations (nil when empty).
func cleanI18n(t store.I18nText) store.I18nText {
	out := store.I18nText{}
	for k, v := range t {
		if v = strings.TrimSpace(v); v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateMapInput(ve *configerr.ValidationError, prefix string, in MapInput) {
	validateI18n(ve, prefix+"name", in.Name, true)
	if in.Width < 1 || in.Width > maxMapSide {
		ve.Add(prefix+"width", "Deve essere tra 1 e 100000.")
	}
	if in.Height < 1 || in.Height > maxMapSide {
		ve.Add(prefix+"height", "Deve essere tra 1 e 100000.")
	}
	if in.GridSize < 1 || (in.Width >= 1 && in.Height >= 1 && in.GridSize > min(in.Width, in.Height)) {
		ve.Add(prefix+"grid_size", "Deve essere almeno 1 e non superare le dimensioni della mappa.")
	}
	if !slices.Contains(SeaSides, in.SeaSide) {
		ve.Add(prefix+"sea_side", "Valore non valido (top, bottom, left, right).")
	}
	if b := in.Background; b != nil {
		validateBackground(ve, prefix+"background", b)
	}
}

func validateBackground(ve *configerr.ValidationError, field string, b *Background) {
	switch b.Type {
	case "image":
		if b.URL == "" {
			ve.Add(field+".url", "Obbligatorio per uno sfondo immagine.")
		} else if len(b.URL) > maxURLLen {
			ve.Add(field+".url", "URL troppo lungo.")
		} else if u, err := url.Parse(b.URL); err != nil || (u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http") || (u.Scheme == "" && !strings.HasPrefix(b.URL, "/")) {
			ve.Add(field+".url", "Usa un URL http(s) o un percorso che inizia con /.")
		}
	case "geojson":
		if len(b.GeoJSON) == 0 {
			ve.Add(field+".geojson", "Obbligatorio per uno sfondo GeoJSON.")
		}
	default:
		ve.Add(field+".type", "Valore non valido (image, geojson).")
	}
	if b.Scale != nil && (!finite(*b.Scale) || *b.Scale <= 0) {
		ve.Add(field+".scale", "Deve essere maggiore di zero.")
	}
	for name, v := range map[string]*float64{"rotation": b.Rotation, "offset_x": b.OffsetX, "offset_y": b.OffsetY} {
		if v != nil && !finite(*v) {
			ve.Add(field+"."+name, "Numero non valido.")
		}
	}
	if b.Opacity != nil && (!finite(*b.Opacity) || *b.Opacity < 0 || *b.Opacity > 1) {
		ve.Add(field+".opacity", "Deve essere tra 0 e 1.")
	}
}

func validateShape(ve *configerr.ValidationError, field string, s store.Shape, m MapInput) {
	if s.Type != ShapeCircle && s.Type != ShapeRect {
		ve.Add(field+".type", "Valore non valido (circle, rect).")
	}
	if !finite(s.W) || s.W <= 0 || s.W > float64(max(m.Width, m.Height)) {
		ve.Add(field+".w", "Deve essere maggiore di zero e non superare la mappa.")
	}
	if !finite(s.H) || s.H <= 0 || s.H > float64(max(m.Width, m.Height)) {
		ve.Add(field+".h", "Deve essere maggiore di zero e non superare la mappa.")
	}
}

// validatePosition checks that (x, y) is a finite point inside the map
// bounds [0, width] x [0, height] and that rotation is finite. Grid snapping
// is a client concern.
func validatePosition(ve *configerr.ValidationError, field string, x, y, rotation float64, m MapInput) {
	if !finite(x) || x < 0 || x > float64(m.Width) {
		ve.Add(field+".x", "Fuori dai limiti della mappa.")
	}
	if !finite(y) || y < 0 || y > float64(m.Height) {
		ve.Add(field+".y", "Fuori dai limiti della mappa.")
	}
	if !finite(rotation) {
		ve.Add(field+".rotation", "Numero non valido.")
	}
}

func validateQuantities(ve *configerr.ValidationError, field string, q map[string]int) {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !codeRe.MatchString(k) {
			ve.Add(field+"."+k, "Codice componente non valido.")
		}
		if q[k] < 0 {
			ve.Add(field+"."+k, "Deve essere un intero non negativo.")
		}
	}
}

// normalizeRotation maps a finite angle to [0, 360).
func normalizeRotation(r float64) float64 {
	if !finite(r) {
		return r
	}
	r = math.Mod(r, 360)
	if r < 0 {
		r += 360
	}
	if r == 0 { // avoid -0
		return 0
	}
	return r
}

// normalizeSpotInput trims strings, normalizes rotation and attributes.
func normalizeSpotInput(s *SpotInput) {
	s.Label = strings.TrimSpace(s.Label)
	s.RowLabel = strings.TrimSpace(s.RowLabel)
	s.Asset = strings.TrimSpace(s.Asset)
	s.Rotation = normalizeRotation(s.Rotation)
	if s.PriceCategoryOverride != nil && strings.TrimSpace(*s.PriceCategoryOverride) == "" {
		s.PriceCategoryOverride = nil
	}
	var attrs []string
	for _, a := range s.Attributes {
		a = strings.TrimSpace(a)
		if a != "" && !slices.Contains(attrs, a) {
			attrs = append(attrs, a)
		}
	}
	s.Attributes = attrs
}

// validateSpotFields checks the fields of one spot that do not need the
// database (zone ownership and ids are checked by the service).
func validateSpotFields(ve *configerr.ValidationError, field string, s SpotInput, m MapInput) {
	if s.Label == "" || utf8.RuneCountInString(s.Label) > maxLabelLen {
		ve.Add(field+".label", fmt.Sprintf("Obbligatoria, massimo %d caratteri.", maxLabelLen))
	}
	if utf8.RuneCountInString(s.RowLabel) > maxRowLabelLen {
		ve.Add(field+".row_label", fmt.Sprintf("Massimo %d caratteri.", maxRowLabelLen))
	}
	if s.Number < 0 {
		ve.Add(field+".number", "Deve essere un intero non negativo.")
	}
	if !slices.Contains(SpotKinds, s.Kind) {
		ve.Add(field+".kind", "Tipo di postazione non valido.")
	}
	validateSpotAsset(ve, field+".asset", s.Asset)
	if s.ZoneID == "" {
		ve.Add(field+".zone_id", "Obbligatoria.")
	}
	validatePosition(ve, field, s.X, s.Y, s.Rotation, m)
	validateShape(ve, field+".shape", s.Shape, m)
	validateQuantities(ve, field+".capacity", s.Capacity)
	validateQuantities(ve, field+".max_extra", s.MaxExtra)
	if s.PriceCategoryOverride != nil && !codeRe.MatchString(*s.PriceCategoryOverride) {
		ve.Add(field+".price_category_override", "Formato non valido (minuscole, cifre, - e _).")
	}
	if len(s.Attributes) > MaxAttributes {
		ve.Add(field+".attributes", fmt.Sprintf("Massimo %d attributi.", MaxAttributes))
	}
	for i, a := range s.Attributes {
		if utf8.RuneCountInString(a) > maxAttrLen {
			ve.Add(fmt.Sprintf("%s.attributes[%d]", field, i), fmt.Sprintf("Massimo %d caratteri.", maxAttrLen))
		}
	}
}

func validateDecorationFields(ve *configerr.ValidationError, field string, d DecorationInput, m MapInput) {
	if !slices.Contains(DecorationKinds, d.Kind) {
		ve.Add(field+".kind", "Tipo di elemento non valido.")
	}
	validateDecorationAsset(ve, field, d.Asset, d.Variant)
	validatePosition(ve, field, d.X, d.Y, d.Rotation, m)
	validateShape(ve, field+".shape", d.Shape, m)
	validateI18n(ve, field+".label", d.Label, false)
}

// validateDocument checks a full document without database access: map
// fields, spots, decorations, label and id uniqueness. It normalizes in.
func validateDocument(in *SaveInput) *configerr.ValidationError {
	ve := &configerr.ValidationError{}
	validateMapInput(ve, "map.", in.Map)
	if len(in.Spots) > MaxSpotsPerMap {
		ve.Add("spots", fmt.Sprintf("Massimo %d postazioni per mappa.", MaxSpotsPerMap))
		return ve
	}
	if len(in.Decorations) > MaxDecorationsPerMap {
		ve.Add("decorations", fmt.Sprintf("Massimo %d elementi decorativi per mappa.", MaxDecorationsPerMap))
		return ve
	}
	labels := make(map[string]int, len(in.Spots))
	ids := make(map[string]bool, len(in.Spots))
	for i := range in.Spots {
		s := &in.Spots[i]
		normalizeSpotInput(s)
		field := fmt.Sprintf("spots[%d]", i)
		validateSpotFields(ve, field, *s, in.Map)
		if s.Label != "" {
			if j, dup := labels[s.Label]; dup {
				ve.Add(field+".label", fmt.Sprintf("Etichetta già usata da spots[%d].", j))
			} else {
				labels[s.Label] = i
			}
		}
		if s.ID != "" {
			if ids[s.ID] {
				ve.Add(field+".id", "Postazione ripetuta nel documento.")
			}
			ids[s.ID] = true
		}
	}
	decIDs := make(map[string]bool, len(in.Decorations))
	for i := range in.Decorations {
		d := &in.Decorations[i]
		d.Rotation = normalizeRotation(d.Rotation)
		d.Label = cleanI18n(d.Label)
		d.Asset, d.Variant = strings.TrimSpace(d.Asset), strings.TrimSpace(d.Variant)
		field := fmt.Sprintf("decorations[%d]", i)
		validateDecorationFields(ve, field, *d, in.Map)
		if d.ID != "" {
			if decIDs[d.ID] {
				ve.Add(field+".id", "Elemento ripetuto nel documento.")
			}
			decIDs[d.ID] = true
		}
	}
	return ve
}

func validateZoneInput(in *ZoneInput) *configerr.ValidationError {
	ve := &configerr.ValidationError{}
	in.Code = strings.TrimSpace(in.Code)
	in.PriceCategory = strings.TrimSpace(in.PriceCategory)
	if !codeRe.MatchString(in.Code) {
		ve.Add("code", "Formato non valido (minuscole, cifre, - e _; massimo 32).")
	}
	validateI18n(ve, "name", in.Name, true)
	in.Name = cleanI18n(in.Name)
	if !slices.Contains(AssignmentModes, in.AssignmentMode) {
		ve.Add("assignment_mode", "Valore non valido (seat, zone, quantity).")
	}
	if in.MapID != nil && *in.MapID == "" {
		in.MapID = nil
	}
	if in.MapID == nil && (in.AssignmentMode == ModeSeat || in.AssignmentMode == ModeZone) {
		ve.Add("map_id", "Obbligatoria per le zone con postazioni (seat, zone).")
	}
	if q := in.OnlineQuota; q != nil {
		switch q.Type {
		case QuotaPercent:
			if q.Value < 0 || q.Value > 100 {
				ve.Add("online_quota.value", "Percentuale tra 0 e 100.")
			}
		case QuotaCount:
			if q.Value < 0 {
				ve.Add("online_quota.value", "Deve essere un intero non negativo.")
			}
		default:
			ve.Add("online_quota.type", "Valore non valido (percent, count).")
		}
	}
	if !codeRe.MatchString(in.PriceCategory) {
		ve.Add("price_category", "Formato non valido (minuscole, cifre, - e _; massimo 32).")
	}
	if in.AutoAssign && in.AssignmentMode != ModeZone {
		ve.Add("auto_assign", "Disponibile solo per le zone in modalità zone.")
	}
	return ve
}

func validatePoolInput(in *PoolInput) *configerr.ValidationError {
	ve := &configerr.ValidationError{}
	in.ComponentCode = strings.TrimSpace(in.ComponentCode)
	if in.ZoneID != nil && *in.ZoneID == "" {
		in.ZoneID = nil
	}
	if !codeRe.MatchString(in.ComponentCode) {
		ve.Add("component_code", "Formato non valido (minuscole, cifre, - e _; massimo 32).")
	}
	validateI18n(ve, "name", in.Name, true)
	in.Name = cleanI18n(in.Name)
	if in.Capacity < 0 {
		ve.Add("capacity", "Deve essere un intero non negativo.")
	}
	if len(in.CapacityOverrides) > MaxCapacityOverrides {
		ve.Add("capacity_overrides", fmt.Sprintf("Massimo %d variazioni.", MaxCapacityOverrides))
		return ve
	}
	for i, o := range in.CapacityOverrides {
		field := fmt.Sprintf("capacity_overrides[%d]", i)
		if o.From.IsZero() || o.To.IsZero() {
			ve.Add(field, "Date obbligatorie.")
			continue
		}
		if o.To.Before(o.From) {
			ve.Add(field+".to", "Deve essere uguale o successiva a from.")
		}
		if o.Capacity < 0 {
			ve.Add(field+".capacity", "Deve essere un intero non negativo.")
		}
	}
	if len(ve.Fields) > 0 {
		return ve
	}
	// Sort by start date and reject overlaps (inclusive ranges).
	sorted := slices.Clone(in.CapacityOverrides)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].From.Before(sorted[b].From) })
	for i := 1; i < len(sorted); i++ {
		if !sorted[i].From.After(sorted[i-1].To) {
			ve.Add("capacity_overrides", fmt.Sprintf("Gli intervalli %s–%s e %s–%s si sovrappongono.",
				sorted[i-1].From, sorted[i-1].To, sorted[i].From, sorted[i].To))
		}
	}
	in.CapacityOverrides = sorted
	return ve
}
