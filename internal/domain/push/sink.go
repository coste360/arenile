package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Sink returns the ext.EventSink that queues push deliveries.
func (s *Service) Sink() ext.EventSink { return sink{s: s} }

type sink struct{ s *Service }

func (sink) Name() string { return "push" }

// Handle implements ext.EventSink (idempotent on ev.ID through the unique
// (registration, event) index).
func (k sink) Handle(ctx context.Context, ev ext.Event) error {
	return k.s.HandleEvent(ctx, ev)
}

// TopicOf maps an outbox event to the push topic it notifies, or "" when
// the event does not notify. Only the fields needed to decide are read;
// nothing of the payload reaches the relay.
func TopicOf(ev ext.Event) string {
	switch ev.Type {
	case event.BookingConfirmed:
		var c event.BookingChange
		if json.Unmarshal(ev.Data, &c) == nil && c.Channel == store.ChannelOnline {
			return store.PushTopicNewOnlineBooking
		}
	case event.BookingApprovalRequested:
		return store.PushTopicApprovalRequested
	case event.PaymentFailed:
		// Provider payments only, like the manager email (spec 19): a
		// manual payment does not fail asynchronously.
		var c event.PaymentChange
		if json.Unmarshal(ev.Data, &c) == nil && c.Provider != "" && c.Provider != "manual" {
			return store.PushTopicPaymentFailed
		}
	}
	return ""
}

// HandleEvent queues one delivery per eligible registration of the
// establishment of ev. Nothing is queued when the relay is not configured
// or the manager has not turned the topic on.
func (s *Service) HandleEvent(ctx context.Context, ev ext.Event) error {
	topic := TopicOf(ev)
	if topic == "" || ev.EstablishmentID == "" || !s.RelayConfigured() {
		return nil
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ev.EstablishmentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("push: sink: %w", err)
	}
	st, err := s.settings(ctx, s.d.DB, es)
	if err != nil || !st.topicOn(topic) {
		return err
	}
	recipients, err := store.ListPushRecipients(ctx, s.d.DB, es)
	if err != nil {
		return fmt.Errorf("push: sink: %w", err)
	}
	now := s.now()
	var eligible []store.PushRecipient
	for _, r := range recipients {
		ok, err := s.eligible(ctx, es, &r, topic, now)
		if err != nil {
			return err
		}
		if ok {
			eligible = append(eligible, r)
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	queued := false
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		for _, r := range eligible {
			next := now
			ok, err := store.InsertPushDelivery(ctx, tx, es, &store.PushDelivery{
				ID: store.NewID(), RegistrationID: r.ID, EventID: ev.ID, MessageKey: topic,
				Status: store.PushDeliveryQueued, NextAttemptAt: &next, CreatedAt: now, UpdatedAt: now,
			})
			if err != nil {
				return err
			}
			queued = queued || ok
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("push: sink: %w", err)
	}
	if queued {
		s.kick() // after commit: the job sees the new rows
	}
	return nil
}

// eligible reports whether r may receive topic now: subscribed, device
// active and not expired, user active with a role that has the topic's
// permission on es (same rules as a request with the device token).
func (s *Service) eligible(ctx context.Context, es store.EstablishmentScope, r *store.PushRecipient, topic string, now time.Time) (bool, error) {
	if !slices.Contains(r.Topics, topic) || r.DeviceRevokedAt != nil || !r.UserActive {
		return false, nil
	}
	if s.d.IdleExpired != nil && s.d.IdleExpired(&store.Device{CreatedAt: r.DeviceCreatedAt, LastUsedAt: r.DeviceLastUsedAt}, now) {
		return false, nil
	}
	if s.d.Principals == nil {
		return false, nil
	}
	p, err := s.d.Principals.PrincipalForDevice(ctx, auth.DeviceIdentity{DeviceID: r.DeviceID, UserID: r.UserID, EstablishmentID: es.ID()})
	if errors.Is(err, auth.ErrUnauthenticated) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("push: recipient: %w", err)
	}
	return p.Can(es, topicPermission[topic]), nil
}
