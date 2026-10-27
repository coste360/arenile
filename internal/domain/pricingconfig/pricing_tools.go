package pricingconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// The panel tools (spec 8.7) are configuration features: they require
// auth.PermEstablishmentConfigure (owner, manager), like editing prices.

// SimulateInput is a quote request with a selectable channel.
type SimulateInput struct {
	// SeasonID empty means the season containing StartsOn.
	SeasonID string
	Channel  pricing.Channel
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    calendar.Slots
	Formula  pricing.Formula
	// Today nil means today in the establishment timezone.
	Today        *calendar.Date
	Lines        []pricing.LineInput
	Extras       []pricing.ExtraInput
	CustomerTags []string
	AgreementID  string
	PromoCode    string
}

// SimulateResult is the engine breakdown with the effective booking policy.
type SimulateResult struct {
	SeasonID  string
	Breakdown pricing.PriceBreakdown
	// Policy is the resolution (matched policy over the default).
	Policy           policy.Resolution
	Violations       []policy.Violation
	ApprovalRequired bool
	// DepositDue is policy.Deposit(total, effective rules).
	DepositDue money.Cents
}

func validateSimulate(in *SimulateInput) error {
	v := &configerr.ValidationError{}
	if !in.Channel.Valid() {
		v.Add("channel", "Canale sconosciuto.")
	}
	if !in.Formula.Valid() {
		v.Add("formula", "Formula sconosciuta.")
	}
	if !in.Slots.Valid() {
		v.Add("slots", "Usa AM, PM o FULL.")
	}
	if in.StartsOn.IsZero() {
		v.Add("starts_on", "Campo obbligatorio.")
	}
	if in.EndsOn.IsZero() {
		v.Add("ends_on", "Campo obbligatorio.")
	} else if !in.StartsOn.IsZero() && in.EndsOn.Before(in.StartsOn) {
		v.Add("ends_on", "Deve essere successiva o uguale a starts_on.")
	}
	if len(in.Lines) == 0 {
		v.Add("lines", "Almeno una riga.")
	}
	for i, l := range in.Lines {
		if l.PackID == "" {
			v.Add(fmt.Sprintf("lines[%d].pack_id", i), "Campo obbligatorio.")
		}
		if l.ZoneID == "" && l.SpotID == "" {
			v.Add(fmt.Sprintf("lines[%d].zone_id", i), "Indica la zona o la postazione.")
		}
	}
	return v.OrNil()
}

// Simulate prices a request with the season's price list, as the booking
// flow will (spec 8.7 simulator): it returns the full breakdown with the
// applied steps, the effective booking policy with its violations, and the
// deposit due. A missing base price is pricing.ErrNoPrice (422 no_price).
func (s *Service) Simulate(ctx context.Context, p *auth.Principal, establishmentID string, in SimulateInput) (*SimulateResult, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	return s.Quote(ctx, s.d.DB, es, in)
}

