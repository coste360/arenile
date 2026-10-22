package setup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/store"
)

// Season statuses (seasons.status).
const (
	SeasonDraft  = "draft"
	SeasonActive = "active"
)

// Limits of the template grid (api.ApplyTemplateRequest).
const (
	maxGridRows        = 52
	maxGridSpotsPerRow = 200
	maxSeasonDays      = 366
)

// Map layout of generated maps, in logical map units.
const (
	layoutMargin      = 80.0
	layoutSeaBand     = 60.0
	layoutSpacingX    = 60.0
	layoutSpacingY    = 70.0
	layoutGazeboX     = 100.0
	layoutGridSize    = 10
	layoutUmbrellaDia = 40.0
	layoutGazeboSide  = 60.0
)

// allFormulas is the allowed_formulas of template packs.
var allFormulas = []string{"half_day", "day", "multi_day", "week", "fortnight", "month", "season"}

// ApplyInput is the request to apply a template (api.ApplyTemplateRequest).
type ApplyInput struct {
	Template Code
	// SeasonYear defaults to the current season (DefaultSeasonYear), or to the
	// year of OpensOn.
	SeasonYear *int
	// OpensOn / ClosesOn default to 1 May / 30 September of the season year.
	OpensOn  *calendar.Date
	ClosesOn *calendar.Date
	// Rows / SpotsPerRow override the template grid (templates with rows only).
	Rows        *int
	SpotsPerRow *int
	// MapName overrides the name of the map holding the grid.
	MapName store.I18nText
	// Force applies a template even when the establishment already has maps
	// or zones: catalog items with an existing code are reused, new zone
	// codes get a numeric suffix. "empty" never needs it.
	Force bool
	// QuickPrices, when set, also generates the quick price list for the
	// season in the same transaction (packs: every pack of the template).
	QuickPrices *QuickPrices
	// ClientIP is recorded in the audit log.
	ClientIP string

	// seasonStatus is the status of a newly created season (draft by default;
	// the demo seed activates it).
	seasonStatus string
}

// ApplyResult reports what applying a template created or reused.
type ApplyResult struct {
	Template      Code
	SeasonID      string
	SeasonYear    int
	SeasonCreated bool
	// MapID is the map holding the grid (or the first map); empty when the
	// template has no map.
	MapID         string
	MapIDs        []string
	ZoneIDs       []string
	PoolIDs       []string
	ComponentIDs  []string
	PackIDs       []string
	ExtraIDs      []string
	RatePeriodIDs []string
	PolicyIDs     []string
	SpotsCreated  int
	// QuickRules are the base rules of QuickPrices (nil when not requested).
	QuickRules []store.PriceRule
}

// Apply applies a template to an establishment in one transaction
// (auth.PermEstablishmentConfigure). It refuses with ErrAlreadyConfigured
// when the establishment already has maps or zones, unless in.Force is set
// or the template is "empty".
func (s *Service) Apply(ctx context.Context, p *auth.Principal, establishmentID string, in ApplyInput) (*ApplyResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	in.seasonStatus = SeasonDraft
	var res *ApplyResult
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		var err error
		res, err = s.applyTx(ctx, tx, es, p.Actor(in.ClientIP), in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// applier holds the state of one template application.
type applier struct {
	s   *Service
	tx  bun.Tx
	es  store.EstablishmentScope
	t   Template
	in  ApplyInput
	res *ApplyResult

	rows, perRow int
	season       *store.Season
	// IDs by code of the zones and packs of this application.
	zoneByCode map[string]*store.Zone
	packCodes  map[string]string // pack code -> id
}

func (s *Service) applyTx(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, actor audit.Actor, in ApplyInput) (*ApplyResult, error) {
	t, ok := TemplateByCode(in.Template)
	if !ok {
		return nil, configerr.Invalid("template", "Modello sconosciuto.")
	}
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return nil, fmt.Errorf("setup: apply: %w", err)
	}
	a := &applier{
		s: s, tx: tx, es: es, t: t, in: in,
		res:        &ApplyResult{Template: t.Code},
		zoneByCode: map[string]*store.Zone{},
		packCodes:  map[string]string{},
	}
	year, opens, closes, err := a.validate(s.today(est))
	if err != nil {
		return nil, err
	}
	if t.Code != CodeEmpty && !in.Force {
		configured, err := a.hasInventory(ctx)
		if err != nil {
			return nil, err
		}
		if configured {
			return nil, ErrAlreadyConfigured
		}
	}
	steps := []func(context.Context) error{
		func(ctx context.Context) error { return a.ensureSeason(ctx, year, opens, closes) },
		a.ensureRatePeriods,
		a.ensureComponents,
		a.createMapsAndZones,
		a.ensurePacks,
		a.ensureExtras,
		a.ensurePolicies,
	}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return nil, err
		}
	}
	if in.QuickPrices != nil {
		qp := *in.QuickPrices
		if len(qp.PackIDs) == 0 {
			qp.PackIDs = a.res.PackIDs
		}
		qr, err := s.quickTx(ctx, tx, es, a.season, qp, true)
		if err != nil {
			return nil, err
		}
		a.res.QuickRules = qr.Rules
	}
	if err := a.record(ctx, actor); err != nil {
		return nil, err
	}
	return a.res, nil
}

