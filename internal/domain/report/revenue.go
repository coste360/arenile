package report

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the revenue report.
//
// VAT (INV-16, consistent with the corrispettivi export): gross amounts are
// accumulated per cell = (row, business date, VAT rate, payment method) and
// each cell is split once with money.VATSplit (half-even); a row's taxable
// and tax are the sums of its cells. For the time, payment method and VAT
// rate groupings a cell is exactly a row of the daily corrispettivi export,
// so the report reconciles to the cent with it (gross, taxable and tax, per
// rate and in total). Grouped by channel, a corrispettivi cell may be split
// across channels: gross still reconciles exactly, taxable/tax may differ by
// at most one cent per split cell. The VAT shares come from
// payments.vat_breakdown (frozen at payment time); a payment without a
// usable split goes entirely to the establishment default rate, as in
// fiscal.BuildReport.
//
// Basis booked: the value of the bookings confirmed in the period, split by
// the VAT rates of the current price snapshot (payment.SplitBySnapshot, the
// same proportional split the payments use) with cell = (row, rate). Packs
// and extras are valued from booking_lines / booking_line_extras before the
// booking-level discounts and rounding, at the catalog VAT rate of the item
// (or the default rate).

// revenueCell is the VAT split granularity of a row.
type revenueCell struct {
	row    string
	date   string
	rateBP int64
	method string
}

// revenueAcc accumulates the rows of a revenue report.
type revenueAcc struct {
	rows  map[string]*RevenueRow
	cells map[revenueCell]money.Cents
	// items counts the movements (or bookings) once each: a row counts
	// those touching it, so with group_by vat_rate a movement split over two
	// rates counts in both rows but once in the totals.
	items int
}

func newRevenueAcc() *revenueAcc {
	return &revenueAcc{rows: map[string]*RevenueRow{}, cells: map[revenueCell]money.Cents{}}
}

func (a *revenueAcc) row(key, label string) *RevenueRow {
	r, ok := a.rows[key]
	if !ok {
		r = &RevenueRow{Key: key, Label: label}
		a.rows[key] = r
	}
	return r
}

// finish splits the cells and returns the rows ordered by less and their
// totals (the sum of the rows; Count is the number of distinct items).
func (a *revenueAcc) finish(less func(a, b *RevenueRow) int) ([]RevenueRow, RevenueRow) {
	for c, gross := range a.cells {
		taxable, tax := money.VATSplit(gross, c.rateBP)
		r := a.rows[c.row]
		r.Taxable += taxable
		r.Tax += tax
	}
	list := make([]*RevenueRow, 0, len(a.rows))
	for _, r := range a.rows {
		list = append(list, r)
	}
	slices.SortFunc(list, less)
	out := make([]RevenueRow, 0, len(list))
	tot := RevenueRow{Key: "total", Label: "Totale"}
	for _, r := range list {
		out = append(out, *r)
		tot.Gross += r.Gross
		tot.Taxable += r.Taxable
		tot.Tax += r.Tax
		tot.Refunds += r.Refunds
	}
	tot.Count = a.items
	return out, tot
}

// timeKey returns the key and label of d for the time groupings.
func timeKey(d calendar.Date, by RevenueGroupBy) (string, string) {
	switch by {
	case RevenueByWeek:
		return WeekKey(d), weekLabel(d)
	case RevenueByMonth:
		return MonthKey(d), monthLabel(d)
	default:
		return d.String(), dayLabel(d)
	}
}

// orderedBy sorts rows by the position of their key in order (unknown keys
// last, by key).
func orderedBy(order []string) func(a, b *RevenueRow) int {
	pos := func(k string) int {
		if i := slices.Index(order, k); i >= 0 {
			return i
		}
		return len(order)
	}
	return func(a, b *RevenueRow) int {
		return cmp.Or(cmp.Compare(pos(a.Key), pos(b.Key)), cmp.Compare(a.Key, b.Key))
	}
}

func byKey(a, b *RevenueRow) int { return cmp.Compare(a.Key, b.Key) }

func byRate(a, b *RevenueRow) int {
	ra, _ := strconv.ParseInt(a.Key, 10, 64)
	rb, _ := strconv.ParseInt(b.Key, 10, 64)
	return cmp.Compare(ra, rb)
}

// sharesOf returns the VAT shares of a payment, or the whole amount at
// defaultRateBP when it has none or they do not add up (fiscal.BuildReport).
func sharesOf(pm *store.ReportPayment, defaultRateBP int64) []store.VATShare {
	var sum money.Cents
	for _, sh := range pm.VATBreakdown {
		sum += sh.Gross
	}
	if len(pm.VATBreakdown) == 0 || sum != pm.Amount {
		return []store.VATShare{{RateBP: defaultRateBP, Gross: pm.Amount}}
	}
	return pm.VATBreakdown
}

