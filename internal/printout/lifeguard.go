package printout

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
	"sync"
)

// Lifeguard list (spec 30.10): per row and spot the occupant's last name,
// the setup, arrival/departure and balance due, A4, self-contained (works
// offline, no script). Its strings live in i18n/lifeguard so that the Z
// report catalogs stay untouched.

//go:embed lifeguard.html.tmpl
var lifeguardTmplSrc string

//go:embed i18n/lifeguard/*.json
var lifeguardFS embed.FS

var (
	lgOnce     sync.Once
	lgCatalogs map[string]map[string]string
	lgTmpl     *template.Template
	lgErr      error
)

func loadLifeguard() error {
	if err := load(); err != nil { // logo
		return err
	}
	lgOnce.Do(func() {
		lgCatalogs = map[string]map[string]string{}
		for _, loc := range Locales {
			raw, err := lifeguardFS.ReadFile("i18n/lifeguard/" + loc + ".json")
			if err != nil {
				lgErr = fmt.Errorf("printout: lifeguard catalog %s: %w", loc, err)
				return
			}
			m := map[string]string{}
			if err := json.Unmarshal(raw, &m); err != nil {
				lgErr = fmt.Errorf("printout: lifeguard catalog %s: %w", loc, err)
				return
			}
			lgCatalogs[loc] = m
		}
		lgTmpl, lgErr = template.New("lifeguard").Parse(lifeguardTmplSrc)
	})
	return lgErr
}

// LifeguardRow is one occupied spot of the list.
type LifeguardRow struct {
	Row, Spot, Occupant, Setup, Group string
	Arriving, Departing, BalanceDue   bool
}

// LifeguardList is the input of RenderLifeguardList.
type LifeguardList struct {
	Locale        string
	Establishment string
	// Date is the calendar date YYYY-MM-DD; Slot is FULL, AM or PM.
	Date, Slot                           string
	Occupied, Free, Arrivals, Departures int
	Rows                                 []LifeguardRow
}

type lgRowView struct {
	Row, Spot, Occupant, Setup, Group, Movement string
	Due, RowStart                               bool
}

type lgView struct {
	Lang, Establishment, Date, Slot      string
	Logo                                 template.URL
	T                                    func(string) string
	Occupied, Free, Arrivals, Departures int
	Rows                                 []lgRowView
}

// RenderLifeguardList writes the lifeguard list as a complete HTML
// document. Pure: no clock, no I/O besides w.
func RenderLifeguardList(w io.Writer, in LifeguardList) error {
	if err := loadLifeguard(); err != nil {
		return err
	}
	loc := NormalizeLocale(in.Locale)
	t := func(k string) string {
		if v, ok := lgCatalogs[loc][k]; ok {
			return v
		}
		if v, ok := lgCatalogs[SourceLocale][k]; ok {
			return v
		}
		return k
	}
	v := lgView{
		Lang: loc, Establishment: in.Establishment, Date: formatDate(in.Date, loc), Slot: t("lg.slot." + in.Slot),
		Logo: logoURI, T: t, Occupied: in.Occupied, Free: in.Free, Arrivals: in.Arrivals, Departures: in.Departures,
	}
	prevRow := "\x00"
	for _, r := range in.Rows {
		var mv []string
		if r.Arriving {
			mv = append(mv, t("lg.arriving"))
		}
		if r.Departing {
			mv = append(mv, t("lg.departing"))
		}
		v.Rows = append(v.Rows, lgRowView{
			Row: r.Row, Spot: r.Spot, Occupant: r.Occupant, Setup: r.Setup, Group: r.Group,
			Movement: strings.Join(mv, " · "), Due: r.BalanceDue, RowStart: prevRow != "\x00" && r.Row != prevRow,
		})
		prevRow = r.Row
	}
	var buf bytes.Buffer
	if err := lgTmpl.Execute(&buf, v); err != nil {
		return fmt.Errorf("printout: render lifeguard list: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// formatDate formats YYYY-MM-DD as "15/07/2027" (de "15.07.2027"); other
// input is returned unchanged.
func formatDate(iso, locale string) string {
	parts := strings.Split(iso, "-")
	if len(parts) != 3 {
		return iso
	}
	sep := "/"
	if locale == "de" {
		sep = "."
	}
	return parts[2] + sep + parts[1] + sep + parts[0]
}
