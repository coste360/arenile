package booking

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// MoveHook runs inside the transaction of a move, after the line has been
// released and re-allocated and the booking row saved, before the audit
// entry and the outbox event are written. An error rolls back the whole
// move (allocations included).
type MoveHook func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) error

// MoveThen is Move with then run in the same transaction (ticket
// assignment, ADR 0020: the ticket row changes atomically with the
// allocation). The booking row is locked first, so every caller touching the
// same booking is serialised. A nil then behaves like Move. Requires
// auth.PermBookingsWrite.
func (s *Service) MoveThen(ctx context.Context, p *auth.Principal, establishmentID, id string, in MoveInput, then MoveHook) (*ChangeResult, error) {
	if in.LineID == "" {
		return nil, configerr.Invalid("line_id", "Campo obbligatorio.")
	}
	if in.SpotID == "" {
		return nil, configerr.Invalid("spot_id", "Campo obbligatorio.")
	}
	return s.changeInTx(ctx, p, establishmentID, id, in.ExpectedVersion, in.DryRun, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, res *ChangeResult) (*outcome, error) {
		o, err := s.moveLocked(ctx, tx, p, es, b, in, res)
		if err != nil || then == nil {
			return o, err
		}
		prev := o.after
		o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
			if prev != nil {
				if err := prev(ctx, tx, b); err != nil {
					return err
				}
			}
			return then(ctx, tx, es, b)
		}
		return o, nil
	})
}
