// Package audit writes the insert-only audit log (spec 6.8). Entries are
// written with the same bun.IDB (usually a bun.Tx) as the change they
// describe, so an action and its audit record commit or roll back together.
package audit

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/store"
)

// ActorType identifies who performed an action.
type ActorType string

const (
	ActorUser     ActorType = "user"
	ActorCustomer ActorType = "customer"
	ActorSystem   ActorType = "system"
	ActorAPIKey   ActorType = "api_key"
)

// ErrInvalidEntry is returned for incomplete entries.
var ErrInvalidEntry = errors.New("audit: invalid entry")

// Actor is the author of an action.
type Actor struct {
	Type ActorType
	ID   string // empty for system
	IP   string
}

// SystemActor is used by jobs and the CLI.
var SystemActor = Actor{Type: ActorSystem}

// Entry is one audited action. Diff must not contain secrets; personal data
// should be limited to what is needed to reconstruct the change.
type Entry struct {
	EstablishmentID string // empty for organization/user level actions
	Actor           Actor
	Action          string // e.g. "auth.login", "booking.cancelled"
	EntityType      string
	EntityID        string
	Diff            any
}

// Writer appends audit entries.
type Writer struct {
	clock clock.Clock
}

// NewWriter returns a Writer using c for timestamps.
func NewWriter(c clock.Clock) *Writer { return &Writer{clock: c} }

// Record inserts e using db (pass the transaction of the audited change).
func (w *Writer) Record(ctx context.Context, db bun.IDB, e Entry) error {
	if e.Action == "" || e.EntityType == "" || e.Actor.Type == "" {
		return ErrInvalidEntry
	}
	switch e.Actor.Type {
	case ActorUser, ActorCustomer, ActorSystem, ActorAPIKey:
	default:
		return fmt.Errorf("%w: actor type %q", ErrInvalidEntry, e.Actor.Type)
	}
	diff, err := store.MarshalJSONValue(e.Diff)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	row := &store.AuditEntry{
		ID:         store.NewID(),
		ActorType:  string(e.Actor.Type),
		Action:     e.Action,
		EntityType: e.EntityType,
		Diff:       diff,
		IP:         e.Actor.IP,
		CreatedAt:  w.clock.Now(),
	}
	if e.EstablishmentID != "" {
		row.EstablishmentID = &e.EstablishmentID
	}
	if e.Actor.ID != "" {
		row.ActorID = &e.Actor.ID
	}
	if e.EntityID != "" {
		row.EntityID = &e.EntityID
	}
	if err := store.InsertAuditEntry(ctx, db, row); err != nil {
		return fmt.Errorf("audit: record %s: %w", e.Action, err)
	}
	return nil
}
