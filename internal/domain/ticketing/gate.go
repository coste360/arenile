package ticketing

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Gate outcomes (api.GateCheckInResult.outcome).
const (
	GateAdmitted       = "admitted"
	GateAlreadyEntered = "already_entered"
	GateVoided         = "voided"
	GateWrongDate      = "wrong_date"
)

// GateResult is the outcome of a scan at the gate.
type GateResult struct {
	Outcome string
	View    *View
}

// CheckIn records the entry of the ticket scanned at the gate
// (auth.PermOperationsRun). One entry per ticket: entered_at is set by one
// conditional UPDATE (entered_at IS NULL AND status valid), so of two
// phones scanning the same QR at once exactly one admits it; the others
// answer already_entered. A voided ticket or a ticket of another day is
// never admitted. Unknown codes are ErrNotFound; sale tickets (spots) are
// not entry tickets.
func (s *Service) CheckIn(ctx context.Context, p *auth.Principal, establishmentID, code string) (*GateResult, error) {
	code = NormalizeCode(code)
	if code == "" {
		return nil, configerr.Invalid("code", "Codice mancante.")
	}
	es, err := s.authorize(ctx, p, establishmentID, auth.PermOperationsRun)
	if err != nil {
		return nil, err
	}
	today, err := Today(ctx, s.d.DB, es, s.d.Clock)
	if err != nil {
		return nil, err
	}
	var out *GateResult
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		t, err := store.GetTicketByBookingCode(ctx, tx, es, code)
		if err != nil {
			return notFound(err)
		}
		if t.Kind != store.TicketKindEntry {
			return configerr.Invalid("code", "Il biglietto non è un ingresso: controllalo dalla mappa.")
		}
		outcome := ""
		switch {
		case t.Status == store.TicketVoided:
			outcome = GateVoided
		case t.EnteredAt != nil:
			outcome = GateAlreadyEntered
		case !t.TicketDate.Equal(today):
			outcome = GateWrongDate
		default:
			now := s.now()
			ok, err := store.MarkTicketEntered(ctx, tx, es, t.ID, userIDOf(p), now)
			if err != nil {
				return err
			}
			if t, err = store.GetTicket(ctx, tx, es, t.ID); err != nil {
				return err
			}
			switch {
			case ok:
				outcome = GateAdmitted
				if err := s.record(ctx, tx, p, es, "ticket.entered", t, nil); err != nil {
					return err
				}
			case t.Status == store.TicketVoided:
				outcome = GateVoided
			default:
				outcome = GateAlreadyEntered
			}
		}
		v, err := view(ctx, tx, es, t)
		if err != nil {
			return err
		}
		out = &GateResult{Outcome: outcome, View: v}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
