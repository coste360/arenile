package report

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWriteRevenueCSV_ItalianFormat(t *testing.T) {
	r := &Revenue{
		Period: per("2027-07-01", "2027-07-31"), GroupBy: RevenueByChannel, Basis: BasisCollected,
		Rows: []RevenueRow{
			{Key: "desk", Label: "Cassa", Count: 2, Gross: 123456, Taxable: 101193, Tax: 22263, Refunds: 1000},
		},
		Totals: RevenueRow{Key: "total", Label: "Totale", Count: 2, Gross: 123456, Taxable: 101193, Tax: 22263, Refunds: 1000},
	}
	var buf bytes.Buffer
	if err := WriteRevenueCSV(&buf, r); err != nil {
		t.Fatal(err)
	}
	want := "\xEF\xBB\xBF" +
		"Chiave;Gruppo;Movimenti;Lordo;Imponibile;IVA;Rimborsi\r\n" +
		"desk;Cassa;2;1234,56;1011,93;222,63;10,00\r\n" +
		"total;Totale;2;1234,56;1011,93;222,63;10,00\r\n"
	if got := buf.String(); got != want {
		t.Fatalf("csv =\n%q\nwant\n%q", got, want)
	}
}

func TestText_NeutralisesFormulas(t *testing.T) {
	cases := map[string]string{"=1+1": "'=1+1", "+39 333": "'+39 333", "@SUM(A1)": "'@SUM(A1)", "Mario": "Mario", "": ""}
	for in, want := range cases {
		if got := text(in); got != want {
			t.Fatalf("text(%q) = %q, want %q", in, got, want)
		}
	}
	var buf bytes.Buffer
	_ = WriteTopCustomersCSV(&buf, &TopCustomers{Rows: []TopCustomerRow{{Rank: 1, DisplayName: "=HYPERLINK(\"x\")"}}})
	if !strings.Contains(buf.String(), "'=HYPERLINK") {
		t.Fatalf("formula not neutralised: %q", buf.String())
	}
}

func TestWriteComparisonCSV_MetricsBySeries(t *testing.T) {
	cur := spanOf(per("2027-07-01", "2027-07-02"), func(int) dayFacts { return dayFacts{capacity: 10, occupied: 5, collected: 1050} })
	prev := spanOf(per("2026-07-01", "2026-07-02"), func(int) dayFacts { return dayFacts{capacity: 10, occupied: 4, collected: 1000} })
	var buf bytes.Buffer
	if err := WriteComparisonCSV(&buf, buildComparison(cur, prev, GranularityDay, AlignCalendar)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Periodo;01/07/2027 - 02/07/2027;01/07/2026 - 02/07/2026;;\r\n",
		"Occupazione %;50,00;40,00;10,00;25,00\r\n",
		"Incassato;21,00;20,00;1,00;5,00\r\n",
		"Unità rilasciate;0;0;0;\r\n",
		"01/07/2027;01/07/2027;01/07/2026;10,50;10,00;50,00;40,00;0;0\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

// TestWriters_AllReports runs every writer on a populated report and checks
// the shared format (BOM, header, CRLF, a totals line where defined).
func TestWriters_AllReports(t *testing.T) {
	occ := buildOccupancy(occupancyFixture(), OccupancyByZone)
	writers := map[string]func(io.Writer) error{
		"summary": func(w io.Writer) error {
			return WriteSummaryCSV(w, &Summary{Period: per("2027-07-01", "2027-07-31"), GeneratedAt: time.Now()})
		},
		"occupancy": func(w io.Writer) error { return WriteOccupancyCSV(w, occ) },
		"bookings": func(w io.Writer) error {
			return WriteBookingsCSV(w, &Bookings{Totals: BookingsRow{Key: "total", Label: "Totale"}})
		},
		"cancellations": func(w io.Writer) error {
			return WriteCancellationsCSV(w, &Cancellations{Totals: CancellationsRow{Key: "total", Label: "Totale"}})
		},
		"passes": func(w io.Writer) error {
			return WriteSeasonPassesCSV(w, &SeasonPasses{PassesSold: 3, PassesValue: 450000})
		},
		"balances": func(w io.Writer) error {
			return WriteOpenBalancesCSV(w, &OpenBalances{Rows: []OpenBalanceRow{{Code: "AB12", DisplayName: "Mario Rossi", Status: "confirmed", StartsOn: d("2027-07-01"), EndsOn: d("2027-07-07"), Total: 10000, Paid: 2500, Balance: 7500}}, TotalBalance: 7500})
		},
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := write(&buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !strings.HasPrefix(out, "\xEF\xBB\xBF") || !strings.HasSuffix(out, "\r\n") || strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
				t.Fatalf("format: %q", out)
			}
		})
	}
	var buf bytes.Buffer
	_ = WriteOccupancyCSV(&buf, occ)
	if !strings.Contains(buf.String(), "total;Totale;44;15;4;2;1;34,09\r\n") {
		t.Fatalf("occupancy totals: %q", buf.String())
	}
	buf.Reset()
	_ = WriteOpenBalancesCSV(&buf, &OpenBalances{Rows: []OpenBalanceRow{{Code: "AB12", DisplayName: "Mario Rossi", Status: "confirmed", StartsOn: d("2027-07-01"), EndsOn: d("2027-07-07"), Total: 10000, Paid: 2500, Balance: 7500}}, TotalBalance: 7500})
	if !strings.Contains(buf.String(), "AB12;Mario Rossi;Confermata;01/07/2027;07/07/2027;100,00;25,00;75,00\r\nTotale;;;;;100,00;25,00;75,00\r\n") {
		t.Fatalf("balances: %q", buf.String())
	}
}
