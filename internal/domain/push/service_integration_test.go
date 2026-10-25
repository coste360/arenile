//go:build integration

package push_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/pushrelay"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

var fastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// fakeRelay records the notifications and answers with the next queued
// result (Delivered when none is queued). Safe for concurrent use.
type fakeRelay struct {
	mu         sync.Mutex
	configured bool
	sent       []pushrelay.Notification
	results    []pushrelay.Result
	delay      time.Duration
}

func (r *fakeRelay) Configured() bool { return r.configured }

func (r *fakeRelay) Send(_ context.Context, n pushrelay.Notification) pushrelay.Result {
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, n)
	if len(r.results) == 0 {
		return pushrelay.Result{Outcome: pushrelay.Delivered, Status: 202}
	}
	res := r.results[0]
	r.results = r.results[1:]
	return res
}

func (r *fakeRelay) notifications() []pushrelay.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]pushrelay.Notification(nil), r.sent...)
}

type instance struct{}

func (instance) PairingTarget(context.Context) (device.PairingTarget, error) {
	return device.PairingTarget{URL: "https://lido-a.example.it"}, nil
}

type fixture struct {
	ctx     context.Context
	db      *store.DB
	clock   *clock.Fake
	auth    *auth.Service
	devices *device.Service
	svc     *push.Service
	relay   *fakeRelay
	estA    *store.Establishment
	estB    *store.Establishment
	owner   *auth.Principal // owner of org A
	manager *auth.Principal // manager of estA
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
	if err := store.InsertEstablishment(ctx, db, estA); err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	keys, err := crypto.NewKeyRing(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	relay := &fakeRelay{configured: true}
	f := &fixture{ctx: ctx, db: db, clock: c, auth: as, relay: relay, estA: estA, estB: estB}
	f.devices = device.NewService(device.Deps{DB: db, Clock: c, Audit: aw, Principals: as, Instance: instance{}})
	f.svc = push.NewService(push.Deps{
		DB: db, Clock: c, Audit: aw, Keys: keys, Principals: as, Relay: relay, IdleExpired: device.IdleExpired,
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

// pairedApp pairs a mobile device for the user of p on estA and returns the
// device principal (what the app authenticates as).
func (f *fixture) pairedApp(t *testing.T, p *auth.Principal, platform string) *auth.Principal {
	t.Helper()
	pc, err := f.devices.CreatePairingCode(f.ctx, p, f.estA.ID, device.PairingCodeInput{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.devices.Pair(f.ctx, device.PairInput{Code: pc.Code, Name: "Telefono", Platform: platform, Kind: store.DeviceKindMobile, IP: "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	dp, err := f.devices.AuthenticateDevice(f.ctx, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	return dp
}

func (f *fixture) enable(t *testing.T, events push.Events) {
	t.Helper()
	cur, err := f.svc.GetSettings(f.ctx, f.manager, f.estA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateSettings(f.ctx, f.manager, f.estA.ID, &cur.Version, push.SettingsInput{Enabled: true, Events: events}, ""); err != nil {
		t.Fatal(err)
	}
}

var allEvents = push.Events{NewOnlineBooking: true, ApprovalRequested: true, PaymentFailed: true}

func (f *fixture) register(t *testing.T, app *auth.Principal, token string, topics ...string) {
	t.Helper()
	if _, err := f.svc.Register(f.ctx, app, push.RegisterInput{Token: token, Locale: "it", Topics: topics}); err != nil {
		t.Fatal(err)
	}
}

func bookingEvent(t *testing.T, estID, typ, channel string) ext.Event {
	t.Helper()
	data, err := json.Marshal(event.BookingChange{
		BookingID: store.NewID(), Code: "LA-2027-000042", Status: "confirmed", Channel: channel,
		CustomerID: store.NewID(), TotalAmount: 4500,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext.Event{ID: store.NewID(), Type: typ, EstablishmentID: estID, OccurredAt: time.Now(), Data: data, Version: 1}
}

func count(t *testing.T, db *store.DB, table, where string, args ...any) int {
	t.Helper()
	n, err := db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func (f *fixture) deliveries(t *testing.T) []store.PushDelivery {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.ctx, f.db, f.estA.ID)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := store.ListPushDeliveries(f.ctx, f.db, es)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestPushService_SettingsDefaultsAndPermissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		st, err := f.svc.GetSettings(f.ctx, f.cashier, f.estA.ID)
		if err != nil || st.Enabled || !st.Events.NewOnlineBooking || !st.Events.ApprovalRequested || !st.Events.PaymentFailed ||
			!st.RelayConfigured || st.Version != 1 {
			t.Fatalf("defaults %+v %v", st, err)
		}
		in := push.SettingsInput{Enabled: true, Events: push.Events{NewOnlineBooking: true}}
		if _, err := f.svc.UpdateSettings(f.ctx, f.cashier, f.estA.ID, nil, in, ""); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier update: %v", err)
		}
		if _, err := f.svc.GetSettings(f.ctx, f.ownerB, f.estA.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("other organization: %v", err)
		}
		stale := int64(5)
		if _, err := f.svc.UpdateSettings(f.ctx, f.manager, f.estA.ID, &stale, in, ""); !errors.Is(err, push.ErrVersionMismatch) {
			t.Fatalf("stale version: %v", err)
		}
		v := int64(1)
		st, err = f.svc.UpdateSettings(f.ctx, f.manager, f.estA.ID, &v, in, "192.0.2.1")
		if err != nil || !st.Enabled || st.Events.ApprovalRequested || st.Version != 2 {
			t.Fatalf("update %+v %v", st, err)
		}
		if _, err := f.svc.UpdateSettings(f.ctx, f.manager, f.estA.ID, &v, in, ""); !errors.Is(err, push.ErrVersionMismatch) {
			t.Fatalf("replayed version: %v", err)
		}
		if n := count(t, db, "audit_log", "action = ?", push.ActionSettingsUpdated); n != 1 {
			t.Fatalf("audit entries %d", n)
		}
		other, err := f.svc.GetSettings(f.ctx, f.ownerB, f.estB.ID)
		if err != nil || other.Enabled {
			t.Fatalf("settings leaked to another establishment: %+v %v", other, err)
		}
	})
}

func TestPushService_RegisterConsentAndValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		app := f.pairedApp(t, f.cashier, store.DevicePlatformIOS)
		const token = "apns-token-0123456789abcdef0123456789abcdef"

		// Manager consent missing: unavailable.
		st, err := f.svc.Status(f.ctx, app)
		if err != nil || st.Available || st.Registered || !st.RelayConfigured || len(st.AllowedTopics) != 0 {
			t.Fatalf("status before consent %+v %v", st, err)
		}
		if _, err := f.svc.Register(f.ctx, app, push.RegisterInput{Token: token, Locale: "it"}); !errors.Is(err, push.ErrUnavailable) {
			t.Fatalf("register without manager consent: %v", err)
		}
		f.enable(t, allEvents)

		// Validation: token shape, locale, environment, topics of the role.
		var ve *configerr.ValidationError
		for _, in := range []push.RegisterInput{
			{Token: "short", Locale: "it"},
			{Token: "token with spaces 0123456789", Locale: "it"},
			{Token: token, Locale: "es"},
			{Token: token, Locale: "it", Environment: "staging"},
			{Token: token, Locale: "it", Topics: []string{"Nuova prenotazione di Rossi"}},
			{Token: token, Locale: "it", Topics: []string{store.PushTopicPaymentFailed}}, // refunds.issue: not a cashier
		} {
			if _, err := f.svc.Register(f.ctx, app, in); !errors.As(err, &ve) {
				t.Fatalf("register %+v: %v", in, err)
			}
		}
		// A user principal (panel session) has no device to register.
		if _, err := f.svc.Register(f.ctx, f.cashier, push.RegisterInput{Token: token, Locale: "it"}); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("register without device: %v", err)
		}

		st, err = f.svc.Register(f.ctx, app, push.RegisterInput{
			Token: token, Locale: "de", Environment: store.PushEnvironmentSandbox,
			Topics: []string{store.PushTopicApprovalRequested, store.PushTopicNewOnlineBooking, store.PushTopicNewOnlineBooking},
		})
		if err != nil || !st.Available || !st.Registered || st.Locale != "de" || st.Environment != store.PushEnvironmentSandbox ||
			strings.Join(st.Topics, ",") != "booking_new_online,booking_approval_requested" ||
			strings.Join(st.AllowedTopics, ",") != "booking_new_online,booking_approval_requested" {
			t.Fatalf("register %+v %v", st, err)
		}
		// The token is stored encrypted only, never in audit.
		if n := count(t, db, "push_registrations", "token_ciphertext LIKE ?", "%"+token+"%"); n != 0 {
			t.Fatal("token stored in clear")
		}
		if n := count(t, db, "audit_log", "action = ? AND CAST(diff AS TEXT) LIKE ?", push.ActionRegistered, "%apns-token%"); n != 0 {
			t.Fatal("token in the audit log")
		}
		// Registering again replaces the row (one per device).
		f.register(t, app, token, store.PushTopicNewOnlineBooking)
		if n := count(t, db, "push_registrations", "1 = 1"); n != 1 {
			t.Fatalf("registrations %d", n)
		}
		// The same token from a phone paired again moves to the new device.
		app2 := f.pairedApp(t, f.cashier, store.DevicePlatformIOS)
		f.register(t, app2, token, store.PushTopicNewOnlineBooking)
		if st, _ := f.svc.Status(f.ctx, app); st.Registered {
			t.Fatal("old device still registered for the moved token")
		}

		// A desktop device cannot register.
		desk := f.pairedApp(t, f.manager, store.DevicePlatformWindows)
		if _, err := f.svc.Register(f.ctx, desk, push.RegisterInput{Token: token, Locale: "it"}); !errors.As(err, &ve) {
			t.Fatalf("desktop device: %v", err)
		}

		// User opt-out: idempotent, audited once.
		for range 2 {
			if err := f.svc.Unregister(f.ctx, app2); err != nil {
				t.Fatal(err)
			}
		}
		if n := count(t, db, "audit_log", "action = ?", push.ActionUnregistered); n != 1 {
			t.Fatalf("unregister audit entries %d", n)
		}

		// Revoking the device forgets its token in the same transaction.
		f.register(t, app2, token, store.PushTopicNewOnlineBooking)
		if err := f.devices.Revoke(f.ctx, f.cashier, f.estA.ID, app2.DeviceID); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "push_registrations", "1 = 1"); n != 0 {
			t.Fatalf("registration survived the revocation: %d", n)
		}
	})
}

func TestPushService_RelayNotConfigured(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		app := f.pairedApp(t, f.manager, store.DevicePlatformAndroid)
		f.enable(t, allEvents)
		f.register(t, app, "fcm-token-0123456789abcdef", store.PushTopicNewOnlineBooking)
		f.relay.configured = false
		st, err := f.svc.Status(f.ctx, app)
		if err != nil || st.Available || st.RelayConfigured {
			t.Fatalf("status %+v %v", st, err)
		}
		if _, err := f.svc.Register(f.ctx, app, push.RegisterInput{Token: "fcm-token-0123456789abcdef", Locale: "it"}); !errors.Is(err, push.ErrUnavailable) {
			t.Fatalf("register: %v", err)
		}
		// Nothing is queued, nothing is sent.
		if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
			t.Fatal(err)
		}
		if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 0 || len(f.relay.notifications()) != 0 || len(f.deliveries(t)) != 0 {
			t.Fatalf("delivered %d %v, relay %v", n, err, f.relay.notifications())
		}
	})
}

