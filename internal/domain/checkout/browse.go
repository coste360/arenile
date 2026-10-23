package checkout

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Reads of the public page (spec 7.8, 12.3 steps 1-3): channel online,
// only zones visible online and packs / extras of channel online; prices
// computed by the pure pricing engine on the season price list.

// loadCatalog reads the catalog the public channel may sell. Spots are
// loaded only for the ids given (selection lines).
func (s *Service) loadCatalog(ctx context.Context, db bun.IDB, es store.EstablishmentScope, spotIDs []string) (*onlineCatalog, error) {
	cat := &onlineCatalog{zones: map[string]store.Zone{}, spots: map[string]store.Spot{}, packs: map[string]store.Pack{}, extras: map[string]store.Extra{}}
	zones, err := store.InventoryZones(ctx, db, es, nil)
	if err != nil {
		return nil, fmt.Errorf("checkout: zones: %w", err)
	}
	for _, z := range zones {
		cat.zones[z.ID] = z
	}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("checkout: packs: %w", err)
	}
	for _, p := range packs {
		cat.packs[p.ID] = p
	}
	extras, err := store.ListAllExtras(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("checkout: extras: %w", err)
	}
	for _, x := range extras {
		cat.extras[x.ID] = x
	}
	if len(spotIDs) > 0 {
		spots, err := store.BookingSpotsByIDs(ctx, db, es, spotIDs)
		if err != nil {
			return nil, fmt.Errorf("checkout: spots: %w", err)
		}
		for _, sp := range spots {
			cat.spots[sp.ID] = sp
		}
	}
	return cat, nil
}

// labels names every pack, extra and zone of the catalog.
func (cat *onlineCatalog) labels() Labels {
	l := Labels{Packs: map[string]store.I18nText{}, Extras: map[string]store.I18nText{}, Zones: map[string]store.I18nText{}}
	for id, p := range cat.packs {
		l.Packs[id] = p.Name
	}
	for id, x := range cat.extras {
		l.Extras[id] = x.Name
	}
	for id, z := range cat.zones {
		l.Zones[id] = z.Name
	}
	return l
}

