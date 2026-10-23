package establishment

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// SupportedLocales are the languages of the product (spec 20). Italian is
// the source language.
var SupportedLocales = []string{"it", "en", "de", "fr"}

// Defaults of a new establishment (spec 6.1).
const (
	DefaultTimezone = "Europe/Rome"
	DefaultCurrency = "EUR"
	DefaultLocale   = "it"
)

// Limits of the establishment configuration.
const (
	MaxNameLength        = 200
	MaxEmbedOrigins      = 20
	MaxLocalHolidays     = 366
	MaxCustomTextKeys    = 30
	MaxCustomTextLength  = 5000
	MaxURLLength         = 2048
	maxContactLength     = 254
	maxAddressPartLength = 200
)

// Theme enumerations (spec 12.5).
var (
	ThemeFontFamilies = []string{"Inter", "system", "serif"}
	ThemeRadii        = []string{"none", "sm", "md", "lg"}
)

var (
	hexColorRe      = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	currencyRe      = regexp.MustCompile(`^[A-Z]{3}$`)
	customTextKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	phoneRe         = regexp.MustCompile(`^\+?[0-9 ()./-]{4,32}$`)
)

// IsSupportedLocale reports whether l is one of SupportedLocales.
func IsSupportedLocale(l string) bool { return slices.Contains(SupportedLocales, l) }

// IsHexColor reports whether s is a CSS hex colour (#RGB or #RRGGBB).
func IsHexColor(s string) bool { return hexColorRe.MatchString(s) }

// ValidateTimezone checks that tz is an IANA zone loadable on this system.
func ValidateTimezone(tz string) error {
	if tz == "" || tz == "Local" || strings.TrimSpace(tz) != tz {
		return errors.New("empty or invalid time zone")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("unknown time zone %q", tz)
	}
	return nil
}

// NormalizeEmbedOrigin validates an origin allowed to frame the public page
// (spec 12.2) and returns its canonical form "https://host[:port]". Only
// HTTPS origins without path, query, fragment or credentials are accepted,
// plus plain HTTP on loopback hosts (localhost, *.localhost, 127.0.0.0/8,
// ::1), which browsers treat as secure contexts: a developer can try the
// widget on a local site before publishing it.
func NormalizeEmbedOrigin(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > MaxURLLength {
		return "", errors.New("empty origin")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("malformed origin")
	}
	if (u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname()))) || u.Opaque != "" || u.User != nil || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("not an https origin")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.ContainsAny(host, "*_ ") {
		return "", errors.New("invalid host")
	}
	if ip := net.ParseIP(host); ip == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return "", errors.New("invalid host")
			}
		}
	}
	scheme, defPort := "https", "443"
	if u.Scheme == "http" {
		scheme, defPort = "http", "80"
	}
	out := scheme + "://" + host
	if strings.Contains(host, ":") { // IPv6 literal
		out = scheme + "://[" + host + "]"
	}
	if p := u.Port(); p != "" {
		if p == defPort {
			return out, nil
		}
		out += ":" + p
	}
	return out, nil
}

