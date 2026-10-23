package checkout

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Pure decisions of the public flow (functional core): what the online
// channel may sell, how a booking is paid, what the page shows next. No
// database, no clock: the shell passes data and "now" in. Table tests in
// plan_test.go.

// Limits of the public flow.
const (
	// MaxLines bounds the lines of one public booking (the policy
	// max_spots_per_booking usually binds earlier).
	MaxLines = 10
	// MaxStayDays bounds a public stay (season passes are sold at the desk).
	MaxStayDays = 120
	// limitedThreshold: a calendar day with at most this many units left
	// (or 10% of the capacity) is "limited".
	limitedThreshold = 3
)

// onlineCatalog is the part of the establishment catalog the public
// channel may sell, loaded by the shell.
type onlineCatalog struct {
	zones  map[string]store.Zone
	spots  map[string]store.Spot
	packs  map[string]store.Pack
	extras map[string]store.Extra
}

// sellsOnline reports whether a catalog item lists the online channel.
func sellsOnline(channels []string) bool { return slices.Contains(channels, store.ChannelOnline) }

// validateSelection checks a selection against the online configuration
// and the catalog the public channel may sell (spec 12.3): slots and
// formulas offered online, booking window, zones visible online, packs and
// extras of channel online, pack formulas and zones, seat/zone/quantity
// line shapes. Prices, policy, capacity and quotas are decided later by
// the engine and the database. Field paths match api.PublicSelection.
func validateSelection(sel Selection, cat *onlineCatalog, flow online.Config, today calendar.Date) error {
	ve := &configerr.ValidationError{}
	if sel.StartsOn.IsZero() {
		ve.Add("starts_on", "Campo obbligatorio.")
	}
	if sel.EndsOn.IsZero() {
		ve.Add("ends_on", "Campo obbligatorio.")
	}
	if !sel.StartsOn.IsZero() && !sel.EndsOn.IsZero() {
		switch {
		case sel.EndsOn.Before(sel.StartsOn):
			ve.Add("ends_on", "Deve essere successiva o uguale a starts_on.")
		case sel.StartsOn.DaysUntil(sel.EndsOn) >= MaxStayDays:
			ve.Add("ends_on", fmt.Sprintf("Durata massima online %d giorni.", MaxStayDays))
		}
		if sel.StartsOn.Before(today) {
			ve.Add("starts_on", "La data è già passata.")
		}
		if flow.MaxDaysAhead > 0 && today.DaysUntil(sel.StartsOn) > flow.MaxDaysAhead {
			ve.Add("starts_on", fmt.Sprintf("Prenotabile online fino a %d giorni prima.", flow.MaxDaysAhead))
		}
	}
	if !sel.Slots.Valid() {
		ve.Add("slots", "Usa AM, PM o FULL.")
	} else if !flow.AllowsSlots(sel.Slots) {
		ve.Add("slots", "Fascia non prenotabile online.")
	}
	if !policy.Formula(sel.Formula).Valid() {
		ve.Add("formula", "Formula sconosciuta.")
	} else if !flow.AllowsFormula(sel.Formula) {
		ve.Add("formula", "Formula non prenotabile online.")
	}
	switch {
	case len(sel.Lines) == 0:
		ve.Add("lines", "Scegli almeno una postazione.")
	case len(sel.Lines) > MaxLines:
		ve.Add("lines", fmt.Sprintf("Al massimo %d righe.", MaxLines))
	}
	perPack := map[string]int{}
	for i, l := range sel.Lines {
		f := fmt.Sprintf("lines[%d]", i)
		pack, ok := cat.packs[l.PackID]
		if !ok || !pack.Active || !sellsOnline(pack.Channels) {
			ve.Add(f+".pack_id", "Pacchetto non disponibile online.")
			continue
		}
		perPack[pack.ID] += max(l.Quantity, 1)
		if len(pack.AllowedFormulas) > 0 && sel.Formula != "" && !slices.Contains(pack.AllowedFormulas, sel.Formula) {
			ve.Add(f+".pack_id", "Pacchetto non disponibile con questa formula.")
		}
		zoneID := l.ZoneID
		if l.SpotID != "" {
			sp, ok := cat.spots[l.SpotID]
			if !ok || !sp.Active {
				ve.Add(f+".spot_id", "Postazione non disponibile online.")
				continue
			}
			if zoneID != "" && zoneID != sp.ZoneID {
				ve.Add(f+".zone_id", "La postazione non appartiene alla zona indicata.")
				continue
			}
			zoneID = sp.ZoneID
			if len(pack.RequiresSpotKind) > 0 && !slices.Contains(pack.RequiresSpotKind, sp.Kind) {
				ve.Add(f+".spot_id", "Il pacchetto non è disponibile su questa postazione.")
			}
		}
		z, ok := cat.zones[zoneID]
		if zoneID == "" || !ok || !z.OnlineVisible {
			ve.Add(f+".zone_id", "Zona non prenotabile online.")
			continue
		}
		if len(pack.AllowedZoneIDs) > 0 && !slices.Contains(pack.AllowedZoneIDs, zoneID) {
			ve.Add(f+".pack_id", "Pacchetto non disponibile in questa zona.")
		}
		switch z.AssignmentMode {
		case "seat":
			if l.SpotID == "" {
				ve.Add(f+".spot_id", "Scegli la postazione sulla mappa.")
			}
		case "zone":
			if l.SpotID != "" {
				ve.Add(f+".spot_id", "In questa zona la postazione viene assegnata automaticamente.")
			}
		}
		if z.AssignmentMode != "quantity" && l.Quantity > 1 {
			ve.Add(f+".quantity", "Quantità maggiore di 1 solo per le zone a quantità.")
		}
		if l.Quantity < 0 || l.Quantity > 50 {
			ve.Add(f+".quantity", "Quantità non valida.")
		}
		for j, x := range l.Extras {
			checkExtra(ve, fmt.Sprintf("%s.extras[%d]", f, j), x, cat, pack.ID)
		}
	}
	for id, n := range perPack {
		if p := cat.packs[id]; p.MaxPerBooking > 0 && n > p.MaxPerBooking {
			ve.Add("lines", fmt.Sprintf("Al massimo %d per prenotazione del pacchetto %s.", p.MaxPerBooking, p.Code))
		}
	}
	for j, x := range sel.Extras {
		checkExtra(ve, fmt.Sprintf("extras[%d]", j), x, cat, "")
	}
	return ve.OrNil()
}

