package online

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

func fieldsOf(t *testing.T, err error) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	var ve *configerr.ValidationError
	if err == nil {
		return out
	}
	if !errors.As(err, &ve) {
		t.Fatalf("not a validation error: %v", err)
	}
	for _, f := range ve.Fields {
		out[f.Field] = true
	}
	return out
}

func TestValidateInput_Normalises(t *testing.T) {
	in, err := ValidateInput(Input{
		AllowedEmbedOrigins: []string{"https://www.Lido.it/", "https://www.lido.it", "https://shop.lido.it:8443"},
		Theme:               store.Theme{PrimaryColor: "#0E6BA8"},
		Flow:                Config{Formulas: []string{"day", "day", "week"}, Slots: []string{"full", "AM"}, MaxDaysAhead: 90},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.AllowedEmbedOrigins) != 2 || in.AllowedEmbedOrigins[0] != "https://www.lido.it" || in.Theme.PrimaryColor != "#0e6ba8" {
		t.Fatalf("origins/theme %+v", in)
	}
	if len(in.Flow.Formulas) != 2 || in.Flow.Slots[0] != "FULL" {
		t.Fatalf("flow %+v", in.Flow)
	}
}

func TestValidateInput_Errors(t *testing.T) {
	tests := []struct {
		name  string
		in    Input
		field string
	}{
		{"http origin", Input{AllowedEmbedOrigins: []string{"http://lido.it"}}, "allowed_embed_origins[0]"},
		{"origin with path", Input{AllowedEmbedOrigins: []string{"https://lido.it/prenota"}}, "allowed_embed_origins[0]"},
		{"wildcard", Input{AllowedEmbedOrigins: []string{"https://*.lido.it"}}, "allowed_embed_origins[0]"},
		{"colour", Input{Theme: store.Theme{AccentColor: "blue"}}, "theme.accent_color"},
		{"formula", Input{Flow: Config{Formulas: []string{"forever"}}}, "flow.formulas[0]"},
		{"slots", Input{Flow: Config{Slots: []string{"NIGHT"}}}, "flow.slots[0]"},
		{"days ahead", Input{Flow: Config{MaxDaysAhead: 5000}}, "flow.max_days_ahead"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateInput(tc.in)
			if !fieldsOf(t, err)[tc.field] {
				t.Fatalf("want error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestValidateTerms_Rules(t *testing.T) {
	ok, err := ValidateTerms(TermsInput{Version: " 2027.1 ", Texts: map[string]store.TermsTexts{"it": {Terms: " Condizioni "}}})
	if err != nil || ok.Version != "2027.1" || ok.Texts["it"].Terms != "Condizioni" {
		t.Fatalf("valid terms: %v %+v", err, ok)
	}
	for name, tc := range map[string]struct {
		in    TermsInput
		field string
	}{
		"no version": {TermsInput{Texts: map[string]store.TermsTexts{"it": {Terms: "x"}}}, "version"},
		"no texts":   {TermsInput{Version: "1"}, "texts"},
		"locale":     {TermsInput{Version: "1", Texts: map[string]store.TermsTexts{"es": {Terms: "x"}}}, "texts.es"},
		"empty text": {TermsInput{Version: "1", Texts: map[string]store.TermsTexts{"en": {Privacy: "p"}}}, "texts.en.terms"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateTerms(tc.in)
			if !fieldsOf(t, err)[tc.field] {
				t.Fatalf("want %s, got %v", tc.field, err)
			}
		})
	}
}

func TestValidateEmailConfig_Addresses(t *testing.T) {
	c, err := ValidateEmailConfig(EmailConfig{SenderName: " Lido ", ReplyTo: "Info@Lido.IT"})
	if err != nil || c.ReplyTo != "info@lido.it" || c.SenderName != "Lido" {
		t.Fatalf("%v %+v", err, c)
	}
	_, err = ValidateEmailConfig(EmailConfig{SenderName: "Lido\r\nBcc: x", Manager: "not-an-email"})
	f := fieldsOf(t, err)
	if !f["sender_name"] || !f["manager_email"] {
		t.Fatalf("fields %v", f)
	}
}

func TestParseConfig_Tolerant(t *testing.T) {
	c := ParseConfig([]byte(`{"formulas":["day"],"unknown":1,"require_phone":true}`))
	if len(c.Formulas) != 1 || !c.RequirePhone || !c.PromoCodesEnabled {
		t.Fatalf("parsed %+v", c)
	}
	if got := ParseConfig([]byte(`{broken`)); got.PromoCodesEnabled != DefaultConfig().PromoCodesEnabled {
		t.Fatalf("malformed must give defaults: %+v", got)
	}
	if !ParseEmailConfig(nil).ToCustomer.Confirmation {
		t.Fatal("email defaults")
	}
	if !c.AllowsSlots(calendar.SlotsAM) || !c.AllowsFormula("day") || c.AllowsFormula("week") {
		t.Fatal("allows")
	}
}

func TestApplyStripe_SecretsWriteOnly(t *testing.T) {
	creds, pub := ext.Credentials{}, map[string]any{}
	sk, wh, pk, emb := "sk_test_abcdefghijklmnop1234", "whsec_abcdefghij", "pk_test_abcdefghijklmn", true
	changed, err := applyStripe(creds, pub, &StripeInput{SecretKey: &sk, WebhookSecret: &wh, PublishableKey: &pk, EmbeddedMode: &emb}, "test")
	if err != nil || len(changed) != 4 {
		t.Fatalf("%v %v", err, changed)
	}
	if pub[pubSecretLast4] != "1234" || pub[pubWebhookSecretSet] != true || pub[pubPublishableKey] != pk {
		t.Fatalf("public hints %v", pub)
	}
	for _, v := range pub {
		if v == sk || v == wh {
			t.Fatal("a secret leaked into the public config")
		}
	}
	// nil keeps, "" clears.
	empty := ""
	if _, err := applyStripe(creds, pub, &StripeInput{WebhookSecret: &empty}, "test"); err != nil {
		t.Fatal(err)
	}
	if creds[CredSecretKey] != sk || creds[CredWebhookSecret] != "" || pub[pubWebhookSecretSet] != false {
		t.Fatalf("after clear %v %v", creds, pub)
	}
	live := "sk_live_abcdefghijklmnop"
	_, err = applyStripe(creds, pub, &StripeInput{SecretKey: &live}, "test")
	if !fieldsOf(t, err)["stripe.secret_key"] {
		t.Fatalf("live key in test mode accepted: %v", err)
	}
}

func TestAAD_Distinct(t *testing.T) {
	if string(AAD("e1", "stripe", "test")) == string(AAD("e1", "stripe", "live")) || Last4("ab") != "ab" || Last4("abcdef") != "cdef" {
		t.Fatal("aad/last4")
	}
}
