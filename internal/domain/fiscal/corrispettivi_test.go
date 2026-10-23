package fiscal

import (
	"bytes"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func pay(day, method, dir string, shares ...store.VATShare) store.Payment {
	var amount money.Cents
	for _, s := range shares {
		amount += s.Gross
	}
	return store.Payment{
		ID: store.NewID(), Method: method, Direction: dir, Amount: amount, Status: store.PaymentSucceeded,
		BusinessDate: calendar.MustParse(day), VATBreakdown: shares,
	}
}

func sh(rate int64, gross money.Cents) store.VATShare {
	tx, tax := money.VATSplit(gross, rate)
	return store.VATShare{RateBP: rate, Gross: gross, Taxable: tx, Tax: tax}
}

func TestBuildReport_Scenarios(t *testing.T) {
	from, to := calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-31")
	payments := []store.Payment{
		pay("2027-07-01", store.PaymentMethodCash, store.PaymentDirectionIn, sh(2200, 1220)),
		pay("2027-07-01", store.PaymentMethodCash, store.PaymentDirectionIn, sh(2200, 1220), sh(1000, 1100)),
		pay("2027-07-01", store.PaymentMethodCard, store.PaymentDirectionIn, sh(2200, 610)),
		pay("2027-07-02", store.PaymentMethodCash, store.PaymentDirectionRefund, sh(2200, 610)),
		// No split: default rate.
		{ID: "x", Method: store.PaymentMethodOther, Direction: store.PaymentDirectionIn, Amount: 500, Status: store.PaymentSucceeded, BusinessDate: calendar.MustParse("2027-07-02")},
		// Not succeeded: ignored.
		{ID: "y", Method: store.PaymentMethodCard, Direction: store.PaymentDirectionIn, Amount: 999, Status: store.PaymentPending, BusinessDate: calendar.MustParse("2027-07-02")},
	}
	t.Run("daily", func(t *testing.T) {
		r := BuildReport(payments, from, to, GroupByDay, "EUR", 2200)
		want := []Row{
			{Period: "2027-07-01", RateBP: 1000, Method: "cash", Count: 1, Gross: 1100, Taxable: 1000, Tax: 100},
			{Period: "2027-07-01", RateBP: 2200, Method: "card", Count: 1, Gross: 610, Taxable: 500, Tax: 110},
			{Period: "2027-07-01", RateBP: 2200, Method: "cash", Count: 2, Gross: 2440, Taxable: 2000, Tax: 440},
			{Period: "2027-07-02", RateBP: 2200, Method: "cash", Count: 1, Gross: -610, Taxable: -500, Tax: -110},
			{Period: "2027-07-02", RateBP: 2200, Method: "other", Count: 1, Gross: 500, Taxable: 410, Tax: 90},
		}
		if len(r.Rows) != len(want) {
			t.Fatalf("rows %+v", r.Rows)
		}
		for i := range want {
			if r.Rows[i] != want[i] {
				t.Fatalf("row %d: got %+v, want %+v", i, r.Rows[i], want[i])
			}
		}
		if r.GrandTotal != 4040 {
			t.Fatalf("grand total %d", r.GrandTotal)
		}
		if len(r.Totals) != 2 || r.Totals[0] != (RateTotal{1000, 1100, 1000, 100}) || r.Totals[1] != (RateTotal{2200, 2940, 2410, 530}) {
			t.Fatalf("totals %+v", r.Totals)
		}
	})
	t.Run("monthly", func(t *testing.T) {
		r := BuildReport(payments, from, to, GroupByMonth, "EUR", 2200)
		if len(r.Rows) != 4 || r.Rows[0].Period != "2027-07" || r.GrandTotal != 4040 {
			t.Fatalf("monthly %+v", r)
		}
		for _, row := range r.Rows {
			if row.Method == "cash" && row.RateBP == 2200 && (row.Gross != 1830 || row.Count != 3) {
				t.Fatalf("cash 22%%: %+v", row)
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		r := BuildReport(nil, from, to, GroupByDay, "EUR", 2200)
		if r.Rows == nil || r.Totals == nil || r.GrandTotal != 0 {
			t.Fatalf("empty %+v", r)
		}
	})
}

// TestBuildReport_ReconcilesProperty: for any set of payments and refunds
// the grand total, the sum of the rows and the sum of the per-rate totals
// all equal sum(in) - sum(refunds), and every row satisfies INV-16.
func TestBuildReport_ReconcilesProperty(t *testing.T) {
	rates := []int64{0, 400, 500, 1000, 2200}
	methods := []string{store.PaymentMethodCash, store.PaymentMethodCard, store.PaymentMethodBankTransfer, store.PaymentMethodCredit}
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 60).Draw(t, "n")
		var ps []store.Payment
		var ledger money.Cents
		for i := 0; i < n; i++ {
			dir := store.PaymentDirectionIn
			if rapid.Bool().Draw(t, "refund") {
				dir = store.PaymentDirectionRefund
			}
			k := rapid.IntRange(0, 3).Draw(t, "shares")
			var shares []store.VATShare
			for j := 0; j < k; j++ {
				shares = append(shares, sh(rapid.SampledFrom(rates).Draw(t, "rate"), money.Cents(rapid.Int64Range(1, 1_000_000).Draw(t, "gross"))))
			}
			p := pay(calendar.MustParse("2027-06-28").AddDays(rapid.IntRange(0, 8).Draw(t, "day")).String(),
				rapid.SampledFrom(methods).Draw(t, "method"), dir, shares...)
			if k == 0 {
				p.Amount = money.Cents(rapid.Int64Range(1, 1_000_000).Draw(t, "amount"))
			}
			ps = append(ps, p)
			ledger += p.Signed()
		}
		for _, g := range []GroupBy{GroupByDay, GroupByMonth} {
			r := BuildReport(ps, calendar.MustParse("2027-06-28"), calendar.MustParse("2027-07-06"), g, "EUR", 2200)
			var rows, totals, taxable, tax money.Cents
			for _, row := range r.Rows {
				if row.Taxable+row.Tax != row.Gross {
					t.Fatalf("INV-16 %+v", row)
				}
				rows += row.Gross
				taxable += row.Taxable
				tax += row.Tax
			}
			var tTaxable, tTax money.Cents
			for _, tt := range r.Totals {
				totals += tt.Gross
				tTaxable += tt.Taxable
				tTax += tt.Tax
			}
			if r.GrandTotal != ledger || rows != ledger || totals != ledger || taxable != tTaxable || tax != tTax {
				t.Fatalf("%s: grand %d rows %d totals %d ledger %d", g, r.GrandTotal, rows, totals, ledger)
			}
		}
	})
}

