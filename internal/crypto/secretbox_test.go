package crypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func mustKey(t *testing.T, fill byte) []byte {
	t.Helper()
	return bytes.Repeat([]byte{fill}, KeySize)
}

func TestKeyRing_Roundtrip(t *testing.T) {
	kr, err := NewKeyRing(mustKey(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		pt   []byte
		aad  []byte
	}{
		{"empty", []byte{}, nil},
		{"text", []byte("sk_live_123"), nil},
		{"with aad", []byte("totp-seed"), []byte("user-1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := kr.Encrypt(tt.pt, tt.aad)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(ct, "v1:"+kr.ActiveKeyID()+":") {
				t.Fatalf("unexpected format %q", ct)
			}
			got, err := kr.Decrypt(ct, tt.aad)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.pt) {
				t.Fatalf("got %q want %q", got, tt.pt)
			}
		})
	}
}

func TestKeyRing_Nondeterministic(t *testing.T) {
	kr, _ := NewKeyRing(mustKey(t, 1))
	a, _ := kr.EncryptString("x")
	b, _ := kr.EncryptString("x")
	if a == b {
		t.Fatal("two encryptions of the same plaintext must differ")
	}
}

func TestKeyRing_TamperAndAAD(t *testing.T) {
	kr, _ := NewKeyRing(mustKey(t, 1))
	ct, _ := kr.Encrypt([]byte("secret"), []byte("a"))
	if _, err := kr.Decrypt(ct, []byte("b")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong aad: got %v", err)
	}
	raw := []byte(ct)
	raw[len(raw)-2] ^= 0x01
	if raw[len(raw)-2] == ct[len(ct)-2] {
		t.Fatal("tamper failed")
	}
	if _, err := kr.Decrypt(string(raw), []byte("a")); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	if _, err := kr.Decrypt("garbage", nil); !errors.Is(err, ErrMalformed) {
		t.Fatalf("garbage: got %v", err)
	}
}

func TestKeyRing_Rotation(t *testing.T) {
	oldKey, newKey := mustKey(t, 1), mustKey(t, 2)
	oldRing, _ := NewKeyRing(oldKey)
	ct, _ := oldRing.EncryptString("webhook-secret")

	// New active key first, old key kept for decryption.
	rotated, err := NewKeyRing(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated.NeedsRotation(ct) {
		t.Fatal("old ciphertext should need rotation")
	}
	pt, err := rotated.DecryptString(ct)
	if err != nil || pt != "webhook-secret" {
		t.Fatalf("decrypt with old key: %q %v", pt, err)
	}
	ct2, err := rotated.Rotate(ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.NeedsRotation(ct2) {
		t.Fatal("rotated ciphertext should use the active key")
	}
	same, _ := rotated.Rotate(ct2, nil)
	if same != ct2 {
		t.Fatal("rotating an up-to-date ciphertext must be a no-op")
	}
	// After dropping the old key, the rotated value still decrypts.
	onlyNew, _ := NewKeyRing(newKey)
	if pt, err := onlyNew.DecryptString(ct2); err != nil || pt != "webhook-secret" {
		t.Fatalf("decrypt rotated: %q %v", pt, err)
	}
	if _, err := onlyNew.DecryptString(ct); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("old ciphertext without old key: got %v", err)
	}
}

func TestParseKeyRing_Formats(t *testing.T) {
	k1 := mustKey(t, 7)
	k2 := mustKey(t, 9)
	tests := []struct {
		name    string
		in      string
		wantErr bool
		wantN   int
	}{
		{"std", base64.StdEncoding.EncodeToString(k1), false, 1},
		{"raw url", base64.RawURLEncoding.EncodeToString(k1), false, 1},
		{"list", base64.StdEncoding.EncodeToString(k1) + " , " + base64.StdEncoding.EncodeToString(k2), false, 2},
		{"duplicate collapsed", base64.StdEncoding.EncodeToString(k1) + "," + base64.StdEncoding.EncodeToString(k1), false, 1},
		{"empty", "", true, 0},
		{"short", base64.StdEncoding.EncodeToString([]byte("short")), true, 0},
		{"not base64", "!!!", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kr, err := ParseKeyRing(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(kr.keys) != tt.wantN {
				t.Fatalf("got %d keys want %d", len(kr.keys), tt.wantN)
			}
		})
	}
}

func TestGenerateKey_Valid(t *testing.T) {
	s, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKeyRing(s); err != nil {
		t.Fatal(err)
	}
}
