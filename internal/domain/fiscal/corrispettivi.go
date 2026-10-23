package fiscal

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the corrispettivi export: BuildReport is pure (no DB,
// no clock) and WriteCSV only formats. The service loads the payments and
// calls them.

// GroupBy is the period of a corrispettivi row.
type GroupBy string

// Grouping periods.
const (
	GroupByDay   GroupBy = "day"
	GroupByMonth GroupBy = "month"
)

// Valid reports whether g is a known grouping.
func (g GroupBy) Valid() bool { return g == GroupByDay || g == GroupByMonth }

// Row is the takings of one period, VAT rate and payment method. Refunds
// are negative. Taxable + Tax == Gross (INV-16), split with money.VATSplit
// (half-even) on the row gross, i.e. the scorporo of the daily (or monthly)
// total per rate.
type Row struct {
	Period  string // YYYY-MM-DD or YYYY-MM
	RateBP  int64
	Method  string
	Count   int // payments (and refunds) contributing to the row
	Gross   money.Cents
	Taxable money.Cents
	Tax     money.Cents
}

// RateTotal is the total of one VAT rate over the whole report: the sum of
// its rows, so that the export always adds up line by line.
type RateTotal struct {
	RateBP  int64
	Gross   money.Cents
	Taxable money.Cents
	Tax     money.Cents
}

// Report is the corrispettivi export of a date range.
type Report struct {
	From       calendar.Date
	To         calendar.Date
	GroupBy    GroupBy
	Currency   string
	Rows       []Row
	Totals     []RateTotal
	GrandTotal money.Cents
}

// BuildReport aggregates succeeded payments (refunds negative) by period,
// VAT rate and method. The gross per rate comes from the VAT split frozen
// on each payment (payments.vat_breakdown); a payment without a split is
// attributed entirely to defaultRateBP. GrandTotal always equals
// sum(payments in) - sum(refunds) of the input (reconciliation property).
func BuildReport(payments []store.Payment, from, to calendar.Date, groupBy GroupBy, currency string, defaultRateBP int64) *Report {
	type key struct {
		period string
		rate   int64
		method string
	}
	type acc struct {
		gross money.Cents
		count int
	}
	groups := map[key]*acc{}
	for i := range payments {
		pm := &payments[i]
		if pm.Status != store.PaymentSucceeded {
			continue
		}
		period := periodOf(pm.BusinessDate, groupBy)
		sign := money.Cents(1)
		if pm.Direction == store.PaymentDirectionRefund {
			sign = -1
		}
		for _, sh := range sharesOf(pm, defaultRateBP) {
			k := key{period, sh.RateBP, pm.Method}
			a := groups[k]
			if a == nil {
				a = &acc{}
				groups[k] = a
			}
			a.gross += sign * sh.Gross
			a.count++
		}
	}
	r := &Report{From: from, To: to, GroupBy: groupBy, Currency: currency, Rows: []Row{}, Totals: []RateTotal{}}
	totals := map[int64]*RateTotal{}
	for k, a := range groups {
		taxable, tax := money.VATSplit(a.gross, k.rate)
		r.Rows = append(r.Rows, Row{Period: k.period, RateBP: k.rate, Method: k.method, Count: a.count, Gross: a.gross, Taxable: taxable, Tax: tax})
		t := totals[k.rate]
		if t == nil {
			t = &RateTotal{RateBP: k.rate}
			totals[k.rate] = t
		}
		t.Gross += a.gross
		t.Taxable += taxable
		t.Tax += tax
		r.GrandTotal += a.gross
	}
	sort.Slice(r.Rows, func(i, j int) bool {
		a, b := r.Rows[i], r.Rows[j]
		if a.Period != b.Period {
			return a.Period < b.Period
		}
		if a.RateBP != b.RateBP {
			return a.RateBP < b.RateBP
		}
		return a.Method < b.Method
	})
	for _, t := range totals {
		r.Totals = append(r.Totals, *t)
	}
	sort.Slice(r.Totals, func(i, j int) bool { return r.Totals[i].RateBP < r.Totals[j].RateBP })
	return r
}

// sharesOf returns the frozen VAT shares of pm, or the whole amount at
// defaultRateBP when the payment has none (or a split that does not add up,
// which never happens for payments recorded by the payment service).
func sharesOf(pm *store.Payment, defaultRateBP int64) []store.VATShare {
	var sum money.Cents
	for _, sh := range pm.VATBreakdown {
		sum += sh.Gross
	}
	if len(pm.VATBreakdown) == 0 || sum != pm.Amount {
		return []store.VATShare{{RateBP: defaultRateBP, Gross: pm.Amount}}
	}
	return pm.VATBreakdown
}

func periodOf(d calendar.Date, g GroupBy) string {
	if g == GroupByMonth {
		return fmt.Sprintf("%04d-%02d", d.Year(), int(d.Month()))
	}
	return d.String()
}

// methodLabels are the Italian labels of the payment methods in the CSV
// export (the file is for the Italian accountant; Italian is the source
// language).
var methodLabels = map[string]string{
	store.PaymentMethodCash:         "Contanti", //nolint:misspell // Italian
	store.PaymentMethodCard:         "Carta",
	store.PaymentMethodBankTransfer: "Bonifico",
	store.PaymentMethodWallet:       "Wallet",
	store.PaymentMethodCredit:       "Credito cliente",
	store.PaymentMethodOther:        "Altro",
}

// WriteCSV writes r as CSV for the accountant (spec 14.2): UTF-8 with BOM,
// ';' separator, amounts in euro with decimal comma and no thousands
// separator, rates as percentages ("22", "5,5"). After the rows come one
// "Totale" line per VAT rate and the grand total.
func WriteCSV(w io.Writer, r *Report) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString("\xEF\xBB\xBF"); err != nil {
		return fmt.Errorf("fiscal: write csv: %w", err)
	}
	line := func(fields ...string) {
		for i, f := range fields {
			if i > 0 {
				_ = bw.WriteByte(';')
			}
			_, _ = bw.WriteString(csvField(f))
		}
		_, _ = bw.WriteString("\r\n")
	}
	period := "Giorno"
	if r.GroupBy == GroupByMonth {
		period = "Mese"
	}
	line(period, "Aliquota IVA %", "Metodo di pagamento", "Movimenti", "Lordo", "Imponibile", "Imposta")
	for _, row := range r.Rows {
		label := methodLabels[row.Method]
		if label == "" {
			label = row.Method
		}
		line(row.Period, FormatRate(row.RateBP), label, strconv.Itoa(row.Count),
			FormatEuro(row.Gross), FormatEuro(row.Taxable), FormatEuro(row.Tax))
	}
	for _, t := range r.Totals {
		line("Totale", FormatRate(t.RateBP), "", "", FormatEuro(t.Gross), FormatEuro(t.Taxable), FormatEuro(t.Tax))
	}
	line("Totale generale", "", "", "", FormatEuro(r.GrandTotal), "", "")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("fiscal: write csv: %w", err)
	}
	return nil
}

// FormatEuro formats cents as euro with a decimal comma: 123456 -> "1234,56",
// -5 -> "-0,05".
func FormatEuro(c money.Cents) string {
	v := int64(c)
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d,%02d", sign, v/100, v%100)
}

// FormatRate formats basis points as a percentage with a decimal comma and
// no trailing zeros: 2200 -> "22", 550 -> "5,5", 0 -> "0".
func FormatRate(bp int64) string {
	whole, frac := bp/100, bp%100
	if frac == 0 {
		return strconv.FormatInt(whole, 10)
	}
	s := fmt.Sprintf("%d,%02d", whole, frac)
	if s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return s
}

// csvField quotes f when it contains the separator, quotes or newlines.
func csvField(f string) string {
	needs := false
	for _, r := range f {
		if r == ';' || r == '"' || r == '\n' || r == '\r' {
			needs = true
			break
		}
	}
	if !needs {
		return f
	}
	out := []byte{'"'}
	for i := 0; i < len(f); i++ {
		if f[i] == '"' {
			out = append(out, '"')
		}
		out = append(out, f[i])
	}
	return string(append(out, '"'))
}
