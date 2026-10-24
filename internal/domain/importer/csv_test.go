package importer

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

func TestDecodeCSV_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		in       string
		wantErr  string
		wantRows int
		wantCols []string
		check    func(t *testing.T, tb *table)
	}{
		{
			name: "semicolon with BOM and aliases", kind: store.ImportKindCustomers,
			in:       "\xef\xbb\xbfSurname;First Name;E-Mail;Cellulare\nRossi;Mario;m@x.it;333\n",
			wantRows: 1, wantCols: []string{colLastName, colFirstName, colEmail, colPhone},
		},
		{
			name: "comma separated with quotes", kind: store.ImportKindCustomers,
			in:       "cognome,note\n\"D'Amico, Anna\",\"riga 1\nriga 2\"\nBianchi,x\n",
			wantRows: 2, wantCols: []string{colLastName, colNotes},
			check: func(t *testing.T, tb *table) {
				if tb.rows[0].values[colNotes] != "riga 1\nriga 2" || tb.rows[1].line != 4 {
					t.Fatalf("rows %+v", tb.rows)
				}
			},
		},
		{
			name: "tab separated", kind: store.ImportKindCustomers,
			in: "cognome\temail\nRossi\tr@x.it\n", wantRows: 1, wantCols: []string{colLastName, colEmail},
		},
		{
			name: "windows-1252 converted", kind: store.ImportKindCustomers,
			in: "cognome;nome\nNicol\xf2;Andr\xe8\n", wantRows: 1, wantCols: []string{colLastName, colFirstName},
			check: func(t *testing.T, tb *table) {
				if !tb.converted || tb.rows[0].values[colLastName] != "Nicolò" || tb.rows[0].values[colFirstName] != "Andrè" {
					t.Fatalf("conversion %+v", tb.rows[0].values)
				}
			},
		},
		{
			name: "unknown columns kept as warnings", kind: store.ImportKindCustomers,
			in: "cognome;codice fiscale\nRossi;X\n", wantRows: 1, wantCols: []string{colLastName},
			check: func(t *testing.T, tb *table) {
				if len(tb.unknown) != 1 || tb.unknown[0] != "codice fiscale" {
					t.Fatalf("unknown %v", tb.unknown)
				}
			},
		},
		{name: "blank rows skipped", kind: store.ImportKindCustomers, in: "cognome\n\n;\nRossi\n", wantRows: 1, wantCols: []string{colLastName}},
		{name: "missing required column", kind: store.ImportKindCustomers, in: "nome;email\nMario;m@x.it\n", wantErr: "missing_column"},
		{name: "missing spot column", kind: store.ImportKindSeasonPasses, in: "cognome\nRossi\n", wantErr: "missing_column"},
		{name: "duplicate column", kind: store.ImportKindCustomers, in: "cognome;surname\nRossi;Rossi\n", wantErr: "duplicate_column"},
		{name: "empty", kind: store.ImportKindCustomers, in: "\xef\xbb\xbf  \n", wantErr: "empty_file"},
		{name: "header only", kind: store.ImportKindCustomers, in: "cognome\n", wantErr: "empty_file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb, err := decodeCSV(tt.kind, []byte(tt.in))
			if tt.wantErr != "" {
				var de *decodeError
				if !errors.As(err, &de) || de.code != tt.wantErr {
					t.Fatalf("err = %v, want %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(tb.rows) != tt.wantRows || strings.Join(tb.recognised(), ",") != strings.Join(tt.wantCols, ",") {
				t.Fatalf("rows %d cols %v", len(tb.rows), tb.recognised())
			}
			if tt.check != nil {
				tt.check(t, tb)
			}
		})
	}
}

func TestDecodeCSV_Limits(t *testing.T) {
	var b strings.Builder
	b.WriteString("cognome\n")
	for range MaxRows + 1 {
		b.WriteString("R\n")
	}
	if _, err := decodeCSV(store.ImportKindCustomers, []byte(b.String())); err == nil || !strings.Contains(err.Error(), "too_many_rows") {
		t.Fatalf("too many rows: %v", err)
	}
	big := make([]byte, MaxFileBytes+1)
	if _, err := decodeCSV(store.ImportKindCustomers, big); err == nil || !strings.Contains(err.Error(), "file_too_large") {
		t.Fatalf("too large: %v", err)
	}
}

