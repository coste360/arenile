package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Booking data for the messages: read at send time (imperative shell,
// loadBooking) and turned into formatted Content and .ics events by pure
// functions. Personal data is read here and never stored in message_log.

// bookingData is everything a booking message needs.
type bookingData struct {
	Est      *store.Establishment
	Booking  *store.Booking
	Customer *store.Customer
	Lines    []store.BookingLine
	Zones    map[string]store.Zone
	Packs    map[string]store.Pack
	Extras   []store.BookingLineExtra
	ExtraDef map[string]store.Extra
	Spots    []string // labels of the assigned spots, line order
}

func loadBooking(ctx context.Context, db bun.IDB, es store.EstablishmentScope, bookingID string) (*bookingData, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("notify: establishment: %w", err)
	}
	b, err := store.GetBooking(ctx, db, es, bookingID)
	if err != nil {
		return nil, fmt.Errorf("notify: booking: %w", err)
	}
	cu, err := store.GetCustomer(ctx, db, es, b.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("notify: customer: %w", err)
	}
	lines, err := store.ListBookingLines(ctx, db, es, b.ID)
	if err != nil {
		return nil, fmt.Errorf("notify: lines: %w", err)
	}
	d := &bookingData{
		Est: est, Booking: b, Customer: cu, Lines: lines,
		Zones: map[string]store.Zone{}, Packs: map[string]store.Pack{}, ExtraDef: map[string]store.Extra{},
	}
	var zoneIDs []string
	for _, l := range lines {
		zoneIDs = append(zoneIDs, l.ZoneID)
		if _, ok := d.Packs[l.PackID]; !ok {
			p, err := store.GetPack(ctx, db, es, l.PackID)
			if err != nil {
				return nil, fmt.Errorf("notify: pack: %w", err)
			}
			d.Packs[l.PackID] = *p
		}
	}
	zones, err := store.ListZonesByIDs(ctx, db, es, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("notify: zones: %w", err)
	}
	for _, z := range zones {
		d.Zones[z.ID] = z
	}
	labels, err := store.CustomerBookingSpotLabels(ctx, db, es, []string{b.ID})
	if err != nil {
		return nil, fmt.Errorf("notify: spots: %w", err)
	}
	d.Spots = labels[b.ID]
	if d.Extras, err = store.ListBookingLineExtras(ctx, db, es, b.ID); err != nil {
		return nil, fmt.Errorf("notify: extras: %w", err)
	}
	for _, x := range d.Extras {
		if _, ok := d.ExtraDef[x.ExtraID]; ok {
			continue
		}
		def, err := store.GetExtra(ctx, db, es, x.ExtraID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("notify: extra: %w", err)
		}
		d.ExtraDef[x.ExtraID] = *def
	}
	return d, nil
}

// messageLocale is the language of the customer communications: booking
// locale, then customer locale, then the establishment default (spec 20).
func (d *bookingData) messageLocale() string {
	var bl string
	if d.Booking.Locale != nil {
		bl = *d.Booking.Locale
	}
	return pickLocale(bl, d.Customer.Locale, d.Est.DefaultLocale)
}

// i18nText picks the text of locale with fallback to the establishment
// default, Italian and any value (spec 20: manager content).
func i18nText(t store.I18nText, locale, fallback string) string {
	for _, l := range []string{locale, fallback, SourceLocale} {
		if v := strings.TrimSpace(t[l]); v != "" {
			return v
		}
	}
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := strings.TrimSpace(t[k]); v != "" {
			return v
		}
	}
	return ""
}

// contentOptions are the per-template additions to a booking message.
type contentOptions struct {
	QR          bool
	ICS         bool
	ManageURL   string
	LinkExpires time.Time
	Policy      bool
	Manager     bool
	// BalanceNotice adds the balance due and its due date (balance reminder).
	BalanceNotice bool
	// Extra variables (refund.amount, payment.amount, message.*).
	Vars map[string]string
}

