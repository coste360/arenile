package importer

import (
	"bytes"
	"encoding/csv"

	"github.com/coste360/arenile/internal/store"
)

// Canonical columns (Italian, the source language). Header names are
// matched case- and accent-insensitively; the aliases cover the English
// names and the common exports of other beach software.
const (
	colLastName      = "cognome"
	colFirstName     = "nome"
	colEmail         = "email"
	colPhone         = "telefono"
	colLocale        = "lingua"
	colNotes         = "note"
	colTags          = "etichette"
	colConsent       = "consenso_marketing"
	colConsentDate   = "data_consenso"
	colConsentText   = "versione_informativa"
	colSpot          = "posto"
	colPack          = "pacchetto"
	colStartsOn      = "dal"
	colEndsOn        = "al"
	colSlots         = "fascia"
	consentSource    = "import"
	defaultConsentTV = "import"
)

// column is a recognised column of a kind.
type column struct {
	name     string
	aliases  []string
	required bool
}

var customerColumns = []column{
	{name: colLastName, aliases: []string{"last_name", "lastname", "surname", "family_name"}, required: true},
	{name: colFirstName, aliases: []string{"first_name", "firstname", "given_name"}},
	{name: colEmail, aliases: []string{"e_mail", "mail", "posta_elettronica", "indirizzo_email"}},
	{name: colPhone, aliases: []string{"phone", "cellulare", "cell", "mobile", "tel", "numero_di_telefono", "telefono_cellulare"}},
	{name: colLocale, aliases: []string{"locale", "language", "lang"}},
	{name: colNotes, aliases: []string{"notes", "annotazioni"}},
	{name: colTags, aliases: []string{"tags", "tag", "etichetta", "categorie"}},
	{name: colConsent, aliases: []string{"marketing_consent", "marketing", "newsletter", "consenso_newsletter", "consenso"}},
	{name: colConsentDate, aliases: []string{"marketing_consent_date", "consent_date", "data_del_consenso"}},
	{name: colConsentText, aliases: []string{"consent_text_version", "privacy_version", "informativa"}},
}

var passColumns = append(append([]column{}, customerColumns...),
	column{name: colSpot, aliases: []string{"postazione", "ombrellone", "spot", "spot_label", "numero_posto"}, required: true},
	column{name: colPack, aliases: []string{"pack", "pack_code", "codice_pacchetto"}},
	column{name: colStartsOn, aliases: []string{"data_inizio", "inizio", "starts_on", "from", "start_date"}},
	column{name: colEndsOn, aliases: []string{"data_fine", "fine", "ends_on", "to", "end_date"}},
	column{name: colSlots, aliases: []string{"slot", "slots", "turno"}},
)

// columnsOf returns the columns of an import kind.
func columnsOf(kind string) []column {
	if kind == store.ImportKindSeasonPasses {
		return passColumns
	}
	return customerColumns
}

// fieldColumn maps the field paths of the customer and pass validation
// errors to the CSV columns.
var fieldColumn = map[string]string{
	"last_name": colLastName, "first_name": colFirstName, "email": colEmail, "phone": colPhone,
	"locale": colLocale, "notes": colNotes, "tags": colTags, "marketing_consent": colConsent,
	"marketing_consent.given_at": colConsentDate, "marketing_consent.text_version": colConsentText,
	"spot_id": colSpot, "pack_id": colPack, "starts_on": colStartsOn, "ends_on": colEndsOn, "slots": colSlots,
}

// Template returns the CSV template of kind: the canonical header only,
// ';'-separated with a UTF-8 BOM so that Excel opens it correctly. No
// sample row: a forgotten example would be imported as a real customer.
func Template(kind string) []byte {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	w.Comma = ';'
	cols := columnsOf(kind)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.name
	}
	_ = w.Write(header)
	w.Flush()
	return b.Bytes()
}

// ValidKind reports whether kind is an import kind.
func ValidKind(kind string) bool {
	return kind == store.ImportKindCustomers || kind == store.ImportKindSeasonPasses
}
