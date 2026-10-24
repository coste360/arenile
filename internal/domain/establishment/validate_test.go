package establishment

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

func fieldsOf(ve *configerr.ValidationError) []string {
	out := make([]string, len(ve.Fields))
	for i, f := range ve.Fields {
		out[i] = f.Field
	}
	return out
}

func TestNormalizeEmbedOrigin_Scenarios(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://www.lido-azzurro.it", "https://www.lido-azzurro.it", false},
		{"https://WWW.Lido.IT/", "https://www.lido.it", false},
		{"https://lido.it:8443", "https://lido.it:8443", false},
		{"https://lido.it:443", "https://lido.it", false},
		{" https://lido.it ", "https://lido.it", false},
		{"https://[::1]:8443", "https://[::1]:8443", false},
		{"http://lido.it", "", true},
		{"http://localhost:5173", "http://localhost:5173", false},
		{"http://127.0.0.1:8091", "http://127.0.0.1:8091", false},
		{"http://shop.localhost:80", "http://shop.localhost", false},
		{"http://[::1]:3000", "http://[::1]:3000", false},
		{"http://192.168.1.10", "", true},
		{"http://localhost.lido.it", "", true},
		{"https://lido.it/booking", "", true},
		{"https://lido.it?x=1", "", true},
		{"https://lido.it#top", "", true},
		{"https://user:pw@lido.it", "", true},
		{"https://*.lido.it", "", true},
		{"https://-lido.it", "", true},
		{"https://lido..it", "", true},
		{"lido.it", "", true},
		{"", "", true},
		{"javascript:alert(1)", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := NormalizeEmbedOrigin(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q, %v; want %q, err=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestValidateEmbedOrigins_DedupeAndLimit(t *testing.T) {
	ve := &configerr.ValidationError{}
	got := validateEmbedOrigins([]string{"https://a.it", "https://A.it/", "https://b.it"}, ve)
	if ve.OrNil() != nil || !slices.Equal(got, []string{"https://a.it", "https://b.it"}) {
		t.Fatalf("got %v %v", got, ve)
	}
	many := make([]string, MaxEmbedOrigins+1)
	for i := range many {
		many[i] = "https://a.it"
	}
	ve = &configerr.ValidationError{}
	validateEmbedOrigins(many, ve)
	if ve.OrNil() == nil {
		t.Fatal("want limit error")
	}
}

func TestValidateTimezone_Scenarios(t *testing.T) {
	tests := []struct {
		tz string
		ok bool
	}{
		{"Europe/Rome", true},
		{"UTC", true},
		{"America/New_York", true},
		{"", false},
		{"Local", false},
		{"Mars/Olympus", false},
		{"\tEurope/Rome", false},
	}
	for _, tt := range tests {
		if err := ValidateTimezone(tt.tz); (err == nil) != tt.ok {
			t.Errorf("%q: err %v", tt.tz, err)
		}
	}
}

func TestValidateLocales_Scenarios(t *testing.T) {
	tests := []struct {
		name    string
		def     string
		enabled []string
		want    []string
		fields  []string
	}{
		{"default only", "it", []string{"it"}, []string{"it"}, nil},
		{"all four, deduplicated", "en", []string{"it", "en", "de", "fr", "en"}, []string{"it", "en", "de", "fr"}, nil},
		{"unsupported locale", "it", []string{"it", "es"}, []string{"it"}, []string{"enabled_locales[1]"}},
		{"default missing from enabled", "de", []string{"it"}, []string{"it"}, []string{"enabled_locales"}},
		{"unsupported default", "es", []string{"it"}, []string{"it"}, []string{"default_locale"}},
		{"empty list", "it", []string{}, []string{}, []string{"enabled_locales"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			got := validateLocales(tt.def, tt.enabled, ve)
			if !slices.Equal(got, tt.want) || !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("got %v %v", got, fieldsOf(ve))
			}
		})
	}
}

func TestValidateTheme_Scenarios(t *testing.T) {
	yes := true
	tests := []struct {
		name   string
		theme  store.Theme
		fields []string
	}{
		{"empty theme", store.Theme{}, nil},
		{"spec 12.5 example", store.Theme{
			LogoURL: "https://cdn.lido.it/logo.png", PrimaryColor: "#0E6BA8", AccentColor: "#F2B134", FontFamily: "Inter",
			Radius: "md", ShowPricesOnMap: &yes, HideHeaderInEmbed: &yes, MapKit: "classic",
			CustomTexts: map[string]map[string]string{"it": {"intro": "Benvenuti", "terms": "..."}},
		}, nil},
		{"short hex and relative logo", store.Theme{PrimaryColor: "#abc", LogoURL: "/uploads/logo.png"}, nil},
		{"bad colours", store.Theme{PrimaryColor: "blue", AccentColor: "#12345"}, []string{"theme.primary_color", "theme.accent_color"}},
		{"bad enums", store.Theme{FontFamily: "Comic Sans", Radius: "xl", MapKit: "vintage"}, []string{"theme.font_family", "theme.radius", "theme.map_kit"}},
		{"every shipped map kit", store.Theme{MapKit: "modern"}, nil},
		{"http logo", store.Theme{LogoURL: "http://lido.it/logo.png"}, []string{"theme.logo_url"}},
		{"protocol-relative logo", store.Theme{LogoURL: "//evil.example/logo.png"}, []string{"theme.logo_url"}},
		{"javascript logo", store.Theme{LogoURL: "javascript:alert(1)"}, []string{"theme.logo_url"}},
		{"unsupported text locale", store.Theme{CustomTexts: map[string]map[string]string{"es": {"intro": "Hola"}}}, []string{"theme.custom_texts.es"}},
		{"bad text key", store.Theme{CustomTexts: map[string]map[string]string{"it": {"Intro Text": "x"}}}, []string{"theme.custom_texts.it.Intro Text"}},
		{"text too long", store.Theme{CustomTexts: map[string]map[string]string{"it": {"intro": strings.Repeat("a", MaxCustomTextLength+1)}}}, []string{"theme.custom_texts.it.intro"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			got := ValidateTheme(tt.theme, ve)
			if !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("fields %v, want %v", fieldsOf(ve), tt.fields)
			}
			if tt.fields == nil && got.PrimaryColor != strings.ToLower(tt.theme.PrimaryColor) {
				t.Fatalf("colour not normalized: %q", got.PrimaryColor)
			}
		})
	}
}

func TestValidateSettings_Scenarios(t *testing.T) {
	mod := func(f func(*Settings)) Settings {
		s := DefaultSettings()
		f(&s)
		return s
	}
	tests := []struct {
		name   string
		in     Settings
		fields []string
	}{
		{"defaults", DefaultSettings(), nil},
		{"full factor and rates", mod(func(s *Settings) {
			s.Pricing.HalfDayFactor = pricing.NewDecimal(1, 0)
			s.Pricing.Rounding = pricing.RoundingFiftyCents
			s.Pricing.DefaultVATRateBP = 2200
		}), nil},
		{"zero factor", mod(func(s *Settings) { s.Pricing.HalfDayFactor = pricing.NewDecimal(0, 0) }), []string{"settings.pricing.half_day_factor"}},
		{"factor above one", mod(func(s *Settings) { s.Pricing.HalfDayFactor = pricing.MustParseDecimal("1.2") }), []string{"settings.pricing.half_day_factor"}},
		{"unknown rounding", mod(func(s *Settings) { s.Pricing.Rounding = "cents" }), []string{"settings.pricing.rounding"}},
		{"negative vat", mod(func(s *Settings) { s.Pricing.DefaultVATRateBP = -1 }), []string{"settings.pricing.default_vat_rate_bp"}},
		{"vat above 100%", mod(func(s *Settings) { s.Pricing.DefaultVATRateBP = 10001 }), []string{"settings.pricing.default_vat_rate_bp"}},
		{"invalid policy", mod(func(s *Settings) { s.DefaultPolicy.MaxLeadTimeDays = policy.Ptr(-3) }), []string{"settings.default_policy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			ValidateSettings(tt.in, ve)
			if !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("fields %v, want %v (%v)", fieldsOf(ve), tt.fields, ve)
			}
		})
	}
}