// bookingContent formats the booking for cat (pure).
func bookingContent(cat catalog, d *bookingData, o contentOptions) Content {
	b, est, loc := d.Booking, d.Est, cat.locale
	cur := est.Currency
	m := func(c money.Cents) string { return formatMoney(c, cur, loc) }
	period := formatPeriod(cat, b.StartsOn, b.EndsOn)
	c := Content{
		Locale: loc, BookingCode: b.Code, QR: o.QR, ICS: o.ICS, ManageURL: o.ManageURL,
		Contacts: establishmentContacts(est), Manager: o.Manager,
		Vars: map[string]string{
			"establishment.name":  est.Name,
			"customer.first_name": firstNonEmpty(d.Customer.FirstName, d.Customer.LastName),
			"customer.last_name":  d.Customer.LastName,
			"booking.code":        b.Code,
			"booking.period":      period,
			"booking.total":       m(b.TotalAmount),
			"booking.paid":        m(b.PaidAmount),
			"booking.balance":     m(b.BalanceDue()),
			"manage_url":          o.ManageURL,
		},
	}
	if !o.LinkExpires.IsZero() && o.ManageURL != "" {
		c.Vars["link.expires"] = cat.t("link_expires", map[string]string{"date": formatDate(cat, calendar.In(o.LinkExpires, estLocation(est)))})
	}
	for k, v := range o.Vars {
		c.Vars[k] = v
	}

	c.Summary = append(c.Summary, Row{cat.t("label_code", nil), b.Code})
	if name := strings.TrimSpace(d.Customer.FirstName + " " + d.Customer.LastName); name != "" {
		c.Summary = append(c.Summary, Row{cat.t("label_guest", nil), name})
	}
	c.Summary = append(c.Summary, Row{cat.t("label_dates", nil), capitalize(period)})
	if b.IsSeasonPass {
		c.Summary = append(c.Summary, Row{cat.t("label_pack", nil), cat.t("season_pass", nil)})
	}
	if b.Slots != "" {
		c.Summary = append(c.Summary, Row{cat.t("label_slot", nil), cat.t("slot_"+string(b.Slots), nil)})
	}
	var zones, packs []string
	pending := false
	for _, l := range d.Lines {
		if z, ok := d.Zones[l.ZoneID]; ok {
			if n := i18nText(z.Name, loc, est.DefaultLocale); n != "" && !slices.Contains(zones, n) {
				zones = append(zones, n)
			}
		}
		if p, ok := d.Packs[l.PackID]; ok {
			name := i18nText(p.Name, loc, est.DefaultLocale)
			if l.Quantity > 1 {
				name += " × " + strconv.Itoa(l.Quantity)
			}
			packs = append(packs, name)
		}
		if l.AssignmentStatus == store.AssignmentPending {
			pending = true
		}
	}
	if len(zones) > 0 {
		c.Summary = append(c.Summary, Row{cat.t("label_zone", nil), strings.Join(zones, ", ")})
	}
	spots := strings.Join(d.Spots, ", ")
	if pending {
		spots = strings.TrimPrefix(spots+", "+cat.t("spot_to_assign", nil), ", ")
	}
	if spots != "" {
		c.Summary = append(c.Summary, Row{cat.t("label_spots", nil), spots})
	}
	c.Vars["booking.spots"] = spots
	if due, onArrival, ok := BalanceDueDate(b); ok {
		dueText := cat.t("balance_due_on_arrival", nil)
		if !onArrival {
			dueText = formatDate(cat, due)
		}
		c.Vars["balance.due"] = dueText
		if bal := b.BalanceDue(); o.BalanceNotice && bal > 0 {
			key, args := "balance_notice", map[string]string{"amount": m(bal), "date": dueText}
			if onArrival {
				key = "balance_notice_on_arrival"
			}
			c.BalanceNotice = cat.t(key, args)
		}
	}
	if len(packs) > 0 && !b.IsSeasonPass {
		c.Summary = append(c.Summary, Row{cat.t("label_pack", nil), strings.Join(packs, ", ")})
	}
	var extras []string
	for _, x := range d.Extras {
		def, ok := d.ExtraDef[x.ExtraID]
		if !ok {
			continue
		}
		extras = append(extras, i18nText(def.Name, loc, est.DefaultLocale)+" × "+strconv.Itoa(x.Quantity))
	}
	if len(extras) > 0 {
		c.Summary = append(c.Summary, Row{cat.t("label_extras", nil), strings.Join(extras, ", ")})
	}

	c.Totals = append(c.Totals, Row{cat.t("label_total", nil), m(b.TotalAmount)})
	if b.PaidAmount > 0 {
		c.Totals = append(c.Totals, Row{cat.t("label_paid", nil), m(b.PaidAmount)})
	}
	if b.DepositDue > b.PaidAmount && b.DepositDue < b.TotalAmount {
		c.Totals = append(c.Totals, Row{cat.t("label_deposit", nil), m(b.DepositDue - b.PaidAmount)})
	}
	if bal := b.BalanceDue(); bal > 0 {
		c.Totals = append(c.Totals, Row{cat.t("label_balance", nil), m(bal)})
	}
	if o.Policy {
		c.Policy = policyLines(cat, b.PolicySnapshot)
	}
	return c
}

