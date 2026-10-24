//go:build integration

package notify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// fixedSettings is a notify.Settings with a fixed email configuration.
type fixedSettings struct{ cfg online.EmailConfig }

func (s fixedSettings) EmailConfigFor(context.Context, bun.IDB, store.EstablishmentScope) (*online.EmailConfig, error) {
	c := s.cfg
	return &c, nil
}

// withSettings rebuilds the harness service with cfg as email settings.
func (h *harness) withSettings(cfg online.EmailConfig) {
	h.T.Helper()
	keys, err := crypto.NewKeyRing([]byte(strings.Repeat("\x07", 32)))
	if err != nil {
		h.T.Fatal(err)
	}
	h.Notify = notify.NewService(notify.Deps{
		DB: h.DB, Clock: h.Clock, Audit: audit.NewWriter(h.Clock), Outbox: event.NewOutbox(h.Clock),
		Keys: keys, Transport: h.Transport, MailFrom: "noreply@arenile.example", BaseURL: "https://arenile.example",
		Settings: fixedSettings{cfg},
	})
}

func remindersOn() online.EmailConfig {
	c := online.DefaultEmailConfig()
	c.ToCustomer.BalanceReminder, c.ToCustomer.DayBeforeReminder = true, true
	return c
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (h *harness) runReminders() int {
	h.T.Helper()
	n, err := h.Notify.RunReminders(h.Ctx)
	if err != nil {
		h.T.Fatal(err)
	}
	return n
}

func (h *harness) exec(q string, args ...any) {
	h.T.Helper()
	if _, err := h.DB.ExecContext(h.Ctx, q, args...); err != nil {
		h.T.Fatal(err)
	}
}

// TestReminders_SchedulingAndEligibility drives the reminder job with the
// fake clock in the Europe/Rome timezone: nothing before the send hour,
// exactly one message per booking and kind afterwards (repeated runs queue
// nothing), test, cancelled and no-email bookings skipped, bookings made
// after the send instant skipped, the balance reminder on the day given by
// the policy balance_due rule and the lead days, and the rendered emails
// carry the edited texts.
func TestReminders_SchedulingAndEligibility(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		h.withSettings(remindersOn())
		b1 := h.book(0) // 2027-07-10..12, balance due on arrival (default policy)
		b2 := h.book(1)
		h.exec("UPDATE bookings SET test = ? WHERE id = ?", true, b2.ID)
		b3 := h.book(2)
		h.exec("UPDATE bookings SET status = 'cancelled' WHERE id = ?", b3.ID)
		b4 := h.book(3)
		now := h.Clock.Now()
		noMail := &store.Customer{ID: store.NewID(), FirstName: "Senza", LastName: "Email", Locale: "it", Tags: []string{}, SearchText: "senza", Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := store.InsertCustomer(h.Ctx, db, h.ES, noMail); err != nil {
			t.Fatal(err)
		}
		h.exec("UPDATE bookings SET customer_id = ? WHERE id = ?", noMail.ID, b4.ID)
		// b5: 2027-07-14, balance due 2 days before (2027-07-12), reminder 2
		// lead days earlier: 2027-07-10 at 10:00 Rome (08:00 UTC).
		in := h.Input(h.Spots[0], "2027-07-14", "2027-07-15")
		in.Customer, in.CustomerID = nil, h.Customer.ID
		b5 := &h.MustCreate(in).Booking
		h.exec(`UPDATE bookings SET policy_snapshot = ? WHERE id = ?`,
			`{"version":1,"rules":{"payment":{"balance_due":"days_before:2"}},"resolved_at":"2027-06-01T08:00:00Z"}`, b5.ID)

		// Edited Italian/English texts are used by the reminder emails.
		owner := *h.Owner
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, &owner, h.Est.ID, notify.TemplateDayBeforeReminder, "en", nil, notify.Texts{
			Subject: "Tomorrow at {stabilimento}, {nome}!", Opening: "Dear {nome} {cognome},\nspot {postazione}.", Signature: "The <b>team</b>",
		}); err != nil {
			t.Fatal(err)
		}

		// Balance of b1 (on arrival, due 2027-07-10, lead 2 -> 2027-07-08 10:00 Rome).
		h.Clock.Set(at(t, "2027-07-08T07:59:00Z"))
		if n := h.runReminders(); n != 0 {
			t.Fatalf("before the balance hour: %d", n)
		}
		h.Clock.Set(at(t, "2027-07-08T08:00:00Z"))
		if n := h.runReminders(); n != 1 {
			t.Fatalf("balance reminders queued %d, want 1 (b1)", n)
		}
		if n := h.runReminders(); n != 0 {
			t.Fatalf("second run queued %d", n)
		}
		// Day before (2027-07-09 18:00 Rome = 16:00 UTC).
		h.Clock.Set(at(t, "2027-07-09T15:59:00Z"))
		if n := h.runReminders(); n != 0 {
			t.Fatalf("before the day-before hour: %d", n)
		}
		h.Clock.Set(at(t, "2027-07-09T16:00:00Z"))
		if n := h.runReminders(); n != 1 {
			t.Fatalf("day-before reminders queued %d, want 1 (b1 only)", n)
		}
		h.Clock.Set(at(t, "2027-07-09T21:59:00Z")) // 23:59 Rome, same local day
		if n := h.runReminders(); n != 0 {
			t.Fatalf("later run queued %d", n)
		}
		for _, b := range []*store.Booking{b2, b3, b4} {
			if got := h.messages("booking_id = ? AND template IN (?)", b.ID, bun.List([]string{notify.TemplateDayBeforeReminder, notify.TemplateBalanceReminder})); len(got) != 0 {
				t.Fatalf("booking %s got %d reminders", b.Code, len(got))
			}
		}
		// b5 created after its send instant: skipped; created before: sent once.
		h.exec("UPDATE bookings SET created_at = ? WHERE id = ?", at(t, "2027-07-10T09:00:00Z"), b5.ID)
		h.Clock.Set(at(t, "2027-07-10T09:30:00Z"))
		if n := h.runReminders(); n != 0 {
			t.Fatalf("booking made after the send instant got %d", n)
		}
		h.exec("UPDATE bookings SET created_at = ? WHERE id = ?", at(t, "2027-06-01T08:00:00Z"), b5.ID)
		if n := h.runReminders(); n != 1 {
			t.Fatalf("b5 balance reminder queued %d", n)
		}
		// The next day the window of b5 is over.
		h.Clock.Set(at(t, "2027-07-11T09:00:00Z"))
		if n := h.runReminders(); n != 0 {
			t.Fatalf("next day queued %d", n)
		}

		if got := h.messages("dedupe_key = ?", notify.ReminderDedupeKey(notify.TemplateDayBeforeReminder, b1.ID)); len(got) != 1 {
			t.Fatalf("day-before rows %d", len(got))
		}
		h.deliver()
		var dayBefore, balance *notify.Email
		for _, e := range h.Transport.Sent() {
			switch e.Headers["X-Arenile-Template"] {
			case notify.TemplateDayBeforeReminder:
				dayBefore = &e
			case notify.TemplateBalanceReminder:
				if strings.Contains(e.Text, b1.Code) {
					balance = &e
				}
			}
		}
		if dayBefore == nil || balance == nil {
			t.Fatalf("emails sent: %d", len(h.Transport.Sent()))
		}
		if dayBefore.Subject != "Tomorrow at Lido Delfino, Mario!" || !strings.Contains(dayBefore.HTML, "Dear Mario Rossi,<br>spot A1.") ||
			!strings.Contains(dayBefore.HTML, "The &lt;b&gt;team&lt;/b&gt;") || !strings.Contains(dayBefore.HTML, "cid:qr@arenile") ||
			!strings.Contains(dayBefore.Text, "/b/lido-delfino/m/") {
			t.Fatalf("day-before email:\n%s\n%s", dayBefore.Subject, dayBefore.Text)
		}
		if !strings.HasPrefix(balance.Subject, "Reminder: balance of booking "+b1.Code) || !strings.Contains(balance.Text, "Balance due:") ||
			!strings.Contains(balance.Text, "on arrival") {
			t.Fatalf("balance email:\n%s\n%s", balance.Subject, balance.Text)
		}
		h.assertNoPII()
	})
}

