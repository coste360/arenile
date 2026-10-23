//go:build integration

package waitlist_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// day is the stay the tests fill up and free (the fixture clock is
// 2027-06-01, season 2027-05-15..09-15).
const day = "2027-07-10"

// fx is the waitlist on the real booking, inventory, customer and checkout
// services (checkouttest) with an in-memory mailer.
type fx struct {
	*checkouttest.Env
	t     testing.TB
	svc   *waitlist.Service
	mails *mails
	sold  []*booking.Detail
}

func newFx(t testing.TB, db *store.DB) *fx {
	t.Helper()
	e := checkouttest.New(t, db)
	f := e.F
	x := &fx{Env: e, t: t, mails: &mails{}}
	cust := customer.NewService(customer.Deps{DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock)})
	x.svc = waitlist.NewService(waitlist.Deps{
		DB: db, Clock: f.Clock, Holds: e.Inv, Checkout: e.Svc, Notify: x.mails, Customers: cust, Terms: e.Online,
		BaseURL: "https://prenota.example.org",
	})
	return x
}

// fillSeats sells every spot of the seat zone on day (desk, confirmed).
func (x *fx) fillSeats() {
	x.t.Helper()
	for _, sp := range x.F.Spots {
		in := x.F.Input(sp, day, day)
		in.Force = true
		d, err := x.Book.Create(x.Ctx, x.F.Owner, x.F.Est.ID, in)
		if err != nil {
			x.t.Fatalf("sell %s: %v", sp.Label, err)
		}
		x.sold = append(x.sold, d)
	}
}

func (x *fx) customer(name string) *store.Customer {
	x.t.Helper()
	email := strings.ToLower(name) + "@example.org"
	c := &store.Customer{
		ID: store.NewID(), EstablishmentID: x.F.Est.ID, FirstName: name, LastName: "Attesa", Email: &email, Locale: "it",
		Tags: []string{}, Version: 1, CreatedAt: x.F.Clock.Now(), UpdatedAt: x.F.Clock.Now(),
	}
	x.F.Insert(c)
	return c
}

// entry adds a desk entry for the seat zone on day.
func (x *fx) entry(name string) *waitlist.Entry {
	x.t.Helper()
	c := x.customer(name)
	e, err := x.svc.Create(x.Ctx, x.F.Owner, x.F.Est.ID, waitlist.Input{
		CustomerID: c.ID, StartsOn: calendar.MustParse(day), EndsOn: calendar.MustParse(day), Slots: calendar.SlotsFull,
		PackID: x.F.Base.ID, ZoneID: x.F.SeatZone.ID,
	})
	if err != nil {
		x.t.Fatalf("entry %s: %v", name, err)
	}
	return e
}

func (x *fx) cancel(d *booking.Detail) {
	x.t.Helper()
	if _, err := x.Book.Cancel(x.Ctx, x.F.Owner, x.F.Est.ID, d.Booking.ID, booking.CancelInput{Reason: "test"}); err != nil {
		x.t.Fatalf("cancel: %v", err)
	}
}

// pump delivers the pending outbox events to the waitlist sink, as the
// dispatcher does, until none is left.
func (x *fx) pump() {
	x.t.Helper()
	for range 10 {
		var rows []store.OutboxEvent
		if err := x.DB.NewSelect().Model(&rows).Where("dispatched_at IS NULL").OrderExpr("id").Scan(x.Ctx); err != nil {
			x.t.Fatal(err)
		}
		if len(rows) == 0 {
			return
		}
		for _, r := range rows {
			if err := x.svc.Handle(x.Ctx, event.FromRow(r)); err != nil {
				x.t.Fatalf("handle %s: %v", r.Type, err)
			}
			if _, err := x.DB.NewUpdate().Model((*store.OutboxEvent)(nil)).Set("dispatched_at = ?", x.F.Clock.Now()).
				Where("id = ?", r.ID).Exec(x.Ctx); err != nil {
				x.t.Fatal(err)
			}
		}
	}
	x.t.Fatal("pump: events keep coming")
}

