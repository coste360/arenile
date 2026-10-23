package online

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// Pure part of the package (functional core): defaults, parsing and
// validation of the online configuration, theme helpers. No I/O, no clock.

// Limits of the online configuration.
const (
	// MaxDaysAheadLimit bounds Config.MaxDaysAhead (two seasons).
	MaxDaysAheadLimit = 730
	// MaxTermsVersionLen bounds the terms version label.
	MaxTermsVersionLen = 50
	// MaxTermsTextLen bounds each terms text (runes).
	MaxTermsTextLen = 50000
	// MaxSenderNameLen bounds the email sender name.
	MaxSenderNameLen = 100
)

// DefaultSlots are the slots offered online when the configuration has none.
var DefaultSlots = []string{string(calendar.SlotsFull), string(calendar.SlotsAM), string(calendar.SlotsPM)}

// DefaultConfig is the online flow configuration of an establishment that
// never saved one.
func DefaultConfig() Config {
	return Config{ShowRatePeriods: false, PromoCodesEnabled: true, RequirePhone: false}
}

// DefaultEmailConfig is the email configuration of an establishment that
// never saved one: every customer notification on, manager notifications
// for new online bookings, approvals and failures.
func DefaultEmailConfig() EmailConfig {
	return EmailConfig{
		ToManager: ManagerNotifications{NewOnlineBooking: true, ApprovalRequested: true, PaymentFailed: true, WebhookError: true},
		ToCustomer: CustomerNotifications{
			Confirmation: true, BalanceReminder: true, DayBeforeReminder: true, Cancellation: true, Refund: true,
		},
	}
}

// ParseConfig decodes online_settings.config over the defaults; unknown
// fields are ignored (forward compatible) and a malformed document yields
// the defaults (a stored row must never break the public page).
func ParseConfig(raw []byte) Config {
	c := DefaultConfig()
	if len(raw) == 0 {
		return c
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return DefaultConfig()
	}
	return c
}

// ParseEmailConfig decodes online_settings.email_config over the defaults.
func ParseEmailConfig(raw []byte) EmailConfig {
	c := DefaultEmailConfig()
	if len(raw) == 0 || string(raw) == "{}" {
		return c
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return DefaultEmailConfig()
	}
	return c
}

// EffectiveSlots returns the slots offered online (DefaultSlots when none).
func (c Config) EffectiveSlots() []string {
	if len(c.Slots) == 0 {
		return slices.Clone(DefaultSlots)
	}
	return slices.Clone(c.Slots)
}

// AllowsSlots reports whether s is offered online.
func (c Config) AllowsSlots(s calendar.Slots) bool {
	return slices.Contains(c.EffectiveSlots(), string(s))
}

// AllowsFormula reports whether f is offered online (every formula when the
// configuration lists none: the packs and the policy decide).
func (c Config) AllowsFormula(f string) bool {
	return len(c.Formulas) == 0 || slices.Contains(c.Formulas, f)
}

// ValidateConfig normalises c (deduplicated formulas and slots) and
// collects every field error under "flow.".
func ValidateConfig(c Config) (Config, error) {
	ve := &configerr.ValidationError{}
	var formulas []string
	for i, f := range c.Formulas {
		f = strings.TrimSpace(f)
		if !policy.Formula(f).Valid() {
			ve.Add(fmt.Sprintf("flow.formulas[%d]", i), "Formula sconosciuta.")
			continue
		}
		if !slices.Contains(formulas, f) {
			formulas = append(formulas, f)
		}
	}
	var slots []string
	for i, s := range c.Slots {
		s = strings.ToUpper(strings.TrimSpace(s))
		if !calendar.Slots(s).Valid() {
			ve.Add(fmt.Sprintf("flow.slots[%d]", i), "Usa AM, PM o FULL.")
			continue
		}
		if !slices.Contains(slots, s) {
			slots = append(slots, s)
		}
	}
	if c.MaxDaysAhead < 0 || c.MaxDaysAhead > MaxDaysAheadLimit {
		ve.Add("flow.max_days_ahead", fmt.Sprintf("Valore tra 0 e %d giorni.", MaxDaysAheadLimit))
	}
	c.Formulas, c.Slots = formulas, slots
	return c, ve.OrNil()
}

