//go:build integration

package device_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var fastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// instance is a device.Instance with a fixed answer.
type instance struct {
	target device.PairingTarget
	err    error
}

func (i instance) PairingTarget(context.Context) (device.PairingTarget, error) {
	return i.target, i.err
}

type fixture struct {
	ctx     context.Context
	db      *store.DB
	clock   *clock.Fake
	auth    *auth.Service
	svc     *device.Service
	estA    *store.Establishment
	estA2   *store.Establishment
	estB    *store.Establishment
	owner   *auth.Principal // organization-wide owner of org A
	manager *auth.Principal // manager of estA (no users.manage)
	cashier *auth.Principal // cashier of estA
	ownerB  *auth.Principal // owner of org B
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	aw := audit.NewWriter(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &fastArgon})
	if err != nil {
		t.Fatal(err)
	}
	resA, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner A", Password: "password-owner-1", OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(resA.Organization.ID, "lido-a", "Lido A", c.Now())
	estA2 := store.NewEstablishmentDefaults(resA.Organization.ID, "lido-a2", "Lido A2", c.Now())
	for _, e := range []*store.Establishment{estA, estA2} {
		if err := store.InsertEstablishment(ctx, db, e); err != nil {
			t.Fatal(err)
		}
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: ctx, db: db, clock: c, auth: as, estA: estA, estA2: estA2, estB: estB}
	f.svc = device.NewService(device.Deps{
		DB: db, Clock: c, Audit: aw, Principals: as,
		Instance: instance{target: device.PairingTarget{URL: "https://lido-a.example.it"}},
	})
	f.owner = f.principal(t, resA.User.ID)
	f.manager = f.member(t, "manager@lido-a.it", resA.Organization.ID, &estA.ID, auth.RoleManager)
	f.cashier = f.member(t, "cassa@lido-a.it", resA.Organization.ID, &estA.ID, auth.RoleCashier)
	f.ownerB = f.member(t, "owner@lido-b.it", orgB.ID, nil, auth.RoleOwner)
	return f
}

func (f *fixture) member(t *testing.T, email, orgID string, estID *string, role auth.Role) *auth.Principal {
	t.Helper()
	now := f.clock.Now()
	u := &store.User{ID: store.NewID(), Email: email, PasswordHash: "x", Name: strings.Split(email, "@")[0], Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := store.InsertUser(f.ctx, f.db, u); err != nil {
		t.Fatal(err)
	}
	m := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: orgID, EstablishmentID: estID, Role: string(role), CreatedAt: now, UpdatedAt: now}
	if err := store.InsertMembership(f.ctx, f.db, m); err != nil {
		t.Fatal(err)
	}
	return f.principal(t, u.ID)
}

