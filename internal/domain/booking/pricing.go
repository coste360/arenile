package booking

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Zone assignment modes (zones.assignment_mode, spec 5.2).
const (
	modeSeat     = "seat"
	modeZone     = "zone"
	modeQuantity = "quantity"
)

// stay is the interval, formula and channel of a booking request.
type stay struct {
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    calendar.Slots
	Formula  string
	Channel  string
	SeasonID string
}

// catalog is the part of the establishment catalog a request touches.
type catalog struct {
	zones  map[string]store.Zone
	spots  map[string]store.Spot
	packs  map[string]store.Pack
	extras map[string]store.Extra
}

// loadCatalog reads the packs, extras, spots and zones referenced by lines
// and extras (all scoped to es: foreign ids are simply missing).
func loadCatalog(ctx context.Context, db bun.IDB, es store.EstablishmentScope, lines []LineInput, extras []ExtraInput) (*catalog, error) {
	c := &catalog{zones: map[string]store.Zone{}, spots: map[string]store.Spot{}, packs: map[string]store.Pack{}, extras: map[string]store.Extra{}}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("booking: packs: %w", err)
	}
	for _, p := range packs {
		c.packs[p.ID] = p
	}
	xs, err := store.ListAllExtras(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("booking: extras: %w", err)
	}
	for _, x := range xs {
		c.extras[x.ID] = x
	}
	var spotIDs, zoneIDs []string
	for _, l := range lines {
		if l.SpotID != "" {
			spotIDs = append(spotIDs, l.SpotID)
		}
		if l.ZoneID != "" {
			zoneIDs = append(zoneIDs, l.ZoneID)
		}
	}
	spots, err := store.BookingSpotsByIDs(ctx, db, es, spotIDs)
	if err != nil {
		return nil, err
	}
	for _, sp := range spots {
		c.spots[sp.ID] = sp
		zoneIDs = append(zoneIDs, sp.ZoneID)
	}
	if len(zoneIDs) > 0 {
		zones, err := store.ListZonesByIDs(ctx, db, es, zoneIDs)
		if err != nil {
			return nil, fmt.Errorf("booking: zones: %w", err)
		}
		for _, z := range zones {
			c.zones[z.ID] = z
		}
	}
	return c, nil
}

// resolve validates the requested lines against the catalog and fills the
// zone of seat lines from their spot. field is the JSON path prefix.
func (c *catalog) resolve(in []LineInput, field string) ([]PricedLine, error) {
	v := &configerr.ValidationError{}
	out := make([]PricedLine, 0, len(in))
	for i, l := range in {
		f := fmt.Sprintf("%s[%d]", field, i)
		pl := PricedLine{PackID: l.PackID, ZoneID: l.ZoneID, SpotID: l.SpotID, Quantity: max(l.Quantity, 1), Extras: l.Extras}
		pack, ok := c.packs[l.PackID]
		switch {
		case l.PackID == "":
			v.Add(f+".pack_id", "Campo obbligatorio.")
		case !ok:
			v.Add(f+".pack_id", "Pack inesistente.")
		case !pack.Active:
			v.Add(f+".pack_id", "Pack non attivo.")
		}
		if l.SpotID != "" {
			sp, ok := c.spots[l.SpotID]
			switch {
			case !ok:
				v.Add(f+".spot_id", "Postazione inesistente.")
			case !sp.Active:
				v.Add(f+".spot_id", "Postazione non attiva.")
			case l.ZoneID != "" && l.ZoneID != sp.ZoneID:
				v.Add(f+".zone_id", "La postazione non appartiene alla zona indicata.")
			default:
				pl.ZoneID = sp.ZoneID
			}
		}
		z, zok := c.zones[pl.ZoneID]
		switch {
		case pl.ZoneID == "" && l.SpotID == "":
			v.Add(f+".zone_id", "Indica la zona o la postazione.")
		case pl.ZoneID != "" && !zok:
			v.Add(f+".zone_id", "Zona inesistente.")
		case zok && z.AssignmentMode == modeSeat && l.SpotID == "":
			v.Add(f+".spot_id", "Le zone a posti assegnati richiedono la postazione.")
		case zok && z.AssignmentMode != modeQuantity && pl.Quantity > 1:
			v.Add(f+".quantity", "Quantità maggiore di 1 solo per le zone a quantità.")
		}
		if ok && len(pack.AllowedZoneIDs) > 0 && pl.ZoneID != "" && !contains(pack.AllowedZoneIDs, pl.ZoneID) {
			v.Add(f+".pack_id", "Pack non disponibile in questa zona.")
		}
		for j, x := range l.Extras {
			c.checkExtra(v, fmt.Sprintf("%s.extras[%d]", f, j), x)
		}
		out = append(out, pl)
	}
	return out, v.OrNil()
}