// ValidateOrigins normalises the origins allowed to frame the public page
// (https only, no path, deduplicated, at most establishment.MaxEmbedOrigins).
func ValidateOrigins(origins []string, ve *configerr.ValidationError) []string {
	if len(origins) > establishment.MaxEmbedOrigins {
		ve.Add("allowed_embed_origins", fmt.Sprintf("Al massimo %d domini.", establishment.MaxEmbedOrigins))
		return []string{}
	}
	out := make([]string, 0, len(origins))
	for i, o := range origins {
		n, err := establishment.NormalizeEmbedOrigin(o)
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

// ValidateInput validates a PUT online-settings request and returns it
// normalised.
func ValidateInput(in Input) (Input, error) {
	ve := &configerr.ValidationError{}
	in.AllowedEmbedOrigins = ValidateOrigins(in.AllowedEmbedOrigins, ve)
	in.Theme = establishment.ValidateTheme(in.Theme, ve)
	flow, err := ValidateConfig(in.Flow)
	if err != nil {
		var fe *configerr.ValidationError
		if ok := asValidation(err, &fe); ok {
			ve.Fields = append(ve.Fields, fe.Fields...)
		}
	}
	in.Flow = flow
	return in, ve.OrNil()
}

// ValidateTerms validates a terms publication: a label (1..50 characters)
// and, for each supported locale given, a non-empty terms text.
func ValidateTerms(in TermsInput) (TermsInput, error) {
	ve := &configerr.ValidationError{}
	in.Version = strings.TrimSpace(in.Version)
	switch {
	case in.Version == "":
		ve.Add("version", "Campo obbligatorio.")
	case utf8.RuneCountInString(in.Version) > MaxTermsVersionLen:
		ve.Add("version", fmt.Sprintf("Al massimo %d caratteri.", MaxTermsVersionLen))
	}
	if len(in.Texts) == 0 {
		ve.Add("texts", "Inserisci il testo delle condizioni in almeno una lingua.")
	}
	out := make(map[string]store.TermsTexts, len(in.Texts))
	for locale, t := range in.Texts {
		f := "texts." + locale
		if !establishment.IsSupportedLocale(locale) {
			ve.Add(f, "Lingua non supportata (it, en, de, fr).")
			continue
		}
		t.Terms, t.Privacy, t.Marketing = strings.TrimSpace(t.Terms), strings.TrimSpace(t.Privacy), strings.TrimSpace(t.Marketing)
		if t.Terms == "" {
			ve.Add(f+".terms", "Il testo delle condizioni è obbligatorio.")
		}
		for name, v := range map[string]string{"terms": t.Terms, "privacy": t.Privacy, "marketing": t.Marketing} {
			if utf8.RuneCountInString(v) > MaxTermsTextLen {
				ve.Add(f+"."+name, fmt.Sprintf("Al massimo %d caratteri.", MaxTermsTextLen))
			}
		}
		out[locale] = t
	}
	in.Texts = out
	return in, ve.OrNil()
}

// ValidateEmailConfig validates the email settings: sender name, optional
// reply-to and manager addresses.
func ValidateEmailConfig(c EmailConfig) (EmailConfig, error) {
	ve := &configerr.ValidationError{}
	c.SenderName = strings.TrimSpace(c.SenderName)
	if utf8.RuneCountInString(c.SenderName) > MaxSenderNameLen || strings.ContainsAny(c.SenderName, "\r\n<>\"") {
		ve.Add("sender_name", fmt.Sprintf("Al massimo %d caratteri, senza a capo né < > \".", MaxSenderNameLen))
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"reply_to", &c.ReplyTo}, {"manager_email", &c.Manager}} {
		*f.v = customer.NormalizeEmail(*f.v)
		if *f.v != "" && !customer.ValidEmail(*f.v) {
			ve.Add(f.name, "Indirizzo email non valido.")
		}
	}
	return c, ve.OrNil()
}

// AAD is the additional authenticated data of the encrypted credentials of
// provider in mode for an establishment (ADR 0013 §7): a ciphertext copied
// to another establishment, provider or mode never decrypts.
func AAD(establishmentID, provider, mode string) []byte {
	return []byte(establishmentID + ":" + provider + ":" + mode)
}

// Last4 returns the last four characters of a secret (for display).
func Last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}

func asValidation(err error, target **configerr.ValidationError) bool {
	return errors.As(err, target)
}
