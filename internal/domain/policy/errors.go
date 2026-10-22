package policy

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors of the policy package.
var (
	// ErrPolicyViolation is returned (wrapped in *ViolationError) when a
	// booking request does not satisfy the effective rules.
	ErrPolicyViolation = errors.New("policy: violation")
	// ErrInvalidRules is returned when a policy configuration is malformed.
	ErrInvalidRules = errors.New("policy: invalid rules")
	// ErrInvalidSnapshot is returned when a policy snapshot cannot be decoded.
	ErrInvalidSnapshot = errors.New("policy: invalid snapshot")
)

// Code is a stable, machine-readable violation code. Codes are part of the API
// contract (problem+json and i18n keys) and must never change meaning.
type Code string

// Violation codes.
const (
	CodeLeadTimeTooShort       Code = "lead_time_too_short"
	CodeCutoffPassed           Code = "cutoff_passed"
	CodeLeadTimeTooLong        Code = "lead_time_too_long"
	CodeSameDayClosed          Code = "same_day_closed"
	CodeFormulaNotAllowed      Code = "formula_not_allowed"
	CodeDurationTooShort       Code = "duration_too_short"
	CodeDurationTooLong        Code = "duration_too_long"
	CodeStartWeekdayNotAllowed Code = "start_weekday_not_allowed"
	CodeTooManySpots           Code = "too_many_spots"
)

// Violation is a single unmet rule. Params carry the values needed to render
// a localized message (e.g. {"min_days": "3"}).
type Violation struct {
	Code   Code              `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// ViolationError carries all violations of a request. errors.Is(err,
// ErrPolicyViolation) is true.
type ViolationError struct {
	Violations []Violation
}

func (e *ViolationError) Error() string {
	codes := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		codes[i] = string(v.Code)
	}
	return fmt.Sprintf("policy: violation: %s", strings.Join(codes, ", "))
}

// Is makes errors.Is(err, ErrPolicyViolation) succeed.
func (e *ViolationError) Is(target error) bool { return target == ErrPolicyViolation }

// Has reports whether the error contains a violation with the given code.
func (e *ViolationError) Has(c Code) bool {
	for _, v := range e.Violations {
		if v.Code == c {
			return true
		}
	}
	return false
}
