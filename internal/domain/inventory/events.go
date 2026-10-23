package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Outbox event types written by the inventory (payload event.AllocationChange,
// version 1). The SSE stream maps them to the topics hold.changed and
// allocation.changed (event.Topic); the dispatcher publishes them after
// commit, so the panel never sees an allocation that was rolled back.
const (
	eventHoldCreated      = event.HoldCreated
	eventHoldReleased     = event.HoldReleased
	eventHoldConverted    = event.HoldConverted
	eventSpotBlockCreated = event.SpotBlockCreated
	eventSpotBlockDeleted = event.SpotBlockDeleted
	eventAssigned         = event.AllocationAssigned
)

// Audit actions of the inventory use cases.
const (
	ActionHoldCreated      = "hold.created"
	ActionHoldReleased     = "hold.released"
	ActionSpotBlockCreated = "spot_block.created"
	ActionSpotBlockDeleted = "spot_block.deleted"
	ActionLinesAssigned    = "allocation.assigned"
)

// changeIDs are the owner ids of an AllocationChange.
type changeIDs struct {
	bookingID, holdID, blockID string
}

// allocationChange builds the event payload (ids, spots and dates only: no
// personal data, spec 21.1 / 23).
func allocationChange(reason string, a *Allocation, set func(*changeIDs)) event.AllocationChange {
	var ids changeIDs
	if set != nil {
		set(&ids)
	}
	c := event.AllocationChange{Reason: reason, BookingID: ids.bookingID, HoldID: ids.holdID, BlockID: ids.blockID}
	if a != nil {
		c.SpotIDs = a.SpotIDs
		c.PoolIDs = a.PoolIDs
		for _, d := range a.Dates {
			c.Dates = append(c.Dates, d.String())
		}
	}
	return c
}

func (s *Service) writeEvent(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, typ string, data event.AllocationChange) error {
	if s.d.Outbox == nil {
		return nil
	}
	_, err := s.d.Outbox.Write(ctx, tx, typ, es.ID(), 1, data)
	return err
}

func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, ip, action, entity, id string, diff any) error {
	if s.d.Audit == nil {
		return nil
	}
	actor := audit.SystemActor
	if p != nil {
		actor = p.Actor(ip)
	}
	return s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: entity, EntityID: id, Diff: diff,
	})
}

// authorize resolves the establishment in the principal's scope and checks
// perm (spec 16.1: reads bookings.read, holds bookings.write, blocks
// operations.run).
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

// wrap adds context while keeping the error kinds (store.ErrNotFound,
// configerr, availability) visible to errors.Is / errors.As.
func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("inventory: %s: %w", op, configerr.ErrNotFound)
	}
	return fmt.Errorf("inventory: %s: %w", op, err)
}
