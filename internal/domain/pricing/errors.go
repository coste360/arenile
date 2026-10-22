package pricing

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/calendar"
)

var (
	// ErrNoPrice means no base rule (or fixed price) prices an item on a day:
	// the item is not sellable. The concrete error is a *NoPriceError.
	ErrNoPrice = errors.New("pricing: no price")
	// ErrInvalidRule means a rule, rate period, fixed price, promo code or
	// agreement is malformed. The concrete error is usually a *RuleError.
	ErrInvalidRule = errors.New("pricing: invalid rule")
	// ErrInvalidInput means the quote input or the catalog is inconsistent.
	ErrInvalidInput = errors.New("pricing: invalid input")
	// ErrPromoNotApplicable means the requested promo code is inactive,
	// outside its validity window, or matches nothing in the booking.
	ErrPromoNotApplicable = errors.New("pricing: promo code not applicable")
)

// NoPriceError details which item lacks a price.
type NoPriceError struct {
	Target        TargetType
	ItemID        string
	ItemCode      string
	PriceCategory string
	Date          calendar.Date
	Slots         calendar.Slots
}

func (e *NoPriceError) Error() string {
	return fmt.Sprintf("pricing: no price for %s %q (category %q) on %s %s",
		e.Target, e.ItemCode, e.PriceCategory, e.Date, e.Slots)
}

// Unwrap makes errors.Is(err, ErrNoPrice) work.
func (e *NoPriceError) Unwrap() error { return ErrNoPrice }

// RuleError details why a piece of price configuration is invalid.
type RuleError struct {
	RuleID string
	Field  string
	Reason string
}

func (e *RuleError) Error() string {
	return fmt.Sprintf("pricing: invalid rule %q: %s: %s", e.RuleID, e.Field, e.Reason)
}

// Unwrap makes errors.Is(err, ErrInvalidRule) work.
func (e *RuleError) Unwrap() error { return ErrInvalidRule }

func invalidInput(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}