// validate checks the request and resolves the season dates and grid size.
func (a *applier) validate(today calendar.Date) (year int, opens, closes calendar.Date, err error) {
	ve := &configerr.ValidationError{}
	in := a.in
	switch {
	case in.SeasonYear != nil:
		year = *in.SeasonYear
	case in.OpensOn != nil:
		year = in.OpensOn.Year()
	default:
		year = DefaultSeasonYear(today)
	}
	if year < 2000 || year > 2100 {
		ve.Add("season_year", "Anno non valido (2000-2100).")
		return 0, opens, closes, ve
	}
	opens, closes = defaultSeasonDates(year)
	if in.OpensOn != nil {
		opens = *in.OpensOn
	}
	if in.ClosesOn != nil {
		closes = *in.ClosesOn
	}
	if opens.Year() != year {
		ve.Add("opens_on", "L'apertura deve cadere nell'anno della stagione.")
	}
	if closes.Before(opens) {
		ve.Add("closes_on", "La chiusura deve essere uguale o successiva all'apertura.")
	} else if opens.DaysUntil(closes)+1 > maxSeasonDays {
		ve.Add("closes_on", "Una stagione non può superare un anno.")
	}
	a.rows, a.perRow = a.t.Rows, a.t.SpotsPerRow
	if a.t.HasGrid() {
		if in.Rows != nil {
			a.rows = *in.Rows
		}
		if in.SpotsPerRow != nil {
			a.perRow = *in.SpotsPerRow
		}
		if a.rows < 1 || a.rows > maxGridRows {
			ve.Add("rows", "Indica da 1 a 52 file.")
		}
		if a.perRow < 1 || a.perRow > maxGridSpotsPerRow {
			ve.Add("spots_per_row", "Indica da 1 a 200 postazioni per fila.")
		}
	}
	return year, opens, closes, ve.OrNil()
}

func (a *applier) hasInventory(ctx context.Context) (bool, error) {
	nMaps, err := a.tx.NewSelect().Model((*store.Map)(nil)).Where("m.establishment_id = ?", a.es.ID()).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("setup: count maps: %w", err)
	}
	nZones, err := a.tx.NewSelect().Model((*store.Zone)(nil)).Where("z.establishment_id = ?", a.es.ID()).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("setup: count zones: %w", err)
	}
	return nMaps+nZones > 0, nil
}

func (a *applier) insert(ctx context.Context, what string, m any) error {
	if _, err := a.tx.NewInsert().Model(m).Exec(ctx); err != nil {
		if store.IsUniqueViolation(err) {
			return fmt.Errorf("%w: %s already exists", configerr.ErrConflict, what)
		}
		return fmt.Errorf("setup: insert %s: %w", what, err)
	}
	return nil
}