func (f *fixture) principal(t *testing.T, userID string) *auth.Principal {
	t.Helper()
	ms, err := store.ListUserMemberships(f.ctx, f.db, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{Kind: auth.PrincipalUser, UserID: userID}
	for _, m := range ms {
		g := auth.Grant{OrganizationID: m.OrganizationID, Role: auth.Role(m.Role)}
		if m.EstablishmentID != nil {
			g.EstablishmentID = *m.EstablishmentID
		}
		p.Grants = append(p.Grants, g)
	}
	return p
}

func (f *fixture) code(t *testing.T, p *auth.Principal, estID string, in device.PairingCodeInput) *device.PairingCode {
	t.Helper()
	pc, err := f.svc.CreatePairingCode(f.ctx, p, estID, in)
	if err != nil {
		t.Fatal(err)
	}
	return pc
}

func pairInput(code string) device.PairInput {
	return device.PairInput{Code: code, Name: "iPhone di Anna", Platform: store.DevicePlatformIOS, Kind: store.DeviceKindMobile, AppVersion: "1.0.0", IP: "192.0.2.10"}
}

func (f *fixture) pair(t *testing.T, p *auth.Principal, estID string) *device.PairResult {
	t.Helper()
	res, err := f.svc.Pair(f.ctx, pairInput(f.code(t, p, estID, device.PairingCodeInput{}).Code))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func count(t *testing.T, db *store.DB, table, where string, args ...any) int {
	t.Helper()
	n, err := db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestDeviceService_PairAndAuthenticate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		pc := f.code(t, f.cashier, f.estA.ID, device.PairingCodeInput{Kind: store.DeviceKindMobile})
		if len(pc.Code) != 9 || pc.UserID != f.cashier.UserID || pc.EstablishmentID != f.estA.ID ||
			!pc.ExpiresAt.Equal(f.clock.Now().Add(5*time.Minute)) {
			t.Fatalf("pairing code %+v", pc)
		}
		qr, err := device.DecodeQR(pc.QRText)
		if err != nil || qr.Code != pc.Code || qr.URL != "https://lido-a.example.it" || qr.EstablishmentName != "Lido A" || qr.Fingerprint != "" {
			t.Fatalf("qr %+v, %v", qr, err)
		}
		if !strings.HasPrefix(pc.QRSVG, "<svg") {
			t.Fatal("qr svg missing")
		}
		// Only the hash is stored.
		hash, _ := device.HashPairingCode(pc.Code)
		if count(t, db, "device_pairing_codes", "code_hash = ?", hash) != 1 ||
			count(t, db, "device_pairing_codes", "code_hash = ?", pc.Code) != 0 {
			t.Fatal("pairing code not stored as hash")
		}

		res, err := f.svc.Pair(f.ctx, pairInput(strings.ToLower(pc.Code)))
		if err != nil {
			t.Fatal(err)
		}
		if !device.IsToken(res.Token) || res.Device.UserID != f.cashier.UserID || res.Device.EstablishmentID != f.estA.ID ||
			res.Device.Role != auth.RoleCashier || res.Device.UserName != "cassa" || res.Device.Name != "iPhone di Anna" {
			t.Fatalf("pair result %+v", res.Device)
		}
		if count(t, db, "devices", "token_hash = ?", device.HashToken(res.Token)) != 1 ||
			count(t, db, "devices", "token_hash = ?", res.Token) != 0 {
			t.Fatal("token not stored as hash")
		}
		p, err := f.svc.AuthenticateDevice(f.ctx, res.Token)
		if err != nil {
			t.Fatal(err)
		}
		if p.Kind != auth.PrincipalUser || p.UserID != f.cashier.UserID || p.DeviceID != res.Device.ID || p.DeviceEstablishmentID != f.estA.ID {
			t.Fatalf("principal %+v", p)
		}
		es, err := p.Authorize(f.ctx, db, f.estA.ID, auth.PermCashOperate)
		if err != nil {
			t.Fatalf("device must carry the cashier permissions: %v", err)
		}
		if p.Can(es, auth.PermUsersManage) {
			t.Fatal("device gained permissions beyond the role")
		}
		// Audit: code creation and pairing, never the code or the token.
		var entries []store.AuditEntry
		if err := db.NewSelect().Model(&entries).Where("action LIKE 'device.%'").OrderExpr("id").Scan(f.ctx); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].Action != "device.pairing_code_created" || entries[1].Action != "device.paired" ||
			*entries[1].ActorID != f.cashier.UserID || entries[1].IP != "192.0.2.10" {
			t.Fatalf("audit %+v", entries)
		}
		for _, e := range entries {
			if s := string(e.Diff); strings.Contains(s, pc.Code) || strings.Contains(s, strings.ReplaceAll(pc.Code, "-", "")) || strings.Contains(s, res.Token) {
				t.Fatalf("secret in audit diff: %s", s)
			}
		}
		if count(t, db, "device_pairing_codes", "device_id = ?", res.Device.ID) != 1 {
			t.Fatal("pairing code not linked to the device")
		}
	})
}

