package importer

import (
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/store"
)

// Pure row validation (functional core): no I/O; the service passes now
// and today in. Issues carry row, column and code only, never the values.

// Options are the import options (api.ImportOptions). JSON names match the
// API so that the stored options and the API body are the same document.
type Options struct {
	MatchBy                 string `json:"match_by,omitempty"`
	UpdateExisting          bool   `json:"update_existing,omitempty"`
	MarketingConsentDefault bool   `json:"marketing_consent_default,omitempty"`
	SeasonID                string `json:"season_id,omitempty"`
	PackID                  string `json:"pack_id,omitempty"`
	CreateMissingCustomers  *bool  `json:"create_missing_customers,omitempty"`
}

// Match criteria (api ImportOptions.match_by).
const (
	MatchByEmail       = "email"
	MatchByPhone       = "phone"
	MatchByExternalRef = "external_ref"
)

// matchBy returns the effective match criterion (default email).
func (o Options) matchBy() string {
	if o.MatchBy == "" {
		return MatchByEmail
	}
	return o.MatchBy
}

// createMissing returns create_missing_customers (default true).
func (o Options) createMissing() bool {
	return o.CreateMissingCustomers == nil || *o.CreateMissingCustomers
}

// Issue is one problem of a row (api.ImportIssue).
type Issue struct {
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// Report is the outcome of a preview or an apply (api.ImportReport).
type Report struct {
	Rows     int      `json:"rows"`
	Valid    int      `json:"valid"`
	Invalid  int      `json:"invalid"`
	ToCreate int      `json:"to_create"`
	ToUpdate int      `json:"to_update"`
	ToSkip   int      `json:"to_skip"`
	Created  *int     `json:"created,omitempty"`
	Updated  *int     `json:"updated,omitempty"`
	Skipped  *int     `json:"skipped,omitempty"`
	Errors   []Issue  `json:"errors"`
	Warnings []Issue  `json:"warnings"`
	Columns  []string `json:"columns,omitempty"`
}

// addError appends an error (capped) and reports nothing: callers count
// invalid rows themselves.
func (r *Report) addError(i Issue) {
	if len(r.Errors) < maxIssues {
		r.Errors = append(r.Errors, i)
	}
}

func (r *Report) addWarning(i Issue) {
	if len(r.Warnings) < maxIssues {
		r.Warnings = append(r.Warnings, i)
	}
}

// Issue codes.
const (
	codeRequired        = "required"
	codeInvalidEmail    = "invalid_email"
	codeInvalidPhone    = "invalid_phone"
	codeInvalidLocale   = "invalid_locale"
	codeInvalidBoolean  = "invalid_boolean"
	codeInvalidDate     = "invalid_date"
	codeInvalidRange    = "invalid_range"
	codeInvalidSlot     = "invalid_slot"
	codeTooLong         = "too_long"
	codeDuplicateInFile = "duplicate_in_file"
	codeUnknownSpot     = "unknown_spot"
	codeAmbiguousSpot   = "ambiguous_spot"
	codeUnknownPack     = "unknown_pack"
	// codePackNeedsApproval: the pack has requires_approval; an imported
	// pass cannot wait for an approval (INV-78).
	codePackNeedsApproval = "pack_requires_approval"

	codeSpotUnavailable = "spot_unavailable"
	codeCustomerMissing = "customer_not_found"
	codeNoPrice         = "no_price"
	codePolicy          = "policy_violation"
	codeOutsideSeason   = "outside_season"
	codeInvalid         = "invalid"
	codeConflict        = "conflict"
	// Warnings.
	codeNoMatchKey        = "no_match_key"
	codePossibleDuplicate = "possible_duplicate"
	codeUnknownColumn     = "unknown_column"
	codeConvertedEncoding = "converted_encoding"
)

// msgPackNeedsApproval is the issue text of codePackNeedsApproval.
const msgPackNeedsApproval = "Il pacchetto richiede l'approvazione del gestore: crea questi abbonamenti dal pannello."

// row is a validated data row. Customer holds only the values of the file
// (empty = not given); Has lists the columns with a value.
type row struct {
	line     int
	customer customer.Input
	has      map[string]bool
	// consent is the marketing consent column (nil = absent or empty).
	consent   *bool
	consentAt time.Time
	consentTV string
	// email and phone are the normalised match keys ("" = none).
	email string
	phone string
	// Season pass fields.
	spotLabel string
	passNotes string
	packRef   string
	startsOn  *calendar.Date
	endsOn    *calendar.Date
	slots     calendar.Slots
}

// validateRows checks every row of t and returns the valid ones; issues
// (errors and warnings) go to rep. now bounds the consent dates.
func validateRows(kind string, t *table, now time.Time, rep *Report) []row {
	out := make([]row, 0, len(t.rows))
	seenEmail := map[string]int{}
	seenPhone := map[string]int{}
	seenSpot := map[string]int{}
	for _, rec := range t.rows {
		r, errs := parseRow(kind, rec, now)
		if kind == store.ImportKindCustomers {
			if r.email != "" {
				if _, dup := seenEmail[r.email]; dup {
					errs = append(errs, Issue{Row: rec.line, Column: colEmail, Code: codeDuplicateInFile, Message: "Stessa email di una riga precedente."})
				} else {
					seenEmail[r.email] = rec.line
				}
			}
			if r.phone != "" {
				if _, dup := seenPhone[r.phone]; dup {
					errs = append(errs, Issue{Row: rec.line, Column: colPhone, Code: codeDuplicateInFile, Message: "Stesso telefono di una riga precedente."})
				} else {
					seenPhone[r.phone] = rec.line
				}
			}
		}
		if kind == store.ImportKindSeasonPasses && r.spotLabel != "" {
			k := strings.ToLower(r.spotLabel)
			if _, dup := seenSpot[k]; dup {
				errs = append(errs, Issue{Row: rec.line, Column: colSpot, Code: codeDuplicateInFile, Message: "Stesso posto di una riga precedente."})
			} else {
				seenSpot[k] = rec.line
			}
		}
		if len(errs) > 0 {
			rep.Invalid++
			for _, e := range errs {
				rep.addError(e)
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// parseRow normalises and checks the values of one record.
func parseRow(kind string, rec record, now time.Time) (row, []Issue) {
	v := rec.values
	r := row{line: rec.line, has: map[string]bool{}}
	for c, x := range v {
		if x != "" {
			r.has[c] = true
		}
	}
	var errs []Issue
	bad := func(col, code, msg string) {
		errs = append(errs, Issue{Row: rec.line, Column: col, Code: code, Message: msg})
	}

	r.customer.LastName = v[colLastName]
	r.customer.FirstName = v[colFirstName]
	if r.customer.LastName == "" {
		bad(colLastName, codeRequired, "Il cognome è obbligatorio.")
	} else if len([]rune(r.customer.LastName)) > customer.MaxNameLen {
		bad(colLastName, codeTooLong, "Al massimo 100 caratteri.")
	}
	if len([]rune(r.customer.FirstName)) > customer.MaxNameLen {
		bad(colFirstName, codeTooLong, "Al massimo 100 caratteri.")
	}
	if e := customer.NormalizeEmail(v[colEmail]); e != "" {
		if !customer.ValidEmail(e) {
			bad(colEmail, codeInvalidEmail, "Indirizzo email non valido.")
		}
		r.email = e
		r.customer.Email = e
	}
	if p := v[colPhone]; p != "" {
		n, ok := customer.NormalizePhone(p)
		if !ok {
			bad(colPhone, codeInvalidPhone, "Numero di telefono non valido.")
		}
		r.phone = n
		r.customer.Phone = n
	}
	if l := strings.ToLower(v[colLocale]); l != "" {
		switch l {
		case "it", "en", "de", "fr":
			r.customer.Locale = l
		default:
			bad(colLocale, codeInvalidLocale, "Lingua non supportata (it, en, de, fr).")
		}
	}
	r.customer.Notes = v[colNotes]
	if len([]rune(r.customer.Notes)) > customer.MaxNotesLen {
		bad(colNotes, codeTooLong, "Al massimo 4000 caratteri.")
	}
	if tags := v[colTags]; tags != "" {
		r.customer.Tags = strings.FieldsFunc(tags, func(c rune) bool { return c == '|' || c == ',' })
	}
	if c := v[colConsent]; c != "" {
		b, ok := parseBool(c)
		if !ok {
			bad(colConsent, codeInvalidBoolean, "Usa sì o no.")
		}
		r.consent = &b
	}
	if d := v[colConsentDate]; d != "" {
		date, ok := parseDate(d)
		switch {
		case !ok:
			bad(colConsentDate, codeInvalidDate, "Data non valida: usa AAAA-MM-GG o GG/MM/AAAA.")
		case date.Time().After(now):
			bad(colConsentDate, codeInvalidDate, "La data del consenso non può essere nel futuro.")
		default:
			r.consentAt = date.Time()
		}
	}
	r.consentTV = v[colConsentText]
	if len([]rune(r.consentTV)) > 100 {
		bad(colConsentText, codeTooLong, "Al massimo 100 caratteri.")
	}

	if kind != store.ImportKindSeasonPasses {
		return r, errs
	}
	// The notes of a season pass row are the internal notes of the pass.
	r.passNotes = r.customer.Notes
	r.customer.Notes = ""
	r.spotLabel = v[colSpot]
	if r.spotLabel == "" {
		bad(colSpot, codeRequired, "Il posto è obbligatorio.")
	}
	r.packRef = v[colPack]
	for _, f := range []struct {
		col string
		dst **calendar.Date
	}{{colStartsOn, &r.startsOn}, {colEndsOn, &r.endsOn}} {
		if s := v[f.col]; s != "" {
			d, ok := parseDate(s)
			if !ok {
				bad(f.col, codeInvalidDate, "Data non valida: usa AAAA-MM-GG o GG/MM/AAAA.")
				continue
			}
			*f.dst = &d
		}
	}
	if r.startsOn != nil && r.endsOn != nil && r.endsOn.Before(*r.startsOn) {
		bad(colEndsOn, codeInvalidRange, "La data di fine è precedente a quella di inizio.")
	}
	r.slots = calendar.SlotsFull
	if s := v[colSlots]; s != "" {
		sl, ok := parseSlots(s)
		if !ok {
			bad(colSlots, codeInvalidSlot, "Fascia non valida: AM, PM o FULL.")
		}
		r.slots = sl
	}
	return r, errs
}

// parseBool accepts the usual spellings of yes and no (it, en, de, fr).
func parseBool(s string) (bool, bool) {
	switch customer.Fold(s) {
	case "si", "s", "yes", "y", "true", "vero", "1", "x", "ja", "oui":
		return true, true
	case "no", "n", "false", "falso", "0", "nein", "non":
		return false, true
	}
	return false, false
}

// parseDate accepts YYYY-MM-DD and the Italian DD/MM/YYYY (also with '.'
// or '-' and a two digit year from 2000).
func parseDate(s string) (calendar.Date, bool) {
	if d, err := calendar.Parse(s); err == nil {
		return d, true
	}
	for _, layout := range []string{"02/01/2006", "2/1/2006", "02.01.2006", "02-01-2006", "02/01/06", "2/1/06"} {
		if t, err := time.Parse(layout, s); err == nil {
			return calendar.New(t.Year(), t.Month(), t.Day()), true
		}
	}
	return calendar.Date{}, false
}

// parseSlots accepts AM/PM/FULL and their Italian names.
func parseSlots(s string) (calendar.Slots, bool) {
	switch customer.Fold(s) {
	case "am", "mattina", "mattino":
		return calendar.SlotsAM, true
	case "pm", "pomeriggio":
		return calendar.SlotsPM, true
	case "full", "giornata", "giornata intera", "intera", "intera giornata", "tutto il giorno":
		return calendar.SlotsFull, true
	}
	return calendar.SlotsFull, false
}

// MaxRowsFor is the row limit of a file of kind.
func MaxRowsFor(kind string) int {
	if kind == store.ImportKindSeasonPasses {
		return MaxPassRows
	}
	return MaxRows
}
