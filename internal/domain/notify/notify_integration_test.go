//go:build integration

package notify_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

const customerEmail = "mario.rossi@example.it"

// harness is a notify service on top of the booking fixture, with an
// in-memory transport and a captured log.
type harness struct {
	*bookingtest.Fixture
	Notify    *notify.Service
	Transport *notify.MemoryTransport
	Logs      *bytes.Buffer
	ES        store.EstablishmentScope
	Customer  *store.Customer
}

func newHarness(t *testing.T, db *store.DB, maxAttempts int) *harness {
	t.Helper()
	f := bookingtest.New(t, db)
	keys, err := crypto.NewKeyRing(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{Fixture: f, Transport: &notify.MemoryTransport{}, Logs: &bytes.Buffer{}}
	log := slog.New(slog.NewJSONHandler(h.Logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.Notify = notify.NewService(notify.Deps{
		DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock), Log: log,
		Keys: keys, Transport: h.Transport, MailFrom: "noreply@arenile.example", BaseURL: "https://arenile.example",
		MaxAttempts: maxAttempts,
	})
	if h.ES, err = store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID); err != nil {
		t.Fatal(err)
	}
	f.Est.ContactEmail = "info@lidodelfino.example"
	if _, err := db.NewUpdate().Model(f.Est).Column("contact_email").WherePK().Exec(f.Ctx); err != nil {
		t.Fatal(err)
	}
	email, now := customerEmail, f.Clock.Now()
	h.Customer = &store.Customer{
		ID: store.NewID(), FirstName: "Mario", LastName: "Rossi", Email: &email, Locale: "en",
		Tags: []string{}, SearchText: "rossi mario", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertCustomer(f.Ctx, db, h.ES, h.Customer); err != nil {
		t.Fatal(err)
	}
	return h
}

// book creates a confirmed desk booking of the harness customer.
func (h *harness) book(spot int) *store.Booking {
	h.T.Helper()
	in := h.Input(h.Spots[spot], "2027-07-10", "2027-07-12")
	in.Customer, in.CustomerID = nil, h.Customer.ID
	return &h.MustCreate(in).Booking
}

// dispatch runs the outbox dispatcher with the notify sink.
func (h *harness) dispatch() {
	h.T.Helper()
	d := event.NewDispatcher(h.DB, h.Clock, []ext.EventSink{h.Notify.Sink()}, nil, nil)
	for {
		n, err := d.DispatchPending(h.Ctx)
		if err != nil {
			h.T.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
}

func (h *harness) messages(where string, args ...any) []store.Message {
	h.T.Helper()
	var out []store.Message
	q := h.DB.NewSelect().Model(&out).OrderExpr("msg.id ASC")
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Scan(h.Ctx); err != nil {
		h.T.Fatal(err)
	}
	return out
}

func (h *harness) deliver() int {
	h.T.Helper()
	n, err := h.Notify.Deliver(h.Ctx)
	if err != nil {
		h.T.Fatal(err)
	}
	return n
}

func (h *harness) assertNoPII() {
	h.T.Helper()
	if strings.Contains(h.Logs.String(), customerEmail) || strings.Contains(h.Logs.String(), "/m/") {
		h.T.Fatalf("logs contain personal data or tokens:\n%s", h.Logs.String())
	}
}

func tokenFromURL(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, "/b/lido-delfino/m/")
	if i < 0 {
		t.Fatalf("no manage link in %q", body)
	}
	return body[i+len("/b/lido-delfino/m/") : i+len("/b/lido-delfino/m/")+43]
}

func TestNotify_OutboxToDelivery(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		h.dispatch()

		msgs := h.messages("")
		if len(msgs) != 1 {
			t.Fatalf("messages %+v", msgs)
		}
		m := msgs[0]
		if m.Template != notify.TemplateBookingConfirmed || m.Status != store.MessageQueued || m.Locale != "en" ||
			m.BookingID == nil || *m.BookingID != b.ID || m.CustomerID == nil || *m.CustomerID != h.Customer.ID {
			t.Fatalf("message %+v", m)
		}
		if m.RecipientHash != notify.RecipientHash(h.Est.ID, customerEmail) || m.RecipientEnc == nil || strings.Contains(*m.RecipientEnc, "mario") {
			t.Fatal("recipient not hashed + encrypted")
		}
		if !strings.HasPrefix(m.DedupeKey, event.BookingConfirmed+":") {
			t.Fatalf("dedupe key %q", m.DedupeKey)
		}

		// At-least-once: the same event again queues nothing.
		var rows []store.OutboxEvent
		if err := db.NewSelect().Model(&rows).Scan(h.Ctx); err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if err := h.Notify.Sink().Handle(h.Ctx, event.FromRow(r)); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(h.messages("")); n != 1 {
			t.Fatalf("redelivery queued %d messages", n)
		}

		if n := h.deliver(); n != 1 {
			t.Fatalf("delivered %d", n)
		}
		sent := h.Transport.Sent()
		if len(sent) != 1 {
			t.Fatalf("sent %d", len(sent))
		}
		e := sent[0]
		if e.To != customerEmail || !strings.Contains(e.Subject, b.Code) || !strings.Contains(e.Subject, "confirmed") ||
			e.From != `"Lido Delfino" <noreply@arenile.example>` || e.ReplyTo != "info@lidodelfino.example" {
			t.Fatalf("email headers %+v", e)
		}
		if len(e.Attach) != 3 || !e.Attach[0].Inline || e.Attach[0].ContentID != notify.QRContentID ||
			!strings.Contains(string(e.Attach[1].Data), "UID:"+b.ID+"@arenile.example") ||
			!e.Attach[2].Inline || e.Attach[2].ContentID != notify.BrandMarkContentID {
			t.Fatalf("attachments %+v", e.Attach)
		}
		m = h.messages("")[0]
		if m.Status != store.MessageSent || m.Attempts != 1 || m.SentAt == nil || m.NextAttemptAt != nil {
			t.Fatalf("after delivery %+v", m)
		}
		// The link in the email resolves; nothing else is due.
		token := tokenFromURL(t, e.Text)
		link, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, token)
		if err != nil || link.BookingID != b.ID || link.LastUsedAt == nil {
			t.Fatalf("resolve: %v %+v", err, link)
		}
		if h.deliver() != 0 {
			t.Fatal("sent twice")
		}
		ics, err := h.Notify.ICS(h.Ctx, db, h.ES, b.ID, "")
		if err != nil || !strings.Contains(string(ics), "DTSTART;VALUE=DATE:20270710") {
			t.Fatalf("ics: %v %s", err, ics)
		}
		h.assertNoPII()
	})
}