func TestDeviceService_CodeSingleUseAndExpiry(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		pc := f.code(t, f.owner, f.estA.ID, device.PairingCodeInput{})
		if _, err := f.svc.Pair(f.ctx, pairInput(pc.Code)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Pair(f.ctx, pairInput(pc.Code)); !errors.Is(err, device.ErrPairingCodeInvalid) {
			t.Fatalf("reused code: %v", err)
		}
		for _, bad := range []string{"", "ZZZZ-ZZZZ", "not a code", "ABCD-EFGU"} {
			if _, err := f.svc.Pair(f.ctx, pairInput(bad)); !errors.Is(err, device.ErrPairingCodeInvalid) {
				t.Fatalf("code %q: %v", bad, err)
			}
		}
		// Expiry: valid up to expires_at excluded.
		short := f.code(t, f.owner, f.estA.ID, device.PairingCodeInput{TTL: time.Minute})
		f.clock.Advance(time.Minute)
		if _, err := f.svc.Pair(f.ctx, pairInput(short.Code)); !errors.Is(err, device.ErrPairingCodeInvalid) {
			t.Fatalf("expired code: %v", err)
		}
		// Invalid device fields do not consume the code.
		fresh := f.code(t, f.owner, f.estA.ID, device.PairingCodeInput{})
		bad := pairInput(fresh.Code)
		bad.Platform = "symbian"
		if _, err := f.svc.Pair(f.ctx, bad); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("bad platform: %v", err)
		}
		if _, err := f.svc.Pair(f.ctx, pairInput(fresh.Code)); err != nil {
			t.Fatalf("code consumed by an invalid request: %v", err)
		}
		// Cleanup keeps codes for a day after expiry.
		f.clock.Advance(23 * time.Hour)
		if err := f.svc.CleanupExpiredCodes(f.ctx); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "device_pairing_codes", "1 = 1"); n != 3 {
			t.Fatalf("codes after 23h: %d", n)
		}
		f.clock.Advance(2 * time.Hour)
		if err := f.svc.CleanupExpiredCodes(f.ctx); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "device_pairing_codes", "1 = 1"); n != 0 {
			t.Fatalf("codes after 25h: %d", n)
		}
		if count(t, db, "devices", "1 = 1") != 2 {
			t.Fatal("cleanup touched devices")
		}
	})
}

func TestDeviceService_RevokeIsImmediate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		res := f.pair(t, f.cashier, f.estA.ID)
		if _, err := f.svc.AuthenticateDevice(f.ctx, res.Token); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Revoke(f.ctx, f.cashier, f.estA.ID, res.Device.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("revoked token: %v", err)
		}
		if err := f.svc.Revoke(f.ctx, f.cashier, f.estA.ID, res.Device.ID); err != nil {
			t.Fatalf("revoke must be idempotent: %v", err)
		}
		if n := count(t, db, "audit_log", "action = ?", "device.revoked"); n != 1 {
			t.Fatalf("device.revoked entries: %d", n)
		}
		page, err := f.svc.List(f.ctx, f.cashier, f.estA.ID, device.ListFilter{})
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("revoked device listed: %+v %v", page.Items, err)
		}
		page, err = f.svc.List(f.ctx, f.cashier, f.estA.ID, device.ListFilter{IncludeRevoked: true})
		if err != nil || len(page.Items) != 1 || page.Items[0].RevokedAt == nil {
			t.Fatalf("include_revoked: %+v %v", page.Items, err)
		}

		// Self unpairing (app logout).
		res2 := f.pair(t, f.cashier, f.estA.ID)
		p, err := f.svc.AuthenticateDevice(f.ctx, res2.Token)
		if err != nil {
			t.Fatal(err)
		}
		cur, err := f.svc.Current(f.ctx, p)
		if err != nil || cur.ID != res2.Device.ID || !cur.Current {
			t.Fatalf("current %+v %v", cur, err)
		}
		if err := f.svc.UnpairCurrent(f.ctx, p); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, res2.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("unpaired token: %v", err)
		}

		// A deactivated user or a user without a role loses every device.
		res3 := f.pair(t, f.cashier, f.estA.ID)
		if _, err := db.NewUpdate().Model((*store.User)(nil)).Set("active = ?", false).Where("id = ?", f.cashier.UserID).Exec(f.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, res3.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("inactive user: %v", err)
		}
		res4 := f.pair(t, f.manager, f.estA.ID)
		pc := f.code(t, f.manager, f.estA.ID, device.PairingCodeInput{})
		if _, err := db.NewDelete().Model((*store.Membership)(nil)).Where("user_id = ?", f.manager.UserID).Exec(f.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, res4.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("user without role: %v", err)
		}
		if _, err := f.svc.Pair(f.ctx, pairInput(pc.Code)); !errors.Is(err, device.ErrPairingCodeInvalid) {
			t.Fatalf("code of a user without role: %v", err)
		}
	})
}