// Quote is Simulate without the permission check, reading through db (the
// caller's transaction): the booking service prices, validates the policy
// and computes the deposit with it inside the transaction that allocates the
// resources (M3), after authorizing the principal itself. Errors are those of
// Simulate (configerr validation, pricing.ErrNoPrice).
func (s *Service) Quote(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in SimulateInput) (*SimulateResult, error) {
	if err := validateSimulate(&in); err != nil {
		return nil, err
	}
	seasonID := strings.TrimSpace(in.SeasonID)
	if seasonID == "" {
		sn, err := store.PricingSeasonForDate(ctx, db, es, in.StartsOn)
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("starts_on", "Nessuna stagione contiene la data di inizio.")
		}
		if err != nil {
			return nil, fmt.Errorf("pricingconfig: %w", err)
		}
		seasonID = sn.ID
	} else {
		sn, err := seasonOf(ctx, db, es, seasonID)
		if err != nil {
			return nil, err
		}
		// The season's rules are meaningless outside its dates (a stay of
		// another year would price as "no rule" or on a wrong period).
		if in.StartsOn.Before(sn.OpensOn) || in.EndsOn.After(sn.ClosesOn) {
			return nil, configerr.Invalid("starts_on", fmt.Sprintf("Le date devono rientrare nella stagione (%s – %s).", sn.OpensOn, sn.ClosesOn))
		}
	}
	spotIDs := make([]string, 0, len(in.Lines))
	for _, l := range in.Lines {
		spotIDs = append(spotIDs, l.SpotID)
		for _, sg := range l.Segments { // split stays (ADR 0028)
			spotIDs = append(spotIDs, sg.SpotID)
		}
	}
	pl, err := LoadPriceList(ctx, db, es, seasonID, spotIDs)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now().In(pl.Location)
	today := calendar.In(now, pl.Location)
	if in.Today != nil && !in.Today.IsZero() {
		// Evaluate as if it were the same time of day on the chosen date.
		d := *in.Today
		now = time.Date(d.Year(), d.Month(), d.Day(), now.Hour(), now.Minute(), now.Second(), 0, pl.Location)
		today = d
	}
	q := pricing.QuoteInput{
		Channel: in.Channel, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: in.Formula,
		Today: today, Lines: in.Lines, Extras: in.Extras, Customer: pricing.CustomerInput{Tags: cleanIDs(in.CustomerTags)},
	}
	if id := strings.TrimSpace(in.AgreementID); id != "" {
		row, err := store.GetAgreement(ctx, db, es, id)
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("agreement_id", "Convenzione inesistente.")
		}
		if err != nil {
			return nil, fmt.Errorf("pricingconfig: %w", err)
		}
		a, err := agreementFromRow(row)
		if err != nil {
			return nil, err
		}
		if err := a.ApplicableTo(today, seasonID, in.Channel); err != nil {
			return nil, err
		}
		ea := a.Engine()
		q.Agreement = &ea
	}
	if code := strings.TrimSpace(in.PromoCode); code != "" {
		row, err := store.GetPromoCodeByCode(ctx, db, es, code)
		if errors.Is(err, store.ErrNotFound) {
			return nil, configerr.Invalid("promo_code", "Codice promozionale inesistente.")
		}
		if err != nil {
			return nil, fmt.Errorf("pricingconfig: %w", err)
		}
		pc, err := promoFromRow(row)
		if err != nil {
			return nil, err
		}
		if pc.Exhausted() {
			return nil, configerr.Invalid("promo_code", "Codice promozionale esaurito.")
		}
		epc := pc.Engine()
		q.PromoCode = &epc
	}
	bd, err := pricing.Quote(q, pl.Catalog, pl.Rules)
	switch {
	case errors.Is(err, pricing.ErrPromoNotApplicable):
		return nil, configerr.Invalid("promo_code", "Il codice promozionale non vale per questa richiesta.")
	case errors.Is(err, pricing.ErrNoPrice):
		return nil, fmt.Errorf("pricingconfig: simulate: %w", err)
	case err != nil:
		return nil, fromEngine(err, "")
	}

	target := policyTarget(pl, in)
	res := policy.Resolve(pl.Policies, pl.Settings.DefaultPolicy, target)
	val := policy.Validate(policy.Request{
		StartsOn: in.StartsOn, EndsOn: in.EndsOn, Slots: in.Slots, Formula: policy.Formula(in.Formula), SpotCount: len(in.Lines),
	}, res.Rules, now, pl.Location)
	out := &SimulateResult{
		SeasonID: seasonID, Breakdown: bd, Policy: res, Violations: val.Violations,
		ApprovalRequired: val.ApprovalRequired || res.Rules.RequiresApproval(),
		DepositDue:       policy.Deposit(bd.Total, res.Rules),
	}
	if out.Violations == nil {
		out.Violations = []policy.Violation{}
	}
	return out, nil
}