// onlineZones lists the zones visible online (optionally one zone), in
// display order.
func (cat *onlineCatalog) onlineZones(zoneID string) []store.Zone {
	out := []store.Zone{}
	for _, z := range cat.zones {
		if z.OnlineVisible && (zoneID == "" || z.ID == zoneID) {
			out = append(out, z)
		}
	}
	slices.SortFunc(out, func(a, b store.Zone) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out
}

// onlinePacks lists the active packs of channel online, in display order.
func (cat *onlineCatalog) onlinePacks() []store.Pack {
	out := []store.Pack{}
	for _, p := range cat.packs {
		if p.Active && sellsOnline(p.Channels) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b store.Pack) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out
}

// ---- calendar ---------------------------------------------------------------------

// Calendar returns the availability summary of a month for the online channel.
func (s *Service) Calendar(ctx context.Context, slug string, q CalendarQuery) (*Calendar, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	first, err := calendar.Parse(strings.TrimSpace(q.Month) + "-01")
	if err != nil || len(strings.TrimSpace(q.Month)) != 7 {
		return nil, configerr.Invalid("month", "Usa il formato AAAA-MM.")
	}
	last := first.AddDays(32)
	last = calendar.MustParse(fmt.Sprintf("%04d-%02d-01", last.Year(), int(last.Month()))).AddDays(-1)
	if q.Slots == "" {
		q.Slots = calendar.SlotsFull
	}
	if !q.Slots.Valid() {
		return nil, configerr.Invalid("slots", "Usa AM, PM o FULL.")
	}
	es, est := pe.Scope, pe.Establishment
	cat, err := s.loadCatalog(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	zones := cat.onlineZones(q.ZoneID)
	if q.ZoneID != "" && len(zones) == 0 {
		return nil, configerr.Invalid("zone_id", "Zona non prenotabile online.")
	}
	seasons, err := store.SeasonsOverlapping(ctx, s.d.DB, es, first, last)
	if err != nil {
		return nil, fmt.Errorf("checkout: seasons: %w", err)
	}
	closures, err := store.ClosureDaysInRange(ctx, s.d.DB, es, first, last)
	if err != nil {
		return nil, fmt.Errorf("checkout: closures: %w", err)
	}
	daily := map[calendar.Date]int{}
	capacity := 0
	if len(seasons) > 0 && len(zones) > 0 {
		res, err := s.d.Inventory.PublicAvailability(ctx, es, inventory.AvailabilityQuery{From: first, To: last, Slots: q.Slots, ZoneID: q.ZoneID})
		if err != nil {
			return nil, err
		}
		visible := map[string]bool{}
		for _, z := range zones {
			visible[z.ID] = true
		}
		for _, za := range res.Zones {
			if !visible[za.ZoneID] {
				continue
			}
			capacity += za.Capacity
			for _, d := range za.Daily {
				daily[d.Date] += d.Available
			}
		}
	}
	today := s.today(est)
	out := &Calendar{Month: first.String()[:7], Days: make([]CalendarDay, 0, 31)}
	var periods []store.RatePeriod
	if pe.Flow.ShowRatePeriods {
		for _, sn := range seasons {
			page, err := store.ListRatePeriods(ctx, s.d.DB, es, sn.ID, "", 200)
			if err != nil {
				return nil, fmt.Errorf("checkout: rate periods: %w", err)
			}
			periods = append(periods, page.Items...)
		}
		for _, rp := range periods {
			for _, r := range rp.Ranges {
				if r.To.Before(first) || r.From.After(last) {
					continue
				}
				out.RatePeriods = append(out.RatePeriods, RatePeriod{Code: rp.Code, Name: rp.Name, Color: rp.Color, StartsOn: r.From, EndsOn: r.To})
			}
		}
	}
	for d := first; !d.After(last); d = d.AddDays(1) {
		day := CalendarDay{Date: d}
		switch {
		case !inSeason(seasons, d) || d.Before(today) || (pe.Flow.MaxDaysAhead > 0 && today.DaysUntil(d) > pe.Flow.MaxDaysAhead):
			day.Status = DayUnavailable
		case closedFor(closures, d, q.Slots):
			day.Status = DayClosed
		default:
			day.Status = dayStatus(daily[d], capacity)
		}
		for _, rp := range periods {
			for _, r := range rp.Ranges {
				if day.RatePeriodCode == "" && !d.Before(r.From) && !d.After(r.To) {
					day.RatePeriodCode = rp.Code
				}
			}
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}

func inSeason(seasons []store.Season, d calendar.Date) bool {
	for _, sn := range seasons {
		if !d.Before(sn.OpensOn) && !d.After(sn.ClosesOn) {
			return true
		}
	}
	return false
}

// closedFor reports whether every requested slot of d is a closure.
func closedFor(closures []store.ClosureDay, d calendar.Date, slots calendar.Slots) bool {
	closed := map[calendar.Slot]bool{}
	for _, c := range closures {
		if c.Date.Equal(d) {
			for _, sl := range c.Slots {
				closed[sl] = true
			}
		}
	}
	for _, sl := range slots.Expand() {
		if !closed[sl] {
			return false
		}
	}
	return true
}

// ---- map --------------------------------------------------------------------------

// Map returns the public geometry of a map: active spots of zones visible
// online, decorations and the effective price category of each spot.
// Unknown maps and maps without online zones are not found.
func (s *Service) Map(ctx context.Context, slug, mapID string) (*MapView, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	es := pe.Scope
	m, err := store.GetMap(ctx, s.d.DB, es, mapID)
	if err != nil {
		return nil, err
	}
	cat, err := s.loadCatalog(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	spots, err := store.ListMapSpots(ctx, s.d.DB, es, mapID)
	if err != nil {
		return nil, fmt.Errorf("checkout: spots: %w", err)
	}
	decorations, err := store.ListMapDecorations(ctx, s.d.DB, es, mapID)
	if err != nil {
		return nil, fmt.Errorf("checkout: decorations: %w", err)
	}
	out := &MapView{Map: *m, Spots: []store.Spot{}, Decorations: decorations, PriceCategory: map[string]string{}}
	if out.Decorations == nil {
		out.Decorations = []store.MapDecoration{}
	}
	online := false
	for _, z := range cat.zones {
		online = online || (z.OnlineVisible && z.MapID != nil && *z.MapID == mapID)
	}
	if !online {
		return nil, store.ErrNotFound
	}
	for _, sp := range spots {
		z, ok := cat.zones[sp.ZoneID]
		if !sp.Active || !ok || !z.OnlineVisible {
			continue
		}
		out.Spots = append(out.Spots, sp)
		out.PriceCategory[sp.ID] = effectiveCategory(sp, z)
	}
	return out, nil
}

func effectiveCategory(sp store.Spot, z store.Zone) string {
	if sp.PriceCategoryOverride != nil && *sp.PriceCategoryOverride != "" {
		return *sp.PriceCategoryOverride
	}
	return z.PriceCategory
}

// ---- availability ------------------------------------------------------------------

// Availability returns the online availability of an interval (quota
// included, expired holds ignored) with indicative "from" prices and the
// price legend of the seat zones.
func (s *Service) Availability(ctx context.Context, slug string, q AvailabilityQuery) (*Availability, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	if q.Slots == "" {
		q.Slots = calendar.SlotsFull
	}
	if !q.Slots.Valid() {
		return nil, configerr.Invalid("slots", "Usa AM, PM o FULL.")
	}
	if q.From.IsZero() || q.To.IsZero() || q.To.Before(q.From) {
		return nil, configerr.Invalid("to", "La data finale non può precedere quella iniziale.")
	}
	if q.From.DaysUntil(q.To) >= MaxStayDays {
		return nil, configerr.Invalid("to", "Intervallo troppo lungo.")
	}
	if q.Formula == "" {
		q.Formula = DefaultFormula(q.From, q.To, q.Slots)
	} else if !policy.Formula(q.Formula).Valid() {
		return nil, configerr.Invalid("formula", "Formula sconosciuta.")
	}
	es := pe.Scope
	cat, err := s.loadCatalog(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	zones := cat.onlineZones(q.ZoneID)
	if q.ZoneID != "" && len(zones) == 0 {
		return nil, configerr.Invalid("zone_id", "Zona non prenotabile online.")
	}
	out := &Availability{Query: q, ClosedDates: []calendar.Date{}, Zones: []ZoneAvailability{}, PriceLegend: []LegendEntry{}}
	if len(zones) == 0 {
		return out, nil
	}
	res, err := s.d.Inventory.PublicAvailability(ctx, es, inventory.AvailabilityQuery{From: q.From, To: q.To, Slots: q.Slots, ZoneID: q.ZoneID})
	if err != nil {
		return nil, err
	}
	out.ClosedDates = append(out.ClosedDates, res.ClosedDates...)
	spotIDs := []string{}
	for _, za := range res.Zones {
		for _, sp := range za.Spots {
			spotIDs = append(spotIDs, sp.SpotID)
		}
	}
	spots, err := store.BookingSpotsByIDs(ctx, s.d.DB, es, spotIDs)
	if err != nil {
		return nil, fmt.Errorf("checkout: spots: %w", err)
	}
	for _, sp := range spots {
		cat.spots[sp.ID] = sp
	}
	prices, err := s.priceTable(ctx, pe, cat, q.From, q.To, q.Slots, q.Formula)
	if err != nil {
		return nil, err
	}
	byZone := map[string]inventory.ZoneAvailability{}
	for _, za := range res.Zones {
		byZone[za.ZoneID] = za
	}
	legend := map[string]money.Cents{}
	for _, z := range zones {
		za, ok := byZone[z.ID]
		if !ok {
			continue
		}
		v := ZoneAvailability{ZoneID: z.ID, Mode: z.AssignmentMode, Available: za.Available, Indicator: indicator(za.Available, za.Capacity)}
		if p, ok := prices.zone[z.ID]; ok {
			p := p
			v.FromPrice = &p
		}
		if z.AssignmentMode == "seat" {
			v.Spots = make([]SpotAvailability, 0, len(za.Spots))
			for _, sa := range za.Spots {
				sp := cat.spots[sa.SpotID]
				c := effectiveCategory(sp, z)
				v.Spots = append(v.Spots, SpotAvailability{SpotID: sa.SpotID, Available: sa.Available, PriceCategory: c})
				if p, ok := prices.category[c]; ok {
					if cur, seen := legend[c]; !seen || p < cur {
						legend[c] = p
					}
				}
			}
		}
		out.Zones = append(out.Zones, v)
	}
	if pe.Establishment.Theme.ShowPricesOnMap == nil || *pe.Establishment.Theme.ShowPricesOnMap {
		out.PriceLegend = legendTones(legend)
	}
	return out, nil
}

// ---- prices -------------------------------------------------------------------------

// priceTable holds the cheapest price of one pack on a stay per zone and
// per price category, and the unit price of each extra (engine totals,
// VAT included).
type priceTable struct {
	zone     map[string]money.Cents
	category map[string]money.Cents
	pack     map[string]money.Cents
	// packAt is the price of a pack in a zone ("pack|zone") and on a price
	// category of a zone ("pack|zone|category"): the catalog of a chosen
	// spot or zone shows its own "from" price, not the beach-wide minimum.
	packAt    map[string]money.Cents
	extraUnit map[string]money.Cents
}

func emptyPrices() *priceTable {
	return &priceTable{
		zone: map[string]money.Cents{}, category: map[string]money.Cents{}, pack: map[string]money.Cents{},
		packAt: map[string]money.Cents{}, extraUnit: map[string]money.Cents{},
	}
}

// packFrom is the "from" price of pack p for the catalog query: on the
// spot's category when a spot is chosen, in the zone when a zone is, else
// anywhere.
func (t *priceTable) packFrom(packID, zoneID, category string) (money.Cents, bool) {
	switch {
	case zoneID != "" && category != "":
		if v, ok := t.packAt[packID+"|"+zoneID+"|"+category]; ok {
			return v, true
		}
		fallthrough
	case zoneID != "":
		v, ok := t.packAt[packID+"|"+zoneID]
		return v, ok
	}
	v, ok := t.pack[packID]
	return v, ok
}

func setMin(m map[string]money.Cents, k string, v money.Cents) {
	if cur, ok := m[k]; !ok || v < cur {
		m[k] = v
	}
}

// priceTable prices every online pack on every online zone (one spot per
// price category for seat zones) with the pure engine on the price list of
// the season of from. Missing prices are simply absent ("no price" is not
// an error on a browse page).
func (s *Service) priceTable(ctx context.Context, pe *online.PublicEstablishment, cat *onlineCatalog, from, to calendar.Date, slots calendar.Slots, formula string) (*priceTable, error) {
	es := pe.Scope
	key := strings.Join([]string{es.ID(), from.String(), to.String(), string(slots), formula}, "|")
	if t, ok := s.prices.get(key); ok {
		return t, nil
	}
	out := emptyPrices()
	season, err := store.PricingSeasonForDate(ctx, s.d.DB, es, from)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checkout: season: %w", err)
	}
	zones := cat.onlineZones("")
	zoneIDs := make([]string, 0, len(zones))
	for _, z := range zones {
		zoneIDs = append(zoneIDs, z.ID)
	}
	spots, err := store.ListZoneSpots(ctx, s.d.DB, es, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("checkout: spots: %w", err)
	}
	// One representative spot per (zone, category) of the seat zones.
	type zc struct{ zone, category string }
	reps := map[zc]string{}
	var repIDs []string
	for _, sp := range spots {
		z := cat.zones[sp.ZoneID]
		if z.AssignmentMode != "seat" {
			continue
		}
		k := zc{z.ID, effectiveCategory(sp, z)}
		if _, ok := reps[k]; !ok {
			reps[k] = sp.ID
			repIDs = append(repIDs, sp.ID)
		}
	}
	pl, err := pricingconfig.LoadPriceList(ctx, s.d.DB, es, season.ID, repIDs)
	if err != nil {
		return nil, err
	}
	today := calendar.In(s.now(), pl.Location)
	quote := func(lines []pricing.LineInput, extras []pricing.ExtraInput) (pricing.PriceBreakdown, bool) {
		bd, err := pricing.Quote(pricing.QuoteInput{
			Channel: pricing.Channel(store.ChannelOnline), StartsOn: from, EndsOn: to, Slots: slots, Formula: pricing.Formula(formula),
			Today: today, Lines: lines, Extras: extras,
		}, pl.Catalog, pl.Rules)
		return bd, err == nil
	}
	var sample *pricing.LineInput
	for _, p := range cat.onlinePacks() {
		if len(p.AllowedFormulas) > 0 && !slices.Contains(p.AllowedFormulas, formula) {
			continue
		}
		for _, z := range zones {
			if len(p.AllowedZoneIDs) > 0 && !slices.Contains(p.AllowedZoneIDs, z.ID) {
				continue
			}
			var lines []pricing.LineInput
			var cats []string
			if z.AssignmentMode == "seat" {
				for k, id := range reps {
					if k.zone == z.ID {
						lines = append(lines, pricing.LineInput{PackID: p.ID, ZoneID: z.ID, SpotID: id})
						cats = append(cats, k.category)
					}
				}
			} else {
				lines = append(lines, pricing.LineInput{PackID: p.ID, ZoneID: z.ID})
				cats = append(cats, z.PriceCategory)
			}
			for i, l := range lines {
				bd, ok := quote([]pricing.LineInput{l}, nil)
				if !ok {
					continue
				}
				setMin(out.zone, z.ID, bd.Total)
				setMin(out.category, cats[i], bd.Total)
				setMin(out.pack, p.ID, bd.Total)
				setMin(out.packAt, p.ID+"|"+z.ID, bd.Total)
				setMin(out.packAt, p.ID+"|"+z.ID+"|"+cats[i], bd.Total)
				if sample == nil {
					l := l
					sample = &l
				}
			}
		}
	}
	if sample != nil {
		base, ok := quote([]pricing.LineInput{*sample}, nil)
		for _, x := range cat.extras {
			if !ok || !x.Active || !sellsOnline(x.Channels) {
				continue
			}
			in := pricing.ExtraInput{ExtraID: x.ID, Quantity: 1}
			var bd pricing.PriceBreakdown
			var priced bool
			if x.AttachTo == "line" {
				l := *sample
				l.Extras = []pricing.ExtraInput{in}
				bd, priced = quote([]pricing.LineInput{l}, nil)
			} else {
				bd, priced = quote([]pricing.LineInput{*sample}, []pricing.ExtraInput{in})
			}
			if priced {
				out.extraUnit[x.ID] = max(bd.Total-base.Total, 0)
			}
		}
	}
	s.prices.put(key, out)
	return out, nil
}

// ---- catalog --------------------------------------------------------------------------

// Catalog returns the online packs and extras compatible with a selection
// (zone, spot, formula), with "from" prices when the stay is given.
func (s *Service) Catalog(ctx context.Context, slug string, q CatalogQuery) (*Catalog, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	es := pe.Scope
	var spotIDs []string
	if q.SpotID != "" {
		spotIDs = []string{q.SpotID}
	}
	cat, err := s.loadCatalog(ctx, s.d.DB, es, spotIDs)
	if err != nil {
		return nil, err
	}
	zoneID := q.ZoneID
	var spot *store.Spot
	if q.SpotID != "" {
		sp, ok := cat.spots[q.SpotID]
		if !ok || !sp.Active {
			return nil, configerr.Invalid("spot_id", "Postazione non disponibile online.")
		}
		spot, zoneID = &sp, sp.ZoneID
	}
	if zoneID != "" {
		if z, ok := cat.zones[zoneID]; !ok || !z.OnlineVisible {
			return nil, configerr.Invalid("zone_id", "Zona non prenotabile online.")
		}
	}
	if q.Formula != "" && !policy.Formula(q.Formula).Valid() {
		return nil, configerr.Invalid("formula", "Formula sconosciuta.")
	}
	var prices *priceTable
	if q.From != nil && q.To != nil && !q.To.Before(*q.From) && q.From.DaysUntil(*q.To) < MaxStayDays {
		slots := q.Slots
		if slots == "" {
			slots = calendar.SlotsFull
		}
		formula := q.Formula
		if formula == "" {
			formula = DefaultFormula(*q.From, *q.To, slots)
		}
		if prices, err = s.priceTable(ctx, pe, cat, *q.From, *q.To, slots, formula); err != nil {
			return nil, err
		}
	}
	components, err := store.ListAllComponents(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("checkout: components: %w", err)
	}
	compByCode := map[string]store.Component{}
	for _, c := range components {
		compByCode[c.Code] = c
	}
	out := &Catalog{Packs: []PackView{}, Extras: []ExtraView{}}
	packIDs := map[string]bool{}
	for _, p := range cat.onlinePacks() {
		switch {
		case q.Formula != "" && len(p.AllowedFormulas) > 0 && !slices.Contains(p.AllowedFormulas, q.Formula):
			continue
		case zoneID != "" && len(p.AllowedZoneIDs) > 0 && !slices.Contains(p.AllowedZoneIDs, zoneID):
			continue
		case spot != nil && len(p.RequiresSpotKind) > 0 && !slices.Contains(p.RequiresSpotKind, spot.Kind):
			continue
		}
		v := PackView{Pack: p, Components: []ComponentView{}}
		codes := make([]string, 0, len(p.Composition))
		for code := range p.Composition {
			codes = append(codes, code)
		}
		slices.Sort(codes)
		for _, code := range codes {
			cv := ComponentView{Code: code, Quantity: p.Composition[code], Name: store.I18nText{"it": code}}
			if c, ok := compByCode[code]; ok {
				cv.Name = c.Name
			}
			v.Components = append(v.Components, cv)
		}
		if prices != nil {
			category := ""
			if spot != nil {
				category = effectiveCategory(*spot, cat.zones[zoneID])
			}
			if pr, ok := prices.packFrom(p.ID, zoneID, category); ok {
				v.FromPrice = &pr
			}
		}
		packIDs[p.ID] = true
		out.Packs = append(out.Packs, v)
	}
	extras := make([]store.Extra, 0, len(cat.extras))
	for _, x := range cat.extras {
		if !x.Active || !sellsOnline(x.Channels) {
			continue
		}
		if len(x.AllowedPackIDs) > 0 && !slices.ContainsFunc(x.AllowedPackIDs, func(id string) bool { return packIDs[id] }) {
			continue
		}
		extras = append(extras, x)
	}
	slices.SortFunc(extras, func(a, b store.Extra) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Code, b.Code)
	})
	for _, x := range extras {
		v := ExtraView{Extra: x}
		if prices != nil {
			if pr, ok := prices.extraUnit[x.ID]; ok {
				pr := pr
				v.UnitPrice = &pr
			}
		}
		out.Extras = append(out.Extras, v)
	}
	return out, nil
}

// ---- quote ------------------------------------------------------------------------------

// Quote prices a selection with the online policy (no writes): engine
// breakdown, policy violations, availability for the online channel (quota
// included), payment plan. An invalid promo code is not an error: the
// quote is computed without it and Promo tells why.
func (s *Service) Quote(ctx context.Context, slug string, in Selection) (*Quote, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	return s.quote(ctx, s.d.DB, pe, in)
}

func selectionSpotIDs(sel Selection) []string {
	var out []string
	for _, l := range sel.Lines {
		if l.SpotID != "" {
			out = append(out, l.SpotID)
		}
	}
	return out
}

func (s *Service) quote(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, in Selection) (*Quote, error) {
	es, est := pe.Scope, pe.Establishment
	if in.Formula == "" && !in.StartsOn.IsZero() && !in.EndsOn.IsZero() {
		in.Formula = DefaultFormula(in.StartsOn, in.EndsOn, in.Slots)
	}
	cat, err := s.loadCatalog(ctx, db, es, selectionSpotIDs(in))
	if err != nil {
		return nil, err
	}
	if err := validateSelection(in, cat, pe.Flow, s.today(est)); err != nil {
		return nil, err
	}
	out := &Quote{Currency: est.Currency, Conflicts: []availability.Conflict{}, Violations: []policy.Violation{}, Items: []QuoteItem{}}
	promo := strings.TrimSpace(in.PromoCode)
	if promo != "" && !pe.Flow.PromoCodesEnabled {
		out.Promo = &PromoStatus{Code: promo, Status: PromoNotApplicable}
		promo = ""
	}
	priced := make([]booking.PricedLine, len(in.Lines))
	for i, l := range in.Lines {
		zoneID := l.ZoneID
		if sp, ok := cat.spots[l.SpotID]; ok {
			zoneID = sp.ZoneID
		}
		priced[i] = booking.PricedLine{PackID: l.PackID, ZoneID: zoneID, SpotID: l.SpotID, Quantity: max(l.Quantity, 1), Extras: l.Extras}
	}
	engineLines, owner := booking.PricingLines(priced)
	sim := pricingconfig.SimulateInput{
		Channel: pricing.Channel(store.ChannelOnline), StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots,
		Formula: pricing.Formula(in.Formula), Lines: engineLines, PromoCode: promo,
	}
	for _, x := range in.Extras {
		sim.Extras = append(sim.Extras, pricing.ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity})
	}
	res, err := s.d.Pricer.Quote(ctx, db, es, sim)
	if promo != "" {
		var ve *configerr.ValidationError
		if errors.As(err, &ve) && len(ve.Fields) > 0 && ve.Fields[0].Field == "promo_code" {
			status := PromoInvalid
			if strings.Contains(ve.Fields[0].Message, "non vale") {
				status = PromoNotApplicable
			}
			out.Promo = &PromoStatus{Code: promo, Status: status}
			sim.PromoCode = ""
			res, err = s.d.Pricer.Quote(ctx, db, es, sim)
		} else if err == nil {
			out.Promo = &PromoStatus{Code: promo, Status: PromoApplied}
		}
	}
	if err != nil {
		return nil, err
	}
	approval := res.ApprovalRequired
	for _, l := range in.Lines {
		approval = approval || cat.packs[l.PackID].RequiresApproval
	}
	out.Total, out.DepositDue = res.Breakdown.Total, res.DepositDue
	out.Violations = append(out.Violations, res.Violations...)
	out.Items = quoteItems(res.Breakdown, owner, in.Lines, in.Extras, cat.packs, cat.extras)
	out.Plan, out.DueNow, out.BalanceLater = paymentPlan(planInput{
		Rules: res.Policy.Rules, ApprovalRequired: approval, ProviderOnline: pe.Online, Total: res.Breakdown.Total, Deposit: res.DepositDue,
	})
	conflicts, err := s.conflicts(ctx, es, in, cat)
	if err != nil {
		return nil, err
	}
	out.Conflicts = append(out.Conflicts, conflicts...)
	out.Available = len(out.Conflicts) == 0
	return out, nil
}

// conflicts checks a selection against the online availability (spots
// free for the whole stay, zone and quantity counts with the quota).
func (s *Service) conflicts(ctx context.Context, es store.EstablishmentScope, in Selection, cat *onlineCatalog) ([]availability.Conflict, error) {
	res, err := s.d.Inventory.PublicAvailability(ctx, es, inventory.AvailabilityQuery{From: in.StartsOn, To: in.EndsOn, Slots: in.Slots})
	if err != nil {
		return nil, err
	}
	byZone := map[string]inventory.ZoneAvailability{}
	for _, za := range res.Zones {
		byZone[za.ZoneID] = za
	}
	first := in.Slots.Expand()[0]
	need := map[string]int{}
	out := []availability.Conflict{}
	for _, l := range in.Lines {
		if l.SpotID != "" {
			sp := cat.spots[l.SpotID]
			za := byZone[sp.ZoneID]
			if za.Mode == "seat" || za.Mode == "" {
				found := false
				for _, sa := range za.Spots {
					if sa.SpotID != l.SpotID {
						continue
					}
					found = true
					if !sa.Available {
						out = append(out, sa.Conflicts...)
						if len(sa.Conflicts) == 0 {
							out = append(out, availability.Conflict{SpotID: l.SpotID, ZoneID: sp.ZoneID, Date: in.StartsOn, Slot: first, Reason: availability.ReasonSpotUnavailable})
						}
					}
				}
				if !found {
					out = append(out, availability.Conflict{SpotID: l.SpotID, ZoneID: sp.ZoneID, Date: in.StartsOn, Slot: first, Reason: availability.ReasonSpotUnavailable})
				}
				continue
			}
		}
		need[l.ZoneID] += max(l.Quantity, 1)
	}
	for zoneID, n := range need {
		if za := byZone[zoneID]; za.Available < n {
			out = append(out, availability.Conflict{ZoneID: zoneID, Date: in.StartsOn, Slot: first, Reason: availability.ReasonPoolExhausted})
		}
	}
	return out, nil
}

// quoteOf rebuilds the public quote of a stored booking from its price
// snapshot (current price after a confirmed modification).
func (s *Service) quoteOf(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, d *booking.Detail) (*Quote, error) {
	raw := currentPrice(&d.Booking)
	cat, err := s.loadCatalog(ctx, db, pe.Scope, nil)
	if err != nil {
		return nil, err
	}
	return s.quoteWith(ctx, db, pe, d, cat, raw)
}

// quoteWith is quoteOf with the catalog already loaded.
func (s *Service) quoteWith(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, d *booking.Detail, cat *onlineCatalog, raw []byte) (*Quote, error) {
	b := &d.Booking
	bd, err := parseBreakdown(raw)
	if err != nil {
		return nil, err
	}
	lines := make([]booking.LineInput, 0, len(d.Lines))
	priced := make([]booking.PricedLine, 0, len(d.Lines))
	for _, l := range d.Lines {
		li := booking.LineInput{PackID: l.PackID, ZoneID: l.ZoneID, Quantity: l.Quantity}
		lines = append(lines, li)
		priced = append(priced, booking.PricedLine{PackID: l.PackID, ZoneID: l.ZoneID, Quantity: l.Quantity})
	}
	var extras []booking.ExtraInput
	for _, x := range d.Extras {
		if x.BookingLineID == nil {
			extras = append(extras, booking.ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity})
		}
	}
	_, owner := booking.PricingLines(priced)
	snap, _ := policy.ParseSnapshot(b.PolicySnapshot)
	approval := snap.Rules.RequiresApproval()
	for _, l := range d.Lines {
		approval = approval || cat.packs[l.PackID].RequiresApproval
	}
	out := &Quote{
		Available: true, Conflicts: []availability.Conflict{}, Currency: pe.Establishment.Currency, Total: b.TotalAmount,
		DepositDue: b.DepositDue, Violations: []policy.Violation{}, Items: quoteItems(bd, owner, lines, extras, cat.packs, cat.extras),
	}
	out.Plan, out.DueNow, out.BalanceLater = paymentPlan(planInput{
		Rules: snap.Rules, ApprovalRequired: approval, ProviderOnline: pe.Online, Total: b.TotalAmount, Deposit: b.DepositDue, Paid: b.PaidAmount,
	})
	if b.PromoCodeID != nil {
		if pc, err := store.GetPromoCode(ctx, db, pe.Scope, *b.PromoCodeID); err == nil {
			out.Promo = &PromoStatus{Code: pc.Code, Status: PromoApplied}
		}
	}
	return out, nil
}

// holdDeadline is the latest hold of a session while paying: the session
// TTL plus the payment window (spec 10.2), so retries cannot extend a hold
// forever.
func (s *Service) holdDeadline(cs *store.CheckoutSession) time.Time {
	return cs.CreatedAt.Add(s.d.HoldTTL + MaxPaymentHold)
}

// currentPrice is the breakdown pricing a booking: price_current after a
// confirmed modification, otherwise the snapshot (ADR 0011 §2).
func currentPrice(b *store.Booking) []byte {
	if len(b.PriceCurrent) > 0 {
		return b.PriceCurrent
	}
	return b.PriceSnapshot
}

// summarize loads the catalog once and returns the quote and labels of a booking.
func (s *Service) summarize(ctx context.Context, db bun.IDB, pe *online.PublicEstablishment, d *booking.Detail) (*Quote, Labels, error) {
	cat, err := s.loadCatalog(ctx, db, pe.Scope, nil)
	if err != nil {
		return nil, Labels{}, err
	}
	q, err := s.quoteWith(ctx, db, pe, d, cat, currentPrice(&d.Booking))
	if err != nil {
		return nil, Labels{}, err
	}
	return q, cat.labels(), nil
}
