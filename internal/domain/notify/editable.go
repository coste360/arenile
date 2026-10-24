package notify

import (
	"errors"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// Editable customer email texts (ADR 0022, functional core). Each customer
// email kind has four editable texts per locale: subject, opening, closing
// and signature. They are plain text (no Markdown, no HTML): blank lines
// separate paragraphs, single newlines become <br>, every character is
// HTML-escaped. Placeholders {nome}, {codice}, ... come from a small fixed
// set (Placeholders) and are validated on save: an unknown one is rejected
// (422 email_template_invalid). Values are substituted before escaping, so
// a customer name can never inject markup. The fixed blocks (QR, summary,
// amounts, manage link, policy, legal footer) live in the built-in
// templates around the {{> opening}}, {{> closing}} and {{> signature}}
// partials and cannot be edited.
//
// The defaults are the front matter of the built-in templates
// (templates/<locale>/<kind>.md: subject, opening, closing, signature), so
// "restore default" is simply deleting the override row.

// EditableKinds are the customer email kinds with editable texts, in the
// order the admin page lists them.
var EditableKinds = []string{
	TemplateBookingConfirmed, TemplateBookingModified, TemplateBookingCancelled, TemplateBookingRejected,
	TemplateApprovalPending, TemplateRefundIssued, TemplateManageLink,
	TemplateBalanceReminder, TemplateDayBeforeReminder,
}

// IsEditableKind reports whether kind has editable texts.
func IsEditableKind(kind string) bool { return slices.Contains(EditableKinds, kind) }

// Placeholders is the safe set of placeholders, without braces.
var Placeholders = []string{
	"nome", "cognome", "codice", "data", "postazione", "stabilimento", "importo", "saldo", "scadenza",
}

// Length limits of the editable texts (runes).
const (
	MaxSubjectLen   = 200
	MaxBodyTextLen  = 2000
	MaxSignatureLen = 500
)

// Texts are the editable texts of one kind and locale.
type Texts struct {
	Subject   string
	Opening   string
	Closing   string
	Signature string
}

// ErrTemplateInvalid is the kind of TemplateError (422 email_template_invalid).
var ErrTemplateInvalid = errors.New("notify: email template invalid")

// TemplateError lists the invalid fields of edited texts.
type TemplateError struct {
	Fields []configerr.FieldError
}

func (e *TemplateError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Message
	}
	return "notify: email template invalid: " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrTemplateInvalid) hold.
func (e *TemplateError) Is(target error) bool { return target == ErrTemplateInvalid }

// rePlaceholder matches {name}; names outside Placeholders are rejected.
var rePlaceholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// reParagraph separates paragraphs (a blank line, possibly with spaces).
var reParagraph = regexp.MustCompile(`\n[ \t]*\n`)

// NormalizeTexts unifies line endings, trims every text and the trailing
// spaces of each line; the subject becomes one line.
func NormalizeTexts(t Texts) Texts {
	norm := func(s string) string {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return Texts{
		Subject: strings.TrimSpace(t.Subject), Opening: norm(t.Opening),
		Closing: norm(t.Closing), Signature: norm(t.Signature),
	}
}

// ValidateTexts checks normalised texts: subject required and on one line,
// length limits, no control characters, only known placeholders.
func ValidateTexts(t Texts) error {
	var fields []configerr.FieldError
	add := func(f, m string) { fields = append(fields, configerr.FieldError{Field: f, Message: m}) }
	check := func(field, v string, maxLen int, multiline bool) {
		if n := utf8.RuneCountInString(v); n > maxLen {
			add(field, "Testo troppo lungo: al massimo "+strconv.Itoa(maxLen)+" caratteri.")
			return
		}
		for _, r := range v {
			if r == '\n' && !multiline {
				add(field, "L'oggetto deve stare su una riga.")
				return
			}
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				add(field, "Il testo contiene caratteri non ammessi.")
				return
			}
		}
		if unknown := UnknownPlaceholders(v); len(unknown) > 0 {
			add(field, "Segnaposto sconosciuto: {"+strings.Join(unknown, "}, {")+"}. Ammessi: {"+strings.Join(Placeholders, "}, {")+"}.")
		}
	}
	if t.Subject == "" {
		add("subject", "L'oggetto è obbligatorio.")
	} else {
		check("subject", t.Subject, MaxSubjectLen, false)
	}
	check("opening", t.Opening, MaxBodyTextLen, true)
	check("closing", t.Closing, MaxBodyTextLen, true)
	check("signature", t.Signature, MaxSignatureLen, true)
	if len(fields) > 0 {
		return &TemplateError{Fields: fields}
	}
	return nil
}

