package customer

import (
	"strings"
	"time"
	"unicode"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Pure normalisation and validation of customer data (functional core):
// no I/O, no clock; the service passes now in. Messages are Italian, the
// source language shown to the staff.

// Limits of the customer fields.
const (
	MaxNameLen  = 100
	MaxEmailLen = 254
	MaxNotesLen = 4000
	MaxTags     = 20
	MaxTagLen   = 50
	// DefaultCountryCode is prefixed to national phone numbers written
	// without an international prefix (Italian establishments).
	DefaultCountryCode = "39"
)

// Locales accepted for customers (api.CustomerInputLocale).
var locales = map[string]bool{"it": true, "en": true, "de": true, "fr": true}

// fields is a validated, normalised Input.
type fields struct {
	FirstName        string
	LastName         string
	Email            *string
	Phone            *string
	Locale           string
	Notes            string
	Tags             []string
	AgreementID      *string
	MarketingConsent *store.MarketingConsent
}

// validate normalises in and collects every field error. defaultLocale is
// used when in.Locale is empty; now fills a consent without given_at.
func validate(in *Input, defaultLocale string, now time.Time) (fields, error) {
	ve := &configerr.ValidationError{}
	f := fields{
		FirstName: collapseSpaces(in.FirstName),
		LastName:  collapseSpaces(in.LastName),
		Notes:     strings.TrimSpace(in.Notes),
	}
	if f.LastName == "" {
		ve.Add("last_name", "Il cognome è obbligatorio.")
	} else if len([]rune(f.LastName)) > MaxNameLen {
		ve.Add("last_name", "Il cognome può avere al massimo 100 caratteri.")
	}
	if len([]rune(f.FirstName)) > MaxNameLen {
		ve.Add("first_name", "Il nome può avere al massimo 100 caratteri.")
	}
	if len([]rune(f.Notes)) > MaxNotesLen {
		ve.Add("notes", "Le note possono avere al massimo 4000 caratteri.")
	}
	if e := NormalizeEmail(in.Email); e != "" {
		if !ValidEmail(e) {
			ve.Add("email", "Indirizzo email non valido.")
		}
		f.Email = &e
	}
	if strings.TrimSpace(in.Phone) != "" {
		p, ok := NormalizePhone(in.Phone)
		if !ok {
			ve.Add("phone", "Numero di telefono non valido: usa il formato internazionale, es. +39 333 1234567.")
		}
		f.Phone = &p
	}
	f.Locale = strings.ToLower(strings.TrimSpace(in.Locale))
	if f.Locale == "" {
		f.Locale = defaultLocale
	}
	if !locales[f.Locale] {
		if in.Locale != "" {
			ve.Add("locale", "Lingua non supportata (it, en, de, fr).")
		}
		f.Locale = "it"
	}
	tags, msg := normalizeTags(in.Tags)
	if msg != "" {
		ve.Add("tags", msg)
	}
	f.Tags = tags
	if a := strings.TrimSpace(in.AgreementID); a != "" {
		f.AgreementID = &a
	}
	if mc := in.MarketingConsent; mc != nil {
		c := *mc
		c.Source = strings.TrimSpace(c.Source)
		c.TextVersion = strings.TrimSpace(c.TextVersion)
		if c.GivenAt.IsZero() {
			c.GivenAt = now
		}
		c.GivenAt = c.GivenAt.UTC()
		if c.Source == "" {
			ve.Add("marketing_consent.source", "Indica la fonte del consenso (es. cassa, modulo).")
		}
		if c.TextVersion == "" {
			ve.Add("marketing_consent.text_version", "Indica la versione del testo dell'informativa accettata.")
		}
		if c.GivenAt.After(now.Add(5 * time.Minute)) {
			ve.Add("marketing_consent.given_at", "La data del consenso non può essere nel futuro.")
		}
		f.MarketingConsent = &c
	}
	return f, ve.OrNil()
}

// NormalizeEmail trims and lowercases an email address.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ValidEmail is a pragmatic syntax check of a normalised address: one "@",
// non-empty local part, a dotted domain, no spaces, at most 254 bytes.
func ValidEmail(e string) bool {
	if len(e) > MaxEmailLen || strings.ContainsAny(e, " \t\r\n,;<>\"") {
		return false
	}
	local, domain, ok := strings.Cut(e, "@")
	if !ok || local == "" || len(local) > 64 || strings.Contains(domain, "@") {
		return false
	}
	if len(domain) < 3 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") ||
		strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return false
	}
	return true
}

