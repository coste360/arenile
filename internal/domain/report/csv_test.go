package report

import (
	"bytes"
	"testing"
)

func TestCSVWriter_Format(t *testing.T) {
	var buf bytes.Buffer
	w := NewCSVWriter(&buf)
	w.Row("Zona", "Occupazione %", "Ricavo")
	w.Row("Prima fila; vista mare", FormatBP(8125), FormatEuro(123456))
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	want := "\xEF\xBB\xBFZona;Occupazione %;Ricavo\r\n\"Prima fila; vista mare\";81,25;1234,56\r\n"
	if got := buf.String(); got != want {
		t.Fatalf("csv = %q, want %q", got, want)
	}
}

func TestFormatBP_Scenarios(t *testing.T) {
	cases := map[int64]string{0: "0,00", 5: "0,05", 10000: "100,00", 1234: "12,34", -250: "-2,50"}
	for in, want := range cases {
		if got := FormatBP(in); got != want {
			t.Fatalf("FormatBP(%d) = %q, want %q", in, got, want)
		}
	}
	if FormatCount(42) != "42" {
		t.Fatal("FormatCount")
	}
}
