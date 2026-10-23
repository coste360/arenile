package device

import (
	"strings"
	"testing"
)

func TestToken_RoundTrip(t *testing.T) {
	tok, prefix, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if !IsToken(tok) || !strings.HasPrefix(tok, DisplayPrefix(prefix)+"_") {
		t.Fatalf("token %q prefix %q", tok, prefix)
	}
	got, ok := ParseToken(tok)
	if !ok || got != prefix {
		t.Fatalf("ParseToken = %q, %v", got, ok)
	}
	if HashToken(tok) == HashToken(tok+"x") || len(HashToken(tok)) != 64 {
		t.Fatal("hash")
	}
}

func TestParseToken_Rejects(t *testing.T) {
	tests := []string{
		"",
		"ar_live_abcdefgh_" + strings.Repeat("a", 52),
		"ar_dev_abcdefg_" + strings.Repeat("a", 52),
		"ar_dev_abcdefgh_" + strings.Repeat("a", 39),
		"ar_dev_abcdefgh_" + strings.Repeat("A", 52),
		"ar_dev_abcdefgh" + strings.Repeat("a", 52),
	}
	for _, tt := range tests {
		if _, ok := ParseToken(tt); ok {
			t.Errorf("ParseToken(%q) accepted", tt)
		}
	}
}

func TestPairingCode_Scenarios(t *testing.T) {
	code, err := NewPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("display form %q", code)
	}
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{code, strings.ReplaceAll(code, "-", ""), true},
		{"abcd-efgh", "ABCDEFGH", true},
		{" ab cd ef gh ", "ABCDEFGH", true},
		{"O1IL-2345", "01112345", true},
		{"ABCD-EFG", "", false},
		{"ABCD-EFGHJ", "", false},
		{"ABCD-EFGU", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizePairingCode(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("NormalizePairingCode(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	h1, _ := HashPairingCode("abcd-efgh")
	h2, _ := HashPairingCode("ABCDEFGH")
	if h1 == "" || h1 != h2 {
		t.Fatal("hash must ignore formatting")
	}
}

func TestQR_RoundTrip(t *testing.T) {
	in := QRPayload{URL: "https://192.168.1.20:8443", Code: "ABCD-EFGH", Fingerprint: "AA:BB"}
	text, err := EncodeQR(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeQR(text)
	if err != nil {
		t.Fatal(err)
	}
	in.V = QRVersion
	if out != in {
		t.Fatalf("roundtrip %+v != %+v", out, in)
	}
	for _, bad := range []string{"", "https://x", "arenile://pair?d=!!", "arenile://pair?d=e30"} {
		if _, err := DecodeQR(bad); err == nil {
			t.Errorf("DecodeQR(%q) accepted", bad)
		}
	}
}