func TestNotify_RetryBackoffAndDeadLetter(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 3)
		var calls atomic.Int32
		h.Transport.Fail = func(notify.Email) error {
			calls.Add(1)
			return errors.New("dial tcp: connection refused for " + customerEmail)
		}
		b := h.book(0)
		h.dispatch()
		start := h.Clock.Now()

		h.deliver()
		m := h.messages("")[0]
		if m.Status != store.MessageFailed || m.Attempts != 1 || m.NextAttemptAt == nil || !m.NextAttemptAt.Equal(start.Add(time.Minute)) {
			t.Fatalf("attempt 1: %+v", m)
		}
		if m.LastError == nil || strings.Contains(*m.LastError, customerEmail) {
			t.Fatalf("last error not redacted: %v", m.LastError)
		}
		if h.deliver() != 0 {
			t.Fatal("retried before the backoff")
		}
		h.Clock.Advance(time.Minute)
		h.deliver()
		m = h.messages("")[0]
		if m.Attempts != 2 || !m.NextAttemptAt.Equal(h.Clock.Now().Add(2*time.Minute)) {
			t.Fatalf("attempt 2: %+v", m)
		}
		h.Clock.Advance(2 * time.Minute)
		h.deliver()
		m = h.messages("")[0]
		if m.Status != store.MessageFailed || m.Attempts != 3 || m.NextAttemptAt != nil {
			t.Fatalf("dead letter: %+v", m)
		}
		if calls.Load() != 3 {
			t.Fatalf("transport calls %d", calls.Load())
		}
		h.Clock.Advance(24 * time.Hour)
		if h.deliver() != 0 {
			t.Fatal("dead letter retried")
		}
		// Links issued for failed attempts were revoked.
		if n := h.Count((*store.MagicLinkToken)(nil), "booking_id = ? AND revoked_at IS NULL", b.ID); n != 0 {
			t.Fatalf("active links %d", n)
		}
		if h.Count((*store.OutboxEvent)(nil), "type = ?", event.MessageFailed) != 1 ||
			h.Count((*store.AuditEntry)(nil), "action = ? AND entity_id = ?", event.MessageFailed, m.ID) != 1 {
			t.Fatal("message.failed event or audit missing")
		}
		// message.failed -> the manager is told, never about a manager message.
		h.Transport.Fail = nil
		h.dispatch()
		mgr := h.messages("template = ?", notify.TemplateManagerDeliveryFailed)
		if len(mgr) != 1 || mgr[0].Locale != "it" || mgr[0].CustomerID != nil {
			t.Fatalf("manager notification %+v", mgr)
		}
		h.deliver()
		sent := h.Transport.Sent()
		if len(sent) != 1 || sent[0].To != "info@lidodelfino.example" || !strings.Contains(sent[0].Text, b.Code) {
			t.Fatalf("manager email %+v", sent)
		}
		h.assertNoPII()
	})
}