func checkExtra(ve *configerr.ValidationError, f string, x booking.ExtraInput, cat *onlineCatalog, packID string) {
	ex, ok := cat.extras[x.ExtraID]
	switch {
	case !ok || !ex.Active || !sellsOnline(ex.Channels):
		ve.Add(f+".extra_id", "Extra non disponibile online.")
	case x.Quantity < 1 || (ex.MaxQuantity > 0 && x.Quantity > ex.MaxQuantity):
		ve.Add(f+".quantity", "Quantità non valida.")
	case packID != "" && len(ex.AllowedPackIDs) > 0 && !slices.Contains(ex.AllowedPackIDs, packID):
		ve.Add(f+".extra_id", "Extra non disponibile con questo pacchetto.")
	}
}

// DefaultFormula derives the formula of a stay when the client gives none.
func DefaultFormula(from, to calendar.Date, slots calendar.Slots) string {
	switch {
	case from.Equal(to) && slots != calendar.SlotsFull:
		return string(policy.FormulaHalfDay)
	case from.Equal(to):
		return string(policy.FormulaDay)
	default:
		return string(policy.FormulaMultiDay)
	}
}

// planInput are the facts deciding how a booking is paid.
type planInput struct {
	Rules            policy.Rules
	ApprovalRequired bool
	ProviderOnline   bool
	Total            money.Cents
	Deposit          money.Cents
	Paid             money.Cents
}

