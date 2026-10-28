package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Value sets of the catalog (spec 6.2, 6.3).
var (
	// SpotKinds are the kinds of spot a pack may require: the one list of
	// the mapping area, so a pack can target every kind a spot can have.
	SpotKinds = mapping.SpotKinds
	// Formulas are the duration formulas a pack may allow.
	Formulas = []string{
		string(pricing.FormulaHalfDay), string(pricing.FormulaDay), string(pricing.FormulaMultiDay),
		string(pricing.FormulaWeek), string(pricing.FormulaFortnight), string(pricing.FormulaMonth),
		string(pricing.FormulaSeason),
	}
	// Channels are the sales channels.
	Channels = []string{string(pricing.ChannelDesk), string(pricing.ChannelOnline), string(pricing.ChannelAPI)}
	// PricingUnits are the pricing units of an extra.
	PricingUnits = []string{string(pricing.PerBookingUnit), string(pricing.PerDayUnit), string(pricing.PerSlotUnit)}
	// AttachTargets are the attach_to values of an extra.
	AttachTargets = []string{AttachLine, AttachBooking}
)

// attach_to values of an extra.
const (
	AttachLine    = "line"
	AttachBooking = "booking"
)

// Bounds of the catalog fields.
const (
	MaxCodeLen         = 64
	MaxNameLen         = 200
	MaxDescriptionLen  = 4000
	MaxImageLen        = 2048
	MaxQuantityPerItem = 1000
	MaxSortOrder       = 1_000_000
	MaxVATRateBP       = 10_000
)

var (
	codeRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
)

// validateCode checks a catalog code (lowercase letters, digits, "_" and "-").
func validateCode(ve *configerr.ValidationError, field, code string) {
	switch {
	case code == "":
		ve.Add(field, "Il codice è obbligatorio.")
	case len(code) > MaxCodeLen:
		ve.Add(field, fmt.Sprintf("Il codice può avere al massimo %d caratteri.", MaxCodeLen))
	case !codeRe.MatchString(code):
		ve.Add(field, "Usa solo lettere minuscole, cifre, \"_\" e \"-\" (es. sunbed, pack-front).")
	}
}

// normalizeText trims the values of a localized text, drops empty ones and
// checks locales and lengths. When required, the text of defaultLocale must
// be present (readers fall back to it, spec 20).
func normalizeText(ve *configerr.ValidationError, field string, in store.I18nText, defaultLocale string, required bool, maxLen int) store.I18nText {
	out := store.I18nText{}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimSpace(in[k])
		if !localeRe.MatchString(k) {
			ve.Add(field+"."+k, "Codice lingua non valido.")
			continue
		}
		if v == "" {
			continue
		}
		if utf8.RuneCountInString(v) > maxLen {
			ve.Add(field+"."+k, fmt.Sprintf("Al massimo %d caratteri.", maxLen))
			continue
		}
		out[k] = v
	}
	if required && out[defaultLocale] == "" {
		ve.Add(field+"."+defaultLocale, "Il testo nella lingua predefinita dello stabilimento è obbligatorio.")
	}
	return out
}

// normalizeSet checks that every value is in allowed, removes duplicates and
// returns the values in the order of allowed. required rejects an empty set.
func normalizeSet(ve *configerr.ValidationError, field string, in []string, allowed []string, required bool) []string {
	seen := map[string]bool{}
	for i, v := range in {
		if !slices.Contains(allowed, v) {
			ve.Add(fmt.Sprintf("%s[%d]", field, i), fmt.Sprintf("Valore non ammesso: usa uno tra %s.", strings.Join(allowed, ", ")))
			continue
		}
		seen[v] = true
	}
	out := make([]string, 0, len(seen))
	for _, a := range allowed {
		if seen[a] {
			out = append(out, a)
		}
	}
	if required && len(in) == 0 {
		ve.Add(field, "Seleziona almeno un valore.")
	}
	return out
}

