package event

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// Dispatcher moves events out of the outbox: it calls the in-process sinks,
// publishes to the real-time bus, schedules webhook deliveries and marks the
// event dispatched. Delivery to sinks is at-least-once.
type Dispatcher struct {
	db        bun.IDB
	clock     clock.Clock
	sinks     []ext.EventSink
	bus       ext.EventBus
	log       *slog.Logger
	batchSize int
}

// NewDispatcher builds a dispatcher. bus may be nil.
func NewDispatcher(db bun.IDB, c clock.Clock, sinks []ext.EventSink, bus ext.EventBus, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Dispatcher{db: db, clock: c, sinks: sinks, bus: bus, log: log, batchSize: 100}
}

// DispatchPending processes up to one batch of pending events and returns
// how many were dispatched.
func (d *Dispatcher) DispatchPending(ctx context.Context) (int, error) {
	rows, err := store.ListPendingOutboxEvents(ctx, d.db, d.batchSize)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		ev := FromRow(row)
		for _, s := range d.sinks {
			if err := d.callSink(ctx, s, ev); err != nil {
				d.log.WarnContext(ctx, "event sink failed", slog.String("sink", s.Name()), slog.String("event_id", ev.ID),
					slog.String("event_type", ev.Type), slog.String("error", err.Error()))
			}
		}
		if d.bus != nil {
			if err := d.bus.Publish(ctx, ev); err != nil {
				d.log.WarnContext(ctx, "event bus publish failed", slog.String("event_id", ev.ID), slog.String("error", err.Error()))
			}
		}
		now := d.clock.Now()
		claimed := false
		err := store.RunInTx(ctx, d.db, func(ctx context.Context, tx bun.Tx) error {
			fresh, err := store.MarkOutboxEventDispatched(ctx, tx, row.ID, now)
			if err != nil || !fresh {
				// Another dispatcher (instance) got there first.
				return err
			}
			claimed = true
			if ev.EstablishmentID == "" {
				return nil
			}
			endpoints, err := store.ListActiveWebhookEndpoints(ctx, tx, ev.EstablishmentID)
			if err != nil {
				return err
			}
			for i := range endpoints {
				if !endpoints[i].Matches(ev.Type) {
					continue
				}
				next := now
				del := &store.WebhookDelivery{
					ID: store.NewID(), EndpointID: endpoints[i].ID, EventID: row.ID,
					NextAttemptAt: &next, CreatedAt: now, UpdatedAt: now,
				}
				if err := store.InsertWebhookDeliveryIfAbsent(ctx, tx, del); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return n, fmt.Errorf("event: dispatch %s: %w", ev.ID, err)
		}
		if claimed {
			n++
		}
	}
	return n, nil
}

func (d *Dispatcher) callSink(ctx context.Context, s ext.EventSink, ev ext.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return s.Handle(ctx, ev)
}
