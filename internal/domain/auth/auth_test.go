package auth

import (
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/store"
)

var fastParams = Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestHasher_HashVerify(t *testing.T) {
	h := NewHasher(fastParams)
	phc, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Fatalf("not a PHC argon2id string: %s", phc)
	}
	tests := []struct {
		name string
		pw   string
		want bool
	}{
		{"correct", "correct horse battery", true},
		{"wrong", "correct horse batterY", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := h.Verify(tt.pw, phc)
			if err != nil || ok != tt.want {
				t.Fatalf("got %v %v", ok, err)
			}
		})
	}
	other, _ := h.Hash("correct horse battery")
	if other == phc {
		t.Fatal("salts must differ")
	}
}

func TestHasher_InvalidAndRehash(t *testing.T) {
	h := NewHasher(fastParams)
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "$argon2id$v=18$m=1024,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "$argon2id$v=19$m=0,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA"} {
		if _, err := h.Verify("x", bad); !errors.Is(err, ErrInvalidHash) {
			t.Fatalf("%q: got %v", bad, err)
		}
	}
	phc, _ := h.Hash("password123")
	if h.NeedsRehash(phc) {
		t.Fatal("same params must not need rehash")
	}
	stronger := NewHasher(Argon2Params{Memory: 2048, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	if !stronger.NeedsRehash(phc) {
		t.Fatal("different params must need rehash")
	}
	if ok, _ := stronger.Verify("password123", phc); !ok {
		t.Fatal("verification must use the params embedded in the hash")
	}
}

func TestValidatePassword_Policy(t *testing.T) {
	tests := []struct {
		pw string
		ok bool
	}{{"short", false}, {"0123456789", true}, {"àèìòùàèìòù", true}, {strings.Repeat("a", 1025), false}}
	for _, tt := range tests {
		if err := ValidatePassword(tt.pw); (err == nil) != tt.ok {
			t.Fatalf("%q: %v", tt.pw, err)
		}
	}
}

func TestSessionToken_HashingAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		tok, err := NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 43 { // 32 bytes base64url without padding
			t.Fatalf("token length %d", len(tok))
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
		h := HashToken(tok)
		if len(h) != 64 || h == tok || HashToken(tok) != h {
			t.Fatal("hash must be deterministic hex sha-256 and differ from token")
		}
	}
}

func TestAPIKey_FormatAndParse(t *testing.T) {
	key, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "ar_live_"+prefix+"_") || len(prefix) != 8 {
		t.Fatalf("bad key %q prefix %q", key, prefix)
	}
	got, ok := ParseAPIKey(key)
	if !ok || got != prefix {
		t.Fatalf("parse: %q %v", got, ok)
	}
	for _, bad := range []string{"", "ar_live_", "ar_test_abcdefgh_" + strings.Repeat("a", 52), "ar_live_abc_" + strings.Repeat("a", 52), "ar_live_ABCDEFGH_" + strings.Repeat("a", 52), "ar_live_abcdefgh_short"} {
		if _, ok := ParseAPIKey(bad); ok {
			t.Fatalf("%q must not parse", bad)
		}
	}
	if DisplayPrefix(prefix) != "ar_live_"+prefix {
		t.Fatal("display prefix")
	}
}

// TestCan_Matrix checks every cell of the permission matrix of spec 16.1.
func TestCan_Matrix(t *testing.T) {
	const Y, N = true, false
	matrixSpec := []struct {
		perm                                        Permission
		owner, manager, cashier, beachStaff, viewer bool
	}{
		{PermEstablishmentConfigure, Y, Y, N, N, N},
		{PermUsersManage, Y, N, N, N, N},
		{PermPaymentsConfigure, Y, N, N, N, N},
		{PermBookingsWrite, Y, Y, Y, N, N},
		{PermCashOperate, Y, Y, Y, N, N},
		{PermRefundsIssue, Y, Y, N, N, N},
		{PermOperationsRun, Y, Y, Y, Y, N},
		{PermBookingsRead, Y, Y, Y, Y, Y},
		{PermReportsRead, Y, Y, N, N, Y},
	}
	if len(matrixSpec) != len(Permissions) {
		t.Fatalf("matrix covers %d permissions, code defines %d", len(matrixSpec), len(Permissions))
	}
	for _, row := range matrixSpec {
		cells := map[Role]bool{RoleOwner: row.owner, RoleManager: row.manager, RoleCashier: row.cashier, RoleBeachStaff: row.beachStaff, RoleViewer: row.viewer}
		for role, want := range cells {
			t.Run(string(role)+"/"+string(row.perm), func(t *testing.T) {
				if got := Can(role, row.perm); got != want {
					t.Fatalf("Can(%s, %s) = %v, want %v", role, row.perm, got, want)
				}
			})
		}
	}
	if Can(Role("admin"), PermBookingsRead) {
		t.Fatal("unknown role must grant nothing")
	}
}