// TestReminders_SkippedAtSendTime: a booking paid or cancelled between
// queuing and delivery gets no reminder (the message is cancelled).
func TestReminders_SkippedAtSendTime(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		h.withSettings(remindersOn())
		b := h.book(0)
		h.Clock.Set(at(t, "2027-07-08T08:00:00Z"))
		if n := h.runReminders(); n != 1 {
			t.Fatalf("queued %d", n)
		}
		h.exec("UPDATE bookings SET paid_amount = total_amount WHERE id = ?", b.ID)
		h.deliver()
		if len(h.Transport.Sent()) != 0 {
			t.Fatal("reminder sent for a paid booking")
		}
		if h.Count((*store.Message)(nil), "status = ?", store.MessageCancelled) != 1 {
			t.Fatal("message not cancelled")
		}
	})
}

// TestReminders_TogglesOff: no reminder when the kinds are off.
func TestReminders_TogglesOff(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		cfg := online.DefaultEmailConfig()
		cfg.ToCustomer.BalanceReminder, cfg.ToCustomer.DayBeforeReminder = false, false
		h.withSettings(cfg)
		h.book(0)
		for _, ts := range []string{"2027-07-08T08:00:00Z", "2027-07-09T16:00:00Z"} {
			h.Clock.Set(at(t, ts))
			if n := h.runReminders(); n != 0 {
				t.Fatalf("%s: queued %d", ts, n)
			}
		}
	})
}

