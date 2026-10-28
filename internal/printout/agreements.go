package printout

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// M8 documents (spec 30.2.4-30.2.5, ADR 0028 decision 3): the agreements
// register for the concession tenders and the partner statement, A4, neutral
// layout, PDF through the browser print like the Z summary. Pure rendering:
// the caller passes the data already computed by internal/domain/report.

//go:embed i18n_agreements/*.json
var agreementsCatalogFS embed.FS

//go:embed agreements_register.html.tmpl
var registerTmpl string

//go:embed partner_statement.html.tmpl
var statementTmpl string

var (
	m8Once     sync.Once
	m8Catalogs map[string]map[string]string
	m8Tmpl     *template.Template
	m8Err      error
)

func loadM8() error {
	if err := load(); err != nil {
		return err
	}
	m8Once.Do(func() {
		m8Catalogs = map[string]map[string]string{}
		for _, loc := range Locales {
			raw, err := agreementsCatalogFS.ReadFile("i18n_agreements/" + loc + ".json")
			if err != nil {
				m8Err = fmt.Errorf("printout: catalog %s: %w", loc, err)
				return
			}
			m := map[string]string{}
			if err := json.Unmarshal(raw, &m); err != nil {
				m8Err = fmt.Errorf("printout: catalog %s: %w", loc, err)
				return
			}
			m8Catalogs[loc] = m
		}
		t := template.New("m8")
		if _, err := t.New("register").Parse(registerTmpl); err != nil {
			m8Err = err
			return
		}
		if _, err := t.New("statement").Parse(statementTmpl); err != nil {
			m8Err = err
			return
		}
		m8Tmpl = t
	})
	return m8Err
}

func m8Translator(loc string) func(string) string {
	return func(k string) string {
		if v, ok := m8Catalogs[loc][k]; ok {
			return v
		}
		if v, ok := m8Catalogs[SourceLocale][k]; ok {
			return v
		}
		return k
	}
}

// FormatDate formats a calendar date: "15/07/2027" (de "15.07.2027").
func FormatDate(d calendar.Date, locale string) string {
	if d.IsZero() {
		return ""
	}
	if locale == "de" {
		return fmt.Sprintf("%02d.%02d.%04d", d.Day(), int(d.Month()), d.Year())
	}
	return fmt.Sprintf("%02d/%02d/%04d", d.Day(), int(d.Month()), d.Year())
}

// RegisterCounters are the counters of a register row.
type RegisterCounters struct {
	Agreements, Bookings, SpotDays, People int
	Revenue, Discount                      money.Cents
}

// RegisterEntry is one agreement of the printed register.
type RegisterEntry struct {
	Name, Code, Recipients, Conditions string
	ValidFrom, ValidTo                 *calendar.Date
	RegisterCounters
}

// RegisterCategory groups the entries of one register category.
type RegisterCategory struct {
	Category string // residents, accommodation, ...
	Entries  []RegisterEntry
	Total    RegisterCounters
}

// AgreementsRegister is the input of RenderAgreementsRegister.
type AgreementsRegister struct {
	Locale      string
	Currency    string
	Location    *time.Location
	Header      Header
	From, To    calendar.Date
	SeasonYear  int
	GeneratedAt time.Time
	Categories  []RegisterCategory
	Total       RegisterCounters
}

type countersView struct {
	Agreements, Bookings, SpotDays, People int
	Revenue, Discount                      string
}

type registerEntryView struct {
	Name, Code, Recipients, Conditions, Validity string
	countersView
}

type registerCategoryView struct {
	Label   string
	Entries []registerEntryView
	Total   countersView
}

type registerView struct {
	Lang, Title, Period, Generated string
	Logo                           template.URL
	Header                         Header
	SeasonYear                     int
	Categories                     []registerCategoryView
	Total                          countersView
	T                              func(string) string
}

// RenderAgreementsRegister writes the register as a complete HTML page.
func RenderAgreementsRegister(w io.Writer, r AgreementsRegister) error {
	if err := loadM8(); err != nil {
		return err
	}
	loc := NormalizeLocale(r.Locale)
	t := m8Translator(loc)
	m := func(c money.Cents) string { return FormatMoney(c, r.Currency, loc) }
	cv := func(c RegisterCounters) countersView {
		return countersView{Agreements: c.Agreements, Bookings: c.Bookings, SpotDays: c.SpotDays, People: c.People, Revenue: m(c.Revenue), Discount: m(c.Discount)}
	}
	tz := r.Location
	if tz == nil {
		tz = time.UTC
	}
	v := registerView{
		Lang: loc, Title: t("reg.title"), Logo: logoURI, Header: r.Header, SeasonYear: r.SeasonYear, T: t,
		Period:    FormatDate(r.From, loc) + " – " + FormatDate(r.To, loc),
		Generated: FormatDateTime(r.GeneratedAt.In(tz), loc), Total: cv(r.Total),
	}
	for _, c := range r.Categories {
		cat := registerCategoryView{Label: t("cat." + c.Category), Total: cv(c.Total)}
		for _, e := range c.Entries {
			ev := registerEntryView{Name: e.Name, Code: e.Code, Recipients: e.Recipients, Conditions: e.Conditions, countersView: cv(e.RegisterCounters)}
			switch {
			case e.ValidFrom != nil && e.ValidTo != nil:
				ev.Validity = FormatDate(*e.ValidFrom, loc) + " – " + FormatDate(*e.ValidTo, loc)
			case e.ValidFrom != nil:
				ev.Validity = t("reg.from") + " " + FormatDate(*e.ValidFrom, loc)
			case e.ValidTo != nil:
				ev.Validity = t("reg.to") + " " + FormatDate(*e.ValidTo, loc)
			}
			cat.Entries = append(cat.Entries, ev)
		}
		v.Categories = append(v.Categories, cat)
	}
	return execute(w, "register", v)
}