func TestPushService_SinkAndDeliver(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		cashierApp := f.pairedApp(t, f.cashier, store.DevicePlatformIOS)
		managerApp := f.pairedApp(t, f.manager, store.DevicePlatformAndroid)
		f.register(t, cashierApp, "apns-cashier-0123456789abcdef", store.PushTopicNewOnlineBooking, store.PushTopicApprovalRequested)
		f.register(t, managerApp, "fcm-manager-0123456789abcdef", store.PushTopicNewOnlineBooking, store.PushTopicPaymentFailed)

		// A desk booking does not notify; an online one does, once per
		// subscribed device, even when the event is redelivered.
		if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelDesk)); err != nil {
			t.Fatal(err)
		}
		online := bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)
		for range 3 {
			if err := f.svc.HandleEvent(f.ctx, online); err != nil {
				t.Fatal(err)
			}
		}
		if ds := f.deliveries(t); len(ds) != 2 {
			t.Fatalf("deliveries %d, want 2", len(ds))
		}
		if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 2 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		sent := f.relay.notifications()
		if len(sent) != 2 {
			t.Fatalf("relay got %d", len(sent))
		}
		tokens := map[string]pushrelay.Notification{}
		for _, n := range sent {
			tokens[n.DevicePushID] = n
			b, _ := json.Marshal(pushrelay.BuildPayload(n))
			// No personal data: neither the booking code nor the amount nor
			// the establishment name reach the relay.
			for _, leak := range []string{"LA-2027-000042", "4500", "Lido A", "cassa", "manager@"} {
				if strings.Contains(string(b), leak) {
					t.Fatalf("payload leaks %q: %s", leak, b)
				}
			}
			if n.Message.Key != store.PushTopicNewOnlineBooking || n.Message.Locale != "it" {
				t.Fatalf("message %+v", n.Message)
			}
		}
		if tokens["apns-cashier-0123456789abcdef"].Platform != "ios" || tokens["fcm-manager-0123456789abcdef"].Platform != "android" {
			t.Fatalf("platforms %+v", tokens)
		}
		for _, d := range f.deliveries(t) {
			if d.Status != store.PushDeliverySent || d.SentAt == nil || d.Attempts != 1 {
				t.Fatalf("delivery %+v", d)
			}
		}

		// payment.failed reaches only the manager (refunds.issue) and only
		// for provider payments.
		pf := func(provider string) ext.Event {
			data, _ := json.Marshal(event.PaymentChange{PaymentID: store.NewID(), Provider: provider, Amount: 4500, Status: "failed"})
			return ext.Event{ID: store.NewID(), Type: event.PaymentFailed, EstablishmentID: f.estA.ID, Data: data}
		}
		if err := f.svc.HandleEvent(f.ctx, pf("manual")); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.HandleEvent(f.ctx, pf("stripe")); err != nil {
			t.Fatal(err)
		}
		if ds := f.deliveries(t); len(ds) != 3 || ds[2].MessageKey != store.PushTopicPaymentFailed {
			t.Fatalf("payment failed deliveries %+v", ds)
		}

		// Retry with backoff, then the relay accepts.
		f.relay.mu.Lock()
		f.relay.results = []pushrelay.Result{{Outcome: pushrelay.Retry, Status: 503, Error: "relay status 503"}}
		f.relay.mu.Unlock()
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		d := f.deliveries(t)[2]
		if d.Status != store.PushDeliveryQueued || d.Attempts != 1 || d.NextAttemptAt == nil || !d.NextAttemptAt.Equal(f.clock.Now().Add(30*time.Second)) {
			t.Fatalf("after retry %+v", d)
		}
		if n, _ := f.svc.DeliverDue(f.ctx); n != 0 {
			t.Fatal("retried before the backoff")
		}
		f.clock.Advance(31 * time.Second)
		if n, _ := f.svc.DeliverDue(f.ctx); n != 1 {
			t.Fatal("not retried after the backoff")
		}
		if d := f.deliveries(t)[2]; d.Status != store.PushDeliverySent || d.Attempts != 2 {
			t.Fatalf("after second attempt %+v", d)
		}

		// Push turned off after queueing: nothing is sent.
		ar := bookingEvent(t, f.estA.ID, event.BookingApprovalRequested, store.ChannelOnline)
		if err := f.svc.HandleEvent(f.ctx, ar); err != nil {
			t.Fatal(err)
		}
		before := len(f.relay.notifications())
		cur, _ := f.svc.GetSettings(f.ctx, f.manager, f.estA.ID)
		if _, err := f.svc.UpdateSettings(f.ctx, f.manager, f.estA.ID, &cur.Version, push.SettingsInput{Enabled: false, Events: allEvents}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if len(f.relay.notifications()) != before {
			t.Fatal("sent after push was turned off")
		}
		ds := f.deliveries(t)
		if last := ds[len(ds)-1]; last.Status != store.PushDeliveryExpired || last.Attempts != 0 {
			t.Fatalf("delivery after turn off %+v", last)
		}
		// Turned off: the sink queues nothing.
		if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
			t.Fatal(err)
		}
		if n := len(f.deliveries(t)); n != len(ds) {
			t.Fatalf("queued while off: %d", n)
		}

		// Topic off for the establishment: not queued.
		f.enable(t, push.Events{ApprovalRequested: true})
		if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
			t.Fatal(err)
		}
		if n := len(f.deliveries(t)); n != len(ds) {
			t.Fatalf("queued a topic turned off: %d", n)
		}
	})
}

