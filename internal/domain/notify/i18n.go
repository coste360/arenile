package notify

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"sync"
)

// Backend texts of the emails (spec 20): one flat JSON catalog per locale
// (i18n/<locale>.json, Italian is the source) for labels shared by every
// template, and one Markdown template per locale and code
// (templates/<locale>/<code>.md). Both are embedded in the binary. Lookups
// fall back to Italian, so a missing translation never breaks an email;
// TestCatalogs_KeyParity keeps the four catalogs and template sets aligned.

// Locales are the built-in locales; SourceLocale is the translation source.
var Locales = []string{"it", "en", "de", "fr"}

// SourceLocale is the language of the source texts (spec 20).
const SourceLocale = "it"

//go:embed i18n/*.json
var catalogFS embed.FS

//go:embed templates/*/*.md
var templateFS embed.FS

var (
	catalogsOnce sync.Once
	catalogs     map[string]map[string]string
	catalogsErr  error
)

func loadCatalogs() (map[string]map[string]string, error) {
	catalogsOnce.Do(func() {
		catalogs = map[string]map[string]string{}
		for _, l := range Locales {
			raw, err := fs.ReadFile(catalogFS, "i18n/"+l+".json")
			if err != nil {
				catalogsErr = fmt.Errorf("notify: catalog %s: %w", l, err)
				return
			}
			m := map[string]string{}
			if err := json.Unmarshal(raw, &m); err != nil {
				catalogsErr = fmt.Errorf("notify: catalog %s: %w", l, err)
				return
			}
			catalogs[l] = m
		}
	})
	return catalogs, catalogsErr
}

// NormalizeLocale maps a BCP 47 tag ("de-CH", "EN") to a built-in locale,
// or "" when unsupported.
func NormalizeLocale(tag string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	base, _, _ = strings.Cut(base, "_")
	for _, l := range Locales {
		if l == base {
			return l
		}
	}
	return ""
}

// pickLocale returns the first supported locale among candidates, else Italian.
func pickLocale(candidates ...string) string {
	for _, c := range candidates {
		if l := NormalizeLocale(c); l != "" {
			return l
		}
	}
	return SourceLocale
}

// catalog is the message lookup of one locale with fallback to Italian.
type catalog struct {
	locale string
	msgs   map[string]string
	source map[string]string
}

func catalogFor(locale string) (catalog, error) {
	all, err := loadCatalogs()
	if err != nil {
		return catalog{}, err
	}
	l := pickLocale(locale)
	return catalog{locale: l, msgs: all[l], source: all[SourceLocale]}, nil
}

// t returns the message key with {placeholders} replaced (no escaping:
// callers escape for HTML).
func (c catalog) t(key string, args map[string]string) string {
	s, ok := c.msgs[key]
	if !ok {
		s, ok = c.source[key]
	}
	if !ok {
		return key
	}
	for k, v := range args {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

func (c catalog) weekday(iso int) string { return c.t("weekday_"+strconv.Itoa(iso), nil) }
func (c catalog) month(m int) string     { return c.t("month_"+strconv.Itoa(m), nil) }

// templateSource is a parsed Markdown template: front matter + body. For
// the editable customer kinds (ADR 0022) the front matter also carries the
// default editable texts: subject, opening, closing and signature in the
// plain-text placeholder syntax ({nome}); "\n" in a value is a newline.
type templateSource struct {
	Subject   string
	Preheader string
	Body      string
	Opening   string
	Closing   string
	Signature string
}

// texts returns the editable texts of the front matter.
func (t templateSource) texts() Texts {
	return Texts{Subject: t.Subject, Opening: t.Opening, Closing: t.Closing, Signature: t.Signature}
}

// builtinTemplate loads the embedded template code in locale (Italian
// fallback).
func builtinTemplate(code, locale string) (templateSource, error) {
	for _, l := range []string{pickLocale(locale), SourceLocale} {
		raw, err := fs.ReadFile(templateFS, "templates/"+l+"/"+code+".md")
		if err == nil {
			return parseTemplate(string(raw))
		}
	}
	return templateSource{}, fmt.Errorf("%w: template %q", ErrUnknownTemplate, code)
}

// parseTemplate splits the "---" front matter (subject, preheader) from
// the Markdown body.
func parseTemplate(raw string) (templateSource, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var t templateSource
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return t, fmt.Errorf("%w: missing front matter", ErrUnknownTemplate)
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return t, fmt.Errorf("%w: unterminated front matter", ErrUnknownTemplate)
	}
	for _, line := range strings.Split(head, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "subject":
			t.Subject = strings.TrimSpace(v)
		case "preheader":
			t.Preheader = strings.TrimSpace(v)
		case "opening":
			t.Opening = frontMatterText(v)
		case "closing":
			t.Closing = frontMatterText(v)
		case "signature":
			t.Signature = frontMatterText(v)
		}
	}
	if t.Subject == "" {
		return t, fmt.Errorf("%w: empty subject", ErrUnknownTemplate)
	}
	t.Body = strings.TrimSpace(body)
	return t, nil
}

// frontMatterText decodes a multi-line front matter value ("\n" escapes).
func frontMatterText(v string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(v), `\n`, "\n"))
}