func (x *fx) get(id string) *store.WaitlistEntry {
	x.t.Helper()
	e, err := store.GetWaitlistEntry(x.Ctx, x.DB, x.ES, id)
	if err != nil {
		x.t.Fatal(err)
	}
	return e
}

func (x *fx) pending(entryID string) *store.WaitlistOffer {
	x.t.Helper()
	o, err := store.PendingWaitlistOffer(x.Ctx, x.DB, x.ES, entryID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		x.t.Fatal(err)
	}
	return o
}

// mails records the waitlist emails (the link is kept to act as the customer).
type mails struct {
	mu   sync.Mutex
	sent []notify.WaitlistMessage
}

func (m *mails) EnqueueWaitlist(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, msg notify.WaitlistMessage) (*store.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return &store.Message{ID: store.NewID(), Template: msg.Template, DedupeKey: msg.DedupeKey}, nil
}

// token returns the link token of the last email of template to recipient.
func (m *mails) token(t testing.TB, template, recipient string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if s := m.sent[i]; s.Template == template && s.Recipient == recipient {
			return s.URL[strings.LastIndex(s.URL, "/")+1:]
		}
	}
	t.Fatalf("no %s email to %s", template, recipient)
	return ""
}

func TestWaitlist_CancellationOffersToFirstEntryAndAcceptGoesToCheckout(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db)
		x.fillSeats()
		first, second := x.entry("Anna"), x.entry("Bruno")
		if first.Position != 1 || second.Position != 2 || first.Priority >= second.Priority {
			t.Fatalf("queue: %d/%d priorities %d/%d", first.Position, second.Position, first.Priority, second.Priority)
		}
		// Nothing frees: no offer.
		x.pump()
		if x.pending(first.ID) != nil {
			t.Fatal("offer without a release")
		}
		x.cancel(x.sold[2])
		x.pump()
		o := x.pending(first.ID)
		if o == nil || o.SpotID == nil || *o.SpotID != x.F.Spots[2].ID || o.Cause != store.WaitlistCauseCancellation {
			t.Fatalf("offer %+v", o)
		}
		if x.get(first.ID).Status != store.WaitlistOffered || x.get(second.ID).Status != store.WaitlistWaiting {
			t.Fatal("statuses after the offer")
		}
		if o.ExpiresAt.Sub(x.F.Clock.Now()) != waitlist.DefaultOfferHoldMinutes*time.Minute {
			t.Fatalf("offer expires at %v", o.ExpiresAt)
		}
		// The hold protects the spot (database constraint).
		in := x.F.Input(x.F.Spots[2], day, day)
		in.Force = true
		if _, err := x.Book.Create(x.Ctx, x.F.Owner, x.F.Est.ID, in); !isConflict(err) {
			t.Fatalf("sale over the offer hold: %v", err)
		}
		// The customer opens the link and accepts: a held booking with a
		// checkout session; the booking is NOT confirmed (INV-19).
		tok := x.mails.token(t, notify.TemplateWaitlistOffer, "anna@example.org")
		pe, err := x.svc.GetByToken(x.Ctx, checkouttest.Slug, tok)
		if err != nil || pe.Offer == nil || pe.Offer.Status != store.WaitlistOfferPending || pe.Offer.SpotLabel != "A3" {
			t.Fatalf("public entry %+v %v", pe, err)
		}
		s, err := x.svc.AcceptByToken(x.Ctx, checkouttest.Slug, tok, "")
		if err != nil {
			t.Fatal(err)
		}
		b := x.Booking(s.BookingID)
		if b.Status != "held" || b.CustomerID != first.CustomerID || s.Token == "" || s.Quote == nil {
			t.Fatalf("booking %s customer %s", b.Status, b.CustomerID)
		}
		if got := x.get(first.ID); got.Status != store.WaitlistConverted || got.ConvertedBookingID == nil || *got.ConvertedBookingID != b.ID {
			t.Fatalf("entry after accept %+v", got)
		}
		if _, err := x.svc.AcceptByToken(x.Ctx, checkouttest.Slug, tok, ""); !errors.Is(err, waitlist.ErrNoOffer) {
			t.Fatalf("second accept: %v", err)
		}
		if n := x.Count((*store.OutboxEvent)(nil), "type = ?", event.WaitlistConverted); n != 1 {
			t.Fatalf("converted events %d", n)
		}
		// The checkout continues normally on the session.
		if _, err := x.Svc.State(x.Ctx, checkouttest.Slug, s.BookingID, s.Token); err != nil {
			t.Fatalf("checkout state: %v", err)
		}
	})
}