func (c *catalog) checkExtra(v *configerr.ValidationError, f string, x ExtraInput) {
	ex, ok := c.extras[x.ExtraID]
	switch {
	case !ok:
		v.Add(f+".extra_id", "Extra inesistente.")
	case !ex.Active:
		v.Add(f+".extra_id", "Extra non attivo.")
	case x.Quantity < 1:
		v.Add(f+".quantity", "Almeno 1.")
	case ex.MaxQuantity > 0 && x.Quantity > ex.MaxQuantity:
		v.Add(f+".quantity", fmt.Sprintf("Massimo %d.", ex.MaxQuantity))
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// priced is a request priced by the engine through the Pricer port.
type priced struct {
	res                *pricingconfig.SimulateResult
	lines              []PricedLine
	totals             []money.Cents
	extraTotals        [][]money.Cents
	extras             []ExtraInput
	bookingExtraTotals []money.Cents
	approval           bool
}

// price quotes the request with the engine and evaluates the policy (the
// Pricer reads through db, i.e. inside the booking transaction).
func (s *Service) price(ctx context.Context, db bun.IDB, es store.EstablishmentScope, st stay, lines []PricedLine, extras []ExtraInput, tags []string, agreementID, promo string, c *catalog) (*priced, error) {
	v := &configerr.ValidationError{}
	for j, x := range extras {
		c.checkExtra(v, fmt.Sprintf("extras[%d]", j), x)
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	engineLines, owner := PricingLines(lines)
	in := pricingconfig.SimulateInput{
		SeasonID: st.SeasonID, Channel: pricing.Channel(st.Channel), StartsOn: st.StartsOn, EndsOn: st.EndsOn,
		Slots: st.Slots, Formula: pricing.Formula(st.Formula), Lines: engineLines, CustomerTags: tags,
		AgreementID: agreementID, PromoCode: strings.TrimSpace(promo),
	}
	for _, x := range extras {
		in.Extras = append(in.Extras, pricing.ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity})
	}
	res, err := s.d.Pricer.Quote(ctx, db, es, in)
	if err != nil {
		return nil, err
	}
	totals, extraTotals := LineTotals(res.Breakdown, owner, len(lines))
	pr := &priced{res: res, lines: lines, totals: totals, extraTotals: extraTotals, extras: extras, approval: res.ApprovalRequired}
	for _, x := range res.Breakdown.Extras {
		pr.bookingExtraTotals = append(pr.bookingExtraTotals, x.Total)
	}
	for _, l := range lines {
		if p, ok := c.packs[l.PackID]; ok && p.RequiresApproval {
			pr.approval = true
		}
	}
	return pr, nil
}

// priceCategory is the spot override or the zone category of a line.
func (c *catalog) priceCategory(l PricedLine) string {
	if sp, ok := c.spots[l.SpotID]; ok && sp.PriceCategoryOverride != nil && *sp.PriceCategoryOverride != "" {
		return *sp.PriceCategoryOverride
	}
	return c.zones[l.ZoneID].PriceCategory
}

// initialAssignment is the assignment of a line before allocation.
func (c *catalog) initialAssignment(l PricedLine) string {
	switch {
	case l.SpotID != "":
		return store.AssignmentAssigned
	case c.zones[l.ZoneID].AssignmentMode == modeQuantity:
		return store.AssignmentNotApplicable
	default:
		return store.AssignmentPending
	}
}

// buildRows makes the booking_lines and booking_line_extras rows of pr.
func buildRows(bookingID string, pr *priced, c *catalog, now time.Time) ([]store.BookingLine, []store.BookingLineExtra) {
	rows := make([]store.BookingLine, 0, len(pr.lines))
	var extras []store.BookingLineExtra
	for i, l := range pr.lines {
		row := store.BookingLine{
			ID: store.NewID(), BookingID: bookingID, PackID: l.PackID, ZoneID: l.ZoneID,
			AssignmentStatus: c.initialAssignment(l), Quantity: max(l.Quantity, 1),
			PriceCategory: c.priceCategory(l), LineTotal: pr.totals[i], SortOrder: i, CreatedAt: now, UpdatedAt: now,
		}
		if l.SpotID != "" {
			sp := l.SpotID
			row.SpotID = &sp
		}
		rows = append(rows, row)
		for j, x := range l.Extras {
			lineID := row.ID
			extras = append(extras, store.BookingLineExtra{
				ID: store.NewID(), BookingID: bookingID, BookingLineID: &lineID, ExtraID: x.ExtraID,
				Quantity: x.Quantity, Total: at(pr.extraTotals[i], j), CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	for j, x := range pr.extras {
		extras = append(extras, store.BookingLineExtra{
			ID: store.NewID(), BookingID: bookingID, ExtraID: x.ExtraID, Quantity: x.Quantity,
			Total: at(pr.bookingExtraTotals, j), CreatedAt: now, UpdatedAt: now,
		})
	}
	return rows, extras
}

func at(xs []money.Cents, i int) money.Cents {
	if i < len(xs) {
		return max(xs[i], 0)
	}
	return 0
}

// allocRequest builds the inventory request of the lines of b: the spot (or
// zone / quantity), and the tracked components of the pack and of the extras
// with a component code (booking-level extras go with the first line).
func allocRequest(b *store.Booking, kind inventory.Kind, expiresAt *time.Time, rows []store.BookingLine, extras []store.BookingLineExtra, c *catalog) inventory.AllocateRequest {
	req := inventory.AllocateRequest{
		BookingID: b.ID, Channel: b.Channel, StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, Kind: kind,
	}
	if kind == inventory.KindHold {
		req.ExpiresAt = expiresAt
	}
	byLine := map[string][]store.BookingLineExtra{}
	var bookingExtras []store.BookingLineExtra
	for _, x := range extras {
		if x.BookingLineID == nil {
			bookingExtras = append(bookingExtras, x)
		} else {
			byLine[*x.BookingLineID] = append(byLine[*x.BookingLineID], x)
		}
	}
	for i, l := range rows {
		lr := inventory.LineRequest{LineID: l.ID, ZoneID: l.ZoneID, Quantity: max(l.Quantity, 1), Components: map[string]int{}}
		if l.SpotID != nil {
			lr.SpotID = *l.SpotID
		}
		for code, n := range c.packs[l.PackID].Composition {
			lr.Components[code] += n * lr.Quantity
		}
		xs := byLine[l.ID]
		if i == 0 {
			xs = append(xs, bookingExtras...)
		}
		for _, x := range xs {
			if ex, ok := c.extras[x.ExtraID]; ok && ex.ComponentCode != nil && *ex.ComponentCode != "" {
				lr.Components[*ex.ComponentCode] += x.Quantity
			}
		}
		if len(lr.Components) == 0 {
			lr.Components = nil
		}
		req.Lines = append(req.Lines, lr)
	}
	return req
}

// currentLines reads the stored lines and extras of b with their catalog.
func (s *Service) currentLines(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) ([]store.BookingLine, []store.BookingLineExtra, *catalog, error) {
	rows, err := store.ListBookingLines(ctx, db, es, b.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	extras, err := store.ListBookingLineExtras(ctx, db, es, b.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	ins, _ := linesToInput(rows, extras)
	c, err := loadCatalog(ctx, db, es, ins, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	return rows, extras, c, nil
}

// linesToInput converts stored lines back into requests (re-pricing).
func linesToInput(rows []store.BookingLine, extras []store.BookingLineExtra) ([]LineInput, []ExtraInput) {
	byLine := map[string][]ExtraInput{}
	var bookingExtras []ExtraInput
	for _, x := range extras {
		in := ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity}
		if x.BookingLineID == nil {
			bookingExtras = append(bookingExtras, in)
		} else {
			byLine[*x.BookingLineID] = append(byLine[*x.BookingLineID], in)
		}
	}
	out := make([]LineInput, 0, len(rows))
	for _, l := range rows {
		in := LineInput{PackID: l.PackID, ZoneID: l.ZoneID, Quantity: l.Quantity, Extras: byLine[l.ID]}
		if l.SpotID != nil {
			in.SpotID = *l.SpotID
		}
		out = append(out, in)
	}
	return out, bookingExtras
}

// refundRequest is the payment port request of a cancellation refund.
func refundRequest(bookingID string, rf RefundPreview, in CancelInput) payment.RefundRequest {
	req := payment.RefundRequest{BookingID: bookingID, Amount: rf.Amount, Reason: strings.TrimSpace(in.Reason)}
	if rf.As == SettleCredit {
		req.As = payment.RefundAsCredit
		req.Method = store.PaymentMethodCredit
	} else {
		req.As = payment.RefundAsRefund
		req.Method = in.RefundMethod
	}
	if req.Reason == "" {
		req.Reason = "cancellation"
	}
	return req
}