func TestNotify_PermanentFailureDeadLettersAtOnce(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		h.Transport.Fail = func(notify.Email) error {
			return errors.Join(notify.ErrPermanent, errors.New("550 mailbox unavailable"))
		}
		h.book(0)
		h.dispatch()
		h.deliver()
		m := h.messages("")[0]
		if m.Status != store.MessageFailed || m.Attempts != 1 || m.NextAttemptAt != nil {
			t.Fatalf("message %+v", m)
		}
	})
}

func TestNotify_AnonymizedCustomerReceivesNothing(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		h.book(0)
		h.dispatch() // queued for the customer
		now := h.Clock.Now()
		if _, err := db.NewUpdate().Model((*store.Customer)(nil)).Set("anonymized_at = ?", now).Set("email = NULL").
			Where("id = ?", h.Customer.ID).Exec(h.Ctx); err != nil {
			t.Fatal(err)
		}
		// A queued message is cancelled at delivery time.
		h.deliver()
		if m := h.messages("")[0]; m.Status != store.MessageCancelled || len(h.Transport.Sent()) != 0 {
			t.Fatalf("message %+v", m)
		}
		// New events queue nothing.
		h.book(1)
		h.dispatch()
		if n := len(h.messages("")); n != 1 {
			t.Fatalf("messages %d", n)
		}
		// customer.anonymized erases the encrypted recipients.
		if _, err := event.NewOutbox(h.Clock).Write(h.Ctx, db, event.CustomerAnonymized, h.Est.ID, 1, event.CustomerChange{CustomerID: h.Customer.ID}); err != nil {
			t.Fatal(err)
		}
		h.dispatch()
		if h.Count((*store.Message)(nil), "recipient_enc IS NOT NULL") != 0 {
			t.Fatal("recipient ciphertext kept after anonymization")
		}
	})
}

func TestNotify_MagicLinks(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		var link *notify.MagicLink
		err := store.RunInTx(h.Ctx, db, func(ctx context.Context, tx bun.Tx) error {
			var err error
			link, err = h.Notify.IssueMagicLink(ctx, tx, h.ES, b.ID, nil)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(link.Token) != 43 || link.URL != "https://arenile.example/b/lido-delfino/m/"+link.Token {
			t.Fatalf("link %+v", link)
		}
		// Expiry: midnight after 2027-07-12 (Europe/Rome, UTC+2) + 30 days.
		if want := time.Date(2027, 8, 11, 22, 0, 0, 0, time.UTC); !link.ExpiresAt.Equal(want) {
			t.Fatalf("expires %v, want %v", link.ExpiresAt, want)
		}
		if h.Count((*store.MagicLinkToken)(nil), "token_hash = ?", notify.HashLinkToken(link.Token)) != 1 ||
			h.Count((*store.MagicLinkToken)(nil), "token_hash = ?", link.Token) != 0 {
			t.Fatal("token stored in clear")
		}
		other, err := store.SystemEstablishmentScope(h.Ctx, db, h.Other.ID)
		if err != nil {
			t.Fatal(err)
		}
		for name, tc := range map[string]struct {
			es    store.EstablishmentScope
			token string
		}{
			"other establishment": {other, link.Token},
			"malformed":           {h.ES, "short"},
			"unknown":             {h.ES, strings.Repeat("A", 43)},
		} {
			if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, tc.es, tc.token); !errors.Is(err, notify.ErrLinkNotFound) {
				t.Errorf("%s: err = %v", name, err)
			}
		}
		// Regeneration revokes the previous link and returns a new one.
		fresh, sent, err := h.Notify.RegenerateLink(h.Ctx, h.Cashier, h.Est.ID, b.ID, true)
		if err != nil || !sent {
			t.Fatalf("regenerate: %v %v", err, sent)
		}
		if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, link.Token); !errors.Is(err, notify.ErrLinkNotFound) {
			t.Fatal("old link still valid")
		}
		// The email of the regeneration carries another link and keeps the desk one valid.
		h.deliver()
		emailed := tokenFromURL(t, h.Transport.Sent()[0].Text)
		for _, tok := range []string{fresh.Token, emailed} {
			if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, tok); err != nil {
				t.Fatalf("link %s: %v", tok[:6], err)
			}
		}
		if _, _, err := h.Notify.RegenerateLink(h.Ctx, h.Viewer, h.Est.ID, b.ID, false); err == nil {
			t.Fatal("viewer regenerated a link")
		}
		// Expired links do not resolve.
		h.Clock.Advance(80 * 24 * time.Hour)
		if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, fresh.Token); !errors.Is(err, notify.ErrLinkNotFound) {
			t.Fatal("expired link resolved")
		}
		h.assertNoPII()
	})
}