func TestWaitlist_ExpiredOfferMovesToNextEntry(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db)
		x.fillSeats()
		first, second := x.entry("Anna"), x.entry("Bruno")
		x.cancel(x.sold[0])
		x.pump()
		o := x.pending(first.ID)
		if o == nil {
			t.Fatal("no offer")
		}
		x.F.Clock.Set(o.ExpiresAt.Add(time.Minute))
		n, err := x.svc.ExpireDue(x.Ctx)
		if err != nil || n != 1 {
			t.Fatalf("expire due %d %v", n, err)
		}
		x.pump()
		if got := x.get(first.ID); got.Status != store.WaitlistWaiting {
			t.Fatalf("first entry %s", got.Status)
		}
		o2 := x.pending(second.ID)
		if o2 == nil || o2.SpotID == nil || *o2.SpotID != x.F.Spots[0].ID {
			t.Fatalf("second offer %+v", o2)
		}
		old, _ := store.GetWaitlistOffer(x.Ctx, x.DB, x.ES, o.ID)
		if old.Status != store.WaitlistOfferExpired || old.RespondedAt == nil {
			t.Fatalf("expired offer %+v", old)
		}
		// The second customer declines and leaves: the spot goes back to the first.
		tok := x.mails.token(t, notify.TemplateWaitlistOffer, "bruno@example.org")
		pe, err := x.svc.DeclineByToken(x.Ctx, checkouttest.Slug, tok, true)
		if err != nil || pe.Entry.Status != store.WaitlistRemoved {
			t.Fatalf("decline: %+v %v", pe, err)
		}
		x.pump()
		if x.pending(first.ID) == nil {
			t.Fatal("the released hold was not offered to the first entry again")
		}
		if _, err := x.svc.GetByToken(x.Ctx, checkouttest.Slug, tok); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("link of a removed entry: %v", err)
		}
	})
}

