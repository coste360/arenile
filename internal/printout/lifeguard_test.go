package printout

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderLifeguardList_Structure(t *testing.T) {
	in := LifeguardList{
		Locale: "it", Establishment: "Lido Delfino", Date: "2027-08-14", Slot: "FULL",
		Occupied: 2, Free: 40, Arrivals: 1, Departures: 1,
		Rows: []LifeguardRow{
			{Row: "A", Spot: "A1", Occupant: "Rossi", Setup: "2L 1S", Arriving: true, BalanceDue: true, Group: "Famiglia <Rossi>"},
			{Row: "B", Spot: "B4", Departing: true},
		},
	}
	for _, loc := range Locales {
		in.Locale = loc
		var buf bytes.Buffer
		if err := RenderLifeguardList(&buf, in); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		for _, want := range []string{"<!doctype html>", `lang="` + loc + `"`, "Lido Delfino", "2L 1S", "Rossi", `class="row-start"`, "Famiglia &lt;Rossi&gt;"} {
			if !strings.Contains(html, want) {
				t.Fatalf("%s: missing %q", loc, want)
			}
		}
		if strings.Contains(html, "<script") || strings.Contains(html, "lg.") {
			t.Fatalf("%s: script or untranslated key", loc)
		}
	}
	var buf bytes.Buffer
	in.Locale, in.Rows = "it", nil
	if err := RenderLifeguardList(&buf, in); err != nil || !strings.Contains(buf.String(), "Nessuna postazione occupata.") || !strings.Contains(buf.String(), "14/08/2027") {
		t.Fatalf("empty list: %v", err)
	}
}

func TestLifeguardCatalogs_KeyParity(t *testing.T) {
	if err := loadLifeguard(); err != nil {
		t.Fatal(err)
	}
	src := lgCatalogs[SourceLocale]
	for _, loc := range Locales {
		if len(lgCatalogs[loc]) != len(src) {
			t.Fatalf("%s has %d keys, it has %d", loc, len(lgCatalogs[loc]), len(src))
		}
		for k := range src {
			if lgCatalogs[loc][k] == "" {
				t.Fatalf("%s misses %s", loc, k)
			}
		}
	}
}