func TestValidateSettings_NormalizesHolidays(t *testing.T) {
	s := DefaultSettings()
	s.Pricing.Rounding = ""
	s.Pricing.Currency = "EUR"
	d1, d2 := calendar.MustParse("2027-08-16"), calendar.MustParse("2027-06-24")
	s.Pricing.LocalHolidays = []calendar.Date{d1, d2, d1}
	md, _ := calendar.ParseMonthDay("06-24")
	md2, _ := calendar.ParseMonthDay("01-31")
	s.Pricing.RecurringLocalHolidays = []calendar.MonthDay{md, md2, md}
	ve := &configerr.ValidationError{}
	got := ValidateSettings(s, ve)
	if ve.OrNil() != nil {
		t.Fatal(ve)
	}
	if got.Pricing.Rounding != pricing.RoundingNone || got.Pricing.Currency != "" {
		t.Fatalf("pricing %+v", got.Pricing)
	}
	if len(got.Pricing.LocalHolidays) != 2 || !got.Pricing.LocalHolidays[0].Equal(d2) {
		t.Fatalf("holidays %v", got.Pricing.LocalHolidays)
	}
	if len(got.Pricing.RecurringLocalHolidays) != 2 || got.Pricing.RecurringLocalHolidays[0] != md2 {
		t.Fatalf("recurring %v", got.Pricing.RecurringLocalHolidays)
	}
}