func TestWaitlist_PanelOperations(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db)
		a, b, c := x.entry("Anna"), x.entry("Bruno"), x.entry("Carla")
		if err := x.svc.Reorder(x.Ctx, x.F.Owner, x.F.Est.ID, []string{c.ID, a.ID}, ""); err != nil {
			t.Fatal(err)
		}
		page, err := x.svc.List(x.Ctx, x.F.Viewer, x.F.Est.ID, waitlist.Filter{}, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		for _, e := range page.Items {
			order = append(order, e.CustomerDisplayName)
		}
		if got := strings.Join(order, ","); !strings.HasPrefix(got, "Attesa Carla") || len(page.Items) != 3 {
			t.Fatalf("order %s", got)
		}
		if page.Items[0].Position != 1 {
			t.Fatalf("position %d", page.Items[0].Position)
		}
		// Manual offer: free seats exist, so an offer is made now.
		o, err := x.svc.Offer(x.Ctx, x.F.Owner, x.F.Est.ID, b.ID, x.F.Spots[1].ID, 30*time.Minute, "")
		if err != nil || o.SpotLabel != "A2" || o.Cause != store.WaitlistCauseManual {
			t.Fatalf("manual offer %+v %v", o, err)
		}
		if _, err := x.svc.Offer(x.Ctx, x.F.Owner, x.F.Est.ID, b.ID, "", 0, ""); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("second offer: %v", err)
		}
		// Another offer on the same spot for another entry conflicts (hold).
		if _, err := x.svc.Offer(x.Ctx, x.F.Owner, x.F.Est.ID, a.ID, x.F.Spots[1].ID, 30*time.Minute, ""); !isConflict(err) {
			t.Fatalf("offer on a held spot: %v", err)
		}
		r, err := x.svc.RevokeOffer(x.Ctx, x.F.Owner, x.F.Est.ID, b.ID, o.ID, "")
		if err != nil || r.Status != store.WaitlistOfferRevoked {
			t.Fatalf("revoke %+v %v", r, err)
		}
		if x.get(b.ID).Status != store.WaitlistWaiting {
			t.Fatal("entry not back to waiting")
		}
		// Request changes only while waiting; If-Match is honoured.
		stale := int64(1)
		q := 2
		if _, err := x.svc.Update(x.Ctx, x.F.Owner, x.F.Est.ID, a.ID, &stale, waitlist.Patch{Quantity: &q}); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale update: %v", err)
		}
		cur := x.get(a.ID)
		if _, err := x.svc.Update(x.Ctx, x.F.Owner, x.F.Est.ID, a.ID, &cur.Version, waitlist.Patch{Quantity: &q}); err != nil {
			t.Fatal(err)
		}
		if err := x.svc.Remove(x.Ctx, x.F.Owner, x.F.Est.ID, c.ID, nil, ""); err != nil {
			t.Fatal(err)
		}
		if got := x.get(c.ID); got.Status != store.WaitlistRemoved || got.RemovedAt == nil {
			t.Fatalf("removed %+v", got)
		}
		// Permissions and tenancy.
		if _, err := x.svc.Create(x.Ctx, x.F.Viewer, x.F.Est.ID, waitlist.Input{}); err == nil {
			t.Fatal("viewer created an entry")
		}
		if _, _, err := x.svc.Get(x.Ctx, x.F.Outsider, x.F.Est.ID, a.ID); err == nil {
			t.Fatal("outsider read an entry")
		}
		// Settings: disabling the waitlist stops new entries.
		s, err := x.svc.GetSettings(x.Ctx, x.F.Owner, x.F.Est.ID)
		if err != nil || !s.Enabled || s.PublicJoin || s.OfferHoldMinutes != waitlist.DefaultOfferHoldMinutes {
			t.Fatalf("defaults %+v %v", s, err)
		}
		if _, err := x.svc.UpdateSettings(x.Ctx, x.F.Owner, x.F.Est.ID, &s.Version, waitlist.Settings{Enabled: false, OfferHoldMinutes: 60}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := x.svc.Create(x.Ctx, x.F.Owner, x.F.Est.ID, waitlist.Input{CustomerID: a.CustomerID}); !errors.Is(err, waitlist.ErrDisabled) {
			t.Fatalf("disabled: %v", err)
		}
	})
}