func TestPushService_DeliveryOutcomes(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		app := f.pairedApp(t, f.manager, store.DevicePlatformAndroid)
		f.register(t, app, "fcm-manager-0123456789abcdef", store.PushTopicNewOnlineBooking)
		queue := func() {
			t.Helper()
			if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
				t.Fatal(err)
			}
		}
		last := func() store.PushDelivery {
			t.Helper()
			ds := f.deliveries(t)
			if len(ds) == 0 {
				t.Fatal("no deliveries")
			}
			return ds[len(ds)-1]
		}

		// Rejected (4xx): failed at once, no retry.
		queue()
		f.relay.results = []pushrelay.Result{{Outcome: pushrelay.Rejected, Status: 400, Error: "relay status 400"}}
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if d := last(); d.Status != store.PushDeliveryFailed || d.Attempts != 1 || d.LastError == nil || *d.LastError != "relay status 400" {
			t.Fatalf("rejected %+v", d)
		}

		// Retry until MaxAttempts, honouring Retry-After (10 minutes, longer
		// than the first backoff steps of 30 s, 2 and 5 minutes, shorter
		// than the last of 15), then failed.
		queue()
		waits := []time.Duration{10 * time.Minute, 10 * time.Minute, 10 * time.Minute, 15 * time.Minute}
		for i := range push.MaxAttempts {
			f.relay.results = []pushrelay.Result{{Outcome: pushrelay.Retry, Status: 429, RetryAfter: 10 * time.Minute}}
			if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 1 {
				t.Fatalf("attempt %d: %d %v", i+1, n, err)
			}
			d := last()
			if i < push.MaxAttempts-1 {
				if d.Status != store.PushDeliveryQueued || d.NextAttemptAt == nil || !d.NextAttemptAt.Equal(f.clock.Now().Add(waits[i])) {
					t.Fatalf("attempt %d: %+v", i+1, d)
				}
				f.clock.Advance(waits[i])
			} else if d.Status != store.PushDeliveryFailed || d.Attempts != push.MaxAttempts {
				t.Fatalf("after max attempts %+v", d)
			}
		}

		// Expired: not sent within MaxAge.
		queue()
		f.clock.Advance(push.MaxAge + time.Minute)
		before := len(f.relay.notifications())
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if d := last(); d.Status != store.PushDeliveryExpired || len(f.relay.notifications()) != before {
			t.Fatalf("expired %+v", d)
		}

		// User deactivated: not eligible, nothing sent.
		queue()
		if _, err := db.NewUpdate().TableExpr("users").Set("active = ?", false).Where("id = ?", f.manager.UserID).Exec(f.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if d := last(); d.Status != store.PushDeliveryExpired || len(f.relay.notifications()) != before {
			t.Fatalf("inactive user %+v", d)
		}
		if _, err := db.NewUpdate().TableExpr("users").Set("active = ?", true).Where("id = ?", f.manager.UserID).Exec(f.ctx); err != nil {
			t.Fatal(err)
		}

		// Gone (410): the registration is forgotten.
		queue()
		f.relay.results = []pushrelay.Result{{Outcome: pushrelay.Gone, Status: 410}}
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "push_registrations", "1 = 1"); n != 0 {
			t.Fatalf("registration kept after 410: %d", n)
		}

		// Retention deletes closed deliveries after a week.
		f.clock.Advance(push.Retention + time.Hour)
		if err := f.svc.Cleanup(f.ctx); err != nil {
			t.Fatal(err)
		}
		if n := len(f.deliveries(t)); n != 0 {
			t.Fatalf("deliveries after retention: %d", n)
		}
	})
}

// TestConcurrency_PushDeliveriesSingleSend: 100 workers run the deliverer
// at the same time on the same queue (two instances, a wake during a
// tick): every delivery reaches the relay exactly once.
func TestConcurrency_PushDeliveriesSingleSend(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		app := f.pairedApp(t, f.manager, store.DevicePlatformAndroid)
		f.register(t, app, "fcm-manager-0123456789abcdef", store.PushTopicNewOnlineBooking)
		const events = 10
		for range events {
			if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
				t.Fatal(err)
			}
		}
		f.relay.delay = 2 * time.Millisecond
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 100)
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if _, err := f.svc.DeliverDue(f.ctx); err != nil {
					errs <- err
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		perID := map[string]int{}
		for _, n := range f.relay.notifications() {
			perID[n.ID]++
		}
		if len(perID) != events {
			t.Fatalf("sent %d distinct deliveries, want %d", len(perID), events)
		}
		for id, n := range perID {
			if n != 1 {
				t.Fatalf("delivery %s sent %d times", id, n)
			}
		}
		for _, d := range f.deliveries(t) {
			if d.Status != store.PushDeliverySent {
				t.Fatalf("delivery %+v", d)
			}
		}
	})
}
