// Package configerr holds the error kinds shared by the configuration
// services (establishment, team, season, mapping, catalog, pricingconfig,
// setup) so that internal/http/problem maps them to stable problem types in
// one place. Domain packages declare their own sentinels by wrapping these
// kinds, e.g.
//
//	var ErrOverlap = fmt.Errorf("%w: rate periods overlap", configerr.ErrConflict)
//
// and return the typed errors below when the response needs extra data.
package configerr

import (
	"errors"
	"fmt"
	"strings"
)

// Error kinds and their problem types.
var (
	// ErrNotFound -> 404 not_found (unknown or out-of-scope resource).
	ErrNotFound = errors.New("config: not found")
	// ErrValidation -> 400 validation_failed (see ValidationError for fields).
	ErrValidation = errors.New("config: validation failed")
	// ErrConflict -> 409 conflict (duplicate code, resource in use...).
	ErrConflict = errors.New("config: conflict")
	// ErrVersionConflict -> 412 version_conflict (see VersionConflictError).
	ErrVersionConflict = errors.New("config: version conflict")
	// ErrPreconditionRequired -> 428 precondition_required (If-Match missing).
	ErrPreconditionRequired = errors.New("config: if-match required")
	// ErrSpotHasFutureAllocations -> 409 spot_has_future_allocations
	// (see SpotsAllocatedError).
	ErrSpotHasFutureAllocations = errors.New("config: spot has future allocations")
	// ErrNotImplemented -> 501 not_implemented: an operation of the M3
	// scaffolding whose implementation has not landed yet.
	ErrNotImplemented = errors.New("not implemented")
)

// FieldError is one invalid field; Field uses the JSON path of the request
// body (e.g. "ranges[1].from"), Message is Italian (source language).
type FieldError struct {
	Field   string
	Message string
}

// ValidationError carries field errors; errors.Is(err, ErrValidation) holds.
type ValidationError struct {
	Fields []FieldError
}

// Invalid builds a ValidationError for one field.
func Invalid(field, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}

// Add appends a field error.
func (e *ValidationError) Add(field, message string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Message: message})
}

// OrNil returns e when it has field errors, nil otherwise.
func (e *ValidationError) OrNil() error {
	if e == nil || len(e.Fields) == 0 {
		return nil
	}
	return e
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Message
	}
	return "config: validation failed: " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrValidation) succeed.
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

// VersionConflictError reports the current version of a resource whose
// If-Match did not match; errors.Is(err, ErrVersionConflict) holds.
type VersionConflictError struct {
	Current int64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("config: version conflict (current %d)", e.Current)
}

// Is makes errors.Is(err, ErrVersionConflict) succeed.
func (e *VersionConflictError) Is(target error) bool { return target == ErrVersionConflict }

// SpotsAllocatedError lists the spots that cannot be deleted because they
// have future allocations; errors.Is(err, ErrSpotHasFutureAllocations) holds.
type SpotsAllocatedError struct {
	SpotIDs []string
}

func (e *SpotsAllocatedError) Error() string {
	return fmt.Sprintf("config: %d spots have future allocations", len(e.SpotIDs))
}

// Is makes errors.Is(err, ErrSpotHasFutureAllocations) succeed.
func (e *SpotsAllocatedError) Is(target error) bool { return target == ErrSpotHasFutureAllocations }

// CheckVersion returns a *VersionConflictError when expected is set and
// differs from current. expected nil means "no If-Match": allowed.
func CheckVersion(expected *int64, current int64) error {
	if expected != nil && *expected != current {
		return &VersionConflictError{Current: current}
	}
	return nil
}
