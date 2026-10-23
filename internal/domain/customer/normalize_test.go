package customer

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

var now = time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC)

func TestNormalizePhone_Formats(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"+39 333 123 4567", "+393331234567", true},
		{"333-123.4567", "+393331234567", true},
		{"(333) 1234567", "+393331234567", true},
		{"0039 333 1234567", "+393331234567", true},
		{"06 1234 5678", "+390612345678", true}, // Italian landline keeps the 0
		{"+49 30 1234567", "+49301234567", true},
		{" +33 6 12 34 56 78 ", "+33612345678", true},
		{"+0 123456789", "+0123456789", false},
		{"12", "+3912", false},
		{"+1234567890123456", "+1234567890123456", false},
		{"333 abc", "333 abc", false},
		{"33+3123", "33+3123", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := NormalizePhone(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("NormalizePhone(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestValidEmail_Cases(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"mario.rossi@example.it", true},
		{"a+b@sub.example.com", true},
		{"mario", false},
		{"@example.it", false},
		{"mario@", false},
		{"mario@localhost", false},
		{"mario@example..it", false},
		{"mario rossi@example.it", false},
		{"a@b@example.it", false},
		{"mario@.example.it", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ValidEmail(NormalizeEmail(tt.in)); got != tt.ok {
				t.Fatalf("ValidEmail(%q) = %v", tt.in, got)
			}
		})
	}
}

func TestFold_Accents(t *testing.T) {
	tests := map[string]string{
		"Nicolò  D'Amico": "nicolo d'amico",
		"ÉLODIE Müller":   "elodie muller",
		"Straße":          "strasse",
		"Çağlar Łukasz":   "caglar lukasz",
		"":                "",
	}
	for in, want := range tests {
		if got := Fold(in); got != want {
			t.Fatalf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchTerms_Queries(t *testing.T) {
	tests := []struct {
		q    string
		want []string
	}{
		{"", nil},
		{"  Rossì Mario ", []string{"rossi", "mario"}},
		{"333 123 4567", []string{"3331234567"}},
		{"+39 333-1234567", []string{"393331234567"}},
		{"12", []string{"12"}}, // too short for a phone: a plain term
		{"mario@example.it", []string{"mario@example.it"}},
	}
	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			if got := SearchTerms(tt.q); !slices.Equal(got, tt.want) {
				t.Fatalf("SearchTerms(%q) = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
}

func TestValidate_Normalises(t *testing.T) {
	in := &Input{
		FirstName: "  Mario ", LastName: " De  Rossi ", Email: " Mario.Rossi@Example.IT ", Phone: "333 1234567",
		Tags: []string{"VIP", "vip", " Cliente  storico "}, Notes: " note ",
		MarketingConsent: &store.MarketingConsent{Source: "desk", TextVersion: "v1"},
	}
	f, err := validate(in, "en", now)
	if err != nil {
		t.Fatal(err)
	}
	if f.FirstName != "Mario" || f.LastName != "De Rossi" || *f.Email != "mario.rossi@example.it" ||
		*f.Phone != "+393331234567" || f.Locale != "en" || f.Notes != "note" {
		t.Fatalf("fields %+v", f)
	}
	if !slices.Equal(f.Tags, []string{"vip", "cliente storico"}) {
		t.Fatalf("tags %q", f.Tags)
	}
	if !f.MarketingConsent.GivenAt.Equal(now) {
		t.Fatalf("consent without given_at must default to now: %v", f.MarketingConsent.GivenAt)
	}
	st := searchText(&f)
	want := "de rossi mario mario.rossi@example.it +393331234567 393331234567 3331234567 |vip|cliente storico|"
	if st != want {
		t.Fatalf("search text %q, want %q", st, want)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name  string
		in    Input
		field string
	}{
		{"last name required", Input{FirstName: "Mario"}, "last_name"},
		{"bad email", Input{LastName: "R", Email: "x@"}, "email"},
		{"bad phone", Input{LastName: "R", Phone: "12"}, "phone"},
		{"bad locale", Input{LastName: "R", Locale: "es"}, "locale"},
		{"tag pipe", Input{LastName: "R", Tags: []string{"a|b"}}, "tags"},
		{"too many tags", Input{LastName: "R", Tags: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21"}}, "tags"},
		{"consent source", Input{LastName: "R", MarketingConsent: &store.MarketingConsent{TextVersion: "v1"}}, "marketing_consent.source"},
		{"consent future", Input{LastName: "R", MarketingConsent: &store.MarketingConsent{GivenAt: now.Add(time.Hour), Source: "desk", TextVersion: "v1"}}, "marketing_consent.given_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validate(&tt.in, "it", now)
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want validation error, got %v", err)
			}
			for _, f := range ve.Fields {
				if f.Field == tt.field {
					return
				}
			}
			t.Fatalf("no error on %q: %+v", tt.field, ve.Fields)
		})
	}
}

func TestChangedFields_NamesOnly(t *testing.T) {
	e1, e2 := "a@example.it", "b@example.it"
	before := &store.Customer{LastName: "Rossi", Email: &e1, Tags: []string{"vip"}}
	after := &store.Customer{LastName: "Rossi", Email: &e2, Tags: []string{"vip"}, Notes: "x"}
	if got := changedFields(before, after); !slices.Equal(got, []string{"email", "notes"}) {
		t.Fatalf("changed %q", got)
	}
}

func TestDuplicateError_IsConflictWithoutPII(t *testing.T) {
	err := error(&DuplicateError{ExistingID: "c-1", Field: "email"})
	if !errors.Is(err, configerr.ErrConflict) {
		t.Fatal("duplicate must be a conflict")
	}
	if got := err.Error(); got != "config: conflict: esiste già un cliente con la stessa email (existing_customer_id=c-1)" {
		t.Fatalf("message %q", got)
	}
}