// isLoopbackHost reports whether host is a loopback name or address.
func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validateEmbedOrigins normalizes and deduplicates origins.
func validateEmbedOrigins(origins []string, ve *configerr.ValidationError) []string {
	if len(origins) > MaxEmbedOrigins {
		ve.Add("allowed_embed_origins", fmt.Sprintf("Al massimo %d domini.", MaxEmbedOrigins))
		return origins
	}
	out := make([]string, 0, len(origins))
	for i, o := range origins {
		n, err := NormalizeEmbedOrigin(o)
		if err != nil {
			ve.Add(fmt.Sprintf("allowed_embed_origins[%d]", i), "Origine non valida: usa la forma https://www.esempio.it, senza percorso.")
			continue
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// validateLocales checks the default and enabled locales: enabled is a
// non-empty subset of SupportedLocales containing the default. It returns the
// deduplicated list.
func validateLocales(def string, enabled []string, ve *configerr.ValidationError) []string {
	if !IsSupportedLocale(def) {
		ve.Add("default_locale", "Lingua non supportata (it, en, de, fr).")
	}
	out := make([]string, 0, len(enabled))
	for i, l := range enabled {
		if !IsSupportedLocale(l) {
			ve.Add(fmt.Sprintf("enabled_locales[%d]", i), "Lingua non supportata (it, en, de, fr).")
			continue
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if len(enabled) == 0 {
		ve.Add("enabled_locales", "Abilita almeno una lingua.")
	} else if IsSupportedLocale(def) && !slices.Contains(out, def) {
		ve.Add("enabled_locales", "Deve includere la lingua predefinita.")
	}
	return out
}

// validateName checks a display name.
func validateName(field, name string, ve *configerr.ValidationError) string {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		ve.Add(field, "Obbligatorio.")
	case utf8.RuneCountInString(name) > MaxNameLength:
		ve.Add(field, fmt.Sprintf("Al massimo %d caratteri.", MaxNameLength))
	case strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		// Names end up in .ics parameters and email headers: one line only.
		ve.Add(field, "Non può contenere caratteri di controllo o a capo.")
	}
	return name
}

func validateContacts(email, phone string, ve *configerr.ValidationError) (string, string) {
	email = strings.TrimSpace(email)
	phone = strings.TrimSpace(phone)
	if email != "" {
		a, err := mail.ParseAddress(email)
		if err != nil || a.Address != email || len(email) > maxContactLength {
			ve.Add("contact_email", "Indirizzo email non valido.")
		}
	}
	if phone != "" && !phoneRe.MatchString(phone) {
		ve.Add("contact_phone", "Numero di telefono non valido.")
	}
	return email, phone
}

func validateAddress(a store.Address, ve *configerr.ValidationError) store.Address {
	parts := []struct {
		field string
		v     *string
	}{
		{"address.street", &a.Street},
		{"address.city", &a.City},
		{"address.province", &a.Province},
		{"address.postal_code", &a.PostalCode},
		{"address.country", &a.Country},
	}
	for _, p := range parts {
		*p.v = strings.TrimSpace(*p.v)
		if utf8.RuneCountInString(*p.v) > maxAddressPartLength {
			ve.Add(p.field, fmt.Sprintf("Al massimo %d caratteri.", maxAddressPartLength))
		}
	}
	return a
}

func validateGeo(g *store.Geo, ve *configerr.ValidationError) {
	if g == nil {
		return
	}
	if g.Lat < -90 || g.Lat > 90 {
		ve.Add("geo.lat", "Valore tra -90 e 90.")
	}
	if g.Lon < -180 || g.Lon > 180 {
		ve.Add("geo.lon", "Valore tra -180 e 180.")
	}
}

// ValidateTheme checks the public page theme (spec 12.5) and returns it
// normalized (trimmed URL, lowercase colours).
func ValidateTheme(t store.Theme, ve *configerr.ValidationError) store.Theme {
	t.LogoURL = strings.TrimSpace(t.LogoURL)
	if t.LogoURL != "" && !validLogoURL(t.LogoURL) {
		ve.Add("theme.logo_url", "Usa un indirizzo https:// oppure un percorso che inizia con /.")
	}
	for _, c := range []struct {
		field string
		v     *string
	}{{"theme.primary_color", &t.PrimaryColor}, {"theme.accent_color", &t.AccentColor}} {
		*c.v = strings.ToLower(strings.TrimSpace(*c.v))
		if *c.v != "" && !IsHexColor(*c.v) {
			ve.Add(c.field, "Colore esadecimale non valido (es. #0e6ba8).")
		}
	}
	if t.FontFamily != "" && !slices.Contains(ThemeFontFamilies, t.FontFamily) {
		ve.Add("theme.font_family", "Valori ammessi: Inter, system, serif.")
	}
	if t.Radius != "" && !slices.Contains(ThemeRadii, t.Radius) {
		ve.Add("theme.radius", "Valori ammessi: none, sm, md, lg.")
	}
	for locale, texts := range t.CustomTexts {
		field := "theme.custom_texts." + locale
		if !IsSupportedLocale(locale) {
			ve.Add(field, "Lingua non supportata (it, en, de, fr).")
			continue
		}
		if len(texts) > MaxCustomTextKeys {
			ve.Add(field, fmt.Sprintf("Al massimo %d testi per lingua.", MaxCustomTextKeys))
			continue
		}
		for k, v := range texts {
			if !customTextKeyRe.MatchString(k) {
				ve.Add(field+"."+k, "Chiave non valida (lettere minuscole, cifre e _).")
			} else if utf8.RuneCountInString(v) > MaxCustomTextLength {
				ve.Add(field+"."+k, fmt.Sprintf("Al massimo %d caratteri.", MaxCustomTextLength))
			}
		}
	}
	if len(t.CustomTexts) == 0 {
		t.CustomTexts = nil
	}
	return t
}

func validLogoURL(s string) bool {
	if len(s) > MaxURLLength || strings.ContainsAny(s, " \t\r\n\"'<>") {
		return false
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

// ValidateSettings checks the settings document (pricing settings of spec
// 8.1/8.5, local holidays, default booking policy of spec 9) and returns it
// normalized: holidays sorted and deduplicated, empty rounding = none.
// Currency is not part of the document (it comes from the establishment).
func ValidateSettings(s Settings, ve *configerr.ValidationError) Settings {
	p := &s.Pricing
	p.Currency = ""
	if p.HalfDayFactor.Sign() <= 0 || p.HalfDayFactor.Cmp(pricing.NewDecimal(1, 0)) > 0 {
		ve.Add("settings.pricing.half_day_factor", "Deve essere maggiore di 0 e al massimo 1.")
	}
	if p.Rounding == "" {
		p.Rounding = pricing.RoundingNone
	}
	if !p.Rounding.Valid() {
		ve.Add("settings.pricing.rounding", "Valori ammessi: none, euro, fifty_cents.")
	}
	if p.DefaultVATRateBP < 0 || p.DefaultVATRateBP > 10000 {
		ve.Add("settings.pricing.default_vat_rate_bp", "Aliquota in punti base tra 0 e 10000.")
	}
	if len(p.LocalHolidays) > MaxLocalHolidays {
		ve.Add("settings.pricing.local_holidays", fmt.Sprintf("Al massimo %d date.", MaxLocalHolidays))
	}
	p.LocalHolidays = sortedUniqueDates(p.LocalHolidays)
	for i, d := range p.LocalHolidays {
		if d.IsZero() {
			ve.Add(fmt.Sprintf("settings.pricing.local_holidays[%d]", i), "Data non valida.")
		}
	}
	if len(p.RecurringLocalHolidays) > MaxLocalHolidays {
		ve.Add("settings.pricing.recurring_local_holidays", fmt.Sprintf("Al massimo %d ricorrenze.", MaxLocalHolidays))
	}
	p.RecurringLocalHolidays = sortedUniqueMonthDays(p.RecurringLocalHolidays)
	for i, md := range p.RecurringLocalHolidays {
		if md.Month < time.January || md.Month > time.December || md.Day < 1 || md.Day > 31 {
			ve.Add(fmt.Sprintf("settings.pricing.recurring_local_holidays[%d]", i), "Ricorrenza non valida (MM-GG).")
		}
	}
	if err := s.DefaultPolicy.CheckConfig(); err != nil {
		ve.Add("settings.default_policy", "Policy non valida: "+err.Error())
	}
	return s
}

func sortedUniqueDates(ds []calendar.Date) []calendar.Date {
	if len(ds) == 0 {
		return nil
	}
	out := slices.Clone(ds)
	slices.SortFunc(out, func(a, b calendar.Date) int { return strings.Compare(a.String(), b.String()) })
	return slices.CompactFunc(out, func(a, b calendar.Date) bool { return a.Equal(b) })
}

func sortedUniqueMonthDays(ms []calendar.MonthDay) []calendar.MonthDay {
	if len(ms) == 0 {
		return nil
	}
	out := slices.Clone(ms)
	slices.SortFunc(out, func(a, b calendar.MonthDay) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(out)
}