func TestWaitlist_PublicJoinAndAnonymisation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db)
		join := waitlist.PublicJoinInput{
			StartsOn: calendar.MustParse(day), EndsOn: calendar.MustParse(day), Slots: calendar.SlotsFull, PackID: x.F.Base.ID,
			FirstName: "Dora", LastName: "Pubblica", Email: "Dora@Example.org", Locale: "en",
		}
		if err := x.svc.Join(x.Ctx, checkouttest.Slug, join); !errors.Is(err, waitlist.ErrDisabled) {
			t.Fatalf("join while public join is off: %v", err)
		}
		if _, err := x.svc.UpdateSettings(x.Ctx, x.F.Owner, x.F.Est.ID, nil, waitlist.Settings{Enabled: true, PublicJoin: true, AutoOffer: true, OfferHoldMinutes: 30}, ""); err != nil {
			t.Fatal(err)
		}
		if err := x.svc.Join(x.Ctx, "nessuno", join); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("unknown slug: %v", err)
		}
		if err := x.svc.Join(x.Ctx, checkouttest.Slug, join); err != nil {
			t.Fatal(err)
		}
		tok := x.mails.token(t, notify.TemplateWaitlistJoined, "dora@example.org")
		pe, err := x.svc.GetByToken(x.Ctx, checkouttest.Slug, tok)
		if err != nil || pe.Entry.Status != store.WaitlistWaiting || pe.Entry.Channel != store.ChannelOnline || pe.Entry.Locale != "en" || pe.PackName != "base" {
			t.Fatalf("public entry %+v %v", pe, err)
		}
		if strings.Contains(fmt.Sprint(x.mails.sent), "Dora") {
			t.Fatal("personal data in the email payload")
		}
		// Wrong token, other slug: plain 404.
		if _, err := x.svc.GetByToken(x.Ctx, checkouttest.Slug, strings.Repeat("A", 43)); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("wrong token: %v", err)
		}
		// customer.anonymized removes the entry and wipes link and note.
		ev := event.FromRow(store.OutboxEvent{
			ID: store.NewID(), EstablishmentID: &x.F.Est.ID, Type: event.CustomerAnonymized, Version: 1,
			Payload: store.JSON(fmt.Sprintf(`{"customer_id":%q}`, pe.Entry.CustomerID)), OccurredAt: x.F.Clock.Now(),
		})
		if err := x.svc.Handle(x.Ctx, ev); err != nil {
			t.Fatal(err)
		}
		got := x.get(pe.Entry.ID)
		if got.Status != store.WaitlistRemoved || got.AccessTokenHash != nil || got.Note != "" {
			t.Fatalf("after anonymisation %+v", got)
		}
	})
}

// TestConcurrency_WaitlistOffer: one freed spot and many waiting entries;
// 100 concurrent deliveries of the release event create exactly one offer
// (the hold is decided by the allocation constraint, one pending offer per
// entry by its unique index).
func TestConcurrency_WaitlistOffer(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		x := newFx(t, db)
		x.fillSeats()
		// The zone-mode zone is not compatible: requests are for zone A only.
		for i := range 10 {
			x.entry(fmt.Sprintf("Cliente%02d", i))
		}
		x.cancel(x.sold[1])
		var row store.OutboxEvent
		if err := db.NewSelect().Model(&row).Where("type = ?", event.BookingCancelled).Limit(1).Scan(x.Ctx); err != nil {
			t.Fatal(err)
		}
		ev := event.FromRow(row)
		var wg sync.WaitGroup
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := x.svc.Handle(x.Ctx, ev); err != nil && !retryable(err) {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if n := x.Count((*store.WaitlistOffer)(nil), "status = ?", store.WaitlistOfferPending); n != 1 {
			t.Fatalf("pending offers %d, want 1", n)
		}
		if n := x.Count((*store.WaitlistEntry)(nil), "status = ?", store.WaitlistOffered); n != 1 {
			t.Fatalf("offered entries %d, want 1", n)
		}
		// One hold: its two half-day rows (AM, PM) on the freed spot.
		if n := x.Count((*store.SpotAllocation)(nil), "spot_id = ? AND date = ?", x.F.Spots[1].ID, day); n != 2 {
			t.Fatalf("allocations of the freed spot %d, want 2 (one hold, two half-days)", n)
		}
		if n := x.Count((*store.Hold)(nil), "released_at IS NULL AND converted_booking_id IS NULL"); n != 1 {
			t.Fatalf("open holds %d, want 1", n)
		}
	})
}

func isConflict(err error) bool {
	var ce *availability.ConflictError
	return errors.As(err, &ce)
}

// retryable reports a transient lock error of the file-backed SQLite
// database (single writer) under heavy contention.
func retryable(err error) bool {
	for _, s := range []string{"database is locked", "SQLITE_BUSY", "deadlock detected", "could not serialize"} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