func TestNotify_RequestLink(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		for _, tc := range []struct{ email, code string }{
			{"someone@else.example", b.Code}, {customerEmail, "LD-0000-000000"}, {"not an email", b.Code},
		} {
			if err := h.Notify.RequestLink(h.Ctx, h.ES, tc.email, tc.code); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(h.messages("template = ?", notify.TemplateManageLink)); n != 0 {
			t.Fatalf("queued %d links for wrong data", n)
		}
		if err := h.Notify.RequestLink(h.Ctx, h.ES, " Mario.Rossi@Example.IT ", strings.ToLower(b.Code)); err != nil {
			t.Fatal(err)
		}
		// A second request within 10 minutes is deduplicated.
		_ = h.Notify.RequestLink(h.Ctx, h.ES, customerEmail, b.Code)
		if n := len(h.messages("template = ?", notify.TemplateManageLink)); n != 1 {
			t.Fatalf("queued %d", n)
		}
		h.deliver()
		sent := h.Transport.Sent()
		if len(sent) != 1 || !strings.Contains(sent[0].Subject, b.Code) {
			t.Fatalf("sent %+v", sent)
		}
		if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, tokenFromURL(t, sent[0].Text)); err != nil {
			t.Fatal(err)
		}
		// Rate limit (3 per booking code per hour, wrong attempts included):
		// the 4th request of the hour is dropped silently, then allowed again.
		h.Clock.Advance(11 * time.Minute)
		if err := h.Notify.RequestLink(h.Ctx, h.ES, customerEmail, b.Code); err != nil {
			t.Fatal(err)
		}
		if n := len(h.messages("template = ?", notify.TemplateManageLink)); n != 1 {
			t.Fatalf("rate limit: %d messages", n)
		}
		h.Clock.Advance(time.Hour)
		_ = h.Notify.RequestLink(h.Ctx, h.ES, customerEmail, b.Code)
		if n := len(h.messages("template = ?", notify.TemplateManageLink)); n != 2 {
			t.Fatalf("after the window: %d messages", n)
		}
		// The new link revokes the previous ones.
		h.deliver()
		sent = h.Transport.Sent()
		if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, tokenFromURL(t, sent[0].Text)); !errors.Is(err, notify.ErrLinkNotFound) {
			t.Fatal("previous link still valid")
		}
		if _, err := h.Notify.ResolveMagicLink(h.Ctx, db, h.ES, tokenFromURL(t, sent[1].Text)); err != nil {
			t.Fatal(err)
		}
		h.assertNoPII()
	})
}

func TestNotify_AdminUseCases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		if _, err := h.Notify.SendTest(h.Ctx, h.Viewer, h.Est.ID, "x@y.example"); err == nil {
			t.Fatal("viewer sent a test email")
		}
		if _, err := h.Notify.SendTest(h.Ctx, h.Manager, h.Est.ID, "not-an-email"); err == nil {
			t.Fatal("invalid address accepted")
		}
		test, err := h.Notify.SendTest(h.Ctx, h.Manager, h.Est.ID, "ops@lidodelfino.example")
		if err != nil {
			t.Fatal(err)
		}
		msg, err := h.Notify.ResendConfirmation(h.Ctx, h.Cashier, h.Est.ID, b.ID)
		if err != nil || msg.Template != notify.TemplateBookingConfirmed {
			t.Fatalf("resend: %v %+v", err, msg)
		}
		if _, err := h.Notify.ResendConfirmation(h.Ctx, h.Outsider, h.Est.ID, b.ID); err == nil {
			t.Fatal("outsider resent a confirmation")
		}
		h.deliver()
		if n := len(h.Transport.Sent()); n != 2 {
			t.Fatalf("sent %d", n)
		}
		page, err := h.Notify.ListMessages(h.Ctx, h.Viewer, h.Est.ID, notify.MessageFilter{}, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("page 1: %v %+v", err, page)
		}
		if page.Items[0].Message.ID != msg.ID || page.Items[0].RecipientMasked != "m***i@e***.it" || page.Items[0].BookingCode != b.Code {
			t.Fatalf("newest first, masked: %+v", page.Items[0])
		}
		page2, err := h.Notify.ListMessages(h.Ctx, h.Viewer, h.Est.ID, notify.MessageFilter{}, page.NextCursor, 10)
		if err != nil || len(page2.Items) != 1 || page2.Items[0].Message.ID != test.ID || page2.Items[0].RecipientMasked != "o***s@l***.example" {
			t.Fatalf("page 2: %v %+v", err, page2)
		}
		filtered, err := h.Notify.ListMessages(h.Ctx, h.Viewer, h.Est.ID, notify.MessageFilter{BookingID: b.ID, Status: store.MessageSent}, "", 10)
		if err != nil || len(filtered.Items) != 1 {
			t.Fatalf("filtered: %v %+v", err, filtered)
		}
		if h.Count((*store.AuditEntry)(nil), "action = ?", "notify.test_email_queued") != 1 ||
			h.Count((*store.AuditEntry)(nil), "action = ?", "booking.confirmation_resent") != 1 {
			t.Fatal("audit missing")
		}
		h.assertNoPII()
	})
}