func TestFormat_Table(t *testing.T) {
	euros := map[money.Cents]string{0: "0,00", 5: "0,05", 123456: "1234,56", -5: "-0,05", -100: "-1,00"}
	for c, want := range euros {
		if got := FormatEuro(c); got != want {
			t.Errorf("FormatEuro(%d) = %q, want %q", c, got, want)
		}
	}
	rates := map[int64]string{0: "0", 2200: "22", 550: "5,5", 1025: "10,25", 400: "4"}
	for bp, want := range rates {
		if got := FormatRate(bp); got != want {
			t.Errorf("FormatRate(%d) = %q, want %q", bp, got, want)
		}
	}
}

func TestWriteCSV_ItalianFormat(t *testing.T) {
	r := BuildReport([]store.Payment{
		pay("2027-07-01", store.PaymentMethodCash, store.PaymentDirectionIn, sh(2200, 123456)),
		pay("2027-07-01", store.PaymentMethodCard, store.PaymentDirectionRefund, sh(550, 1055)),
	}, calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-01"), GroupByDay, "EUR", 2200)
	var buf bytes.Buffer
	if err := WriteCSV(&buf, r); err != nil {
		t.Fatal(err)
	}
	want := "\xEF\xBB\xBF" +
		"Giorno;Aliquota IVA %;Metodo di pagamento;Movimenti;Lordo;Imponibile;Imposta\r\n" +
		"2027-07-01;5,5;Carta;1;-10,55;-10,00;-0,55\r\n" +
		"2027-07-01;22;Contanti;1;1234,56;1011,93;222,63\r\n" + //nolint:misspell // Italian
		"Totale;5,5;;;-10,55;-10,00;-0,55\r\n" +
		"Totale;22;;;1234,56;1011,93;222,63\r\n" +
		"Totale generale;;;;1224,01;;\r\n"
	if got := buf.String(); got != want {
		t.Fatalf("csv:\n%q\nwant\n%q", got, want)
	}
	if !strings.HasPrefix(buf.String(), "\xEF\xBB\xBF") {
		t.Fatal("missing BOM")
	}
}

func TestCSVField_Quoting(t *testing.T) {
	cases := map[string]string{"plain": "plain", "a;b": `"a;b"`, `say "hi"`: `"say ""hi"""`}
	for in, want := range cases {
		if got := csvField(in); got != want {
			t.Errorf("csvField(%q) = %q, want %q", in, got, want)
		}
	}
}