func TestPrincipal_CanCombinesGrants(t *testing.T) {
	es := store.EstablishmentScope{}
	p := &Principal{Kind: PrincipalUser}
	if p.Can(es, PermBookingsRead) {
		t.Fatal("invalid scope must deny")
	}
	if !p.Scope().Empty() {
		t.Fatal("no grants means empty scope")
	}
	p.Grants = []Grant{{OrganizationID: "o1", Role: RoleViewer}, {OrganizationID: "o1", EstablishmentID: "e1", Role: RoleCashier}}
	s := p.Scope()
	if !s.Allows("e9", "o1") || !s.Allows("e1", "o1") || s.Allows("e2", "o2") {
		t.Fatal("scope mismatch")
	}
	key := &Principal{Kind: PrincipalAPIKey, APIKeyID: "k", APIKeyEstablishmentID: "e1", Scopes: []Permission{PermBookingsRead}}
	if key.Scope().Allows("e2", "o1") || !key.Scope().Allows("e1", "") {
		t.Fatal("api key scope must be its establishment only")
	}
	if key.ID() != "api_key:k" || p.Actor("1.2.3.4").Type != "user" {
		t.Fatal("ids")
	}
}

func TestTOTP_RFC6238Vectors(t *testing.T) {
	// RFC 6238 appendix B, SHA1 seed "12345678901234567890", truncated to 6 digits.
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, tt := range tests {
		got, err := TOTPCode(secret, TOTPStep(time.Unix(tt.unix, 0)))
		if err != nil || got != tt.want {
			t.Fatalf("t=%d got %s want %s (%v)", tt.unix, got, tt.want, err)
		}
	}
}

func TestVerifyTOTP_Skew(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	cur := TOTPStep(now)
	tests := []struct {
		name string
		step int64
		ok   bool
	}{
		{"current", cur, true},
		{"previous", cur - 1, true},
		{"next", cur + 1, true},
		{"too old", cur - 2, false},
		{"too new", cur + 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := TOTPCode(secret, tt.step)
			step, ok := VerifyTOTP(secret, code, now)
			if ok != tt.ok || (ok && step != tt.step) {
				t.Fatalf("got %d %v", step, ok)
			}
		})
	}
	if _, ok := VerifyTOTP(secret, "12345", now); ok {
		t.Fatal("short code")
	}
	u := TOTPURL("Arenile", "a@b.it", secret)
	if !strings.HasPrefix(u, "otpauth://totp/Arenile:a@b.it?") || !strings.Contains(u, "secret="+secret) {
		t.Fatal(u)
	}
}

func TestRateLimiter_Window(t *testing.T) {
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	l := NewRateLimiter(c, 3, time.Minute)
	for i := range 3 {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatalf("attempt %d must pass", i)
		}
	}
	ok, retry := l.Allow("ip")
	if ok || retry <= 0 || retry > time.Minute {
		t.Fatalf("4th must be limited, retry=%s", retry)
	}
	if ok, _ := l.Allow("other"); !ok {
		t.Fatal("keys are independent")
	}
	c.Advance(time.Minute)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("new window must pass")
	}
}

func TestLoginThrottle_AccountLockout(t *testing.T) {
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	th := NewLoginThrottle(c)
	for range 8 {
		if ok, _ := th.Check("1.1.1.1", "a@b.it"); !ok {
			t.Fatal("must pass before failures accumulate")
		}
		th.Failure("a@b.it")
	}
	if ok, _ := th.Check("2.2.2.2", "a@b.it"); ok {
		t.Fatal("account must be locked regardless of IP")
	}
	if ok, _ := th.Check("2.2.2.2", "c@d.it"); !ok {
		t.Fatal("other accounts unaffected")
	}
	th.Success("a@b.it")
	if ok, _ := th.Check("2.2.2.2", "a@b.it"); !ok {
		t.Fatal("success resets the account counter")
	}
	c.Advance(16 * time.Minute)
	for i := range 30 {
		if ok, _ := th.Check("3.3.3.3", "x"+string(rune('a'+i%26))); !ok {
			t.Fatalf("ip attempt %d", i)
		}
	}
	if ok, _ := th.Check("3.3.3.3", "zz"); ok {
		t.Fatal("ip limit")
	}
}

func TestRateLimitError_Is(t *testing.T) {
	var err error = &RateLimitError{RetryAfter: time.Second}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatal("must match ErrRateLimited")
	}
}