// StatementPartner is the partner block of a statement.
type StatementPartner struct {
	Name, Address, Municipality, VATNumber, TaxCode string
}

// StatementBooking is one booking of a printed statement.
type StatementBooking struct {
	Code, Guest, Group, Agreement string
	StartsOn, EndsOn              calendar.Date
	SpotDays, People              int
	Total, Discount, Partner      money.Cents
}

// StatementPayment is one payment of a printed statement.
type StatementPayment struct {
	ReceivedOn calendar.Date
	Method     string
	Reference  string
	Amount     money.Cents
	Voided     bool
}

// PartnerStatement is the input of RenderPartnerStatement.
type PartnerStatement struct {
	Locale      string
	Currency    string
	Location    *time.Location
	Header      Header
	Partner     StatementPartner
	From, To    calendar.Date
	GeneratedAt time.Time
	Bookings    []StatementBooking
	Payments    []StatementPayment
	SpotDays    int
	People      int
	Total       money.Cents
	Discount    money.Cents
	Opening     money.Cents
	Charged     money.Cents
	Paid        money.Cents
	Closing     money.Cents
}

type statementBookingView struct {
	Code, Guest, Group, Agreement, Stay, Total, Discount, Partner string
	SpotDays, People                                              int
}

type statementPaymentView struct {
	Date, Method, Reference, Amount string
	Voided                          bool
}

type statementView struct {
	Lang, Title, Period, Generated                                string
	Logo                                                          template.URL
	Header                                                        Header
	Partner                                                       StatementPartner
	Bookings                                                      []statementBookingView
	Payments                                                      []statementPaymentView
	SpotDays, People                                              int
	Total, Discount, Opening, Charged, Paid, Closing, ClosingTone string
	T                                                             func(string) string
}

// RenderPartnerStatement writes a partner statement as a complete HTML page.
func RenderPartnerStatement(w io.Writer, s PartnerStatement) error {
	if err := loadM8(); err != nil {
		return err
	}
	loc := NormalizeLocale(s.Locale)
	t := m8Translator(loc)
	m := func(c money.Cents) string { return FormatMoney(c, s.Currency, loc) }
	tz := s.Location
	if tz == nil {
		tz = time.UTC
	}
	v := statementView{
		Lang: loc, Title: t("st.title"), Logo: logoURI, Header: s.Header, Partner: s.Partner, T: t,
		Period:    FormatDate(s.From, loc) + " – " + FormatDate(s.To, loc),
		Generated: FormatDateTime(s.GeneratedAt.In(tz), loc), SpotDays: s.SpotDays, People: s.People,
		Total: m(s.Total), Discount: m(s.Discount), Opening: m(s.Opening), Charged: m(s.Charged), Paid: m(s.Paid),
		Closing: m(s.Closing), ClosingTone: tone(s.Closing),
	}
	for _, b := range s.Bookings {
		v.Bookings = append(v.Bookings, statementBookingView{
			Code: b.Code, Guest: b.Guest, Group: b.Group, Agreement: b.Agreement,
			Stay: FormatDate(b.StartsOn, loc) + " – " + FormatDate(b.EndsOn, loc), SpotDays: b.SpotDays, People: b.People,
			Total: m(b.Total), Discount: m(b.Discount), Partner: m(b.Partner),
		})
	}
	for _, p := range s.Payments {
		v.Payments = append(v.Payments, statementPaymentView{
			Date: FormatDate(p.ReceivedOn, loc), Method: t("method." + p.Method), Reference: p.Reference, Amount: m(p.Amount), Voided: p.Voided,
		})
	}
	return execute(w, "statement", v)
}

func execute(w io.Writer, name string, v any) error {
	var buf bytes.Buffer
	if err := m8Tmpl.ExecuteTemplate(&buf, name, v); err != nil {
		return fmt.Errorf("printout: render %s: %w", name, err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}