// paymentPlan decides the payment plan and the amount due now (spec 9
// payment, 13.2): approval first (nothing paid until the manager decides),
// then payment mode none or a provider without online payments (paid at
// the desk), then deposit or full online payment. Free bookings are
// confirmed without payment (at_desk with nothing due).
func paymentPlan(in planInput) (plan string, dueNow, balanceLater money.Cents) {
	remaining := max(in.Total-in.Paid, 0)
	switch {
	case in.ApprovalRequired:
		return PlanApproval, 0, remaining
	case in.Rules.EffectivePaymentMode() == policy.PaymentNone || !in.ProviderOnline || remaining == 0:
		return PlanAtDesk, 0, remaining
	case in.Rules.EffectivePaymentMode() == policy.PaymentDeposit && in.Deposit < in.Total:
		due := min(max(in.Deposit-in.Paid, 0), remaining)
		return PlanOnlineDeposit, due, remaining - due
	default:
		return PlanOnlineFull, remaining, 0
	}
}

// detailsComplete reports whether step 4 is done: a real customer is
// attached and, when the establishment publishes terms, the accepted
// version is the current one.
func detailsComplete(b *store.Booking, placeholder bool, current *store.TermsVersion) bool {
	if placeholder {
		return false
	}
	if current == nil {
		return true
	}
	return b.TermsVersion != nil && *b.TermsVersion == current.Version
}

// flowState is the derived state of a checkout session.
type flowState struct {
	Session string
	Next    string
}

// deriveState maps the booking status (and the latest payment) to the
// session state and the next action of the page (api.CheckoutNextAction).
// A held booking whose hold elapsed reads as expired even before the
// sweeper runs (reads never write).
func deriveState(stored string, b *store.Booking, details bool, payment *PaymentState, now time.Time) flowState {
	if stored == store.CheckoutAbandoned {
		return flowState{store.CheckoutAbandoned, NextExpired}
	}
	held := b.HoldExpiresAt != nil && !b.HoldExpiresAt.After(now)
	switch booking.Status(b.Status) {
	case booking.StatusHeld:
		if held {
			return flowState{store.CheckoutExpired, NextExpired}
		}
		if !details {
			return flowState{store.CheckoutOpen, NextDetails}
		}
		return flowState{store.CheckoutOpen, NextPay}
	case booking.StatusPendingPayment:
		if held {
			return flowState{store.CheckoutExpired, NextExpired}
		}
		if payment != nil && payment.Status == "failed" {
			return flowState{store.CheckoutPaying, NextPay}
		}
		return flowState{store.CheckoutPaying, NextWait}
	case booking.StatusAwaitingApproval:
		return flowState{store.CheckoutCompleted, NextWait}
	case booking.StatusConfirmed, booking.StatusCheckedIn, booking.StatusCompleted:
		return flowState{store.CheckoutCompleted, NextDone}
	case booking.StatusRejected:
		return flowState{store.CheckoutExpired, NextRejected}
	default: // expired, cancelled, no_show
		return flowState{store.CheckoutExpired, NextExpired}
	}
}

// paymentState maps the latest provider intent of a booking.
func paymentState(pi *store.PaymentIntent) *PaymentState {
	if pi == nil {
		return &PaymentState{Status: "none"}
	}
	out := &PaymentState{Amount: pi.Amount}
	switch pi.Status {
	case store.IntentSucceeded:
		out.Status = "succeeded"
	case store.IntentFailed, store.IntentCancelled, store.IntentExpired:
		out.Status = "failed"
		if pi.FailureReason != nil {
			out.FailureReason = *pi.FailureReason
		}
	default:
		out.Status = "pending"
	}
	return out
}

// indicator maps an available count to the public indicator (spec 12.3:
// "ultimi 3 posti").
func indicator(available, capacity int) string {
	switch {
	case available <= 0:
		return IndicatorNone
	case available <= limitedThreshold:
		return IndicatorLast
	case available <= max(5, capacity/5):
		return IndicatorFew
	default:
		return IndicatorPlenty
	}
}

// dayStatus is the public calendar status of a date with available units
// left over a capacity (open, in season, inside the booking window).
func dayStatus(available, capacity int) string {
	switch {
	case available <= 0:
		return DayFull
	case available <= max(limitedThreshold, capacity/10):
		return DayLimited
	default:
		return DayAvailable
	}
}

