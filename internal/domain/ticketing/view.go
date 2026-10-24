package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// View is a ticket with everything the list, the detail and the receipt
// show: booking code (the QR payload), zone and spot, slot and formula,
// pack, extras, entry items and the VAT split of the price.
type View struct {
	Ticket      store.Ticket
	State       State
	NumberLabel string
	BookingCode string
	ZoneName    store.I18nText
	// SpotID and SpotLabel are the assigned spot (empty while to assign and
	// for entry tickets).
	SpotID     string
	SpotLabel  string
	Slots      calendar.Slots
	Formula    string
	PackID     string
	PackName   store.I18nText
	Extras     []ViewItem
	EntryItems []ViewItem
	VAT        []pricing.VATBreakdown
}

// ViewItem is one extra (ID = extra id) or one entry ticket type (ID =
// entry ticket type id) of a ticket.
type ViewItem struct {
	ID       string
	Name     store.I18nText
	Quantity int
	Total    money.Cents
}

// views loads the views of tickets of one establishment with a fixed
// number of queries.
func views(ctx context.Context, db bun.IDB, es store.EstablishmentScope, tickets []store.Ticket) ([]View, error) {
	out := make([]View, 0, len(tickets))
	if len(tickets) == 0 {
		return out, nil
	}
	var zoneIDs, bookingIDs []string
	for i := range tickets {
		zoneIDs = append(zoneIDs, tickets[i].ZoneID)
		bookingIDs = append(bookingIDs, tickets[i].BookingID)
	}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("ticketing: packs: %w", err)
	}
	packName := make(map[string]store.I18nText, len(packs))
	for _, p := range packs {
		packName[p.ID] = p.Name
	}
	extras, err := store.ListAllExtras(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("ticketing: extras: %w", err)
	}
	extraName := make(map[string]store.I18nText, len(extras))
	for _, x := range extras {
		extraName[x.ID] = x.Name
	}
	types, err := store.ListEntryTicketTypes(ctx, db, es)
	if err != nil {
		return nil, err
	}
	typeOfPack := make(map[string]*store.EntryTicketType, len(types))
	for i := range types {
		typeOfPack[types[i].PackID] = &types[i]
	}
	zones, err := store.InventoryZones(ctx, db, es, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("ticketing: zones: %w", err)
	}
	zoneName := make(map[string]store.I18nText, len(zones))
	for _, z := range zones {
		zoneName[z.ID] = z.Name
	}
	bookings, err := store.BookingsByIDs(ctx, db, es, bookingIDs)
	if err != nil {
		return nil, err
	}
	bookingOf := make(map[string]*store.Booking, len(bookings))
	for i := range bookings {
		bookingOf[bookings[i].ID] = &bookings[i]
	}
	lines, err := store.ListBookingLines(ctx, db, es, bookingIDs...)
	if err != nil {
		return nil, fmt.Errorf("ticketing: lines: %w", err)
	}
	linesOf := map[string][]store.BookingLine{}
	var spotIDs []string
	for _, l := range lines {
		linesOf[l.BookingID] = append(linesOf[l.BookingID], l)
		if l.SpotID != nil {
			spotIDs = append(spotIDs, *l.SpotID)
		}
	}
	spots, err := store.BookingSpotsByIDs(ctx, db, es, spotIDs)
	if err != nil {
		return nil, err
	}
	spotLabel := make(map[string]string, len(spots))
	for _, sp := range spots {
		spotLabel[sp.ID] = sp.Label
	}
	lineExtras, err := store.BookingLineExtrasOf(ctx, db, es, bookingIDs)
	if err != nil {
		return nil, err
	}
	extrasOf := map[string][]store.BookingLineExtra{}
	for _, x := range lineExtras {
		extrasOf[x.BookingID] = append(extrasOf[x.BookingID], x)
	}

	for i := range tickets {
		t := tickets[i]
		v := View{
			Ticket: t, State: StateOf(&t), NumberLabel: NumberLabel(t.TicketNumber), ZoneName: zoneName[t.ZoneID],
			Extras: []ViewItem{}, EntryItems: []ViewItem{}, VAT: []pricing.VATBreakdown{},
		}
		if b := bookingOf[t.BookingID]; b != nil {
			v.BookingCode, v.Slots, v.Formula = b.Code, b.Slots, b.Formula
			var bd pricing.PriceBreakdown
			if len(b.PriceSnapshot) > 0 && json.Unmarshal(b.PriceSnapshot, &bd) == nil && bd.VAT != nil {
				v.VAT = bd.VAT
			}
		}
		ls := linesOf[t.BookingID]
		sort.SliceStable(ls, func(a, b int) bool { return ls[a].SortOrder < ls[b].SortOrder })
		if t.Kind == store.TicketKindEntry {
			for _, l := range ls {
				it := ViewItem{ID: "", Name: packName[l.PackID], Quantity: l.Quantity, Total: l.LineTotal}
				if et := typeOfPack[l.PackID]; et != nil {
					it.ID, it.Name = et.ID, et.Name
				}
				v.EntryItems = append(v.EntryItems, it)
			}
		} else if len(ls) > 0 {
			l := ls[0]
			v.PackID, v.PackName = l.PackID, packName[l.PackID]
			if l.SpotID != nil && !t.NeedsAssignment {
				v.SpotID, v.SpotLabel = *l.SpotID, spotLabel[*l.SpotID]
			}
		}
		for _, x := range extrasOf[t.BookingID] {
			v.Extras = append(v.Extras, ViewItem{ID: x.ExtraID, Name: extraName[x.ExtraID], Quantity: x.Quantity, Total: x.Total})
		}
		out = append(out, v)
	}
	return out, nil
}

func view(ctx context.Context, db bun.IDB, es store.EstablishmentScope, t *store.Ticket) (*View, error) {
	vs, err := views(ctx, db, es, []store.Ticket{*t})
	if err != nil {
		return nil, err
	}
	return &vs[0], nil
}