func TestValidateRows_Scenarios(t *testing.T) {
	now := time.Date(2027, 6, 1, 10, 0, 0, 0, time.UTC)
	in := "cognome;email;telefono;lingua;consenso_marketing;data_consenso;etichette\n" +
		"Rossi;R@X.IT;333 123 4567;IT;Sì;01/05/2027;vip|famiglia\n" + // 2 ok
		"Bianchi;r@x.it;;;;;\n" + // 3 duplicate email
		"Verdi;;+393331234567;;;;\n" + // 4 duplicate phone
		"Neri;bad;12;xx;forse;2030-01-01;\n" + // 5 many errors
		";n@x.it;;;;;\n" // 6 required
	tb, err := decodeCSV(store.ImportKindCustomers, []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	rep := &Report{}
	rows := validateRows(store.ImportKindCustomers, tb, now, rep)
	if len(rows) != 1 || rep.Invalid != 4 {
		t.Fatalf("rows %d invalid %d errors %+v", len(rows), rep.Invalid, rep.Errors)
	}
	r := rows[0]
	if r.email != "r@x.it" || r.phone != "+393331234567" || r.customer.Locale != "it" || r.consent == nil || !*r.consent ||
		r.consentAt.Format("2006-01-02") != "2027-05-01" || len(r.customer.Tags) != 2 {
		t.Fatalf("row %+v", r)
	}
	want := []Issue{
		{Row: 3, Column: colEmail, Code: codeDuplicateInFile},
		{Row: 4, Column: colPhone, Code: codeDuplicateInFile},
		{Row: 5, Column: colEmail, Code: codeInvalidEmail},
		{Row: 5, Column: colPhone, Code: codeInvalidPhone},
		{Row: 5, Column: colLocale, Code: codeInvalidLocale},
		{Row: 5, Column: colConsent, Code: codeInvalidBoolean},
		{Row: 5, Column: colConsentDate, Code: codeInvalidDate},
		{Row: 6, Column: colLastName, Code: codeRequired},
	}
	for _, w := range want {
		found := false
		for _, e := range rep.Errors {
			if e.Row == w.Row && e.Column == w.Column && e.Code == w.Code {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %+v", w)
		}
	}
	for _, e := range rep.Errors {
		if strings.Contains(e.Message, "bad") || strings.Contains(e.Message, "r@x.it") {
			t.Fatalf("issue leaks a value: %+v", e)
		}
	}
}

func TestValidateRows_SeasonPasses(t *testing.T) {
	in := "cognome;posto;dal;al;fascia;note\n" +
		"Rossi;A1;2027-06-01;30/06/2027;mattina;davanti\n" +
		"Rossi;a1;;;;\n" +
		"Bianchi;B2;2027-07-10;2027-07-01;sera;\n" +
		"Verdi;;;;;\n"
	tb, err := decodeCSV(store.ImportKindSeasonPasses, []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	rep := &Report{}
	rows := validateRows(store.ImportKindSeasonPasses, tb, time.Now(), rep)
	if len(rows) != 1 || rep.Invalid != 3 {
		t.Fatalf("rows %d invalid %d %+v", len(rows), rep.Invalid, rep.Errors)
	}
	r := rows[0]
	if r.slots != calendar.SlotsAM || r.startsOn.String() != "2027-06-01" || r.endsOn.String() != "2027-06-30" ||
		r.passNotes != "davanti" || r.customer.Notes != "" {
		t.Fatalf("row %+v", r)
	}
	for _, code := range []string{codeDuplicateInFile, codeInvalidRange, codeInvalidSlot, codeRequired} {
		found := false
		for _, e := range rep.Errors {
			found = found || e.Code == code
		}
		if !found {
			t.Errorf("missing %s in %+v", code, rep.Errors)
		}
	}
}

func TestTemplate_Header(t *testing.T) {
	got := string(Template(store.ImportKindSeasonPasses))
	if !strings.HasPrefix(got, "\xef\xbb\xbfcognome;nome;email;telefono;") || !strings.Contains(got, ";posto;pacchetto;dal;al;fascia") ||
		strings.Count(got, "\n") != 1 {
		t.Fatalf("template %q", got)
	}
	// The template is accepted back (header only = empty file).
	if _, err := decodeCSV(store.ImportKindSeasonPasses, Template(store.ImportKindSeasonPasses)); err == nil || !strings.Contains(err.Error(), "empty_file") {
		t.Fatalf("template round trip: %v", err)
	}
}

func TestParseHelpers_Scenarios(t *testing.T) {
	for _, s := range []string{"sì", "SI", "yes", "1", "x", "Vero"} {
		if v, ok := parseBool(s); !ok || !v {
			t.Errorf("parseBool(%q)", s)
		}
	}
	for _, s := range []string{"no", "0", "false"} {
		if v, ok := parseBool(s); !ok || v {
			t.Errorf("parseBool(%q)", s)
		}
	}
	for in, want := range map[string]string{"2027-06-01": "2027-06-01", "01/06/2027": "2027-06-01", "1/6/2027": "2027-06-01", "01.06.2027": "2027-06-01", "01/06/27": "2027-06-01"} {
		if d, ok := parseDate(in); !ok || d.String() != want {
			t.Errorf("parseDate(%q) = %v %v", in, d, ok)
		}
	}
	if _, ok := parseDate("31/02/2027"); ok {
		t.Error("31/02 accepted")
	}
	if normalizeHeader("  Data del Consenso ") != "data_del_consenso" || normalizeHeader("E-Mail") != "e_mail" {
		t.Error("normalizeHeader")
	}
}
