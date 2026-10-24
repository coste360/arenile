package ticketing

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
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
// answer already_entered. A voided ticket, a ticket whose booking is no
// longer confirmed or a ticket of another day is never admitted.
//
// The code is the gate token of the QR (random, 128 bit): a booking code or
// a daily number alone never admits (ErrNotFound), because booking codes
// are sequential and guessable. Only a manual entry confirmed by the staff
// (manual = true) resolves a typed booking code or today's ticket number,
// and the audit entry records it. Sale tickets (spots) are not entry
// tickets.
func (s *Service) CheckIn(ctx context.Context, p *auth.Principal, establishmentID, code string, manual bool) (*GateResult, error) {
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
		t, err := resolveGateCode(ctx, tx, es, code, manual, today)
		if err != nil {
			return err
		}
		if t.Kind != store.TicketKindEntry {
			return configerr.Invalid("code", "Il biglietto non è un ingresso: controllalo dalla mappa.")
		}
		// The booking behind the ticket must still be sold (a cancellation
		// outside the void is refused by the booking service, INV-84; this
		// is the belt to that brace).
		bk, err := store.GetBooking(ctx, tx, es, t.BookingID)
		if err != nil {
			return fmt.Errorf("ticketing: gate booking: %w", err)
		}
		outcome := ""
		switch {
		case t.Status == store.TicketVoided, !bookingAdmits(bk.Status):
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
				if err := s.record(ctx, tx, p, es, "ticket.entered", t, map[string]any{"manual": manual}); err != nil {
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

// bookingAdmits reports whether the booking behind an entry ticket is still
// sold: confirmed or checked in (never cancelled, expired or refunded).
func bookingAdmits(status string) bool {
	return status == "confirmed" || status == "checked_in"
}

// resolveGateCode finds the ticket of a code typed or scanned at the gate:
// the gate token of the QR always; with manual, also today's ticket number
// (digits only, the number printed large) or the booking code printed under
// the QR. Anything else is ErrNotFound.
func resolveGateCode(ctx context.Context, db bun.IDB, es store.EstablishmentScope, code string, manual bool, today calendar.Date) (*store.Ticket, error) {
	t, err := store.GetTicketByGateToken(ctx, db, es, code)
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if !manual {
		return nil, ErrNotFound
	}
	if n, ok := ticketNumberOf(code); ok {
		t, err = store.GetTicketByNumber(ctx, db, es, today, n)
	} else {
		t, err = store.GetTicketByBookingCode(ctx, db, es, code)
	}
	if err != nil {
		return nil, notFound(err)
	}
	return t, nil
}

// ticketNumberOf parses a typed daily number ("37", "#037", "037").
func ticketNumberOf(code string) (int64, bool) {
	s := strings.TrimPrefix(code, "#")
	if s == "" || len(s) > 6 {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// gateTokenBytes is the entropy of a gate token (128 bit).
const gateTokenBytes = 16

// NewGateToken returns the random QR payload of a ticket: 26 upper-case
// base32 characters (QR alphanumeric mode), unguessable, unique by index.
func NewGateToken() (string, error) {
	b := make([]byte, gateTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("ticketing: gate token: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}
