package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestColumnName_Table(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA", 16383: "XFD"} {
		if got := ColumnName(i); got != want {
			t.Errorf("ColumnName(%d) = %s, want %s", i, got, want)
		}
	}
}

func TestSheetName_Sanitised(t *testing.T) {
	tests := map[string]string{
		"":                        "Foglio1",
		"Ricavi":                  "Ricavi",
		"a/b:c*d?e[f]g\\h":        "a_b_c_d_e_f_g_h",
		"'quoted'":                "quoted",
		strings.Repeat("x", 40):   strings.Repeat("x", 31),
		"Prenotazioni 2027-07-01": "Prenotazioni 2027-07-01",
	}
	for in, want := range tests {
		if got := SheetName(in); got != want {
			t.Errorf("SheetName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCell_Table(t *testing.T) {
	tests := []struct {
		in   string
		want Cell
	}{
		{"", Cell{}},
		{"Lido", Text("Lido")},
		{"1234,56", Decimal2(123456)},
		{"-0,05", Decimal2(-5)},
		{"0,00", Decimal2(0)},
		{"42", Int(42)},
		{"-7", Int(-7)},
		{"0", Int(0)},
		{"00123", Text("00123")},
		{"12,5", Decimal2(1250)},
		{"5,5", Decimal2(550)},
		{"-0,5", Decimal2(-50)},
		{"1,234", Text("1,234")},
		{"12,", Text("12,")},
		{"1.234,56", Text("1.234,56")},
		{"31/12/2027", Date(2027, time.December, 31)},
		{"31/02/2027", Text("31/02/2027")},
		{"'=HYPERLINK(1)", Text("=HYPERLINK(1)")},
		{"'-negativo", Text("-negativo")},
		{"'ciao", Text("'ciao")},
		{"LD-2027-000001", Text("LD-2027-000001")},
		{"1234567890123456", Text("1234567890123456")},
	}
	for _, tt := range tests {
		got := ParseCell(tt.in)
		if got != tt.want {
			t.Errorf("ParseCell(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestWriter_ValidPackage(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, Options{Sheet: "Prova", Header: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Row(Text("Codice"), Text("Importo"), Text("Data"), Text("Pezzi"))
	_ = w.Row(Text("=1+1 <&> \"x\""), Decimal2(-123456), Date(2027, time.July, 1), Int(3))
	_ = w.Row(Cell{}, Decimal2(5))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"[Content_Types].xml": true, "_rels/.rels": true, "docProps/app.xml": true, "xl/workbook.xml": true,
		"xl/_rels/workbook.xml.rels": true, "xl/styles.xml": true, "xl/worksheets/sheet1.xml": true,
	}
	for _, f := range zr.File {
		if !want[f.Name] {
			t.Errorf("unexpected part %s", f.Name)
		}
		delete(want, f.Name)
		rc, _ := f.Open()
		raw, _ := io.ReadAll(rc)
		_ = rc.Close()
		// Every part is well-formed XML.
		d := xml.NewDecoder(bytes.NewReader(raw))
		for {
			if _, err := d.Token(); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
		if f.Name == "xl/workbook.xml" && !strings.Contains(string(raw), `name="Prova"`) {
			t.Error("sheet name missing")
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing parts %v", want)
	}
	rows, err := ReadSheet(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0].Style != "1" || rows[0][0].Value != "Codice" {
		t.Fatalf("header %+v", rows)
	}
	r := rows[1]
	if r[0].Type != "inlineStr" || r[0].Value != "=1+1 <&> \"x\"" {
		t.Fatalf("text cell %+v", r[0])
	}
	if r[1].Value != "-1234.56" || r[1].Style != "3" {
		t.Fatalf("decimal cell %+v", r[1])
	}
	if r[2].Value != "46569" || r[2].Style != "4" { // 2027-07-01
		t.Fatalf("date cell %+v", r[2])
	}
	if r[3].Value != "3" || r[3].Style != "2" {
		t.Fatalf("int cell %+v", r[3])
	}
	if len(rows[2]) != 1 || rows[2][0].Ref != "B3" {
		t.Fatalf("empty cell skipped: %+v", rows[2])
	}
}

func TestFromCSV_RoundTrip(t *testing.T) {
	src := "\xEF\xBB\xBFCodice;Cliente;Totale;Dal\r\nLD-1;'=cmd;1234,50;01/07/2027\r\nTotale;;1234,50;\r\n"
	var buf bytes.Buffer
	if err := FromCSV(&buf, strings.NewReader(src), "Saldi"); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadSheet(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := [][]string{}
	for _, r := range rows {
		var vs []string
		for _, c := range r {
			vs = append(vs, c.Ref+"="+c.Value)
		}
		got = append(got, vs)
	}
	want := [][]string{
		{"A1=Codice", "B1=Cliente", "C1=Totale", "D1=Dal"},
		{"A2=LD-1", "B2==cmd", "C2=1234.50", "D2=46569"},
		{"A3=Totale", "C3=1234.50"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows %v", got)
	}
	for i := range want {
		if strings.Join(got[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
	if rows[1][1].Type != "inlineStr" {
		t.Fatal("formula-like text must stay an inline string")
	}
}

// TestWriter_TextProperty: any text survives the round trip (invalid XML
// characters become U+FFFD) and the package stays well-formed.
func TestWriter_TextProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringN(0, 200, -1).Draw(t, "text")
		var buf bytes.Buffer
		w, _ := NewWriter(&buf, Options{Sheet: s})
		_ = w.Row(Text(s), Int(rapid.Int64().Draw(t, "n")))
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		rows, err := ReadSheet(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if s != "" && len(rows) != 1 {
			t.Fatalf("rows %d", len(rows))
		}
	})
}

// TestFromCSV_RatesAreNumbers: a corrispettivi file mixes integer and
// one-decimal VAT rates ("22", "5,5") in the same column: every rate is a
// number cell (the column sorts and sums in Excel), never a text cell.
func TestFromCSV_RatesAreNumbers(t *testing.T) {
	src := "Aliquota;Imponibile;Imposta\n22;1000,00;220,00\n5,5;200,00;11,00\n10;50,00;5,00\n"
	var buf bytes.Buffer
	if err := FromCSV(&buf, strings.NewReader(src), "Corrispettivi"); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadSheet(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A2": "22", "A3": "5.50", "A4": "10", "C3": "11.00"}
	for _, r := range rows[1:] {
		for _, c := range r {
			if w, ok := want[c.Ref]; ok {
				if c.Type == "inlineStr" || c.Value != w {
					t.Fatalf("%s = %q (%s), want number %q", c.Ref, c.Value, c.Type, w)
				}
			}
		}
	}
}
