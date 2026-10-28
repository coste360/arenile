package printout

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
)

func TestAgreementsCatalogs_SameKeys(t *testing.T) {
	keys := func(loc string) map[string]bool {
		raw, err := agreementsCatalogFS.ReadFile("i18n_agreements/" + loc + ".json")
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for k, v := range m {
			if strings.TrimSpace(v) == "" {
				t.Fatalf("%s: empty %s", loc, k)
			}
			out[k] = true
		}
		return out
	}
	src := keys(SourceLocale)
	for _, loc := range Locales {
		got := keys(loc)
		for k := range src {
			if !got[k] {
				t.Errorf("%s: missing %s", loc, k)
			}
		}
		if len(got) != len(src) {
			t.Errorf("%s: %d keys, source has %d", loc, len(got), len(src))
		}
	}
}

func TestRenderAgreementsRegister(t *testing.T) {
	from, to := calendar.MustParse("2027-06-01"), calendar.MustParse("2027-09-30")
	vf := calendar.MustParse("2027-06-01")
	doc := AgreementsRegister{
		Locale: "it", Currency: "EUR", Header: Header{Name: "Lido Delfino", LegalName: "Delfino Srl", VATNumber: "01234567890"},
		From: from, To: to, SeasonYear: 2027, GeneratedAt: time.Date(2027, 10, 1, 8, 0, 0, 0, time.UTC),
		Categories: []RegisterCategory{{
			Category: "residents",
			Entries: []RegisterEntry{{
				Name: "Residenti <Gaeta>", Code: "RES", Recipients: "Residenti", Conditions: "Sconto del 10%", ValidFrom: &vf,
				RegisterCounters: RegisterCounters{Agreements: 1, Bookings: 3, SpotDays: 7, People: 9, Revenue: 12345, Discount: 1372},
			}},
			Total: RegisterCounters{Agreements: 1, Bookings: 3, SpotDays: 7, People: 9, Revenue: 12345, Discount: 1372},
		}},
		Total: RegisterCounters{Agreements: 1, Bookings: 3, SpotDays: 7, People: 9, Revenue: 12345, Discount: 1372},
	}
	var buf bytes.Buffer
	if err := RenderAgreementsRegister(&buf, doc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Registro delle convenzioni", "Delfino Srl", "01234567890", "01/06/2027 – 30/09/2027", "Residenti &lt;Gaeta&gt;",
		"123,45 €", "13,72 €", "Firma del legale rappresentante", `data-testid="register-total"`, "dal 01/06/2027",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, loc := range []string{"en", "de", "fr"} {
		buf.Reset()
		doc.Locale = loc
		if err := RenderAgreementsRegister(&buf, doc); err != nil || strings.Contains(buf.String(), "reg.") {
			t.Fatalf("%s: %v (untranslated key)", loc, err)
		}
	}
}

func TestRenderPartnerStatement(t *testing.T) {
	doc := PartnerStatement{
		Locale: "it", Currency: "EUR", Header: Header{Name: "Lido Delfino"}, Partner: StatementPartner{Name: "Hotel Miramare", VATNumber: "IT1"},
		From: calendar.MustParse("2027-07-01"), To: calendar.MustParse("2027-07-31"), GeneratedAt: time.Date(2027, 8, 1, 8, 0, 0, 0, time.UTC),
		Bookings: []StatementBooking{{Code: "LD-1", Guest: "Rossi Mario", StartsOn: calendar.MustParse("2027-07-10"), EndsOn: calendar.MustParse("2027-07-11"), SpotDays: 2, People: 3, Total: 3600, Discount: 400, Partner: 3600}},
		Payments: []StatementPayment{{ReceivedOn: calendar.MustParse("2027-07-12"), Method: "bank_transfer", Reference: "CRO", Amount: 1000}, {ReceivedOn: calendar.MustParse("2027-07-13"), Method: "cash", Amount: 50, Voided: true}},
		Opening:  0, Charged: 3600, Paid: 1000, Closing: 2600,
	}
	var buf bytes.Buffer
	if err := RenderPartnerStatement(&buf, doc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Estratto conto", "Hotel Miramare", "Bonifico", "annullato", "26,00 €", `class="void"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}
