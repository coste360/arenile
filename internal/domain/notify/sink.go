package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Outbox -> message_log (spec 19 "invio asincrono tramite outbox"). The
// sink runs after the outbox dispatcher picked an event; it only decides
// who gets which template and queues rows in one transaction. Rendering
// and sending happen later in Deliver, with retries.
//
// Idempotency: DedupeKey "<event type>:<event id>:<template>" is unique per
// establishment, so redelivered events (at-least-once) queue nothing new.
//
//	booking.confirmed           -> booking_confirmed (customer), manager_new_booking (online channel)
//	booking.modified            -> booking_modified, only when what the customer sees changed
//	booking.cancelled           -> booking_cancelled (bookings the customer had seen)
//	booking.rejected            -> booking_rejected
//	booking.approval_requested  -> booking_approval_pending, manager_approval_requested
//	payment.refunded            -> refund_issued
//	payment.failed              -> manager_payment_failed (provider payments)
//	message.failed              -> manager_delivery_failed (never for manager templates: no loops)
//	customer.anonymized         -> pending messages cancelled, recipients erased (spec 23)

// Sink returns the ext.EventSink that turns outbox events into messages.
func (s *Service) Sink() ext.EventSink { return sink{s: s} }

type sink struct{ s *Service }

func (sink) Name() string { return "notify" }

// Handle implements ext.EventSink (idempotent on ev.ID through DedupeKey).
func (k sink) Handle(ctx context.Context, ev ext.Event) error {
	if !handled[ev.Type] || ev.EstablishmentID == "" {
		return nil
	}
	return k.s.HandleEvent(ctx, ev)
}

var handled = map[string]bool{
	event.BookingConfirmed: true, event.BookingModified: true, event.BookingCancelled: true,
	event.BookingRejected: true, event.BookingApprovalRequested: true,
	event.PaymentRefunded: true, event.PaymentFailed: true, event.MessageFailed: true,
	event.CustomerAnonymized: true,
}

// planned is one message decided by the sink.
type planned struct {
	template string
	manager  bool
	payload  map[string]any
}

// HandleEvent queues the messages of one outbox event.
func (s *Service) HandleEvent(ctx context.Context, ev ext.Event) error {
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ev.EstablishmentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("notify: sink: %w", err)
	}
	queued := false
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if ev.Type == event.CustomerAnonymized {
			var c event.CustomerChange
			if err := json.Unmarshal(ev.Data, &c); err != nil || c.CustomerID == "" {
				return nil
			}
			_, err := store.ForgetCustomerMessages(ctx, tx, es, c.CustomerID, s.now())
			return err
		}
		cfg, err := s.emailConfig(ctx, tx, es)
		if err != nil {
			return err
		}
		bookingID, plan, err := s.plan(ctx, tx, es, cfg, ev)
		if err != nil || len(plan) == 0 {
			return err
		}
		queued = true
		return s.enqueuePlan(ctx, tx, es, cfg, ev, bookingID, plan)
	})
	if err == nil && queued {
		s.kick() // after commit: the job sees the new rows
	}
	return err
}

