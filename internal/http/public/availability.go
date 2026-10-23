package public

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Public reads of the booking flow (spec 12.3 steps 1-3). Owner: checkout
// builder. Each handler maps the query to the checkout.Service method of
// the same name and the result to the api.Public* schema:
//
//	GetPublicCalendar     -> Checkout.Calendar     -> api.PublicCalendar (Cache-Control public, max-age=30)
//	GetPublicMap          -> Checkout.Map          -> api.PublicMapDocument (content ETag, If-None-Match -> 304)
//	GetPublicAvailability -> Checkout.Availability -> api.PublicAvailability (Cache-Control no-cache)
//	GetPublicCatalog      -> Checkout.Catalog      -> api.PublicCatalog
//	QuotePublicBooking    -> Checkout.Quote        -> api.PublicQuote

// GetPublicCalendar implements GET /api/public/v1/{slug}/calendar.
func (h *Handlers) GetPublicCalendar(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicCalendarParams) {
	if !h.ready(w, r) {
		return
	}
	q := checkout.CalendarQuery{Month: params.Month}
	if params.Slots != nil {
		q.Slots = slotsOf(*params.Slots)
	}
	if params.ZoneId != nil {
		q.ZoneID = *params.ZoneId
	}
	c, err := h.Checkout.Calendar(r.Context(), slug, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicCalendar{Month: c.Month, Days: make([]api.PublicCalendarDay, len(c.Days))}
	for i, d := range c.Days {
		out.Days[i] = api.PublicCalendarDay{Date: d.Date, Status: api.PublicCalendarDayStatus(d.Status), RatePeriodCode: optStr(d.RatePeriodCode)}
	}
	if c.RatePeriods != nil {
		rps := make([]api.PublicRatePeriod, len(c.RatePeriods))
		for i, rp := range c.RatePeriods {
			rps[i] = api.PublicRatePeriod{Code: rp.Code, Name: text(rp.Name), Color: optStr(rp.Color), StartsOn: rp.StartsOn, EndsOn: rp.EndsOn}
		}
		out.RatePeriods = &rps
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, out)
}

// GetPublicMap implements GET /api/public/v1/{slug}/maps/{mapId}.
func (h *Handlers) GetPublicMap(w http.ResponseWriter, r *http.Request, slug api.Slug, mapID api.MapID, params api.GetPublicMapParams) {
	if !h.ready(w, r) {
		return
	}
	m, err := h.Checkout.Map(r.Context(), slug, mapID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	body, err := json.Marshal(mapToAPI(m))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	// Content ETag: the map version alone misses zone visibility and spot
	// activation changes, which do not bump it.
	sum := sha256.Sum256(body)
	tag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "public, max-age=60")
	if params.IfNoneMatch != nil && etagMatches(*params.IfNoneMatch, tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(body, '\n'))
}

// GetPublicAvailability implements GET /api/public/v1/{slug}/availability.
func (h *Handlers) GetPublicAvailability(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicAvailabilityParams) {
	if !h.ready(w, r) {
		return
	}
	q := checkout.AvailabilityQuery{From: params.From, To: params.To, Slots: slotsOf(params.Slots)}
	if params.Formula != nil {
		q.Formula = string(*params.Formula)
	}
	if params.ZoneId != nil {
		q.ZoneID = *params.ZoneId
	}
	a, err := h.Checkout.Availability(r.Context(), slug, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicAvailability{
		From: a.Query.From, To: a.Query.To, Slots: api.Slots(a.Query.Slots), Formula: ptr(api.Formula(a.Query.Formula)),
		Zones: make([]api.PublicZoneAvailability, len(a.Zones)), PriceLegend: make([]api.PriceLegendEntry, len(a.PriceLegend)),
	}
	closed := append([]api.Date{}, a.ClosedDates...)
	out.ClosedDates = &closed
	for i, z := range a.Zones {
		za := api.PublicZoneAvailability{
			ZoneId: z.ZoneID, AssignmentMode: api.AssignmentMode(z.Mode), Available: z.Available,
			Indicator: api.AvailabilityIndicator(z.Indicator), PoolId: optStr(z.PoolID),
		}
		if z.FromPrice != nil {
			za.FromPrice = ptr(int64(*z.FromPrice))
		}
		if z.Mode == "seat" {
			spots := make([]api.PublicSpotAvailability, len(z.Spots))
			for j, s := range z.Spots {
				spots[j] = api.PublicSpotAvailability{SpotId: s.SpotID, Available: s.Available, PriceCategory: s.PriceCategory}
			}
			za.Spots = &spots
		}
		out.Zones[i] = za
	}
	for i, e := range a.PriceLegend {
		le := api.PriceLegendEntry{PriceCategory: e.PriceCategory, FromPrice: int64(e.FromPrice), Tone: e.Tone}
		if len(e.Label) > 0 {
			l := api.I18nText(e.Label)
			le.Label = &l
		}
		out.PriceLegend[i] = le
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, out)
}

// GetPublicCatalog implements GET /api/public/v1/{slug}/catalog.
func (h *Handlers) GetPublicCatalog(w http.ResponseWriter, r *http.Request, slug api.Slug, params api.GetPublicCatalogParams) {
	if !h.ready(w, r) {
		return
	}
	q := checkout.CatalogQuery{From: params.From, To: params.To}
	if params.Slots != nil {
		q.Slots = slotsOf(*params.Slots)
	}
	if params.Formula != nil {
		q.Formula = string(*params.Formula)
	}
	if params.ZoneId != nil {
		q.ZoneID = *params.ZoneId
	}
	if params.SpotId != nil {
		q.SpotID = *params.SpotId
	}
	c, err := h.Checkout.Catalog(r.Context(), slug, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicCatalog{Packs: make([]api.PublicPack, len(c.Packs)), Extras: make([]api.PublicExtra, len(c.Extras))}
	for i, p := range c.Packs {
		pp := api.PublicPack{
			Id: p.Pack.ID, Code: p.Pack.Code, Name: text(p.Pack.Name), Description: text(p.Pack.Description), Image: p.Pack.Image,
			AllowedFormulas: formulas(p.Pack.AllowedFormulas), MaxPerBooking: p.Pack.MaxPerBooking, RequiresApproval: p.Pack.RequiresApproval,
			Components: make([]api.PublicPackComponent, len(p.Components)),
		}
		if p.FromPrice != nil {
			pp.FromPrice = ptr(int64(*p.FromPrice))
		}
		for j, c := range p.Components {
			pp.Components[j] = api.PublicPackComponent{Code: c.Code, Name: text(c.Name), Quantity: c.Quantity, Icon: optStr(c.Icon)}
		}
		out.Packs[i] = pp
	}
	for i, x := range c.Extras {
		e := api.PublicExtra{
			Id: x.Extra.ID, Code: x.Extra.Code, Name: text(x.Extra.Name), Description: text(x.Extra.Description),
			AttachTo: api.ExtraAttachTo(x.Extra.AttachTo), MaxQuantity: x.Extra.MaxQuantity, PricingUnit: api.PricingUnit(x.Extra.PricingUnit),
		}
		if len(x.Extra.AllowedPackIDs) > 0 {
			ids := append([]string{}, x.Extra.AllowedPackIDs...)
			e.AllowedPackIds = &ids
		}
		if x.UnitPrice != nil {
			e.UnitPrice = ptr(int64(*x.UnitPrice))
		}
		out.Extras[i] = e
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, out)
}

// QuotePublicBooking implements POST /api/public/v1/{slug}/quote.
func (h *Handlers) QuotePublicBooking(w http.ResponseWriter, r *http.Request, slug api.Slug) {
	if !h.ready(w, r) {
		return
	}
	var req api.PublicSelection
	if !decode(w, r, &req) {
		return
	}
	q, err := h.Checkout.Quote(r.Context(), slug, selectionFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, quoteToAPI(q))
}
