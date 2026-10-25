//go:build integration

package push_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// pairedAppOn pairs a mobile device for the user of p on establishmentID.
func (f *fixture) pairedAppOn(t *testing.T, p *auth.Principal, establishmentID, platform string) *auth.Principal {
	t.Helper()
	pc, err := f.devices.CreatePairingCode(f.ctx, p, establishmentID, device.PairingCodeInput{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.devices.Pair(f.ctx, device.PairInput{Code: pc.Code, Name: "Telefono", Platform: platform, Kind: store.DeviceKindMobile, IP: "192.0.2.20"})
	if err != nil {
		t.Fatal(err)
	}
	dp, err := f.devices.AuthenticateDevice(f.ctx, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	return dp
}

func (f *fixture) scope(t *testing.T, establishmentID string) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.ctx, f.db, establishmentID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// TestPushService_CrossTenant (INV-26, INV-100): push settings,
// registrations and deliveries never cross establishments. An event of B
// reaches only the devices of B, the same phone token registered on two
// establishments stays two isolated registrations, and the scoped store
// functions and the device revocation of B never see or touch a
// registration of A.
func TestPushService_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		if _, err := f.svc.UpdateSettings(f.ctx, f.ownerB, f.estB.ID, nil, push.SettingsInput{Enabled: true, Events: allEvents}, ""); err != nil {
			t.Fatal(err)
		}
		// A user of A cannot change the settings of B.
		if _, err := f.svc.UpdateSettings(f.ctx, f.manager, f.estB.ID, nil, push.SettingsInput{Enabled: false}, ""); err == nil {
			t.Fatal("manager of A changed the push settings of B")
		}

		const shared = "fcm-shared-phone-0123456789abcdef"
		appA := f.pairedAppOn(t, f.manager, f.estA.ID, store.DevicePlatformAndroid)
		appB := f.pairedAppOn(t, f.ownerB, f.estB.ID, store.DevicePlatformAndroid)
		f.register(t, appA, shared, store.PushTopicNewOnlineBooking)
		f.register(t, appB, shared, store.PushTopicNewOnlineBooking)
		if n := count(t, db, "push_registrations", "1 = 1"); n != 2 {
			t.Fatalf("registrations %d: the same token on two establishments must stay two rows", n)
		}
		esA, esB := f.scope(t, f.estA.ID), f.scope(t, f.estB.ID)
		regA, err := store.GetPushRegistrationByDevice(f.ctx, db, esA, appA.DeviceID)
		if err != nil {
			t.Fatal(err)
		}

		// An event of B queues only for the device of B.
		evB := bookingEvent(t, f.estB.ID, event.BookingConfirmed, store.ChannelOnline)
		if err := f.svc.HandleEvent(f.ctx, evB); err != nil {
			t.Fatal(err)
		}
		if ds := f.deliveries(t); len(ds) != 0 {
			t.Fatalf("event of B queued %d deliveries on A", len(ds))
		}
		dsB, err := store.ListPushDeliveries(f.ctx, db, esB)
		if err != nil || len(dsB) != 1 {
			t.Fatalf("deliveries of B %d %v", len(dsB), err)
		}
		if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 1 {
			t.Fatalf("deliver %d %v", n, err)
		}
		sent := f.relay.notifications()
		if len(sent) != 1 || sent[0].ID != dsB[0].ID {
			t.Fatalf("relay got %+v, want only the delivery of B", sent)
		}

		// Scoped store functions of B never see or touch the registration of A.
		if _, err := store.GetPushRecipient(f.ctx, db, esB, regA.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("recipient of A read through B: %v", err)
		}
		if _, err := store.GetPushRegistrationByDevice(f.ctx, db, esB, appA.DeviceID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("registration of A read through B: %v", err)
		}
		if err := store.DeletePushRegistration(f.ctx, db, esB, regA.ID); err != nil {
			t.Fatal(err)
		}
		if deleted, err := store.DeletePushRegistrationByDevice(f.ctx, db, esB, appA.DeviceID); err != nil || deleted {
			t.Fatalf("registration of A deleted through B: %v %v", deleted, err)
		}
		recipientsB, err := store.ListPushRecipients(f.ctx, db, esB)
		if err != nil || len(recipientsB) != 1 || recipientsB[0].DeviceID != appB.DeviceID {
			t.Fatalf("recipients of B %+v %v", recipientsB, err)
		}

		// The owner of B cannot revoke the device of A, neither through A
		// nor by naming it on B; the registration of A survives.
		if err := f.devices.Revoke(f.ctx, f.ownerB, f.estA.ID, appA.DeviceID); err == nil {
			t.Fatal("owner of B revoked a device of A")
		}
		_ = f.devices.Revoke(f.ctx, f.ownerB, f.estB.ID, appA.DeviceID)
		if st, err := f.svc.Status(f.ctx, appA); err != nil || !st.Registered {
			t.Fatalf("registration of A after the attempts of B: %+v %v", st, err)
		}

		// Withdrawing consent on B leaves A registered.
		if err := f.svc.Unregister(f.ctx, appB); err != nil {
			t.Fatal(err)
		}
		if st, err := f.svc.Status(f.ctx, appA); err != nil || !st.Registered {
			t.Fatalf("registration of A after the opt-out on B: %+v %v", st, err)
		}
	})
}