// policyTarget lists the channel, the zones and packs (IDs and codes) and the
// rate periods touched by the request.
func policyTarget(pl *PriceList, in SimulateInput) policy.Target {
	t := policy.Target{Channel: policy.Channel(in.Channel)}
	add := func(list *[]string, vs ...string) {
		for _, v := range vs {
			if v == "" {
				continue
			}
			found := false
			for _, x := range *list {
				if x == v {
					found = true
				}
			}
			if !found {
				*list = append(*list, v)
			}
		}
	}
	spotZone := map[string]string{}
	for _, sp := range pl.Catalog.Spots {
		spotZone[sp.ID] = sp.ZoneID
	}
	for _, l := range in.Lines {
		zoneID := l.ZoneID
		if zoneID == "" {
			zoneID = spotZone[l.SpotID]
		}
		for _, z := range pl.Zones {
			if z.ID == zoneID {
				add(&t.Zones, z.ID, z.Code)
			}
		}
		for _, p := range pl.Packs {
			if p.ID == l.PackID {
				add(&t.Packs, p.ID, p.Code)
			}
		}
	}
	for _, d := range calendar.Range(in.StartsOn, in.EndsOn) {
		for _, rp := range pl.Rules.RatePeriods {
			for _, r := range rp.Ranges {
				if r.Contains(d) {
					add(&t.RatePeriods, rp.Code)
				}
			}
		}
	}
	return t
}

// MatrixInput selects the price matrix of a season.
type MatrixInput struct {
	SeasonID string
	PackIDs  []string
	Channel  pricing.Channel
	Slots    calendar.Slots
	Formula  pricing.Formula
}

// MatrixResult is the pack x category x rate period price list (spec 8.7).
type MatrixResult struct {
	SeasonID    string
	Channel     pricing.Channel
	Slots       calendar.Slots
	Formula     pricing.Formula
	PackIDs     []string
	Categories  []string
	RatePeriods []string
	Cells       []pricing.MatrixCell
}

