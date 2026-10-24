package checkout

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/online"
)

// TestValidateCustomer_PhoneOptional: the public checkout requires names
// and email (confirmation, magic link) but never the phone, even when the
// deprecated require_phone flag is still stored (ADR 0020); a given phone
// must be valid.
func TestValidateCustomer_PhoneOptional(t *testing.T) {
	legacy := online.Config{RequirePhone: true}
	tests := []struct {
		name      string
		in        CustomerInput
		wantField string
	}{
		{"no phone", CustomerInput{FirstName: "Mario", LastName: "Rossi", Email: "mario@example.it"}, ""},
		{"valid phone", CustomerInput{FirstName: "Mario", LastName: "Rossi", Email: "mario@example.it", Phone: "+39 333 1234567"}, ""},
		{"invalid phone", CustomerInput{FirstName: "Mario", LastName: "Rossi", Email: "mario@example.it", Phone: "12"}, "phone"},
		{"email still required", CustomerInput{FirstName: "Mario", LastName: "Rossi"}, "email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCustomer(tt.in, legacy)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != tt.wantField {
				t.Fatalf("err = %v, want one error on %s", err, tt.wantField)
			}
		})
	}
}