// buildRevenueCollected aggregates succeeded payments (refunds negative) by
// groupBy. Count is the number of movements; Refunds is the (positive)
// total of the refunds, already subtracted from Gross.
func buildRevenueCollected(p Period, payments []store.ReportPayment, groupBy RevenueGroupBy, defaultRateBP int64) *Revenue {
	a := newRevenueAcc()
	for i := range payments {
		pm := &payments[i]
		if !p.Contains(pm.BusinessDate) {
			continue
		}
		channel := ChannelCounter
		if pm.Channel != nil && *pm.Channel != "" {
			channel = *pm.Channel
		}
		shares := sharesOf(pm, defaultRateBP)
		a.items++
		sign := money.Cents(1)
		if pm.Direction == store.PaymentDirectionRefund {
			sign = -1
		}
		var keyOf func(rate int64) (string, string)
		switch groupBy {
		case RevenueByChannel:
			keyOf = func(int64) (string, string) { return channel, labelOr(channelLabels, channel) }
		case RevenueByPaymentMethod:
			keyOf = func(int64) (string, string) { return pm.Method, labelOr(methodLabels, pm.Method) }
		case RevenueByVATRate:
			keyOf = func(rate int64) (string, string) { return strconv.FormatInt(rate, 10), rateLabel(rate) }
		default:
			k, l := timeKey(pm.BusinessDate, groupBy)
			keyOf = func(int64) (string, string) { return k, l }
		}
		counted := map[string]bool{}
		for _, sh := range shares {
			k, l := keyOf(sh.RateBP)
			r := a.row(k, l)
			if !counted[k] {
				counted[k] = true
				r.Count++
			}
			r.Gross += sign * sh.Gross
			if sign < 0 {
				r.Refunds += sh.Gross
			}
			a.cells[revenueCell{row: k, date: pm.BusinessDate.String(), rateBP: sh.RateBP, method: pm.Method}] += sign * sh.Gross
		}
	}
	less := byKey
	switch groupBy {
	case RevenueByChannel:
		less = orderedBy(channelOrder)
	case RevenueByPaymentMethod:
		less = orderedBy(methodOrder)
	case RevenueByVATRate:
		less = byRate
	}
	rows, tot := a.finish(less)
	return &Revenue{Period: p, GroupBy: groupBy, Basis: BasisCollected, Rows: rows, Totals: tot}
}

// buildRevenueBooked aggregates the value of the bookings in a booked status
// confirmed in p (confirmed_at in loc) by groupBy (day, week, month,
// channel, vat_rate). Count is the number of bookings.
func buildRevenueBooked(p Period, bookings []store.ReportBooking, groupBy RevenueGroupBy, loc *time.Location, defaultRateBP int64) *Revenue {
	a := newRevenueAcc()
	for i := range bookings {
		b := &bookings[i]
		day, ok := bookedOn(b, p, loc)
		if !ok {
			continue
		}
		shares := payment.SplitBySnapshot(b.CurrentPrice(), b.TotalAmount, defaultRateBP)
		a.items++
		var keyOf func(rate int64) (string, string)
		switch groupBy {
		case RevenueByChannel:
			keyOf = func(int64) (string, string) { return b.Channel, labelOr(channelLabels, b.Channel) }
		case RevenueByVATRate:
			keyOf = func(rate int64) (string, string) { return strconv.FormatInt(rate, 10), rateLabel(rate) }
		default:
			k, l := timeKey(day, groupBy)
			keyOf = func(int64) (string, string) { return k, l }
		}
		if len(shares) == 0 { // zero-value booking: counted, no VAT
			k, l := keyOf(defaultRateBP)
			a.row(k, l).Count++
			continue
		}
		counted := map[string]bool{}
		for _, sh := range shares {
			k, l := keyOf(sh.RateBP)
			r := a.row(k, l)
			if !counted[k] {
				counted[k] = true
				r.Count++
			}
			r.Gross += sh.Gross
			a.cells[revenueCell{row: k, rateBP: sh.RateBP}] += sh.Gross
		}
	}
	less := byKey
	switch groupBy {
	case RevenueByChannel:
		less = orderedBy(channelOrder)
	case RevenueByVATRate:
		less = byRate
	}
	rows, tot := a.finish(less)
	return &Revenue{Period: p, GroupBy: groupBy, Basis: BasisBooked, Rows: rows, Totals: tot}
}

// buildRevenueCatalog makes the pack or extra rows of the booked basis:
// one row per item, split at the item VAT rate (or the default rate).
func buildRevenueCatalog(p Period, groupBy RevenueGroupBy, items []store.ReportCatalogRevenue, defaultRateBP int64) *Revenue {
	a := newRevenueAcc()
	for _, it := range items {
		rate := defaultRateBP
		if it.VATRateBP != nil {
			rate = *it.VATRateBP
		}
		r := a.row(it.Code, i18nName(it.Name, it.Code))
		r.Count += it.Count
		a.items += it.Count
		r.Gross += it.Gross
		a.cells[revenueCell{row: it.Code, rateBP: rate}] += it.Gross
	}
	rows, tot := a.finish(byKey)
	return &Revenue{Period: p, GroupBy: groupBy, Basis: BasisBooked, Rows: rows, Totals: tot}
}

// bookedOn returns the confirmation date of b in loc when b is in a booked
// status and was confirmed inside p.
func bookedOn(b *store.ReportBooking, p Period, loc *time.Location) (calendar.Date, bool) {
	if b.ConfirmedAt == nil || !slices.Contains(store.ReportBookedStatuses, b.Status) {
		return calendar.Date{}, false
	}
	d := calendar.In(*b.ConfirmedAt, loc)
	return d, p.Contains(d)
}

// soldOn returns the sale date of b (created_at in loc) when it is inside p.
func soldOn(b *store.ReportBooking, p Period, loc *time.Location) (calendar.Date, bool) {
	d := calendar.In(b.CreatedAt, loc)
	return d, p.Contains(d)
}