// UnknownPlaceholders returns the placeholder names of s outside the safe
// set, in order of appearance and without duplicates.
func UnknownPlaceholders(s string) []string {
	var out []string
	for _, m := range rePlaceholder.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(Placeholders, m[1]) && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// ResolvePlaceholders replaces the known placeholders of s with their plain
// text values (missing values become empty); anything else is left as is.
func ResolvePlaceholders(s string, values map[string]string) string {
	return rePlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if !slices.Contains(Placeholders, name) {
			return m
		}
		return values[name]
	})
}

// ResolveTexts resolves the placeholders of every text.
func ResolveTexts(t Texts, values map[string]string) Texts {
	return Texts{
		Subject:   ResolvePlaceholders(t.Subject, values),
		Opening:   ResolvePlaceholders(t.Opening, values),
		Closing:   ResolvePlaceholders(t.Closing, values),
		Signature: ResolvePlaceholders(t.Signature, values),
	}
}

// plainTextHTML renders resolved plain text: paragraphs on blank lines,
// <br> on single newlines, everything escaped. Empty text renders nothing.
func plainTextHTML(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	var h, t []string
	for _, p := range reParagraph.Split(s, -1) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lines := strings.Split(p, "\n")
		esc := make([]string, len(lines))
		for i, l := range lines {
			esc[i] = html.EscapeString(strings.TrimSpace(l))
		}
		h = append(h, `<p style="`+styleP+`">`+strings.Join(esc, "<br>")+`</p>`)
		t = append(t, p)
	}
	return strings.Join(h, "\n"), strings.Join(t, "\n\n")
}

// placeholderValues maps the safe placeholders to the formatted values of
// a message (Content.Vars): {importo} is the refund amount in the refund
// email and the booking total elsewhere.
func placeholderValues(kind string, vars map[string]string) map[string]string {
	amount := vars["booking.total"]
	if kind == TemplateRefundIssued && vars["refund.amount"] != "" {
		amount = vars["refund.amount"]
	}
	return map[string]string{
		"nome": vars["customer.first_name"], "cognome": vars["customer.last_name"],
		"codice": vars["booking.code"], "data": vars["booking.period"], "postazione": vars["booking.spots"],
		"stabilimento": vars["establishment.name"], "importo": amount, "saldo": vars["booking.balance"],
		"scadenza": vars["balance.due"],
	}
}

// defaultTexts returns the built-in texts of an editable kind in locale.
func defaultTexts(kind, locale string) (Texts, error) {
	src, err := builtinTemplate(kind, locale)
	if err != nil {
		return Texts{}, err
	}
	return src.texts(), nil
}

// FixedBlocks lists the partials of the built-in template of kind between
// the editable texts (shown by the admin page).
func FixedBlocks(kind, locale string) []string {
	src, err := builtinTemplate(kind, locale)
	if err != nil {
		return nil
	}
	var out []string
	for _, b := range parseBlocks(src.Body) {
		if b.kind != "p" || len(b.lines) != 1 {
			continue
		}
		if m := rePartial.FindStringSubmatch(b.lines[0]); m != nil {
			switch m[1] {
			case "opening", "closing", "signature":
			default:
				out = append(out, m[1])
			}
		}
	}
	return out
}

// BalanceDueDate is the day the balance of b is due according to its
// policy snapshot (spec 9 balance_due): days_before:N -> start - N,
// on_arrival (or no rule) -> start date with onArrival = true. ok is false
// when the snapshot cannot be read.
func BalanceDueDate(b *store.Booking) (due calendar.Date, onArrival bool, ok bool) {
	snap, err := policy.ParseSnapshot(b.PolicySnapshot)
	if err != nil {
		return calendar.Date{}, false, false
	}
	if p := snap.Rules.Payment; p != nil && p.BalanceDue != nil {
		bd, err := policy.ParseBalanceDue(*p.BalanceDue)
		if err != nil {
			return calendar.Date{}, false, false
		}
		if !bd.OnArrival {
			return b.StartsOn.AddDays(-bd.DaysBefore), false, true
		}
	}
	return b.StartsOn, true, true
}