// NormalizePhone returns the E.164 form of a phone number written by the
// staff: separators (spaces, dots, dashes, slashes, parentheses) are
// removed, "00" becomes "+", and numbers without an international prefix
// are Italian (+39, keeping the leading 0 of landlines). It reports false
// when the result is not "+" followed by 8 to 15 digits (first digit not 0).
func NormalizePhone(s string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '-' || r == '/' || r == '(' || r == ')' || r == ' ':
		default:
			return strings.TrimSpace(s), false
		}
	}
	p := b.String()
	switch {
	case strings.HasPrefix(p, "+"):
	case strings.HasPrefix(p, "00"):
		p = "+" + p[2:]
	default:
		p = "+" + DefaultCountryCode + p
	}
	digits := p[1:]
	if len(digits) < 8 || len(digits) > 15 || digits[0] == '0' {
		return p, false
	}
	return p, true
}

// normalizeTags trims, lowercases and deduplicates tags (order kept). The
// "|" character is reserved for the search markers.
func normalizeTags(in []string) ([]string, string) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(collapseSpaces(t))
		if t == "" || seen[t] {
			continue
		}
		if len([]rune(t)) > MaxTagLen {
			return out, "Ogni etichetta può avere al massimo 50 caratteri."
		}
		if strings.Contains(t, "|") {
			return out, "Le etichette non possono contenere il carattere |."
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxTags {
		return out[:MaxTags], "Al massimo 20 etichette per cliente."
	}
	return out, ""
}

// NormalizeTag is the form of a tag used to filter (same rules as storage).
func NormalizeTag(t string) string { return strings.ToLower(collapseSpaces(t)) }

// searchText builds customers.search_text: lowercase, accent-free
// "last first email phone national-phone" followed by one "|tag|" marker
// per tag, so that a free text search matches names, email, phone (also
// without the +39 prefix) and tags, and the tag filter matches "|tag|"
// exactly.
func searchText(f *fields) string {
	parts := []string{Fold(f.LastName), Fold(f.FirstName)}
	if f.Email != nil {
		parts = append(parts, *f.Email)
	}
	if f.Phone != nil {
		parts = append(parts, *f.Phone, strings.TrimPrefix(*f.Phone, "+"))
		if national, ok := strings.CutPrefix(*f.Phone, "+"+DefaultCountryCode); ok {
			parts = append(parts, national)
		}
	}
	s := strings.Join(nonEmpty(parts), " ")
	if len(f.Tags) > 0 {
		s += " |" + strings.Join(foldAll(f.Tags), "|") + "|"
	}
	return s
}

// SearchTerms splits a free text query into normalised terms (all must
// match). A query that looks like a phone number becomes one term of
// digits so that "333 123 4567" finds "+393331234567".
func SearchTerms(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	if looksLikePhone(q) {
		var b strings.Builder
		for _, r := range q {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		if b.Len() > 0 {
			return []string{b.String()}
		}
	}
	terms := strings.Fields(Fold(q))
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

func looksLikePhone(q string) bool {
	digits := 0
	for _, r := range q {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == ' ' || r == '.' || r == '-' || r == '/' || r == '(' || r == ')':
		default:
			return false
		}
	}
	return digits >= 3
}

// Fold lowercases s and removes the diacritics of Latin letters
// ("Nicolò D'Amico" -> "nicolo d'amico"), collapsing spaces.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(collapseSpaces(s)) {
		if rep, ok := foldMap[r]; ok {
			b.WriteString(rep)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// foldMap covers the Latin-1 and Latin Extended-A letters found in Italian,
// German, French and Spanish names (no external dependency needed).
var foldMap = func() map[rune]string {
	m := map[rune]string{'ß': "ss", 'æ': "ae", 'œ': "oe", 'ø': "o", 'đ': "d", 'ł': "l", 'ı': "i"}
	groups := map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ď", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀ", "n": "ñńņňŉ", "o": "òóôõöōŏő",
		"r": "ŕŗř", "s": "śŝşš", "t": "ţťŧ", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
	}
	for base, letters := range groups {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()

func collapseSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

func foldAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = Fold(s)
	}
	return out
}

func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// changedFields lists the names of the fields that differ between two
// customers. Audit entries record only these names, never the values: the
// audit log is insert-only and must not keep personal data after an
// anonymization (INV-30, spec 23).
func changedFields(before, after *store.Customer) []string {
	var out []string
	add := func(name string, changed bool) {
		if changed {
			out = append(out, name)
		}
	}
	add("first_name", before.FirstName != after.FirstName)
	add("last_name", before.LastName != after.LastName)
	add("email", deref(before.Email) != deref(after.Email))
	add("phone", deref(before.Phone) != deref(after.Phone))
	add("locale", before.Locale != after.Locale)
	add("notes", before.Notes != after.Notes)
	add("tags", strings.Join(before.Tags, "|") != strings.Join(after.Tags, "|"))
	add("agreement_id", deref(before.AgreementID) != deref(after.AgreementID))
	add("marketing_consent", consentKey(before.MarketingConsent) != consentKey(after.MarketingConsent))
	return out
}

func consentKey(c *store.MarketingConsent) string {
	if c == nil {
		return ""
	}
	return c.GivenAt.UTC().Format(time.RFC3339) + "|" + c.Source + "|" + c.TextVersion
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