// dedupeIDs trims and dedupes ids preserving order; empty ids are errors.
func dedupeIDs(ve *configerr.ValidationError, field string, in []string) []string {
	out := make([]string, 0, len(in))
	for i, id := range in {
		id = strings.TrimSpace(id)
		if id == "" {
			ve.Add(fmt.Sprintf("%s[%d]", field, i), "ID mancante.")
			continue
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func validateVAT(ve *configerr.ValidationError, v *int64) *int64 {
	if v == nil {
		return nil
	}
	if *v < 0 || *v > MaxVATRateBP {
		ve.Add("vat_rate_bp", "L'aliquota deve essere tra 0 e 10000 punti base (100%).")
		return nil
	}
	x := *v
	return &x
}

func validateSortOrder(ve *configerr.ValidationError, v int) {
	if v < -MaxSortOrder || v > MaxSortOrder {
		ve.Add("sort_order", "Valore fuori intervallo.")
	}
}

func validateImage(ve *configerr.ValidationError, img *string) *string {
	if img == nil {
		return nil
	}
	s := strings.TrimSpace(*img)
	if s == "" {
		return nil
	}
	if len(s) > MaxImageLen {
		ve.Add("image", fmt.Sprintf("Al massimo %d caratteri.", MaxImageLen))
		return nil
	}
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "/") {
		ve.Add("image", "Indica un URL http(s) o un percorso che inizia con \"/\".")
		return nil
	}
	return &s
}

func boundedInt(ve *configerr.ValidationError, field string, v *int, def int) int {
	if v == nil {
		return def
	}
	if *v < 1 || *v > MaxQuantityPerItem {
		ve.Add(field, fmt.Sprintf("Deve essere tra 1 e %d.", MaxQuantityPerItem))
		return def
	}
	return *v
}

// ---- inputs -----------------------------------------------------------------

// ComponentInput creates or replaces a component.
type ComponentInput struct {
	Code    string
	Name    store.I18nText
	Tracked bool
	// PoolID is required when Tracked, forbidden otherwise.
	PoolID *string
}

// PackInput creates or replaces a pack. Optional fields (nil) take their
// default: MaxPerBooking 1, Active true on create and unchanged on update,
// VATRateBP the establishment default.
type PackInput struct {
	Code             string
	Name             store.I18nText
	Description      store.I18nText
	Image            *string
	Composition      map[string]int
	RequiresSpotKind []string // empty = every kind
	AllowedZoneIDs   []string // empty = every zone
	AllowedFormulas  []string
	Channels         []string
	MaxPerBooking    *int
	RequiresApproval bool
	SortOrder        int
	Active           *bool
	VATRateBP        *int64
}

// ExtraInput creates or replaces an extra. Optional fields (nil) take their
// default: MaxQuantity 1, Active true on create and unchanged on update,
// VATRateBP the establishment default.
type ExtraInput struct {
	Code           string
	Name           store.I18nText
	Description    store.I18nText
	ComponentCode  *string
	PricingUnit    string
	MaxQuantity    *int
	AttachTo       string
	AllowedPackIDs []string // empty = every pack
	Channels       []string
	SortOrder      int
	Active         *bool
	VATRateBP      *int64
	// ShowOnMap / MapLabel (spec 30.4): the extra appears in the spot
	// setup of the operational map ("2L 1S"); nil keeps the stored value.
	ShowOnMap *bool
	MapLabel  *string
}

// componentFields is a validated ComponentInput.
type componentFields struct {
	Code    string
	Name    store.I18nText
	Tracked bool
	PoolID  *string
}

// validateComponent checks the self-contained rules of a component.
func validateComponent(in ComponentInput, defaultLocale string) (componentFields, *configerr.ValidationError) {
	ve := &configerr.ValidationError{}
	f := componentFields{Code: strings.TrimSpace(in.Code), Tracked: in.Tracked}
	validateCode(ve, "code", f.Code)
	f.Name = normalizeText(ve, "name", in.Name, defaultLocale, true, MaxNameLen)
	var pool string
	if in.PoolID != nil {
		pool = strings.TrimSpace(*in.PoolID)
	}
	switch {
	case in.Tracked && pool == "":
		ve.Add("pool_id", "Un componente tracciato richiede il pool da cui consumare.")
	case !in.Tracked && pool != "":
		ve.Add("pool_id", "Il pool si indica solo per i componenti tracciati.")
	case pool != "":
		f.PoolID = &pool
	}
	return f, ve
}

// packFields is a validated PackInput (before the checks against the DB).
type packFields struct {
	Code             string
	Name             store.I18nText
	Description      store.I18nText
	Image            *string
	Composition      map[string]int
	RequiresSpotKind []string
	AllowedZoneIDs   []string
	AllowedFormulas  []string
	Channels         []string
	MaxPerBooking    int
	RequiresApproval bool
	SortOrder        int
	Active           *bool
	VATRateBP        *int64
}

// validatePack checks the self-contained rules of a pack.
func validatePack(in PackInput, defaultLocale string) (packFields, *configerr.ValidationError) {
	ve := &configerr.ValidationError{}
	f := packFields{
		Code:             strings.TrimSpace(in.Code),
		RequiresApproval: in.RequiresApproval,
		SortOrder:        in.SortOrder,
		Active:           in.Active,
	}
	validateCode(ve, "code", f.Code)
	f.Name = normalizeText(ve, "name", in.Name, defaultLocale, true, MaxNameLen)
	f.Description = normalizeText(ve, "description", in.Description, defaultLocale, false, MaxDescriptionLen)
	f.Image = validateImage(ve, in.Image)
	f.Composition = map[string]int{}
	if len(in.Composition) == 0 {
		ve.Add("composition", "Indica almeno un componente.")
	}
	codes := make([]string, 0, len(in.Composition))
	for c := range in.Composition {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		q := in.Composition[c]
		if q < 1 || q > MaxQuantityPerItem {
			ve.Add("composition."+c, fmt.Sprintf("La quantità deve essere tra 1 e %d.", MaxQuantityPerItem))
			continue
		}
		f.Composition[c] = q
	}
	if kinds := normalizeSet(ve, "requires_spot_kind", in.RequiresSpotKind, SpotKinds, false); len(kinds) > 0 {
		f.RequiresSpotKind = kinds
	}
	if zones := dedupeIDs(ve, "allowed_zone_ids", in.AllowedZoneIDs); len(zones) > 0 {
		f.AllowedZoneIDs = zones
	}
	f.AllowedFormulas = normalizeSet(ve, "allowed_formulas", in.AllowedFormulas, Formulas, true)
	f.Channels = normalizeSet(ve, "channels", in.Channels, Channels, true)
	f.MaxPerBooking = boundedInt(ve, "max_per_booking", in.MaxPerBooking, 1)
	validateSortOrder(ve, in.SortOrder)
	f.VATRateBP = validateVAT(ve, in.VATRateBP)
	return f, ve
}

// extraFields is a validated ExtraInput (before the checks against the DB).
type extraFields struct {
	Code           string
	Name           store.I18nText
	Description    store.I18nText
	ComponentCode  *string
	PricingUnit    string
	MaxQuantity    int
	AttachTo       string
	AllowedPackIDs []string
	Channels       []string
	SortOrder      int
	Active         *bool
	VATRateBP      *int64
	ShowOnMap      *bool
	MapLabel       *string
}

// validateExtra checks the self-contained rules of an extra.
func validateExtra(in ExtraInput, defaultLocale string) (extraFields, *configerr.ValidationError) {
	ve := &configerr.ValidationError{}
	f := extraFields{
		Code:        strings.TrimSpace(in.Code),
		PricingUnit: in.PricingUnit,
		AttachTo:    in.AttachTo,
		SortOrder:   in.SortOrder,
		Active:      in.Active,
	}
	validateCode(ve, "code", f.Code)
	f.Name = normalizeText(ve, "name", in.Name, defaultLocale, true, MaxNameLen)
	f.Description = normalizeText(ve, "description", in.Description, defaultLocale, false, MaxDescriptionLen)
	if in.ComponentCode != nil {
		if c := strings.TrimSpace(*in.ComponentCode); c != "" {
			f.ComponentCode = &c
		}
	}
	if !slices.Contains(PricingUnits, in.PricingUnit) {
		ve.Add("pricing_unit", "Usa per_booking, per_day o per_slot.")
	}
	if f.AttachTo == "" {
		f.AttachTo = AttachLine
	}
	if !slices.Contains(AttachTargets, f.AttachTo) {
		ve.Add("attach_to", "Usa line o booking.")
	}
	f.MaxQuantity = boundedInt(ve, "max_quantity", in.MaxQuantity, 1)
	if ids := dedupeIDs(ve, "allowed_pack_ids", in.AllowedPackIDs); len(ids) > 0 {
		f.AllowedPackIDs = ids
		if f.AttachTo == AttachBooking {
			ve.Add("allowed_pack_ids", "Un extra legato alla prenotazione non può essere riservato a specifici pack.")
		}
	}
	f.Channels = normalizeSet(ve, "channels", in.Channels, Channels, true)
	validateSortOrder(ve, in.SortOrder)
	f.VATRateBP = validateVAT(ve, in.VATRateBP)
	validateMapSetup(ve, &f, in)
	return f, ve
}