// legendTones assigns a tone index to each price category, from the
// cheapest (0) to the most expensive.
func legendTones(prices map[string]money.Cents) []LegendEntry {
	out := make([]LegendEntry, 0, len(prices))
	for cat, p := range prices {
		out = append(out, LegendEntry{PriceCategory: cat, FromPrice: p})
	}
	slices.SortFunc(out, func(a, b LegendEntry) int {
		if a.FromPrice != b.FromPrice {
			return int(a.FromPrice - b.FromPrice)
		}
		return strings.Compare(a.PriceCategory, b.PriceCategory)
	})
	for i := range out {
		out[i].Tone = i
	}
	return out
}

// quoteItems turns a price breakdown into the items of the public summary
// (lines with their extras, booking extras, discounts, rounding): their
// amounts always sum to the total.
func quoteItems(bd pricing.PriceBreakdown, owner []int, lines []booking.LineInput, extras []booking.ExtraInput, packs map[string]store.Pack, xs map[string]store.Extra) []QuoteItem {
	totals, _ := booking.LineTotals(bd, owner, len(lines))
	items := make([]QuoteItem, 0, len(lines)+len(extras)+len(bd.Discounts)+1)
	for i, l := range lines {
		items = append(items, QuoteItem{Kind: ItemLine, RefID: l.PackID, Label: packs[l.PackID].Name, Quantity: max(l.Quantity, 1), Amount: totals[i]})
	}
	for j, x := range bd.Extras {
		it := QuoteItem{Kind: ItemExtra, Label: store.I18nText{"it": x.Code}, Quantity: x.Qty, Amount: x.Total}
		if j < len(extras) {
			it.RefID = extras[j].ExtraID
			if ex, ok := xs[extras[j].ExtraID]; ok {
				it.Label = ex.Name
			}
		}
		items = append(items, it)
	}
	for _, d := range bd.Discounts {
		items = append(items, QuoteItem{Kind: ItemDiscount, RefID: d.Source, Label: discountLabel(d.Source), Quantity: 1, Amount: d.Amount})
	}
	if bd.Rounding != 0 {
		items = append(items, QuoteItem{Kind: ItemRounding, Label: store.I18nText{"it": "Arrotondamento", "en": "Rounding"}, Quantity: 1, Amount: bd.Rounding})
	}
	return items
}

// discountLabel names a discount source ("promo:CODE", "agreement:X", ...).
func discountLabel(source string) store.I18nText {
	kind, code, _ := strings.Cut(source, ":")
	switch kind {
	case "promo":
		return store.I18nText{"it": "Codice " + code, "en": "Code " + code}
	case "agreement":
		return store.I18nText{"it": "Convenzione", "en": "Agreement"}
	}
	return store.I18nText{"it": "Sconto", "en": "Discount"}
}

// manageActions decides what the customer may do from the manage area
// (spec 12.4) on a booking with its policy snapshot rules, at today in the
// establishment timezone.
func manageActions(b *store.Booking, rules policy.Rules, today calendar.Date, providerOnline bool) Actions {
	st := booking.Status(b.Status)
	daysBefore := today.DaysUntil(b.StartsOn)
	var a Actions
	active := st == booking.StatusConfirmed || st == booking.StatusCheckedIn
	a.PayBalance = active && providerOnline && b.TotalAmount-b.PaidAmount > 0
	if m := rules.Modification; m != nil && m.AllowedUntilDaysBefore != nil && !b.IsSeasonPass {
		until := b.StartsOn.AddDays(-*m.AllowedUntilDaysBefore)
		a.ModifyUntil = &until
		a.Modify = st == booking.StatusConfirmed && !today.After(until)
	}
	a.Cancel = (st == booking.StatusConfirmed || st == booking.StatusAwaitingApproval) && daysBefore >= 0
	best := -1
	for _, t := range rules.Cancellation {
		if t.RefundPercent >= 100 && t.As != policy.AsCredit && t.UntilDaysBefore > best {
			best = t.UntilDaysBefore
		}
	}
	if best >= 0 {
		free := b.StartsOn.AddDays(-best)
		a.CancelUntilFree = &free
	}
	a.ReleaseDays = b.IsSeasonPass && active && rules.CustomerCanRelease()
	return a
}
