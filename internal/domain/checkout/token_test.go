package checkout

import (
	"regexp"
	"testing"
)

func TestNewToken_Format(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	seen := map[string]bool{}
	for range 100 {
		tok, hash, err := NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(tok) {
			t.Fatalf("token %q is not 43 base64url characters", tok)
		}
		if hash != HashToken(tok) || len(hash) != 64 {
			t.Fatalf("hash %q does not match the token", hash)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}

func TestHashToken_Stable(t *testing.T) {
	const want = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := HashToken(""); got != want {
		t.Fatalf("HashToken(\"\") = %s", got)
	}
}
