package dailysale

import (
	"errors"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
)

// Formulas of a daily sale (bookings.formula).
const (
	FormulaDay     = "day"
	FormulaHalfDay = "half_day"
)

// ErrTarget: a daily sale needs exactly one of spot or zone.
var ErrTarget = errors.New("dailysale: exactly one of spot_id and zone_id is required")

// FormulaFor maps the chosen slot to the booking formula: FULL is a full
// day, AM or PM a half day.
func FormulaFor(s calendar.Slots) string {
	if s == calendar.SlotsFull {
		return FormulaDay
	}
	return FormulaHalfDay
}

// Target is where a daily sale goes: a precise spot (spot-first) or only a
// zone (zone-first, the ticket is "da assegnare").
type Target struct {
	SpotID string
	ZoneID string
}

// ZoneFirst reports whether the sale only picks a zone.
func (t Target) ZoneFirst() bool { return t.SpotID == "" && t.ZoneID != "" }

// Validate checks that exactly one of spot and zone is set.
func (t Target) Validate() error {
	if (t.SpotID == "") == (t.ZoneID == "") {
		return ErrTarget
	}
	return nil
}

// HolderName is the optional map label from first and last name: the last
// name when given (map labels show surnames), else the first name, trimmed
// and capped at 60 characters.
func HolderName(first, last string) string {
	name := strings.TrimSpace(last)
	if name == "" {
		name = strings.TrimSpace(first)
	}
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	return name
}
