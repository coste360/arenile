package renewal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/store"
)

// ---- proposals (panel) ------------------------------------------------------------

// ListProposals lists the proposals of a campaign (bookings.read).
func (s *Service) ListProposals(ctx context.Context, p *auth.Principal, establishmentID, campaignID string, f ProposalFilter, cursor string, limit int) (store.Page[Proposal], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[Proposal]{}, err
	}
	if _, err := store.GetRenewalCampaign(ctx, s.d.DB, es, campaignID); err != nil {
		return store.Page[Proposal]{}, notFound(err)
	}
	page, err := store.ListRenewalProposalRows(ctx, s.d.DB, es, campaignID, store.RenewalProposalFilter{
		Status: f.Status, Sent: f.Sent, ZoneID: f.ZoneID, Query: f.Query,
	}, cursor, limit)
	if err != nil {
		return store.Page[Proposal]{}, err
	}
	out := store.Page[Proposal]{Items: make([]Proposal, len(page.Items)), NextCursor: page.NextCursor}
	for i := range page.Items {
		out.Items[i] = toProposal(page.Items[i])
	}
	return out, nil
}

// GetProposal returns one proposal (bookings.read).
func (s *Service) GetProposal(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Proposal, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	prop, err := store.GetRenewalProposal(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.proposalView(ctx, s.d.DB, es, prop)
}

// errSkipped reports a bulk item that does not apply (wrong status, already
// sent): counted as skipped, not as a failure.
var errSkipped = errors.New("renewal: not applicable")

// change runs fn on the locked proposal id in its own transaction (If-Match
// on expected) and returns the updated view.
func (s *Service) change(ctx context.Context, es store.EstablishmentScope, id string, expected *int64,
	fn func(ctx context.Context, tx bun.Tx, prop *store.RenewalProposal, now time.Time) error,
) (*Proposal, error) {
	var out *Proposal
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		prop, err := store.LockRenewalProposal(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, prop.Version); err != nil {
			return err
		}
		if err := fn(ctx, tx, prop, s.now()); err != nil {
			return err
		}
		out, err = s.proposalView(ctx, tx, es, prop)
		return err
	})
	return out, err
}

// Accept accepts on behalf of the customer (bookings.write). With
// in.Confirm the booking is confirmed now through the booking service
// (manual driver, desk) and the proposal converts; otherwise the booking
// waits for the payment (pending_payment, hold kept until the deadline).
func (s *Service) Accept(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in AcceptInput) (*Proposal, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(in.Note) > maxNote {
		return nil, configerr.Invalid("note", fmt.Sprintf("Massimo %d caratteri.", maxNote))
	}
	actor := p.Actor(in.IP)
	return s.change(ctx, es, id, expected, func(ctx context.Context, tx bun.Tx, prop *store.RenewalProposal, now time.Time) error {
		err := s.acceptLocked(ctx, tx, actor, es, prop, in, now)
		if errors.Is(err, errSkipped) {
			// Already accepted and still waiting for the payment: a repeated
			// (double-clicked) accept is idempotent and returns the view.
			return nil
		}
		return err
	})
}

func (s *Service) acceptLocked(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, in AcceptInput, now time.Time) error {
	if err := checkOpen(prop, now); err != nil {
		return err
	}
	if err := sourceRenewable(ctx, tx, es, prop); err != nil {
		return err
	}
	version, wasProposed := prop.Version, prop.Status == store.RenewalProposed
	if in.Confirm {
		if _, err := s.d.Bookings.ConfirmRenewal(ctx, tx, actor, es, prop.BookingID, strings.TrimSpace(in.Note)); err != nil {
			return err
		}
		respond(prop, store.RenewalConverted, store.RenewalResponseDesk, now)
	} else {
		if prop.Status == store.RenewalAccepted {
			return errSkipped
		}
		if _, err := s.d.Bookings.StartPayment(ctx, tx, actor, es, prop.BookingID, now); err != nil {
			return err
		}
		respond(prop, store.RenewalAccepted, store.RenewalResponseDesk, now)
	}
	if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
		return err
	}
	ev := ""
	if wasProposed {
		ev = event.SeasonRenewalAccepted
	}
	return s.record(ctx, tx, es, actor, "renewal_proposal.accepted", prop, map[string]any{
		"confirmed": in.Confirm, "channel": store.RenewalResponseDesk, "note": in.Note != "",
	}, ev, store.RenewalResponseDesk)
}