func TestOwnedOrganization_Scenarios(t *testing.T) {
	owner := func(orgs ...string) *auth.Principal {
		p := &auth.Principal{Kind: auth.PrincipalUser, UserID: "u"}
		for _, o := range orgs {
			p.Grants = append(p.Grants, auth.Grant{OrganizationID: o, Role: auth.RoleOwner})
		}
		return p
	}
	estOwner := &auth.Principal{Kind: auth.PrincipalUser, Grants: []auth.Grant{{OrganizationID: "o1", EstablishmentID: "e1", Role: auth.RoleOwner}}}
	manager := &auth.Principal{Kind: auth.PrincipalUser, Grants: []auth.Grant{{OrganizationID: "o1", Role: auth.RoleManager}}}
	apiKey := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyOrganizationID: "o1"}
	totp := owner("o1")
	totp.TOTPEnrolmentRequired = true
	tests := []struct {
		name      string
		p         *auth.Principal
		requested string
		want      string
		wantErr   error
	}{
		{"single owned organization", owner("o1"), "", "o1", nil},
		{"requested owned organization", owner("o1", "o2"), "o2", "o2", nil},
		{"several organizations need a choice", owner("o1", "o2"), "", "", configerr.ErrValidation},
		{"requested foreign organization", owner("o1"), "o9", "", auth.ErrForbidden},
		{"establishment-level owner", estOwner, "", "", auth.ErrForbidden},
		{"manager", manager, "", "", auth.ErrForbidden},
		{"api key", apiKey, "", "", auth.ErrForbidden},
		{"totp enrolment pending", totp, "", "", auth.ErrTOTPEnrolmentNeeded},
		{"anonymous", nil, "", "", auth.ErrUnauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ownedOrganization(tt.p, tt.requested)
			if got != tt.want || (tt.wantErr == nil) != (err == nil) || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("got %q %v; want %q %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestApplyUpdate_PartialAndValidation(t *testing.T) {
	cur := store.NewEstablishmentDefaults("o1", "lido-a", "Lido A", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	name, slug, tz := "  Lido Azzurro ", "Lido-Azzurro", "Europe/Berlin"
	next := *cur
	changes, settingsChanged, err := applyUpdate(&next, cur, UpdateInput{Name: &name, Slug: &slug, Timezone: &tz})
	if err != nil || settingsChanged {
		t.Fatalf("err %v settings %v", err, settingsChanged)
	}
	if next.Name != "Lido Azzurro" || next.Slug != "lido-azzurro" || next.Timezone != "Europe/Berlin" || next.Currency != "EUR" {
		t.Fatalf("next %+v", next)
	}
	if _, ok := changes["slug"]; !ok {
		t.Fatalf("changes %v", changes)
	}

	bad := "assets"
	badTZ := "Nowhere/City"
	locales := []string{"en"}
	next = *cur
	_, _, err = applyUpdate(&next, cur, UpdateInput{Slug: &bad, Timezone: &badTZ, EnabledLocales: &locales})
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) || !slices.Equal(fieldsOf(ve), []string{"slug", "timezone", "enabled_locales"}) {
		t.Fatalf("err %v", err)
	}

	s := DefaultSettings()
	s.Pricing.DefaultVATRateBP = 2200
	next = *cur
	_, settingsChanged, err = applyUpdate(&next, cur, UpdateInput{Settings: &s})
	if err != nil || !settingsChanged {
		t.Fatalf("settings change: %v %v", err, settingsChanged)
	}
	parsed, err := ParseSettings(next.Settings, "EUR")
	if err != nil || parsed.Pricing.DefaultVATRateBP != 2200 || parsed.Pricing.Currency != "EUR" {
		t.Fatalf("stored settings %s %v", next.Settings, err)
	}

	empty := ""
	ref := " CONC-12 "
	next = *cur
	if _, _, err := applyUpdate(&next, cur, UpdateInput{ConcessionRef: &ref}); err != nil || *next.ConcessionRef != "CONC-12" {
		t.Fatalf("concession ref %v", err)
	}
	cur2 := next
	if _, _, err := applyUpdate(&next, &cur2, UpdateInput{ConcessionRef: &empty}); err != nil || next.ConcessionRef != nil {
		t.Fatalf("remove concession ref %v", err)
	}
}
