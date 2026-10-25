package middleware

import "testing"

func TestIPPrefixKey_Networks(t *testing.T) {
	tests := []struct{ ip, want string }{
		{"203.0.113.7", "203.0.113.7"},
		{"2001:db8:1:2:aaaa::1", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:ffff:ffff:ffff:ffff", "2001:db8:1:2::/64"},
		{"::ffff:203.0.113.7", "::ffff:203.0.113.7"},
		{"not-an-ip", "not-an-ip"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := IPPrefixKey(tt.ip); got != tt.want {
				t.Fatalf("IPPrefixKey(%q) = %q, want %q", tt.ip, got, tt.want)
			}
		})
	}
}