// policyLines summarises the cancellation and modification rules of the
// booking snapshot (spec 9), most generous tier first.
func policyLines(cat catalog, raw store.JSON) []string {
	snap, err := policy.ParseSnapshot(raw)
	if err != nil {
		return nil
	}
	tiers := slices.Clone(snap.Rules.Cancellation)
	sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].UntilDaysBefore > tiers[j].UntilDaysBefore })
	var out []string
	for _, t := range tiers {
		if t.RefundPercent <= 0 {
			continue
		}
		key := "policy_tier_refund"
		if t.As == policy.AsCredit {
			key = "policy_tier_credit"
		}
		out = append(out, cat.t(key, map[string]string{"days": strconv.Itoa(t.UntilDaysBefore), "percent": strconv.Itoa(t.RefundPercent)}))
	}
	if len(out) == 0 {
		out = append(out, cat.t("policy_none", nil))
	} else {
		out = append(out, cat.t("policy_after", nil))
	}
	if mod := snap.Rules.Modification; mod != nil && mod.AllowedUntilDaysBefore != nil {
		out = append(out, cat.t("policy_modification", map[string]string{"days": strconv.Itoa(*mod.AllowedUntilDaysBefore)}))
	}
	return out
}

// establishmentContacts are the footer lines: phone, email, address.
func establishmentContacts(est *store.Establishment) []string {
	var out []string
	if est.ContactPhone != "" {
		out = append(out, est.ContactPhone)
	}
	if est.ContactEmail != "" {
		out = append(out, est.ContactEmail)
	}
	if a := formatAddress(est.Address); a != "" {
		out = append(out, a)
	}
	return out
}

func formatAddress(a store.Address) string {
	city := strings.TrimSpace(strings.Join(nonEmpty(a.PostalCode, a.City), " "))
	if a.Province != "" {
		city = strings.TrimSpace(city + " (" + a.Province + ")")
	}
	return strings.Join(nonEmpty(a.Street, city), ", ")
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func estLocation(est *store.Establishment) *time.Location {
	if loc, err := time.LoadLocation(est.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// icsHost is the host part of the stable UIDs ("<booking id>@<host>").
func icsHost(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "arenile.invalid"
}

// bookingICS builds the calendar of a booking (pure). Half-day slots stay
// all-day events: slot hours are not configured per establishment yet;
// ICSEvent supports timed events with TZID for when they are.
func bookingICS(cat catalog, d *bookingData, baseURL, manageURL string, stamp time.Time) ICSCalendar {
	b, est := d.Booking, d.Est
	args := map[string]string{"name": est.Name, "code": b.Code}
	desc := cat.t("ics_description", args)
	if b.Slots != "" && b.Slots != calendar.SlotsFull {
		desc += "\n" + cat.t("label_slot", nil) + ": " + cat.t("slot_"+string(b.Slots), nil)
	}
	if len(d.Spots) > 0 {
		desc += "\n" + cat.t("label_spots", nil) + ": " + strings.Join(d.Spots, ", ")
	}
	if manageURL != "" {
		desc += "\n" + manageURL
	}
	ev := ICSEvent{
		UID: b.ID + "@" + icsHost(baseURL), Summary: cat.t("ics_summary", args), Description: desc,
		Location: strings.Join(nonEmpty(est.Name, formatAddress(est.Address)), ", "), URL: manageURL,
		OrganizerName: est.Name, OrganizerEmail: est.ContactEmail,
		From: b.StartsOn, To: b.EndsOn, TZ: estLocation(est),
		Stamp: stamp, Sequence: int(b.Version - 1),
		Cancelled: b.Status == statusCancelled || b.Status == statusRejected || b.Status == statusExpired,
	}
	return ICSCalendar{Name: est.Name, Events: []ICSEvent{ev}}
}

// bookingFingerprint identifies what the customer sees of a booking
// (dates, slots, spots, lines, total): booking.modified events that do not
// change it (notes, internal fields) send no email.
func bookingFingerprint(d *bookingData) string {
	b := d.Booking
	parts := []string{b.StartsOn.String(), b.EndsOn.String(), string(b.Slots), strconv.FormatInt(int64(b.TotalAmount), 10), b.Status}
	spots := slices.Clone(d.Spots)
	sort.Strings(spots)
	parts = append(parts, strings.Join(spots, ","))
	var lines []string
	for _, l := range d.Lines {
		lines = append(lines, l.PackID+"/"+l.ZoneID+"/"+strconv.Itoa(l.Quantity))
	}
	sort.Strings(lines)
	parts = append(parts, strings.Join(lines, ","))
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}
