package configerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestKinds_Is(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind error
	}{
		{"validation", Invalid("code", "obbligatorio"), ErrValidation},
		{"version", &VersionConflictError{Current: 3}, ErrVersionConflict},
		{"spots", &SpotsAllocatedError{SpotIDs: []string{"a"}}, ErrSpotHasFutureAllocations},
		{"wrapped", fmt.Errorf("season: %w", &VersionConflictError{Current: 1}), ErrVersionConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.kind) {
				t.Fatalf("errors.Is(%v, %v) = false", tt.err, tt.kind)
			}
		})
	}
}

func TestCheckVersion_Scenarios(t *testing.T) {
	v := func(n int64) *int64 { return &n }
	if err := CheckVersion(nil, 4); err != nil {
		t.Fatalf("nil expected: %v", err)
	}
	if err := CheckVersion(v(4), 4); err != nil {
		t.Fatalf("match: %v", err)
	}
	var vc *VersionConflictError
	if err := CheckVersion(v(3), 4); !errors.As(err, &vc) || vc.Current != 4 {
		t.Fatalf("mismatch: %v", err)
	}
}

func TestValidationError_OrNil(t *testing.T) {
	var e ValidationError
	if e.OrNil() != nil {
		t.Fatal("empty must be nil")
	}
	e.Add("name", "obbligatorio")
	if e.OrNil() == nil || e.Error() == "" {
		t.Fatal("non-empty must be an error")
	}
}