// ensureSeason reuses the season of the year or creates it.
func (a *applier) ensureSeason(ctx context.Context, year int, opens, closes calendar.Date) error {
	var existing store.Season
	err := a.tx.NewSelect().Model(&existing).
		Where("sn.establishment_id = ?", a.es.ID()).Where("sn.year = ?", year).
		Limit(1).Scan(ctx)
	switch {
	case err == nil:
		a.season = &existing
	case isNoRows(err):
		now := a.s.d.Clock.Now()
		status := a.in.seasonStatus
		if status == "" {
			status = SeasonDraft
		}
		a.season = &store.Season{
			ID: store.NewID(), EstablishmentID: a.es.ID(), Year: year, OpensOn: opens, ClosesOn: closes,
			Status: status, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := a.insert(ctx, "season", a.season); err != nil {
			return err
		}
		a.res.SeasonCreated = true
	default:
		return fmt.Errorf("setup: load season: %w", err)
	}
	a.res.SeasonID, a.res.SeasonYear = a.season.ID, a.season.Year
	return nil
}

// ensureRatePeriods keeps the periods of an existing season, or creates the
// template periods covering every day of the season.
func (a *applier) ensureRatePeriods(ctx context.Context) error {
	var existing []store.RatePeriod
	if err := a.tx.NewSelect().Model(&existing).
		Where("rp.establishment_id = ?", a.es.ID()).Where("rp.season_id = ?", a.season.ID).
		OrderExpr("rp.sort_order, rp.id").Scan(ctx); err != nil {
		return fmt.Errorf("setup: load rate periods: %w", err)
	}
	if len(existing) > 0 {
		for _, rp := range existing {
			a.res.RatePeriodIDs = append(a.res.RatePeriodIDs, rp.ID)
		}
		return nil
	}
	ranges := periodRanges(a.season.OpensOn, a.season.ClosesOn)
	now := a.s.d.Clock.Now()
	rows := make([]store.RatePeriod, 0, len(Periods))
	for i, p := range Periods {
		rs := ranges[p.Code]
		if len(rs) == 0 {
			continue
		}
		rows = append(rows, store.RatePeriod{
			ID: store.NewID(), EstablishmentID: a.es.ID(), SeasonID: a.season.ID, Code: p.Code,
			Name: cloneText(p.Name), Color: p.Color, Ranges: rs, SortOrder: i, Version: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := a.insert(ctx, "rate periods", &rows); err != nil {
		return err
	}
	for _, rp := range rows {
		a.res.RatePeriodIDs = append(a.res.RatePeriodIDs, rp.ID)
	}
	return nil
}

// ensureComponents reuses components by code and creates the missing ones
// (with their establishment-level pool when tracked).
func (a *applier) ensureComponents(ctx context.Context) error {
	if len(a.t.Components) == 0 {
		return nil
	}
	var existing []store.Component
	if err := a.tx.NewSelect().Model(&existing).Where("cp.establishment_id = ?", a.es.ID()).Scan(ctx); err != nil {
		return fmt.Errorf("setup: load components: %w", err)
	}
	byCode := map[string]string{}
	for _, c := range existing {
		byCode[c.Code] = c.ID
	}
	now := a.s.d.Clock.Now()
	for _, spec := range a.t.Components {
		if id, ok := byCode[spec.Code]; ok {
			a.res.ComponentIDs = append(a.res.ComponentIDs, id)
			continue
		}
		c := &store.Component{
			ID: store.NewID(), EstablishmentID: a.es.ID(), Code: spec.Code, Name: cloneText(spec.Name),
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if spec.Pool != nil {
			pl := a.newPool(nil, *spec.Pool, false)
			if err := a.insert(ctx, "pool", pl); err != nil {
				return err
			}
			a.res.PoolIDs = append(a.res.PoolIDs, pl.ID)
			c.Tracked, c.PoolID = true, &pl.ID
		}
		if err := a.insert(ctx, "component "+spec.Code, c); err != nil {
			return err
		}
		a.res.ComponentIDs = append(a.res.ComponentIDs, c.ID)
	}
	return nil
}

func (a *applier) newPool(zoneID *string, spec PoolSpec, system bool) *store.Pool {
	now := a.s.d.Clock.Now()
	return &store.Pool{
		ID: store.NewID(), EstablishmentID: a.es.ID(), ZoneID: zoneID, ComponentCode: spec.ComponentCode,
		Name: cloneText(spec.Name), Capacity: spec.Capacity, CapacityOverrides: []store.CapacityOverride{},
		SystemManaged: system, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

// layoutBlock is a block of equally long rows of a generated map, listed
// from the sea outwards; zoneOf gives the zone code of each row of the block.
type layoutBlock struct {
	zoneOf   func(row int) string
	kind     string
	rows     int
	spots    int
	spacingX float64
	firstRow string
}

// createMapsAndZones creates the template maps, their zones and spots, the
// implicit pools of `zone` zones and the quantity zones with their pools.
func (a *applier) createMapsAndZones(ctx context.Context) error {
	if len(a.t.Zones) == 0 {
		return nil
	}
	taken, err := a.zoneCodes(ctx)
	if err != nil {
		return err
	}
	mapCount, err := a.tx.NewSelect().Model((*store.Map)(nil)).Where("m.establishment_id = ?", a.es.ID()).Count(ctx)
	if err != nil {
		return fmt.Errorf("setup: count maps: %w", err)
	}
	now := a.s.d.Clock.Now()
	spotsByZone := map[string][]mapping.SpotInput{}
	kindByZone := map[string]string{}
	mapOfZone := map[string]string{}
	var created []*store.Map
	var decorations []store.MapDecoration
	for i, ms := range a.t.Maps {
		m, spots, decos := a.layoutMap(ms, int(mapCount)+i)
		created = append(created, m)
		decorations = append(decorations, decos...)
		for zone, ss := range spots {
			spotsByZone[zone] = append(spotsByZone[zone], ss.spots...)
			kindByZone[zone] = ss.kind
			mapOfZone[zone] = m.ID
		}
	}
	for _, m := range created {
		if err := a.insert(ctx, "map", m); err != nil {
			return err
		}
		a.res.MapIDs = append(a.res.MapIDs, m.ID)
	}
	if len(created) > 0 {
		a.res.MapID = created[0].ID
	}
	if len(decorations) > 0 {
		if err := a.insert(ctx, "map decorations", &decorations); err != nil {
			return err
		}
	}
	var spots []store.Spot
	for i, zs := range a.t.Zones {
		if zs.AssignmentMode != "quantity" && len(spotsByZone[zs.Code]) == 0 {
			continue // e.g. "row2" with a one-row grid
		}
		z := &store.Zone{
			ID: store.NewID(), EstablishmentID: a.es.ID(), Code: uniqueCode(zs.Code, taken), Name: cloneText(zs.Name),
			AssignmentMode: zs.AssignmentMode, OnlineVisible: true, PriceCategory: zs.PriceCategory,
			AutoAssign: zs.AutoAssign, SortOrder: i, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if mid, ok := mapOfZone[zs.Code]; ok {
			z.MapID = &mid
		}
		if err := a.insert(ctx, "zone "+z.Code, z); err != nil {
			return err
		}
		a.zoneByCode[zs.Code] = z
		a.res.ZoneIDs = append(a.res.ZoneIDs, z.ID)
		for _, g := range spotsByZone[zs.Code] {
			spots = append(spots, newSpot(a.es.ID(), *z.MapID, z.ID, kindByZone[zs.Code], g, now))
		}
		switch zs.AssignmentMode {
		case "zone":
			// Implicit capacity pool of a `zone` zone (spec 7.5).
			pl := a.newPool(&z.ID, PoolSpec{ComponentCode: mapping.SystemPoolComponentCode, Name: cloneText(zs.Name), Capacity: len(spotsByZone[zs.Code])}, true)
			if err := a.insert(ctx, "zone pool", pl); err != nil {
				return err
			}
			a.res.PoolIDs = append(a.res.PoolIDs, pl.ID)
		case "quantity":
			for _, ps := range zs.Pools {
				pl := a.newPool(&z.ID, ps, false)
				if err := a.insert(ctx, "pool", pl); err != nil {
					return err
				}
				a.res.PoolIDs = append(a.res.PoolIDs, pl.ID)
			}
		}
	}
	if len(spots) > 0 {
		if err := a.insert(ctx, "spots", &spots); err != nil {
			return err
		}
	}
	a.res.SpotsCreated = len(spots)
	return nil
}

func (a *applier) zoneCodes(ctx context.Context) (map[string]bool, error) {
	var codes []string
	if err := a.tx.NewSelect().Model((*store.Zone)(nil)).Column("code").
		Where("z.establishment_id = ?", a.es.ID()).Scan(ctx, &codes); err != nil {
		return nil, fmt.Errorf("setup: load zone codes: %w", err)
	}
	taken := make(map[string]bool, len(codes))
	for _, c := range codes {
		taken[c] = true
	}
	return taken, nil
}

// uniqueCode returns code, or code_2, code_3... when taken, and marks it.
func uniqueCode(code string, taken map[string]bool) string {
	c := code
	for n := 2; taken[c]; n++ {
		c = code + "_" + strconv.Itoa(n)
	}
	taken[c] = true
	return c
}

type zoneSpots struct {
	kind  string
	spots []mapping.SpotInput
}

// layoutMap computes a map with its spots (by zone code) and decorations.
// Rows are listed from the sea outwards; the sea is at the bottom.
func (a *applier) layoutMap(ms MapSpec, sortOrder int) (*store.Map, map[string]*zoneSpots, []store.MapDecoration) {
	var blocks []layoutBlock
	if ms.Grid {
		blocks = append(blocks, layoutBlock{
			zoneOf: func(row int) string {
				switch row {
				case 0:
					return "row1"
				case 1:
					return "row2"
				default:
					return "standard"
				}
			},
			kind: "umbrella", rows: a.rows, spots: a.perRow, spacingX: layoutSpacingX, firstRow: "A",
		})
	}
	for _, fr := range ms.Fixed {
		spacing := layoutSpacingX
		if fr.Kind == "gazebo" {
			spacing = layoutGazeboX
		}
		zone := fr.Zone
		blocks = append(blocks, layoutBlock{
			zoneOf: func(int) string { return zone },
			kind:   fr.Kind, rows: fr.Rows, spots: fr.SpotsPerRow, spacingX: spacing, firstRow: fr.FirstRow,
		})
	}
	maxW, totalRows := 0.0, 0
	for _, b := range blocks {
		maxW = max(maxW, float64(b.spots-1)*b.spacingX)
		totalRows += b.rows
	}
	width := 2*layoutMargin + maxW
	height := 2*layoutMargin + layoutSeaBand + float64(max(totalRows-1, 0))*layoutSpacingY
	now := a.s.d.Clock.Now()
	name := cloneText(ms.Name)
	if ms.Grid && len(a.in.MapName) > 0 {
		name = cloneText(a.in.MapName)
	}
	m := &store.Map{
		ID: store.NewID(), EstablishmentID: a.es.ID(), Name: name, Width: int(width), Height: int(height),
		GridSize: layoutGridSize, SeaSide: ms.SeaSide, SortOrder: sortOrder, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	out := map[string]*zoneSpots{}
	k := 0 // index of the first row of the block, from the sea
	for _, b := range blocks {
		// The shared mapping generator lays out the block with the sea at
		// the bottom: row A is the nearest to the sea, rows move upwards.
		gen, err := mapping.Generate(mapping.GenerateParams{
			ZoneID: "template", Rows: b.rows, SpotsPerRow: b.spots, // zones are assigned per row below
			OriginX:  layoutMargin + (maxW-float64(b.spots-1)*b.spacingX)/2,
			OriginY:  height - layoutMargin - layoutSeaBand - float64(k)*layoutSpacingY,
			SpacingX: b.spacingX, SpacingY: layoutSpacingY,
			RowLabelStyle: mapping.RowLabelsLetters, FirstRowLabel: b.firstRow,
			Kind: b.kind, SeaSide: mapping.SeaBottom,
		})
		if err != nil {
			k += b.rows
			continue // unreachable: template sizes are validated before
		}
		for i, sp := range gen {
			zone := b.zoneOf(i / b.spots)
			zs := out[zone]
			if zs == nil {
				zs = &zoneSpots{kind: b.kind}
				out[zone] = zs
			}
			zs.spots = append(zs.spots, sp)
		}
		k += b.rows
	}
	deco := func(kind string, x, y, w, h float64, label store.I18nText) store.MapDecoration {
		return store.MapDecoration{
			ID: store.NewID(), EstablishmentID: a.es.ID(), MapID: m.ID, Kind: kind, X: x, Y: y,
			Shape: store.Shape{Type: "rect", W: w, H: h}, Label: label, CreatedAt: now, UpdatedAt: now,
		}
	}
	decos := []store.MapDecoration{
		deco("entrance", layoutMargin/2, layoutMargin/2, 40, 20, i18n("Ingresso", "Entrance", "Eingang", "Entrée")),
		deco("bar", width-layoutMargin/2, layoutMargin/2, 40, 30, i18n("Bar", "Bar", "Bar", "Bar")),
		deco("shower", layoutMargin/2, height-layoutMargin, 20, 20, i18n("Docce", "Showers", "Duschen", "Douches")),
		deco("lifeguard", width/2, height-layoutMargin/2, 30, 30, i18n("Bagnino", "Lifeguard", "Rettungsschwimmer", "Maître-nageur")),
	}
	return m, out, decos
}

func newSpot(establishmentID, mapID, zoneID, kind string, g mapping.SpotInput, now time.Time) store.Spot {
	sp := store.Spot{
		ID: store.NewID(), EstablishmentID: establishmentID, MapID: mapID, ZoneID: zoneID,
		Label: g.Label, RowLabel: g.RowLabel, Number: g.Number, Kind: kind, X: g.X, Y: g.Y,
		Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
	}
	if kind == "gazebo" {
		sp.Shape = store.Shape{Type: "rect", W: layoutGazeboSide, H: layoutGazeboSide}
		sp.Capacity = map[string]int{"sunbed": 2, "deckchair": 2}
		sp.MaxExtra = map[string]int{"sunbed": 2}
	} else {
		sp.Shape = store.Shape{Type: "circle", W: layoutUmbrellaDia, H: layoutUmbrellaDia}
		sp.Capacity = map[string]int{"umbrella": 1, "sunbed": 2}
		sp.MaxExtra = map[string]int{"sunbed": 2}
	}
	return sp
}

// ensurePacks reuses packs by code and creates the missing ones.
func (a *applier) ensurePacks(ctx context.Context) error {
	if len(a.t.Packs) == 0 {
		return nil
	}
	var existing []store.Pack
	if err := a.tx.NewSelect().Model(&existing).Where("pa.establishment_id = ?", a.es.ID()).Scan(ctx); err != nil {
		return fmt.Errorf("setup: load packs: %w", err)
	}
	for _, p := range existing {
		a.packCodes[p.Code] = p.ID
	}
	now := a.s.d.Clock.Now()
	for i, spec := range a.t.Packs {
		if id, ok := a.packCodes[spec.Code]; ok {
			a.res.PackIDs = append(a.res.PackIDs, id)
			continue
		}
		p := &store.Pack{
			ID: store.NewID(), EstablishmentID: a.es.ID(), Code: spec.Code, Name: cloneText(spec.Name),
			Description: cloneText(spec.Description), Composition: maps.Clone(spec.Composition),
			RequiresSpotKind: slices.Clone(spec.RequiresSpotKind), AllowedFormulas: slices.Clone(allFormulas),
			Channels: []string{"desk", "online"}, MaxPerBooking: max(spec.MaxPerBooking, 1),
			RequiresApproval: spec.RequiresApproval, SortOrder: i, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := a.insert(ctx, "pack "+spec.Code, p); err != nil {
			return err
		}
		a.packCodes[spec.Code] = p.ID
		a.res.PackIDs = append(a.res.PackIDs, p.ID)
	}
	return nil
}

// ensureExtras reuses extras by code and creates the missing ones.
func (a *applier) ensureExtras(ctx context.Context) error {
	if len(a.t.Extras) == 0 {
		return nil
	}
	var existing []store.Extra
	if err := a.tx.NewSelect().Model(&existing).Where("ex.establishment_id = ?", a.es.ID()).Scan(ctx); err != nil {
		return fmt.Errorf("setup: load extras: %w", err)
	}
	byCode := map[string]string{}
	for _, x := range existing {
		byCode[x.Code] = x.ID
	}
	now := a.s.d.Clock.Now()
	for i, spec := range a.t.Extras {
		if id, ok := byCode[spec.Code]; ok {
			a.res.ExtraIDs = append(a.res.ExtraIDs, id)
			continue
		}
		x := &store.Extra{
			ID: store.NewID(), EstablishmentID: a.es.ID(), Code: spec.Code, Name: cloneText(spec.Name),
			Description: cloneText(spec.Description), PricingUnit: spec.PricingUnit, MaxQuantity: max(spec.MaxQuantity, 1),
			AttachTo: spec.AttachTo, Channels: []string{"desk", "online"}, SortOrder: i, Active: true,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if spec.ComponentCode != "" {
			cc := spec.ComponentCode
			x.ComponentCode = &cc
		}
		if err := a.insert(ctx, "extra "+spec.Code, x); err != nil {
			return err
		}
		a.res.ExtraIDs = append(a.res.ExtraIDs, x.ID)
	}
	return nil
}

// ensurePolicies creates the template policies for the season, unless the
// season already has policies.
func (a *applier) ensurePolicies(ctx context.Context) error {
	if len(a.t.Policies) == 0 {
		return nil
	}
	n, err := a.tx.NewSelect().Model((*store.BookingPolicy)(nil)).
		Where("bp.establishment_id = ?", a.es.ID()).Where("bp.season_id = ?", a.season.ID).Count(ctx)
	if err != nil {
		return fmt.Errorf("setup: count policies: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, spec := range a.t.Policies {
		if spec.RequiresPack != "" && a.packCodes[spec.RequiresPack] == "" {
			continue
		}
		id, err := a.s.insertPolicy(ctx, a.tx, a.es, a.season.ID, spec)
		if err != nil {
			return err
		}
		a.res.PolicyIDs = append(a.res.PolicyIDs, id)
	}
	return nil
}

// insertPolicy validates and inserts a booking policy of a season.
func (s *Service) insertPolicy(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, seasonID string, spec PolicySpec) (string, error) {
	if err := spec.Rules.CheckConfig(); err != nil {
		return "", fmt.Errorf("setup: policy %q: %w", spec.Name, err)
	}
	scope, err := store.MarshalJSONValue(spec.Scope)
	if err != nil {
		return "", err
	}
	rules, err := store.MarshalJSONValue(spec.Rules)
	if err != nil {
		return "", err
	}
	now := s.d.Clock.Now()
	bp := &store.BookingPolicy{
		ID: store.NewID(), EstablishmentID: es.ID(), SeasonID: seasonID, Name: spec.Name, Scope: scope, Rules: rules,
		Priority: spec.Priority, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.NewInsert().Model(bp).Exec(ctx); err != nil {
		return "", fmt.Errorf("setup: insert policy: %w", err)
	}
	return bp.ID, nil
}

// record writes the audit entry and the outbox events of the application.
func (a *applier) record(ctx context.Context, actor audit.Actor) error {
	r := a.res
	diff := map[string]any{
		"template": r.Template, "season_id": r.SeasonID, "season_created": r.SeasonCreated,
		"map_ids": nonNilIDs(r.MapIDs), "zones": len(r.ZoneIDs), "spots": r.SpotsCreated, "pools": len(r.PoolIDs),
		"components": len(r.ComponentIDs), "packs": len(r.PackIDs), "extras": len(r.ExtraIDs),
		"rate_periods": len(r.RatePeriodIDs), "policies": len(r.PolicyIDs), "quick_rules": len(r.QuickRules),
		"force": a.in.Force,
	}
	if err := a.s.d.Audit.Record(ctx, a.tx, audit.Entry{
		EstablishmentID: a.es.ID(), Actor: actor, Action: "setup.template_applied",
		EntityType: "establishment", EntityID: a.es.ID(), Diff: diff,
	}); err != nil {
		return err
	}
	// One event per type for the whole application (ADR 0008), with the
	// shared payload: entity "setup", reason "setup_template".
	if len(r.MapIDs) > 0 || len(r.ZoneIDs) > 0 {
		if _, err := a.s.d.Outbox.Write(ctx, a.tx, event.ConfigMapChanged, a.es.ID(), 1, event.ConfigChange{
			Entity: event.EntitySetup, EntityID: a.es.ID(), Change: event.ChangeApplied, Reason: "setup_template",
			Template: string(r.Template), MapIDs: nonNilIDs(r.MapIDs), ZoneIDs: nonNilIDs(r.ZoneIDs),
			Counts: map[string]int{"spots": r.SpotsCreated, "pools": len(r.PoolIDs)},
		}); err != nil {
			return err
		}
	}
	if _, err := a.s.d.Outbox.Write(ctx, a.tx, event.ConfigPricingChanged, a.es.ID(), 1, event.ConfigChange{
		Entity: event.EntitySetup, EntityID: a.es.ID(), Change: event.ChangeApplied, Reason: "setup_template",
		Template: string(r.Template), SeasonID: r.SeasonID,
		Counts: map[string]int{"rules_created": len(r.QuickRules), "packs": len(r.PackIDs), "extras": len(r.ExtraIDs), "policies": len(r.PolicyIDs)},
	}); err != nil {
		return err
	}
	return nil
}

func nonNilIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func cloneText(t store.I18nText) store.I18nText { return maps.Clone(t) }

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
