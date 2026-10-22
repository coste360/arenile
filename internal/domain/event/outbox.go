// Package event implements the transactional outbox (spec 18): domain events
// are written in the same transaction as the change they describe, then a
// dispatcher job fans them out to in-process sinks, the real-time bus and
// outgoing webhooks (signed, retried with exponential backoff).
package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Event types of spec 18.1.
const (
	BookingCreated           = "booking.created"
	BookingHeld              = "booking.held"
	BookingConfirmed         = "booking.confirmed"
	BookingModified          = "booking.modified"
	BookingCancelled         = "booking.cancelled"
	BookingCheckedIn         = "booking.checked_in"
	BookingNoShow            = "booking.no_show"
	BookingCompleted         = "booking.completed"
	BookingApprovalRequested = "booking.approval_requested"
	AllocationReleased       = "allocation.released"
	SeasonDayReleased        = "season.day_released"
	SeasonRenewalOffered     = "season.renewal_offered"
	PaymentSucceeded         = "payment.succeeded"
	PaymentFailed            = "payment.failed"
	PaymentRefunded          = "payment.refunded"
	CustomerCreated          = "customer.created"
	CustomerAnonymized       = "customer.anonymized"
	CashSessionClosed        = "cash_session.closed"
	ConfigPricingChanged     = "config.pricing_changed"
	ConfigMapChanged         = "config.map_changed"
)

// IDPrefix prefixes event ids in envelopes ("evt_<uuidv7>").
const IDPrefix = "evt_"

// ErrInvalidEvent is returned for malformed events.
var ErrInvalidEvent = errors.New("event: invalid event")

var typeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Outbox writes events into outbox_events.
type Outbox struct {
	clock clock.Clock
}

// NewOutbox returns an Outbox using c for timestamps.
func NewOutbox(c clock.Clock) *Outbox { return &Outbox{clock: c} }

// Write builds an envelope for data and inserts it using db, which must be
// the transaction (bun.Tx) of the state change, so that the event exists if
// and only if the change commits. establishmentID may be empty for
// installation-level events.
func (o *Outbox) Write(ctx context.Context, db bun.IDB, eventType, establishmentID string, version int, data any) (ext.Event, error) {
	return o.write(ctx, db, eventType, establishmentID, version, data)
}

func (o *Outbox) write(ctx context.Context, db bun.IDB, eventType, establishmentID string, version int, data any) (ext.Event, error) {
	if !typeRe.MatchString(eventType) {
		return ext.Event{}, fmt.Errorf("%w: type %q", ErrInvalidEvent, eventType)
	}
	if version <= 0 {
		version = 1
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return ext.Event{}, fmt.Errorf("event: marshal data: %w", err)
	}
	if data == nil {
		raw = []byte("{}")
	}
	id := uuid.Must(uuid.NewV7()).String()
	now := o.clock.Now()
	ev := ext.Event{
		ID:              IDPrefix + id,
		Type:            eventType,
		EstablishmentID: establishmentID,
		OccurredAt:      now,
		Data:            raw,
		Version:         version,
	}
	row := &store.OutboxEvent{
		ID:         id,
		Type:       eventType,
		Payload:    store.JSON(raw),
		Version:    version,
		OccurredAt: now,
		CreatedAt:  now,
	}
	if establishmentID != "" {
		row.EstablishmentID = &establishmentID
	}
	if err := store.InsertOutboxEvent(ctx, db, row); err != nil {
		return ext.Event{}, fmt.Errorf("event: write %s: %w", eventType, err)
	}
	return ev, nil
}

// FromRow converts an outbox row into its envelope.
func FromRow(r store.OutboxEvent) ext.Event {
	ev := ext.Event{
		ID:         IDPrefix + r.ID,
		Type:       r.Type,
		OccurredAt: r.OccurredAt.UTC(),
		Data:       json.RawMessage(r.Payload),
		Version:    r.Version,
	}
	if r.EstablishmentID != nil {
		ev.EstablishmentID = *r.EstablishmentID
	}
	if len(ev.Data) == 0 {
		ev.Data = json.RawMessage("{}")
	}
	return ev
}

// RowID strips the "evt_" prefix of an envelope id.
func RowID(eventID string) string { return strings.TrimPrefix(eventID, IDPrefix) }
