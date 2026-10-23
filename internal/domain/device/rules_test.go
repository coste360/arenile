package device

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func TestPairingTTL_Scenarios(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want time.Duration
		ok   bool
	}{
		{0, DefaultPairingTTL, true},
		{time.Minute, time.Minute, true},
		{15 * time.Minute, 15 * time.Minute, true},
		{59 * time.Second, 0, false},
		{901 * time.Second, 0, false},
		{-1, 0, false},
	}
	for _, tt := range tests {
		got, err := pairingTTL(tt.in)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("pairingTTL(%v) = %v, %v", tt.in, got, err)
		}
		if err != nil && !errors.Is(err, configerr.ErrValidation) {
			t.Errorf("pairingTTL(%v) error %v is not a validation error", tt.in, err)
		}
	}
}

func TestNormalizePairInput_Scenarios(t *testing.T) {
	ok := PairInput{Name: "  iPhone di Anna ", Platform: store.DevicePlatformIOS, Kind: store.DeviceKindMobile, AppVersion: " 1.2.0 "}
	got, err := normalizePairInput(ok)
	if err != nil || got.Name != "iPhone di Anna" || got.AppVersion != "1.2.0" {
		t.Fatalf("normalize = %+v, %v", got, err)
	}
	bad := []struct {
		name  string
		mod   func(*PairInput)
		field string
	}{
		{"empty name", func(p *PairInput) { p.Name = "   " }, "name"},
		{"long name", func(p *PairInput) { p.Name = strings.Repeat("à", MaxNameLen+1) }, "name"},
		{"control char", func(p *PairInput) { p.Name = "PC\ncassa" }, "name"},
		{"platform", func(p *PairInput) { p.Platform = "symbian" }, "platform"},
		{"kind", func(p *PairInput) { p.Kind = "watch" }, "kind"},
		{"app version", func(p *PairInput) { p.AppVersion = strings.Repeat("1", MaxAppVersionLen+1) }, "app_version"},
	}
	for _, tt := range bad {
		in := ok
		tt.mod(&in)
		_, err := normalizePairInput(in)
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != tt.field {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
	if _, err := normalizePairInput(PairInput{Name: strings.Repeat("à", MaxNameLen), Platform: "other", Kind: "desktop"}); err != nil {
		t.Errorf("80 runes must be accepted: %v", err)
	}
}

func TestEffectiveRole_Scenarios(t *testing.T) {
	grants := []store.TeamGrant{
		{UserID: "u1", Role: "viewer", UserActive: true},
		{UserID: "u1", Role: "manager", UserActive: true},
		{UserID: "u2", Role: "cashier", UserActive: false},
		{UserID: "u3", Role: "bogus", UserActive: true},
	}
	tests := []struct {
		user   string
		role   auth.Role
		ok     bool
		active bool
	}{
		{"u1", auth.RoleManager, true, true},
		{"u2", auth.RoleCashier, true, false},
		{"u3", "", false, false},
		{"u4", "", false, false},
	}
	for _, tt := range tests {
		role, ok := effectiveRole(grants, tt.user)
		if role != tt.role || ok != tt.ok || activeMember(grants, tt.user) != tt.active {
			t.Errorf("%s: role %q ok %v active %v", tt.user, role, ok, activeMember(grants, tt.user))
		}
	}
}

func TestPaginate_Scenarios(t *testing.T) {
	t0 := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	at := func(m int) *time.Time { v := t0.Add(time.Duration(m) * time.Minute); return &v }
	mk := func(id string, used *time.Time, created int) View {
		return View{Device: store.Device{ID: id, LastUsedAt: used, CreatedAt: t0.Add(time.Duration(created) * time.Minute)}}
	}
	vs := []View{
		mk("a", nil, 1),     // never used: sorts by creation (t0+1)
		mk("b", at(30), 0),  // most recent use
		mk("c", at(10), 0),  // t0+10
		mk("d", at(10), 0),  // tie with c: id descending -> d before c
		mk("e", nil, 20),    // t0+20
		mk("f", at(-5), -9), // oldest
	}
	sortViews(vs)
	var order []string
	for _, v := range vs {
		order = append(order, v.ID)
	}
	if got := strings.Join(order, ""); got != "bedcaf" {
		t.Fatalf("order %q, want bedcaf", got)
	}

	var all []string
	after := ""
	for range 10 {
		page, next, ok := paginate(vs, after, 4)
		if !ok {
			t.Fatal("cursor rejected")
		}
		for _, v := range page {
			all = append(all, v.ID)
		}
		if next == "" {
			break
		}
		after = next
	}
	if got := strings.Join(all, ""); got != "bedcaf" {
		t.Fatalf("pages %q", got)
	}

	// The device of the cursor is gone (revoked meanwhile): the next page
	// still starts right after its position.
	_, next, _ := paginate(vs, "", 2) // b, e
	rest := append([]View{}, vs[:1]...)
	rest = append(rest, vs[2:]...) // without e
	page, _, _ := paginate(rest, next, 10)
	if len(page) != 4 || page[0].ID != "d" {
		t.Fatalf("after vanished cursor: %+v", page)
	}

	if _, _, ok := paginate(vs, "garbage", 2); ok {
		t.Fatal("malformed cursor accepted")
	}
	if page, next, _ := paginate(vs, keyOf(vs[5].Device).String(), 2); len(page) != 0 || next != "" {
		t.Fatalf("past the end: %v %q", page, next)
	}
}

func TestRenderQRSVG_Scenarios(t *testing.T) {
	text, err := EncodeQR(QRPayload{URL: "https://lido.example.it", Code: "ABCD-EFGH", EstablishmentName: "Lido Azzurro"})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := RenderQRSVG(text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg"`) || !strings.Contains(svg, `fill="#fff"`) || !strings.Contains(svg, "<path") {
		t.Fatalf("svg %q", svg[:min(len(svg), 120)])
	}
	if strings.Contains(svg, "ABCD") {
		t.Fatal("the code must not appear as text in the image")
	}
	for _, bad := range []string{"", strings.Repeat("x", maxQRText+1)} {
		if _, err := RenderQRSVG(bad); err == nil {
			t.Errorf("RenderQRSVG(len %d) accepted", len(bad))
		}
	}
}
