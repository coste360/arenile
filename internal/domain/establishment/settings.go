package establishment

import (
	"fmt"

	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Settings is the versioned settings document of an establishment
// (establishments.settings, spec 6.1): pricing settings (spec 8.1, 8.5) and
// the default booking policy that scoped policies inherit from (spec 9).
type Settings struct {
	Pricing       pricing.Settings `json:"pricing"`
	DefaultPolicy policy.Rules     `json:"default_policy"`
}

// DefaultHalfDayFactor is the half-day factor of a new establishment.
var DefaultHalfDayFactor = pricing.NewDecimal(5, 1) // 0.5

// DefaultSettings returns the settings of a new establishment. The VAT rate
// is deliberately 0: rates are set by the manager, never fixed in code
// (spec 8.5).
func DefaultSettings() Settings {
	return Settings{
		Pricing:       pricing.Settings{HalfDayFactor: DefaultHalfDayFactor, Rounding: pricing.RoundingNone},
		DefaultPolicy: policy.DefaultRules(),
	}
}

// ParseSettings decodes a stored settings document over DefaultSettings, so
// that missing keys keep their default. Currency comes from the
// establishment row, not from the document.
func ParseSettings(raw store.JSON, currency string) (Settings, error) {
	s := DefaultSettings()
	if err := raw.Unmarshal(&s); err != nil {
		return DefaultSettings(), fmt.Errorf("establishment: settings: %w", err)
	}
	if s.Pricing.HalfDayFactor.Sign() <= 0 {
		s.Pricing.HalfDayFactor = DefaultHalfDayFactor
	}
	if s.Pricing.Rounding == "" {
		s.Pricing.Rounding = pricing.RoundingNone
	}
	s.Pricing.Currency = currency
	return s, nil
}