// TestConcurrency_ReminderOnce: 100 parallel attempts to queue the same
// reminder (job runs on several instances, overlapping runs) create exactly
// one message (INV-95, unique message_log dedupe key).
func TestConcurrency_ReminderOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		h.withSettings(remindersOn())
		b := h.book(0)
		h.Clock.Set(at(t, "2027-07-09T16:00:00Z"))
		var wg sync.WaitGroup
		var created atomic.Int64
		errs := make(chan error, 200)
		for i := range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i%2 == 0 {
					ok, err := h.Notify.QueueReminder(h.Ctx, h.ES, notify.TemplateDayBeforeReminder, b)
					if err != nil {
						errs <- err
					}
					if ok {
						created.Add(1)
					}
					return
				}
				n, err := h.Notify.RunReminders(h.Ctx)
				if err != nil {
					errs <- err
				}
				created.Add(int64(n))
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		if got := created.Load(); got != 1 {
			t.Fatalf("created %d messages, want 1", got)
		}
		if n := h.Count((*store.Message)(nil), "template = ?", notify.TemplateDayBeforeReminder); n != 1 {
			t.Fatalf("message_log has %d day-before reminders", n)
		}
	})
}

// TestEmailTemplates_EditorUseCases covers the editor: defaults, save with
// If-Match, 422 on an unknown placeholder, version conflicts, permissions,
// tenancy, audit, preview with escaping and sample data, test email to the
// current user and restore default.
func TestEmailTemplates_EditorUseCases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h := newHarness(t, db, 0)
		kind := notify.TemplateBookingConfirmed
		list, err := h.Notify.ListEmailTemplates(h.Ctx, h.Viewer, h.Est.ID)
		if err != nil || len(list) != len(notify.EditableKinds)*len(notify.Locales) {
			t.Fatalf("list: %v %d", err, len(list))
		}
		if list[0].Customized || list[0].Version != 0 || list[0].Texts != list[0].Defaults || len(list[0].FixedBlocks) == 0 {
			t.Fatalf("default view %+v", list[0])
		}
		in := notify.Texts{Subject: "Confermata {codice}", Opening: "Ciao {nome}!\n\nSecondo paragrafo.", Closing: "A presto", Signature: "Staff {stabilimento}"}
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Cashier, h.Est.ID, kind, "it", nil, in); err == nil {
			t.Fatal("cashier saved a template")
		}
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Outsider, h.Est.ID, kind, "it", nil, in); err == nil {
			t.Fatal("outsider saved a template")
		}
		bad := in
		bad.Opening = "Ciao {email}"
		var te *notify.TemplateError
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", nil, bad); !errors.As(err, &te) || te.Fields[0].Field != "opening" {
			t.Fatalf("unknown placeholder: %v", err)
		}
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, notify.TemplateManagerNewBooking, "it", nil, in); !errors.Is(err, notify.ErrTemplateNotFound) {
			t.Fatalf("manager template editable: %v", err)
		}
		v1 := int64(1)
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", &v1, in); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale create: %v", err)
		}
		zero := int64(0)
		saved, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", &zero, in)
		if err != nil || saved.Version != 1 || !saved.Customized || saved.Texts.Subject != "Confermata {codice}" {
			t.Fatalf("save: %v %+v", err, saved)
		}
		if _, err := h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", &zero, in); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale update: %v", err)
		}
		in.Closing = "Arrivederci"
		saved, err = h.Notify.UpdateEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", &v1, in)
		if err != nil || saved.Version != 2 {
			t.Fatalf("update: %v %+v", err, saved)
		}
		// Other establishments keep the defaults.
		if other, err := h.Notify.GetEmailTemplate(h.Ctx, h.Outsider, h.Other.ID, kind, "it"); err != nil || other.Customized {
			t.Fatalf("other establishment: %v %+v", err, other)
		}

		// Preview: escaped sample data, inline images as data URIs.
		pv, err := h.Notify.PreviewEmailTemplate(h.Ctx, h.Viewer, h.Est.ID, kind, "it", notify.Texts{Subject: "Ciao {nome}", Opening: "<script>x</script> {saldo}"})
		if err != nil {
			t.Fatal(err)
		}
		if pv.Subject != "Ciao Mario" || strings.Contains(pv.HTML, "<script>") || !strings.Contains(pv.HTML, "&lt;script&gt;") ||
			strings.Contains(pv.HTML, "cid:") || !strings.Contains(pv.HTML, "data:image/png;base64,") || !strings.Contains(pv.HTML, "120,00") {
			t.Fatalf("preview:\n%s\n%s", pv.Subject, pv.Text)
		}
		if _, err := h.Notify.PreviewEmailTemplate(h.Ctx, h.Viewer, h.Est.ID, kind, "it", notify.Texts{Subject: "{x}"}); !errors.As(err, &te) {
			t.Fatalf("preview unknown placeholder: %v", err)
		}

		// Test email to the current user.
		if _, err := h.Notify.SendEmailTemplateTest(h.Ctx, h.Manager, h.Est.ID, kind, "de", in); !errors.Is(err, notify.ErrNoUserEmail) {
			t.Fatalf("no user email: %v", err)
		}
		mgr := *h.Manager
		mgr.Email = "gestore@lidodelfino.example"
		if _, err := h.Notify.SendEmailTemplateTest(h.Ctx, &mgr, h.Est.ID, kind, "de", in); err != nil {
			t.Fatal(err)
		}
		h.deliver()
		sent := h.Transport.Sent()
		if len(sent) != 1 || sent[0].To != "gestore@lidodelfino.example" || sent[0].Subject != notify.TestSubjectPrefix+"Confermata PROVA-2027-000123" ||
			!strings.Contains(sent[0].Text, "Staff Lido Delfino") || !strings.Contains(sent[0].Text, "Arrivederci") {
			t.Fatalf("test email: %+v", sent)
		}

		// Restore default.
		v2 := int64(2)
		reset, err := h.Notify.ResetEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", &v2)
		if err != nil || reset.Customized || reset.Version != 0 || reset.Texts != reset.Defaults {
			t.Fatalf("reset: %v %+v", err, reset)
		}
		if _, err := h.Notify.ResetEmailTemplate(h.Ctx, h.Manager, h.Est.ID, kind, "it", nil); err != nil {
			t.Fatalf("reset again: %v", err)
		}
		for action, want := range map[string]int{
			"notify.email_template_updated": 2, "notify.email_template_reset": 1, "notify.email_template_test_queued": 1,
		} {
			if got := h.Count((*store.AuditEntry)(nil), "action = ?", action); got != want {
				t.Errorf("audit %s = %d, want %d", action, got, want)
			}
		}
		h.assertNoPII()
	})
}