// TestPushService_CrashedWorkerLeaseReclaimed (INV-100): a worker that dies
// while sending keeps the delivery leased; no other worker sends it until
// the lease expires, then it is sent again with the same delivery id (the
// relay deduplicates on it) and the lost attempt is counted.
func TestPushService_CrashedWorkerLeaseReclaimed(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		app := f.pairedApp(t, f.manager, store.DevicePlatformAndroid)
		f.register(t, app, "fcm-manager-0123456789abcdef", store.PushTopicNewOnlineBooking)
		if err := f.svc.HandleEvent(f.ctx, bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)); err != nil {
			t.Fatal(err)
		}
		ds := f.deliveries(t)
		if len(ds) != 1 {
			t.Fatalf("deliveries %d", len(ds))
		}
		// The crashed worker: claimed, never finished.
		now := f.clock.Now()
		if _, err := store.ClaimPushDelivery(f.ctx, db, ds[0].ID, now, now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 0 || len(f.relay.notifications()) != 0 {
			t.Fatalf("sent while another worker holds the lease: %d %v", n, err)
		}
		f.clock.Advance(2*time.Minute + time.Second)
		if n, err := f.svc.DeliverDue(f.ctx); err != nil || n != 1 {
			t.Fatalf("not reclaimed after the lease: %d %v", n, err)
		}
		sent := f.relay.notifications()
		if len(sent) != 1 || sent[0].ID != ds[0].ID {
			t.Fatalf("relay got %+v, want the delivery %s once", sent, ds[0].ID)
		}
		if d := f.deliveries(t)[0]; d.Status != store.PushDeliverySent || d.Attempts != 2 {
			t.Fatalf("after reclaim %+v, want sent with the lost attempt counted", d)
		}
	})
}

// TestConcurrency_PushSinkRedelivery (INV-100): the same outbox event handed
// to the sink 100 times in parallel (several dispatchers before one marks
// it dispatched) queues exactly one delivery per subscribed registration.
func TestConcurrency_PushSinkRedelivery(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.enable(t, allEvents)
		f.register(t, f.pairedApp(t, f.manager, store.DevicePlatformAndroid), "fcm-manager-0123456789abcdef", store.PushTopicNewOnlineBooking)
		f.register(t, f.pairedApp(t, f.cashier, store.DevicePlatformIOS), "apns-cashier-0123456789abcdef", store.PushTopicNewOnlineBooking)
		ev := bookingEvent(t, f.estA.ID, event.BookingConfirmed, store.ChannelOnline)
		const n = 100
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, n)
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := f.svc.HandleEvent(f.ctx, ev); err != nil {
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
		ds := f.deliveries(t)
		if len(ds) != 2 {
			t.Fatalf("deliveries %d, want one per registration (2)", len(ds))
		}
		if _, err := f.svc.DeliverDue(f.ctx); err != nil {
			t.Fatal(err)
		}
		if got := len(f.relay.notifications()); got != 2 {
			t.Fatalf("relay got %d notifications, want 2", got)
		}
	})
}
