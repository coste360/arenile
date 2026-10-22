package setup

import (
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// Code identifies a setup template (spec 15.2).
type Code string

// Template codes (api.SetupTemplateCode).
const (
	CodeClassicRows   Code = "classic_rows"
	CodeQuantityBeach Code = "quantity_beach"
	CodeLidoWithPool  Code = "lido_with_pool"
	CodeEmpty         Code = "empty"
)

// Valid reports whether c is a known template.
func (c Code) Valid() bool {
	switch c {
	case CodeClassicRows, CodeQuantityBeach, CodeLidoWithPool, CodeEmpty:
		return true
	}
	return false
}

// Zone codes, price categories and rate period codes created by templates.
// The demo seed and the tests rely on them.
const (
	CategoryRow1     = "row1"
	CategoryRow2     = "row2"
	CategoryStandard = "standard"
	CategoryGazebo   = "gazebo"
	CategoryPool     = "pool"
	CategoryFree     = "free"

	PeriodLow  = "low"
	PeriodMid  = "mid"
	PeriodHigh = "high"
	PeriodPeak = "peak"

	PackBase      = "base"
	PackDeckchair = "comfort"
	PackGazebo    = "gazebo"
	PackSunbed    = "sunbed_only"
)

// ZoneSpec is a zone proposed by a template.
type ZoneSpec struct {
	Code           string
	Name           store.I18nText
	AssignmentMode string // seat | zone | quantity
	PriceCategory  string
	// Map is the key of the template map holding the zone ("" for quantity
	// zones, which have no map).
	Map        string
	AutoAssign bool
	// Pools are the quantity pools of a quantity zone.
	Pools []PoolSpec
}

// PoolSpec is a quantity pool proposed by a template.
type PoolSpec struct {
	ComponentCode string
	Name          store.I18nText
	Capacity      int
}

// ComponentSpec is a catalog component proposed by a template. A component
// with Pool is tracked through an establishment-level pool (cabins, parking).
type ComponentSpec struct {
	Code string
	Name store.I18nText
	Pool *PoolSpec
}

// PackSpec is a pack proposed by a template.
type PackSpec struct {
	Code             string
	Name             store.I18nText
	Description      store.I18nText
	Composition      map[string]int
	RequiresSpotKind []string
	RequiresApproval bool
	MaxPerBooking    int
}

// ExtraSpec is an extra proposed by a template.
type ExtraSpec struct {
	Code          string
	Name          store.I18nText
	Description   store.I18nText
	ComponentCode string
	PricingUnit   string // per_booking | per_day | per_slot
	MaxQuantity   int
	AttachTo      string // line | booking
}

// PeriodSpec is a rate period proposed by templates.
type PeriodSpec struct {
	Code  string
	Name  store.I18nText
	Color string
}

// PolicySpec is a booking policy proposed by a template.
type PolicySpec struct {
	Name     string
	Scope    policy.Scope
	Rules    policy.Rules
	Priority int
	// RequiresPack skips the policy when the template has no such pack.
	RequiresPack string
}

// MapSpec is a map of a template. The grid of the map ("main" map) is sized
// by the request (rows x spots per row); extra rows (gazebos, pool area) are
// fixed.
type MapSpec struct {
	Key     string
	Name    store.I18nText
	SeaSide string
	// Grid is true for the map that holds the configurable rows grid.
	Grid bool
	// Fixed are rows added behind the grid (or the only rows of the map).
	Fixed []FixedRows
}

// FixedRows is a fixed block of rows of a template map.
type FixedRows struct {
	Zone        string
	FirstRow    string
	Rows        int
	SpotsPerRow int
	Kind        string
}

// Template is a guided setup template.
type Template struct {
	Code        Code
	Name        store.I18nText
	Description store.I18nText
	// Rows and SpotsPerRow are the default grid (0 when the template has no
	// map with rows).
	Rows        int
	SpotsPerRow int
	Zones       []ZoneSpec
	Maps        []MapSpec
	Components  []ComponentSpec
	Packs       []PackSpec
	Extras      []ExtraSpec
	Policies    []PolicySpec
}

// RatePeriodCodes returns the codes of the periods every template creates
// (all templates share Periods).
func (t Template) RatePeriodCodes() []string {
	out := make([]string, len(Periods))
	for i, p := range Periods {
		out[i] = p.Code
	}
	return out
}

// HasGrid reports whether the template generates a rows grid.
func (t Template) HasGrid() bool { return t.Rows > 0 }

func i18n(it, en, de, fr string) store.I18nText {
	return store.I18nText{"it": it, "en": en, "de": de, "fr": fr}
}

// Periods are the rate periods created by every template, in sort order.
// Their date ranges are computed by periodOf for the season dates.
var Periods = []PeriodSpec{
	{Code: PeriodLow, Name: i18n("Bassa stagione", "Low season", "Nebensaison", "Basse saison"), Color: "#7FB3D5"},
	{Code: PeriodMid, Name: i18n("Media stagione", "Mid season", "Zwischensaison", "Moyenne saison"), Color: "#76D7C4"},
	{Code: PeriodHigh, Name: i18n("Alta stagione", "High season", "Hochsaison", "Haute saison"), Color: "#F5B041"},
	{Code: PeriodPeak, Name: i18n("Ferragosto", "Peak (August)", "Hauptsaison (August)", "Pic (août)"), Color: "#E74C3C"},
}

// Shared catalog building blocks.
var (
	compUmbrella  = ComponentSpec{Code: "umbrella", Name: i18n("Ombrellone", "Beach umbrella", "Sonnenschirm", "Parasol")}
	compSunbed    = ComponentSpec{Code: "sunbed", Name: i18n("Lettino", "Sunbed", "Sonnenliege", "Transat")}
	compDeckchair = ComponentSpec{Code: "deckchair", Name: i18n("Sdraio", "Deckchair", "Liegestuhl", "Chaise longue")}
	compCabin     = ComponentSpec{
		Code: "cabin", Name: i18n("Cabina", "Changing cabin", "Umkleidekabine", "Cabine"),
		Pool: &PoolSpec{ComponentCode: "cabin", Name: i18n("Cabine", "Changing cabins", "Umkleidekabinen", "Cabines"), Capacity: 10}, //nolint:misspell // French
	}
	compParking = ComponentSpec{
		Code: "parking", Name: i18n("Posto auto", "Parking space", "Parkplatz", "Place de parking"),
		Pool: &PoolSpec{ComponentCode: "parking", Name: i18n("Parcheggio", "Car park", "Parkplatz", "Parking"), Capacity: 30},
	}

	packBase = PackSpec{
		Code: PackBase, Name: i18n("Base", "Basic", "Basis", "Base"),
		Description: i18n("1 ombrellone e 2 lettini", "1 umbrella and 2 sunbeds", "1 Sonnenschirm und 2 Liegen", "1 parasol et 2 transats"),
		Composition: map[string]int{"umbrella": 1, "sunbed": 2}, MaxPerBooking: 4,
	}
	packComfort = PackSpec{
		Code: PackDeckchair, Name: i18n("Comfort", "Comfort", "Komfort", "Confort"),
		Description: i18n("1 ombrellone, 1 lettino e 1 sdraio", "1 umbrella, 1 sunbed and 1 deckchair", "1 Sonnenschirm, 1 Liege und 1 Liegestuhl", "1 parasol, 1 transat et 1 chaise longue"),
		Composition: map[string]int{"umbrella": 1, "sunbed": 1, "deckchair": 1}, MaxPerBooking: 4,
	}
	packGazebo = PackSpec{
		Code: PackGazebo, Name: i18n("Gazebo", "Gazebo", "Pavillon", "Gazebo"),
		Description: i18n("Gazebo con 2 lettini e 2 sdraio, su approvazione", "Gazebo with 2 sunbeds and 2 deckchairs, subject to approval", "Pavillon mit 2 Liegen und 2 Liegestühlen, nach Genehmigung", "Gazebo avec 2 transats et 2 chaises longues, sur approbation"),
		Composition: map[string]int{"sunbed": 2, "deckchair": 2}, RequiresSpotKind: []string{"gazebo"}, RequiresApproval: true, MaxPerBooking: 1,
	}
	packSunbed = PackSpec{
		Code: PackSunbed, Name: i18n("Solo lettino", "Sunbed only", "Nur Liege", "Transat seul"),
		Description: i18n("1 lettino senza ombrellone", "1 sunbed without umbrella", "1 Liege ohne Sonnenschirm", "1 transat sans parasol"),
		Composition: map[string]int{"sunbed": 1}, MaxPerBooking: 6,
	}

	extraSunbed = ExtraSpec{
		Code: "extra_sunbed", Name: i18n("Lettino aggiuntivo", "Extra sunbed", "Zusätzliche Liege", "Transat supplémentaire"),
		Description:   i18n("Un lettino in più accanto all'ombrellone", "One more sunbed next to the umbrella", "Eine weitere Liege neben dem Schirm", "Un transat de plus près du parasol"),
		ComponentCode: "sunbed", PricingUnit: "per_day", MaxQuantity: 2, AttachTo: "line",
	}
	extraCabin = ExtraSpec{
		Code: "cabin", Name: i18n("Cabina", "Changing cabin", "Umkleidekabine", "Cabine"),
		Description:   i18n("Cabina spogliatoio privata", "Private changing cabin", "Private Umkleidekabine", "Cabine privée"),
		ComponentCode: "cabin", PricingUnit: "per_day", MaxQuantity: 1, AttachTo: "booking",
	}
	extraTowel = ExtraSpec{
		Code: "towel", Name: i18n("Telo mare", "Beach towel", "Strandtuch", "Serviette de plage"),
		Description: i18n("Telo mare a noleggio", "Beach towel rental", "Strandtuch zum Ausleihen", "Location de serviette de plage"),
		PricingUnit: "per_booking", MaxQuantity: 4, AttachTo: "line",
	}
	extraParking = ExtraSpec{
		Code: "parking", Name: i18n("Parcheggio", "Parking", "Parkplatz", "Parking"),
		Description:   i18n("Posto auto nel parcheggio del lido", "Parking space in the lido car park", "Parkplatz am Strandbad", "Place dans le parking de la plage"),
		ComponentCode: "parking", PricingUnit: "per_day", MaxQuantity: 1, AttachTo: "booking",
	}

	zoneRow1     = ZoneSpec{Code: "row1", Name: i18n("Prima fila", "Front row", "Erste Reihe", "Premier rang"), AssignmentMode: "seat", PriceCategory: CategoryRow1, Map: "beach"}
	zoneRow2     = ZoneSpec{Code: "row2", Name: i18n("Seconda fila", "Second row", "Zweite Reihe", "Deuxième rang"), AssignmentMode: "seat", PriceCategory: CategoryRow2, Map: "beach"}
	zoneStandard = ZoneSpec{Code: "standard", Name: i18n("File successive", "Back rows", "Hintere Reihen", "Rangs suivants"), AssignmentMode: "seat", PriceCategory: CategoryStandard, Map: "beach"}

	policyOnline = PolicySpec{
		Name:     "Online: pagamento completo",
		Scope:    policy.Scope{Channels: []policy.Channel{policy.ChannelOnline}},
		Rules:    policy.Rules{Payment: &policy.Payment{Mode: policy.Ptr(policy.PaymentFull)}},
		Priority: 10,
	}
	policyDesk = PolicySpec{
		Name:  "Cassa: pagamento all'arrivo",
		Scope: policy.Scope{Channels: []policy.Channel{policy.ChannelDesk}},
		Rules: policy.Rules{
			Payment:     &policy.Payment{Mode: policy.Ptr(policy.PaymentNone), BalanceDue: policy.Ptr(policy.BalanceDueOnArrival)},
			MinLeadTime: &policy.MinLeadTime{Days: policy.Ptr(0)},
		},
		Priority: 10,
	}
	policyGazebo = PolicySpec{
		Name:         "Gazebo: approvazione del gestore",
		Scope:        policy.Scope{Packs: []string{PackGazebo}},
		Rules:        policy.Rules{ApprovalRequired: policy.Ptr(true)},
		Priority:     20,
		RequiresPack: PackGazebo,
	}
)

var templates = []Template{
	{
		Code: CodeClassicRows,
		Name: i18n("Lido classico a file", "Classic lido with rows", "Klassisches Strandbad mit Reihen", "Plage classique en rangs"),
		Description: i18n(
			"Spiaggia con file di ombrelloni (prima e seconda fila con prezzo proprio), una fila di gazebo su approvazione, cabine e parcheggio.",
			"Beach with rows of umbrellas (front and second row priced separately), a row of gazebos subject to approval, cabins and parking.",
			"Strand mit Schirmreihen (erste und zweite Reihe mit eigenem Preis), eine Reihe Pavillons nach Genehmigung, Kabinen und Parkplatz.",
			"Plage avec rangs de parasols (premier et deuxième rang à tarif dédié), un rang de gazebos sur approbation, cabines et parking.", //nolint:misspell // French
		),
		Rows: 8, SpotsPerRow: 15,
		Zones: []ZoneSpec{
			zoneRow1, zoneRow2, zoneStandard,
			{Code: "gazebo", Name: i18n("Gazebo", "Gazebos", "Pavillons", "Gazebos"), AssignmentMode: "seat", PriceCategory: CategoryGazebo, Map: "beach"},
		},
		Maps: []MapSpec{{
			Key: "beach", Name: i18n("Spiaggia", "Beach", "Strand", "Plage"), SeaSide: "bottom", Grid: true,
			// "GZ" cannot collide with the grid rows (A..AZ, at most 52).
			Fixed: []FixedRows{{Zone: "gazebo", FirstRow: "GZ", Rows: 1, SpotsPerRow: 4, Kind: "gazebo"}},
		}},
		Components: []ComponentSpec{compUmbrella, compSunbed, compDeckchair, compCabin, compParking},
		Packs:      []PackSpec{withSpotKind(packBase, "umbrella"), withSpotKind(packComfort, "umbrella"), packGazebo},
		Extras:     []ExtraSpec{extraSunbed, extraCabin, extraTowel, extraParking},
		Policies:   []PolicySpec{policyOnline, policyDesk, policyGazebo},
	},
	{
		Code: CodeQuantityBeach,
		Name: i18n("Spiaggia attrezzata a quantità", "Equipped beach by quantity", "Ausgestatteter Strand nach Menge", "Plage équipée à la quantité"),
		Description: i18n(
			"Nessuna mappa: il cliente sceglie quanti ombrelloni e lettini, la disponibilità è un contatore.",
			"No map: customers choose how many umbrellas and sunbeds, availability is a counter.",
			"Keine Karte: Gäste wählen die Anzahl der Schirme und Liegen, die Verfügbarkeit ist ein Zähler.",
			"Pas de plan : le client choisit combien de parasols et transats, la disponibilité est un compteur.",
		),
		Zones: []ZoneSpec{{
			Code: "beach", Name: i18n("Spiaggia attrezzata", "Equipped beach", "Ausgestatteter Strand", "Plage équipée"),
			AssignmentMode: "quantity", PriceCategory: CategoryStandard,
			Pools: []PoolSpec{
				{ComponentCode: "umbrella", Name: i18n("Ombrelloni", "Umbrellas", "Sonnenschirme", "Parasols"), Capacity: 80},
				{ComponentCode: "sunbed", Name: i18n("Lettini", "Sunbeds", "Liegen", "Transats"), Capacity: 200},
			},
		}},
		Components: []ComponentSpec{compUmbrella, compSunbed},
		Packs:      []PackSpec{packBase, packSunbed},
		Extras:     []ExtraSpec{extraSunbed, extraTowel},
		Policies:   []PolicySpec{policyOnline, policyDesk},
	},
	{
		Code: CodeLidoWithPool,
		Name: i18n("Lido con piscina", "Lido with pool", "Strandbad mit Pool", "Plage avec piscine"),
		Description: i18n(
			"Spiaggia a file, area piscina con assegnazione del posto da parte del gestore e spiaggia libera attrezzata a quantità.",
			"Beach with rows, a pool area where the manager assigns the spot and an equipped free beach sold by quantity.",
			"Strand mit Reihen, Poolbereich mit Platzzuweisung durch den Betreiber und ausgestatteter freier Strand nach Menge.",
			"Plage en rangs, espace piscine avec attribution de la place par le gérant et plage libre équipée à la quantité.",
		),
		Rows: 6, SpotsPerRow: 12,
		Zones: []ZoneSpec{
			zoneRow1, zoneRow2, zoneStandard,
			{Code: "pool", Name: i18n("Piscina", "Pool", "Pool", "Piscine"), AssignmentMode: "zone", PriceCategory: CategoryPool, Map: "pool", AutoAssign: true},
			{
				Code: "free_beach", Name: i18n("Spiaggia libera attrezzata", "Equipped free beach", "Ausgestatteter freier Strand", "Plage libre équipée"),
				AssignmentMode: "quantity", PriceCategory: CategoryFree,
				Pools: []PoolSpec{
					{ComponentCode: "umbrella", Name: i18n("Ombrelloni spiaggia libera", "Free beach umbrellas", "Schirme freier Strand", "Parasols plage libre"), Capacity: 30},
					{ComponentCode: "sunbed", Name: i18n("Lettini spiaggia libera", "Free beach sunbeds", "Liegen freier Strand", "Transats plage libre"), Capacity: 60},
				},
			},
		},
		Maps: []MapSpec{
			{Key: "beach", Name: i18n("Spiaggia", "Beach", "Strand", "Plage"), SeaSide: "bottom", Grid: true},
			{
				Key: "pool", Name: i18n("Piscina", "Pool", "Pool", "Piscine"), SeaSide: "bottom",
				Fixed: []FixedRows{{Zone: "pool", FirstRow: "P", Rows: 2, SpotsPerRow: 10, Kind: "umbrella"}},
			},
		},
		Components: []ComponentSpec{compUmbrella, compSunbed, compDeckchair, compCabin, compParking},
		Packs:      []PackSpec{packBase, packComfort},
		Extras:     []ExtraSpec{extraSunbed, extraCabin, extraTowel, extraParking},
		Policies:   []PolicySpec{policyOnline, policyDesk},
	},
	{
		Code: CodeEmpty,
		Name: i18n("Vuoto", "Empty", "Leer", "Vide"),
		Description: i18n(
			"Solo la stagione e i periodi tariffari: mappa, zone e catalogo si configurano a mano.",
			"Only the season and its rate periods: map, zones and catalog are configured by hand.",
			"Nur Saison und Tarifperioden: Karte, Zonen und Katalog werden manuell konfiguriert.",
			"Seulement la saison et ses périodes tarifaires : plan, zones et catalogue se configurent à la main.",
		),
	},
}

func withSpotKind(p PackSpec, kinds ...string) PackSpec {
	p.RequiresSpotKind = kinds
	return p
}

// Templates returns the setup templates in display order.
func Templates() []Template {
	out := make([]Template, len(templates))
	copy(out, templates)
	return out
}

// TemplateByCode returns the template with code.
func TemplateByCode(c Code) (Template, bool) {
	for _, t := range templates {
		if t.Code == c {
			return t, true
		}
	}
	return Template{}, false
}