// plan decides the templates of an event (no writes).
func (s *Service) plan(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, cfg online.EmailConfig, ev ext.Event) (string, []planned, error) {
	switch ev.Type {
	case event.BookingConfirmed, event.BookingModified, event.BookingCancelled, event.BookingRejected, event.BookingApprovalRequested:
		var c event.BookingChange
		if err := json.Unmarshal(ev.Data, &c); err != nil || c.BookingID == "" {
			return "", nil, nil
		}
		var out []planned
		switch ev.Type {
		case event.BookingConfirmed:
			if cfg.ToCustomer.Confirmation {
				_, fp, err := s.customerVisibleChange(ctx, tx, es, c.BookingID)
				if err != nil {
					return "", nil, err
				}
				out = append(out, planned{template: TemplateBookingConfirmed, payload: map[string]any{"fingerprint": fp}})
			}
			if c.Channel == store.ChannelOnline && cfg.ToManager.NewOnlineBooking {
				out = append(out, planned{template: TemplateManagerNewBooking, manager: true})
			}
		case event.BookingModified:
			if cfg.ToCustomer.Confirmation && (c.Status == statusConfirmed || c.Status == statusCheckedIn) {
				changed, fp, err := s.customerVisibleChange(ctx, tx, es, c.BookingID)
				if err != nil {
					return "", nil, err
				}
				if changed {
					out = append(out, planned{template: TemplateBookingModified, payload: map[string]any{"fingerprint": fp}})
				}
			}
		case event.BookingCancelled:
			// Only bookings the customer was told about: confirmed at some
			// point or awaiting approval. An abandoned or expired checkout
			// (held, pending payment) never got an email.
			b, err := store.GetBooking(ctx, tx, es, c.BookingID)
			if errors.Is(err, store.ErrNotFound) {
				return "", nil, nil
			}
			if err != nil {
				return "", nil, fmt.Errorf("notify: sink: %w", err)
			}
			if (b.ConfirmedAt != nil || c.PreviousStatus == statusAwaitingApproval) && cfg.ToCustomer.Cancellation {
				out = append(out, planned{template: TemplateBookingCancelled})
			}
		case event.BookingRejected:
			if cfg.ToCustomer.Cancellation {
				out = append(out, planned{template: TemplateBookingRejected})
			}
		case event.BookingApprovalRequested:
			if cfg.ToCustomer.Confirmation {
				out = append(out, planned{template: TemplateApprovalPending})
			}
			if cfg.ToManager.ApprovalRequested {
				out = append(out, planned{template: TemplateManagerApproval, manager: true})
			}
		}
		return c.BookingID, out, nil
	case event.PaymentRefunded, event.PaymentFailed:
		var c event.PaymentChange
		if err := json.Unmarshal(ev.Data, &c); err != nil || c.BookingID == "" {
			return "", nil, nil
		}
		payload := map[string]any{"amount": abs(c.Amount), "payment_id": c.PaymentID}
		if ev.Type == event.PaymentRefunded && cfg.ToCustomer.Refund {
			return c.BookingID, []planned{{template: TemplateRefundIssued, payload: payload}}, nil
		}
		if ev.Type == event.PaymentFailed && cfg.ToManager.PaymentFailed && c.Provider != "manual" {
			return c.BookingID, []planned{{template: TemplateManagerPaymentFailed, manager: true, payload: payload}}, nil
		}
	case event.MessageFailed:
		var c event.MessageChange
		if err := json.Unmarshal(ev.Data, &c); err != nil || managerTemplates[c.Template] || !cfg.ToManager.WebhookError {
			return "", nil, nil
		}
		return c.BookingID, []planned{{
			template: TemplateManagerDeliveryFailed, manager: true,
			payload: map[string]any{"failed_template": c.Template, "attempts": c.Attempts, "failed_message_id": c.MessageID},
		}}, nil
	}
	return "", nil, nil
}

// customerVisibleChange compares the booking with the fingerprint of the
// last confirmation or modification email.
func (s *Service) customerVisibleChange(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string) (bool, string, error) {
	d, err := loadBooking(ctx, tx, es, bookingID)
	if errors.Is(err, store.ErrNotFound) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	fp := bookingFingerprint(d)
	last, err := store.LatestBookingMessage(ctx, tx, es, bookingID, TemplateBookingConfirmed, TemplateBookingModified)
	if errors.Is(err, store.ErrNotFound) {
		return true, fp, nil
	}
	if err != nil {
		return false, "", fmt.Errorf("notify: sink: %w", err)
	}
	prev, _ := last.Payload["fingerprint"].(string)
	return prev != fp, fp, nil
}

func (s *Service) enqueuePlan(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, cfg online.EmailConfig, ev ext.Event, bookingID string, plan []planned) error {
	var (
		b  *store.Booking
		cu *store.Customer
	)
	if bookingID != "" {
		var err error
		b, cu, err = s.bookingRecipient(ctx, tx, es, bookingID)
		if errors.Is(err, ErrBookingNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	var est *store.Establishment
	for _, p := range plan {
		e := Enqueue{Template: p.template, BookingID: bookingID, DedupeKey: ev.Type + ":" + ev.ID + ":" + p.template, Payload: p.payload}
		if e.Payload == nil {
			e.Payload = map[string]any{}
		}
		e.Payload["event_id"] = ev.ID
		if p.manager {
			if est == nil {
				var err error
				if est, err = store.GetEstablishment(ctx, tx, es); err != nil {
					return fmt.Errorf("notify: sink: %w", err)
				}
			}
			to := cfg.Manager
			if to == "" {
				to = est.ContactEmail
			}
			if to == "" {
				continue
			}
			e.Recipient, e.Locale = to, est.DefaultLocale
		} else {
			if cu == nil {
				continue // anonymized, placeholder or without email (spec 23)
			}
			e.Recipient, e.CustomerID, e.Locale = *cu.Email, cu.ID, bookingLocale(b, cu)
		}
		if _, err := s.Enqueue(ctx, tx, es, e); err != nil {
			if errors.Is(err, errInvalidRecipient) {
				continue
			}
			return err
		}
	}
	return nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
