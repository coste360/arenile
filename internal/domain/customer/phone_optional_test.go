package customer

import "testing"

// TestValidate_PhoneOptional: a customer needs only the last name; the
// phone (ADR 0020) and the email are optional and stay nil when empty, so
// they never take part in the dedupe.
func TestValidate_PhoneOptional(t *testing.T) {
	tests := []struct {
		name    string
		in      Input
		wantErr bool
	}{
		{"last name only", Input{LastName: "Rossi"}, false},
		{"blank phone", Input{LastName: "Rossi", Phone: "   "}, false},
		{"email without phone", Input{LastName: "Rossi", Email: "a@b.it"}, false},
		{"invalid phone when given", Input{LastName: "Rossi", Phone: "abc"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			f, err := validate(&in, "it", now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && f.Phone != nil {
				t.Fatalf("phone = %q, want nil", *f.Phone)
			}
		})
	}
}