func TestDeviceService_TouchThrottled(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		res := f.pair(t, f.cashier, f.estA.ID)
		lastUsed := func() time.Time {
			t.Helper()
			d := new(store.Device)
			if err := db.NewSelect().Model(d).Where("id = ?", res.Device.ID).Scan(f.ctx); err != nil {
				t.Fatal(err)
			}
			if d.LastUsedAt == nil {
				return time.Time{}
			}
			return d.LastUsedAt.UTC()
		}
		if !lastUsed().IsZero() {
			t.Fatal("never used device has last_used_at")
		}
		t0 := f.clock.Now()
		auth1 := func() {
			t.Helper()
			if _, err := f.svc.AuthenticateDevice(f.ctx, res.Token); err != nil {
				t.Fatal(err)
			}
		}
		auth1()
		if !lastUsed().Equal(t0) {
			t.Fatalf("first use: %v", lastUsed())
		}
		f.clock.Advance(30 * time.Second)
		auth1()
		if !lastUsed().Equal(t0) {
			t.Fatal("touched again within the interval")
		}
		f.clock.Advance(31 * time.Second)
		auth1()
		if !lastUsed().Equal(t0.Add(61 * time.Second)) {
			t.Fatalf("not touched after the interval: %v", lastUsed())
		}
	})
}

