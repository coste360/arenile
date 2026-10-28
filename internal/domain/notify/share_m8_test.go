package notify

import "testing"

func TestWaNumber_Formats(t *testing.T) {
	cases := map[string]string{
		"+393331234567":     "393331234567",
		"+39 333 123 4567":  "393331234567",
		"0041791234567":     "41791234567",
		"+49 (30) 1234 567": "49301234567",
		"123":               "",
		"":                  "",
	}
	for in, want := range cases {
		if got := waNumber(in); got != want {
			t.Errorf("waNumber(%q) = %q, want %q", in, got, want)
		}
	}
}