// Matrix computes the day price of every active (or selected) pack for every
// zone price category and rate period of the season with pricing.Matrix, the
// same engine used by Simulate: a cell equals the day total of a one-day
// simulation on cell.Date in cell.ZoneID with today = cell.Date.
func (s *Service) Matrix(ctx context.Context, p *auth.Principal, establishmentID string, in MatrixInput) (*MatrixResult, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	v := &configerr.ValidationError{}
	if in.Channel == "" {
		in.Channel = pricing.ChannelDesk
	}
	if !in.Channel.Valid() {
		v.Add("channel", "Canale sconosciuto.")
	}
	if in.Slots == "" {
		in.Slots = calendar.SlotsFull
	}
	if !in.Slots.Valid() {
		v.Add("slots", "Usa AM, PM o FULL.")
	}
	if in.Formula == "" {
		in.Formula = pricing.FormulaDay
		if in.Slots != calendar.SlotsFull {
			in.Formula = pricing.FormulaHalfDay
		}
	}
	if in.Formula != pricing.FormulaDay && in.Formula != pricing.FormulaHalfDay {
		v.Add("formula", "La matrice mostra prezzi giornalieri: usa day o half_day.")
	}
	if in.Formula == pricing.FormulaHalfDay && in.Slots == calendar.SlotsFull {
		v.Add("slots", "La formula half_day richiede AM o PM.")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	if _, err := seasonOf(ctx, s.d.DB, es, in.SeasonID); err != nil {
		return nil, err
	}
	pl, err := LoadPriceList(ctx, s.d.DB, es, in.SeasonID, nil)
	if err != nil {
		return nil, err
	}
	packIDs, err := selectPacks(pl, in.PackIDs)
	if err != nil {
		return nil, err
	}
	out := &MatrixResult{
		SeasonID: in.SeasonID, Channel: in.Channel, Slots: in.Slots, Formula: in.Formula,
		PackIDs: packIDs, Categories: pl.categories(), RatePeriods: []string{}, Cells: []pricing.MatrixCell{},
	}
	for _, rp := range pl.Rules.RatePeriods {
		if len(rp.Ranges) > 0 {
			out.RatePeriods = append(out.RatePeriods, rp.Code)
		}
	}
	if len(packIDs) == 0 || len(out.Categories) == 0 || len(out.RatePeriods) == 0 {
		return out, nil
	}
	cells, err := pricing.Matrix(pricing.MatrixRequest{
		PackIDs: packIDs, Categories: out.Categories, RatePeriodCodes: out.RatePeriods,
		Channel: in.Channel, Slots: in.Slots, Formula: in.Formula,
	}, pl.Catalog, pl.Rules)
	if err != nil {
		return nil, fromEngine(err, "")
	}
	out.Cells = cells
	return out, nil
}

// selectPacks returns the requested pack IDs (validated) or every active pack.
func selectPacks(pl *PriceList, ids []string) ([]string, error) {
	ids = cleanIDs(ids)
	if len(ids) == 0 {
		out := []string{}
		for _, p := range pl.Catalog.Packs {
			if p.Active {
				out = append(out, p.ID)
			}
		}
		return out, nil
	}
	known := map[string]bool{}
	for _, p := range pl.Catalog.Packs {
		known[p.ID] = true
	}
	v := &configerr.ValidationError{}
	for i, id := range ids {
		if !known[id] {
			v.Add(fmt.Sprintf("pack_ids[%d]", i), "Pack inesistente.")
		}
	}
	return ids, v.OrNil()
}

// CoverageInput selects the coverage check of a season.
type CoverageInput struct {
	SeasonID string
	PackIDs  []string
	Channels []pricing.Channel
	Slots    calendar.Slots
}

// CoverageResult lists the gaps of the price list over the season.
type CoverageResult struct {
	SeasonID string
	From     calendar.Date
	To       calendar.Date
	Gaps     []pricing.CoverageGap
}

// Complete reports whether there are no gaps.
func (r CoverageResult) Complete() bool { return len(r.Gaps) == 0 }

// Coverage reports, over the whole season, the days on which an active pack
// has no base price for a zone price category and channel (spec 8.7). Days
// closed for every slot (closure days) are not reported.
func (s *Service) Coverage(ctx context.Context, p *auth.Principal, establishmentID string, in CoverageInput) (*CoverageResult, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	v := &configerr.ValidationError{}
	for i, ch := range in.Channels {
		if !ch.Valid() {
			v.Add(fmt.Sprintf("channels[%d]", i), "Canale sconosciuto.")
		}
	}
	if in.Slots == "" {
		in.Slots = calendar.SlotsFull
	}
	if !in.Slots.Valid() {
		v.Add("slots", "Usa AM, PM o FULL.")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	sn, err := seasonOf(ctx, s.d.DB, es, in.SeasonID)
	if err != nil {
		return nil, err
	}
	pl, err := LoadPriceList(ctx, s.d.DB, es, sn.ID, nil)
	if err != nil {
		return nil, err
	}
	packIDs, err := selectPacks(pl, in.PackIDs)
	if err != nil {
		return nil, err
	}
	out := &CoverageResult{SeasonID: sn.ID, From: sn.OpensOn, To: sn.ClosesOn, Gaps: []pricing.CoverageGap{}}
	cats := pl.categories()
	if len(packIDs) == 0 || len(cats) == 0 {
		return out, nil
	}
	gaps, err := pricing.Coverage(pricing.CoverageRequest{
		From: sn.OpensOn, To: sn.ClosesOn, PackIDs: packIDs, Categories: cats, Channels: in.Channels, Slots: in.Slots,
	}, pl.Catalog, pl.Rules)
	if err != nil {
		return nil, fromEngine(err, "")
	}
	closures, err := store.PricingListClosureDays(ctx, s.d.DB, es, sn.OpensOn, sn.ClosesOn)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	out.Gaps = withoutClosedDays(gaps, closedDays(closures, in.Slots))
	return out, nil
}

// closedDays returns the dates on which every requested slot is closed.
func closedDays(closures []store.ClosureDay, slots calendar.Slots) map[calendar.Date]bool {
	out := map[calendar.Date]bool{}
	for _, c := range closures {
		closed := map[calendar.Slot]bool{}
		for _, s := range c.Slots {
			closed[s] = true
		}
		all := true
		for _, s := range slots.Expand() {
			if !closed[s] {
				all = false
			}
		}
		if all {
			out[c.Date] = true
		}
	}
	return out
}

// withoutClosedDays splits gaps around closed dates.
func withoutClosedDays(gaps []pricing.CoverageGap, closed map[calendar.Date]bool) []pricing.CoverageGap {
	if len(closed) == 0 {
		return gaps
	}
	out := []pricing.CoverageGap{}
	for _, g := range gaps {
		var cur *pricing.CoverageGap
		for _, d := range calendar.Range(g.From, g.To) {
			if closed[d] {
				cur = nil
				continue
			}
			if cur == nil {
				ng := g
				ng.From, ng.To = d, d
				out = append(out, ng)
				cur = &out[len(out)-1]
				continue
			}
			cur.To = d
		}
	}
	return out
}