func TestDeviceService_PermissionsByRole(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		ownerDev := f.pair(t, f.owner, f.estA.ID)
		cashierDev := f.pair(t, f.cashier, f.estA.ID)
		managerDev := f.pair(t, f.manager, f.estA.ID)

		// Codes for another user need users.manage.
		for _, p := range []*auth.Principal{f.cashier, f.manager} {
			if _, err := f.svc.CreatePairingCode(f.ctx, p, f.estA.ID, device.PairingCodeInput{UserID: f.owner.UserID}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("code for another user: %v", err)
			}
		}
		forCashier := f.code(t, f.owner, f.estA.ID, device.PairingCodeInput{UserID: f.cashier.UserID})
		if forCashier.UserID != f.cashier.UserID {
			t.Fatal("owner code for cashier")
		}
		// ... and the target must be a member of the establishment.
		if _, err := f.svc.CreatePairingCode(f.ctx, f.owner, f.estA.ID, device.PairingCodeInput{UserID: f.ownerB.UserID}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("code for a stranger: %v", err)
		}

		// Listing: everything with users.manage, own devices otherwise.
		ids := func(p *auth.Principal) map[string]bool {
			t.Helper()
			page, err := f.svc.List(f.ctx, p, f.estA.ID, device.ListFilter{})
			if err != nil {
				t.Fatal(err)
			}
			out := map[string]bool{}
			for _, v := range page.Items {
				out[v.ID] = true
			}
			return out
		}
		if got := ids(f.owner); len(got) != 3 {
			t.Fatalf("owner sees %v", got)
		}
		if got := ids(f.cashier); len(got) != 1 || !got[cashierDev.Device.ID] {
			t.Fatalf("cashier sees %v", got)
		}
		if got := ids(f.manager); len(got) != 1 || !got[managerDev.Device.ID] {
			t.Fatalf("manager sees %v", got)
		}

		// Revoking the device of someone else: not found without users.manage.
		if err := f.svc.Revoke(f.ctx, f.cashier, f.estA.ID, ownerDev.Device.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cashier revokes owner device: %v", err)
		}
		if err := f.svc.Revoke(f.ctx, f.owner, f.estA.ID, cashierDev.Device.ID); err != nil {
			t.Fatalf("owner revokes cashier device: %v", err)
		}

		// Devices and API keys do not mint pairing codes.
		dp, err := f.svc.AuthenticateDevice(f.ctx, managerDev.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.CreatePairingCode(f.ctx, dp, f.estA.ID, device.PairingCodeInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("device creates a code: %v", err)
		}
		key := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "k", APIKeyEstablishmentID: f.estA.ID, Scopes: auth.Permissions}
		if _, err := f.svc.CreatePairingCode(f.ctx, key, f.estA.ID, device.PairingCodeInput{UserID: f.owner.UserID}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("api key creates a code: %v", err)
		}
		readKey := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "r", APIKeyEstablishmentID: f.estA.ID, Scopes: []auth.Permission{auth.PermBookingsRead}}
		if _, err := f.svc.List(f.ctx, readKey, f.estA.ID, device.ListFilter{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("api key without users.manage lists devices: %v", err)
		}
		if page, err := f.svc.List(f.ctx, key, f.estA.ID, device.ListFilter{}); err != nil || len(page.Items) != 2 {
			t.Fatalf("api key with users.manage: %d %v", len(page.Items), err)
		}

		// The device principal is narrowed to its establishment.
		op, err := f.svc.AuthenticateDevice(f.ctx, ownerDev.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := op.Authorize(f.ctx, db, f.estA2.ID, auth.PermBookingsRead); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("org-wide owner device reaches estA2: %v", err)
		}
	})
}

func TestDeviceService_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		devB := f.pair(t, f.ownerB, f.estB.ID)
		if _, err := f.svc.CreatePairingCode(f.ctx, f.owner, f.estB.ID, device.PairingCodeInput{}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("code on foreign establishment: %v", err)
		}
		if _, err := f.svc.List(f.ctx, f.owner, f.estB.ID, device.ListFilter{}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("list foreign establishment: %v", err)
		}
		if err := f.svc.Revoke(f.ctx, f.owner, f.estB.ID, devB.Device.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("revoke on foreign establishment: %v", err)
		}
		if err := f.svc.Revoke(f.ctx, f.owner, f.estA.ID, devB.Device.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("revoke foreign device through own establishment: %v", err)
		}
		if page, err := f.svc.List(f.ctx, f.owner, f.estA.ID, device.ListFilter{IncludeRevoked: true}); err != nil || len(page.Items) != 0 {
			t.Fatalf("foreign device listed: %+v %v", page.Items, err)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, devB.Token); err != nil {
			t.Fatalf("foreign device must still work: %v", err)
		}
		pB, _ := f.svc.AuthenticateDevice(f.ctx, devB.Token)
		if _, err := pB.Authorize(f.ctx, db, f.estA.ID, auth.PermBookingsRead); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("device of B reaches A: %v", err)
		}
	})
}