// checkOpen refuses proposals that are closed (ErrNotOpen) or whose
// pre-emption deadline passed while still proposed (ErrExpired).
func checkOpen(prop *store.RenewalProposal, now time.Time) error {
	switch {
	case prop.Status == store.RenewalExpired:
		return ErrExpired
	case !Open(prop.Status):
		return ErrNotOpen
	case prop.Status == store.RenewalProposed && !prop.DeadlineAt.After(now):
		return ErrExpired
	}
	return nil
}

// Decline records a decline and releases the spot (bookings.write).
func (s *Service) Decline(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, reason, ip string) (*Proposal, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReason {
		return nil, configerr.Invalid("reason", fmt.Sprintf("Massimo %d caratteri.", maxReason))
	}
	actor := p.Actor(ip)
	return s.change(ctx, es, id, expected, func(ctx context.Context, tx bun.Tx, prop *store.RenewalProposal, now time.Time) error {
		return s.declineLocked(ctx, tx, actor, es, prop, reason, store.RenewalResponseDesk, now)
	})
}

func (s *Service) declineLocked(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, reason, channel string, now time.Time) error {
	if prop.Status == store.RenewalExpired {
		return ErrExpired
	}
	if !Open(prop.Status) {
		return ErrNotOpen
	}
	if _, err := s.d.Bookings.EndRenewal(ctx, tx, actor, es, prop.BookingID, true, reason); err != nil {
		return err
	}
	version := prop.Version
	respond(prop, store.RenewalDeclined, channel, now)
	prop.DeclineReason = reason
	if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
		return err
	}
	// The free-text reason stays on the proposal (anonymisable), never in
	// the insert-only audit log (INV-30).
	return s.record(ctx, tx, es, actor, "renewal_proposal.declined", prop, map[string]any{
		"channel": channel, "reason": reason != "",
	}, event.SeasonRenewalDeclined, channel)
}

// Send emails (or re-emails) the proposal with a fresh magic link
// (bookings.write): the first email is the proposal, the next ones the
// reminder. The link is issued by notify when the message is composed.
func (s *Service) Send(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) (*Proposal, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	actor := p.Actor(ip)
	return s.change(ctx, es, id, nil, func(ctx context.Context, tx bun.Tx, prop *store.RenewalProposal, now time.Time) error {
		return s.sendLocked(ctx, tx, actor, es, prop, now)
	})
}

func (s *Service) sendLocked(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, now time.Time) error {
	if err := checkOpen(prop, now); err != nil {
		return err
	}
	if prop.Status != store.RenewalProposed {
		return ErrNotOpen
	}
	if s.d.Notify == nil {
		return fmt.Errorf("renewal: %w: notifier not configured", configerr.ErrNotImplemented)
	}
	cu, err := store.GetCustomer(ctx, tx, es, prop.CustomerID)
	if err != nil {
		return err
	}
	if cu.AnonymizedAt != nil || cu.Email == nil || strings.TrimSpace(*cu.Email) == "" {
		return notify.ErrNoRecipient
	}
	b, err := store.GetBooking(ctx, tx, es, prop.BookingID)
	if err != nil {
		return err
	}
	locale := cu.Locale
	if b.Locale != nil && *b.Locale != "" {
		locale = *b.Locale
	}
	tpl := notify.TemplateRenewalProposal
	if prop.SendCount > 0 {
		tpl = notify.TemplateRenewalReminder
	}
	if _, err := s.d.Notify.Enqueue(ctx, tx, es, notify.Enqueue{
		Template: tpl, BookingID: prop.BookingID, CustomerID: prop.CustomerID, Recipient: *cu.Email, Locale: locale,
		DedupeKey: "renewal:" + prop.ID + ":" + strconv.Itoa(prop.SendCount+1),
		Payload: map[string]any{
			"renewal_deadline": prop.DeadlineAt.Format(time.RFC3339), "deposit_due": int64(prop.DepositDue),
			"previous_amount": int64(prop.PreviousAmount),
		},
	}); err != nil {
		return err
	}
	version, first := prop.Version, prop.SendCount == 0
	prop.SendCount++
	prop.LastSentAt = &now
	if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
		return err
	}
	ev := ""
	if first {
		ev = event.SeasonRenewalOffered
	}
	return s.record(ctx, tx, es, actor, "renewal_proposal.sent", prop, map[string]any{"send_count": prop.SendCount, "template": tpl}, ev, "")
}