func TestNotify_ModifiedOnlyOnVisibleChange(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		h.dispatch()
		ob := event.NewOutbox(h.Clock)
		modified := func() {
			t.Helper()
			fresh, err := store.GetBooking(h.Ctx, db, h.ES, b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ob.Write(h.Ctx, db, event.BookingModified, h.Est.ID, 1, event.BookingChange{
				BookingID: b.ID, Code: b.Code, Status: fresh.Status, Channel: fresh.Channel, CustomerID: fresh.CustomerID, Version: fresh.Version,
			}); err != nil {
				t.Fatal(err)
			}
			h.dispatch()
		}
		modified() // nothing the customer sees changed (e.g. internal notes)
		if n := len(h.messages("template = ?", notify.TemplateBookingModified)); n != 0 {
			t.Fatalf("modified emails %d", n)
		}
		if _, err := db.NewUpdate().Model((*store.Booking)(nil)).Set("total_amount = total_amount + 500").Where("id = ?", b.ID).Exec(h.Ctx); err != nil {
			t.Fatal(err)
		}
		modified()
		modified() // same state again: no second email
		if n := len(h.messages("template = ?", notify.TemplateBookingModified)); n != 1 {
			t.Fatalf("modified emails %d", n)
		}
	})
}

func TestNotify_CancellationAndRefund(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		b := h.book(0)
		h.dispatch()
		if _, err := h.Svc.Cancel(h.Ctx, h.Manager, h.Est.ID, b.ID, booking.CancelInput{Reason: "meteo"}); err != nil {
			t.Fatal(err)
		}
		// A refund recorded by the payment gateway (payload of payment.refunded).
		if _, err := event.NewOutbox(h.Clock).Write(h.Ctx, db, event.PaymentRefunded, h.Est.ID, 1, event.PaymentChange{
			PaymentID: store.NewID(), BookingID: b.ID, Provider: "stripe", Method: "card", Direction: "out", Amount: -1500, Status: "succeeded",
		}); err != nil {
			t.Fatal(err)
		}
		h.dispatch()
		if n := len(h.messages("template = ?", notify.TemplateBookingCancelled)); n != 1 {
			t.Fatalf("cancellation emails %d", n)
		}
		if n := len(h.messages("template = ?", notify.TemplateRefundIssued)); n != 1 {
			t.Fatalf("refund emails %d", n)
		}
		h.deliver()
		var refund *notify.Email
		for _, e := range h.Transport.Sent() {
			if strings.Contains(e.Subject, "Refund") {
				e := e
				refund = &e
			}
		}
		if refund == nil || !strings.Contains(refund.Text, "€15.00") {
			t.Fatalf("refund email %+v", refund)
		}
		h.assertNoPII()
	})
}

// TestConcurrency_DeliverClaimsOnce: 100 workers race on the same due
// messages; the conditional claim lets exactly one send each.
func TestConcurrency_DeliverClaimsOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		const msgs = 5
		for range msgs {
			if _, err := h.Notify.SendTest(h.Ctx, h.Manager, h.Est.ID, "ops@lidodelfino.example"); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		errs := make(chan error, 100)
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := h.Notify.Deliver(h.Ctx); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		sent := h.Transport.Sent()
		seen := map[string]bool{}
		for _, e := range sent {
			if seen[e.DedupeID] {
				t.Fatalf("message %s sent twice", e.DedupeID)
			}
			seen[e.DedupeID] = true
		}
		if len(sent) != msgs || h.Count((*store.Message)(nil), "status = ?", store.MessageSent) != msgs {
			t.Fatalf("sent %d of %d", len(sent), msgs)
		}
	})
}