func TestDeviceService_ListPagination(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		var tokens []string
		for range 5 {
			tokens = append(tokens, f.pair(t, f.owner, f.estA.ID).Token)
			f.clock.Advance(2 * time.Minute)
		}
		// Use the second device: it moves to the top.
		if _, err := f.svc.AuthenticateDevice(f.ctx, tokens[1]); err != nil {
			t.Fatal(err)
		}
		first, err := f.svc.List(f.ctx, f.owner, f.estA.ID, device.ListFilter{Limit: 2})
		if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
			t.Fatalf("first page %+v %v", first, err)
		}
		if first.Items[0].LastUsedAt == nil {
			t.Fatal("most recently used device not first")
		}
		seen := map[string]bool{}
		page := first
		for {
			for _, v := range page.Items {
				if seen[v.ID] {
					t.Fatalf("device %s repeated", v.ID)
				}
				seen[v.ID] = true
			}
			if page.NextCursor == "" {
				break
			}
			if page, err = f.svc.List(f.ctx, f.owner, f.estA.ID, device.ListFilter{Limit: 2, Cursor: page.NextCursor}); err != nil {
				t.Fatal(err)
			}
		}
		if len(seen) != 5 {
			t.Fatalf("listed %d devices", len(seen))
		}
		if _, err := f.svc.List(f.ctx, f.owner, f.estA.ID, device.ListFilter{Cursor: "!!"}); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}
	})
}

func TestDeviceService_InstanceAndValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		local := device.NewService(device.Deps{
			DB: db, Clock: f.clock, Audit: audit.NewWriter(f.clock), Principals: f.auth,
			Instance: instance{err: device.ErrLANModeRequired},
		})
		if _, err := local.CreatePairingCode(f.ctx, f.owner, f.estA.ID, device.PairingCodeInput{}); !errors.Is(err, device.ErrLANModeRequired) {
			t.Fatalf("desktop local mode: %v", err)
		}
		lan := device.NewService(device.Deps{
			DB: db, Clock: f.clock, Audit: audit.NewWriter(f.clock), Principals: f.auth,
			Instance: instance{target: device.PairingTarget{URL: "https://192.168.1.20:8443", Fingerprint: "AB:CD"}},
		})
		pc, err := lan.CreatePairingCode(f.ctx, f.owner, f.estA.ID, device.PairingCodeInput{})
		if err != nil || pc.QR.Fingerprint != "AB:CD" || pc.QR.URL != "https://192.168.1.20:8443" {
			t.Fatalf("lan mode code %+v %v", pc, err)
		}
		for _, in := range []device.PairingCodeInput{{TTL: 30 * time.Second}, {TTL: time.Hour}, {Kind: "watch"}} {
			if _, err := f.svc.CreatePairingCode(f.ctx, f.owner, f.estA.ID, in); !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("input %+v: %v", in, err)
			}
		}
		if count(t, db, "device_pairing_codes", "1 = 1") != 1 {
			t.Fatal("rejected requests stored codes")
		}
	})
}

// TestConcurrency_PairingCodeSingleUse: 100 parallel exchanges of the same
// code produce exactly one device and one token; every other attempt is
// pairing_code_invalid and leaves no row behind.
func TestConcurrency_PairingCodeSingleUse(t *testing.T) {
	const parallel = 100
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		pc := f.code(t, f.cashier, f.estA.ID, device.PairingCodeInput{})
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			tokens  []string
			invalid int
			other   []error
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				in := pairInput(pc.Code)
				in.Name = "Telefono " + string(rune('A'+i%26))
				res, err := f.svc.Pair(f.ctx, in)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					tokens = append(tokens, res.Token)
				case errors.Is(err, device.ErrPairingCodeInvalid):
					invalid++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if len(tokens) != 1 || invalid != parallel-1 {
			t.Fatalf("tokens %d, invalid %d", len(tokens), invalid)
		}
		if n := count(t, db, "devices", "1 = 1"); n != 1 {
			t.Fatalf("devices %d", n)
		}
		if n := count(t, db, "audit_log", "action = ?", "device.paired"); n != 1 {
			t.Fatalf("device.paired entries %d", n)
		}
		if _, err := f.svc.AuthenticateDevice(f.ctx, tokens[0]); err != nil {
			t.Fatal(err)
		}
	})
}