// Bulk applies an action to many proposals, one transaction each: the
// proposals in.ProposalIDs and/or in status in.Status of the campaign. A
// proposal the action does not apply to is skipped; errors are reported
// per proposal.
func (s *Service) Bulk(ctx context.Context, p *auth.Principal, establishmentID, campaignID string, in BulkInput) (*BulkResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	v := &configerr.ValidationError{}
	switch in.Action {
	case ActionSend, ActionAccept, ActionDecline, ActionRelease:
	case ActionExtendDeadline:
		if in.DeadlineAt == nil {
			v.Add("deadline_at", "Obbligatoria per extend_deadline.")
		}
	default:
		v.Add("action", "Azione sconosciuta.")
	}
	if len(in.ProposalIDs) == 0 && in.Status == "" {
		v.Add("proposal_ids", "Indica le offerte di rinnovo o lo stato.")
	}
	if len(in.ProposalIDs) > maxPassIDs {
		v.Add("proposal_ids", fmt.Sprintf("Massimo %d elementi per richiesta.", maxPassIDs))
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	c, err := store.GetRenewalCampaign(ctx, s.d.DB, es, campaignID)
	if err != nil {
		return nil, notFound(err)
	}
	if in.Action == ActionExtendDeadline {
		target, err := store.GetSeason(ctx, s.d.DB, es, c.TargetSeasonID)
		if err != nil {
			return nil, err
		}
		est, err := store.GetEstablishment(ctx, s.d.DB, es)
		if err != nil {
			return nil, err
		}
		d := in.DeadlineAt.UTC()
		in.DeadlineAt = &d
		if err := ValidateDeadline("deadline_at", d, s.now(), midnight(target.OpensOn, location(est))); err != nil {
			return nil, err
		}
	}
	ids, err := store.ListRenewalProposalIDs(ctx, s.d.DB, es, campaignID, store.RenewalProposalFilter{IDs: in.ProposalIDs, Status: in.Status})
	if err != nil {
		return nil, err
	}
	actor := p.Actor(in.IP)
	res := &BulkResult{Action: in.Action, Matched: len(ids), Failed: []BulkFailure{}}
	for _, id := range ids {
		_, err := s.change(ctx, es, id, nil, func(ctx context.Context, tx bun.Tx, prop *store.RenewalProposal, now time.Time) error {
			return s.bulkOne(ctx, tx, actor, es, prop, in, now)
		})
		switch {
		case err == nil:
			res.Succeeded++
		case errors.Is(err, errSkipped):
			res.Skipped++
		default:
			res.Failed = append(res.Failed, BulkFailure{ProposalID: id, Err: err})
		}
	}
	return res, nil
}

func (s *Service) bulkOne(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, in BulkInput, now time.Time) error {
	switch in.Action {
	case ActionSend:
		if prop.Status != store.RenewalProposed || (in.OnlyUnsent && prop.SendCount > 0) {
			return errSkipped
		}
		return s.sendLocked(ctx, tx, actor, es, prop, now)
	case ActionAccept:
		if !Open(prop.Status) {
			return errSkipped
		}
		return s.acceptLocked(ctx, tx, actor, es, prop, AcceptInput{Confirm: in.Confirm}, now)
	case ActionDecline:
		if !Open(prop.Status) {
			return errSkipped
		}
		return s.declineLocked(ctx, tx, actor, es, prop, "", store.RenewalResponseDesk, now)
	case ActionExtendDeadline:
		if prop.Status != store.RenewalProposed || !prop.DeadlineAt.After(now) {
			return errSkipped
		}
		return s.extend(ctx, tx, actor, es, prop, *in.DeadlineAt, now)
	case ActionRelease:
		if prop.Status != store.RenewalProposed || (prop.DeadlineAt.After(now) && !in.Force) {
			return errSkipped
		}
		why := "deadline_passed"
		if prop.DeadlineAt.After(now) {
			why = "forced_release"
		}
		return s.expire(ctx, tx, actor, es, prop, why, now)
	}
	return errSkipped
}
