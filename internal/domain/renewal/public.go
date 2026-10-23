package renewal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// ---- customer (magic link) ----------------------------------------------------

// linked is a magic link resolved to its proposal.
type linked struct {
	ref  *checkout.ManageRef
	prop *store.RenewalProposal
}

func (s *Service) resolve(ctx context.Context, slug, token string) (*linked, error) {
	if s.d.Checkout == nil {
		return nil, fmt.Errorf("renewal: %w: checkout not configured", configerr.ErrNotImplemented)
	}
	ref, err := s.d.Checkout.ResolveManage(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	prop, err := store.GetRenewalProposalByBooking(ctx, s.d.DB, ref.Scope, ref.BookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotRenewal
	}
	if err != nil {
		return nil, err
	}
	return &linked{ref: ref, prop: prop}, nil
}

// customerActor is the audit actor of the holder of a magic link.
func customerActor(ctx context.Context, linkID string) audit.Actor {
	return audit.Actor{Type: audit.ActorCustomer, ID: linkID, IP: reqctx.From(ctx).ClientIP}
}

// GetByToken returns the proposal of the magic link (ErrNotRenewal if the
// booking is not a proposal) with the actions allowed now.
func (s *Service) GetByToken(ctx context.Context, slug, token string) (*PublicProposal, error) {
	l, err := s.resolve(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	return s.publicView(ctx, s.d.DB, l)
}

func (s *Service) publicView(ctx context.Context, db bun.IDB, l *linked) (*PublicProposal, error) {
	es, est := l.ref.Scope, l.ref.Establishment
	prop, err := s.proposalView(ctx, db, es, l.prop)
	if err != nil {
		return nil, err
	}
	b, err := store.GetBooking(ctx, db, es, l.prop.BookingID)
	if err != nil {
		return nil, err
	}
	out := &PublicProposal{Proposal: *prop, TermsVersion: l.ref.TermsVersion}
	if b.SeasonID != nil {
		if sn, err := store.GetSeason(ctx, db, es, *b.SeasonID); err == nil {
			out.SeasonYear = sn.Year
		}
	}
	if out.SeasonYear == 0 {
		out.SeasonYear = b.StartsOn.Year()
	}
	locale := est.DefaultLocale
	if cu, err := store.GetCustomer(ctx, db, es, l.prop.CustomerID); err == nil && cu.Locale != "" {
		locale = cu.Locale
	}
	if pk, err := store.GetPack(ctx, db, es, l.prop.PackID); err == nil {
		out.PackName = text(pk.Name, locale, est.DefaultLocale)
	}
	if prop.ZoneID != "" {
		if z, err := store.GetZone(ctx, db, es, prop.ZoneID); err == nil {
			out.ZoneName = text(z.Name, locale, est.DefaultLocale)
		}
	}
	now := s.now()
	if s.d.Credits != nil {
		today := calendar.In(now, location(est))
		if c, err := s.d.Credits.CreditBalance(ctx, db, es, l.prop.CustomerID, today); err == nil {
			out.CreditAvailable = max(c, 0)
		} else {
			s.d.Log.WarnContext(ctx, "renewal: credit balance unavailable", slog.String("proposal_id", l.prop.ID), slog.String("error", err.Error()))
		}
	}
	a := PublicActions(ActionsInput{
		Status: l.prop.Status, BookingStatus: b.Status, Deadline: l.prop.DeadlineAt, HoldExpiresAt: b.HoldExpiresAt, Now: now,
		Total: b.TotalAmount, Paid: b.PaidAmount, Deposit: b.DepositDue, Online: l.ref.Online,
		Credit: out.CreditAvailable, CanApply: s.d.CreditApplier != nil,
	})
	out.CanAccept, out.CanDecline, out.CanPayDeposit, out.CanPayFull, out.CanUseCredit = a.Accept, a.Decline, a.PayDeposit, a.PayFull, a.UseCredit
	return out, nil
}

// text picks the locale of an i18n text, then the fallback, then any.
func text(t store.I18nText, locale, fallback string) string {
	if v := t[locale]; v != "" {
		return v
	}
	if v := t[fallback]; v != "" {
		return v
	}
	for _, l := range []string{"it", "en", "de", "fr"} {
		if v := t[l]; v != "" {
			return v
		}
	}
	return ""
}

// AcceptByToken accepts the proposal of a magic link and starts the
// payment (spec 10.4): the terms version is recorded on the booking, the
// proposal becomes accepted (season.renewal_accepted), the customer credit
// is applied first when asked, then the online payment of the deposit or of
// the balance starts through the M4 gateway. The booking is confirmed only
// by the provider webhook (or the manual driver); without an online
// provider, or with nothing left to pay online, there is no online step
// (pay.mode none) and the desk collects. ErrExpired after the deadline.
func (s *Service) AcceptByToken(ctx context.Context, slug, token string, in PublicAcceptInput) (*PublicAcceptResult, error) {
	switch in.Payment {
	case PaymentDeposit, PaymentFull:
	default:
		return nil, configerr.Invalid("payment", "Usa deposit o full.")
	}
	in.AcceptTermsVersion = strings.TrimSpace(in.AcceptTermsVersion)
	if utf8.RuneCountInString(in.AcceptTermsVersion) > 40 {
		return nil, configerr.Invalid("accept_terms_version", "Versione non valida.")
	}
	l, err := s.resolve(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	if l.ref.TermsVersion != "" && in.AcceptTermsVersion != l.ref.TermsVersion {
		return nil, checkout.ErrTermsOutdated
	}
	es := l.ref.Scope
	actor := customerActor(ctx, l.ref.LinkID)
	view, err := s.publicView(ctx, s.d.DB, l)
	if err != nil {
		return nil, err
	}
	if err := checkOpen(l.prop, s.now()); err != nil {
		return nil, err
	}
	if !view.CanAccept {
		return nil, ErrExpired
	}
	if in.Payment == PaymentDeposit && !view.CanPayDeposit && l.ref.Online {
		return nil, configerr.Invalid("payment", "L'acconto non è previsto per questo rinnovo: scegli il pagamento intero.")
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		prop, err := store.LockRenewalProposal(ctx, tx, es, l.prop.ID)
		if err != nil {
			return err
		}
		now := s.now()
		if err := checkOpen(prop, now); err != nil {
			return err
		}
		if err := sourceRenewable(ctx, tx, es, prop); err != nil {
			return err
		}
		if err := s.d.Bookings.AcceptRenewalTerms(ctx, tx, actor, es, prop.BookingID, in.AcceptTermsVersion); err != nil {
			return err
		}
		if prop.Status == store.RenewalProposed {
			version := prop.Version
			respond(prop, store.RenewalAccepted, store.RenewalResponseOnline, now)
			if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
				return err
			}
			if err := s.record(ctx, tx, es, actor, "renewal_proposal.accepted", prop, map[string]any{
				"channel": store.RenewalResponseOnline, "payment": in.Payment, "use_credit": in.UseCredit, "terms_version": in.AcceptTermsVersion,
			}, event.SeasonRenewalAccepted, store.RenewalResponseOnline); err != nil {
				return err
			}
		}
		l.prop = prop
		return nil
	})
	if err != nil {
		return nil, err
	}
	res := &PublicAcceptResult{}
	if in.UseCredit && view.CanUseCredit {
		key := in.IdempotencyKey
		if key != "" {
			key = "renewal-credit:" + key
		}
		applied, err := s.d.CreditApplier.ApplyCreditToBooking(ctx, es, l.prop.BookingID, min(view.CreditAvailable, view.Proposal.TotalAmount-view.Proposal.PaidAmount), key)
		if err != nil {
			return nil, err
		}
		res.CreditApplied = applied
	}
	b, err := store.GetBooking(ctx, s.d.DB, es, l.prop.BookingID)
	if err != nil {
		return nil, err
	}
	due := AmountDue(in.Payment, b.TotalAmount, b.PaidAmount, b.DepositDue)
	switch {
	case b.Status != "held" && b.Status != "pending_payment":
		// Confirmed by the credit payment (manual driver) or closed meanwhile.
		res.Pay = checkout.PayOutcome{Mode: checkout.PayNone, Status: b.Status}
	case due <= 0 && b.TotalAmount <= b.PaidAmount:
		// Nothing to collect (free renewal or fully paid): the manual
		// driver confirms, as the desk would.
		if err := s.confirmFree(ctx, es, actor, l.prop.ID); err != nil {
			return nil, err
		}
		b, err = store.GetBooking(ctx, s.d.DB, es, l.prop.BookingID)
		if err != nil {
			return nil, err
		}
		res.Pay = checkout.PayOutcome{Mode: checkout.PayNone, Status: b.Status}
	case due <= 0 || !l.ref.Online:
		res.Pay = checkout.PayOutcome{Mode: checkout.PayNone, Status: b.Status, HoldExpiresAt: b.HoldExpiresAt}
	default:
		pay, err := s.d.Checkout.PayManagedAmount(ctx, slug, token, due, in.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		res.Pay = *pay
	}
	if l.prop, err = store.GetRenewalProposal(ctx, s.d.DB, es, l.prop.ID); err != nil {
		return nil, err
	}
	view, err = s.publicView(ctx, s.d.DB, l)
	if err != nil {
		return nil, err
	}
	res.Proposal = *view
	return res, nil
}

// confirmFree confirms the booking of an accepted proposal with nothing to
// pay and converts the proposal, in one transaction.
func (s *Service) confirmFree(ctx context.Context, es store.EstablishmentScope, actor audit.Actor, id string) error {
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		prop, err := store.LockRenewalProposal(ctx, tx, es, id)
		if err != nil {
			return err
		}
		if !Open(prop.Status) {
			return nil
		}
		now := s.now()
		if _, err := s.d.Bookings.ConfirmRenewal(ctx, tx, actor, es, prop.BookingID, ""); err != nil {
			return err
		}
		version := prop.Version
		respond(prop, store.RenewalConverted, store.RenewalResponseOnline, now)
		if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
			return err
		}
		return s.record(ctx, tx, es, actor, "renewal_proposal.converted", prop, map[string]any{"reason": "nothing_to_pay"}, "", "")
	})
}

// DeclineByToken declines and releases the spot.
func (s *Service) DeclineByToken(ctx context.Context, slug, token, reason string) (*PublicProposal, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReason {
		return nil, configerr.Invalid("reason", fmt.Sprintf("Massimo %d caratteri.", maxReason))
	}
	l, err := s.resolve(ctx, slug, token)
	if err != nil {
		return nil, err
	}
	es := l.ref.Scope
	actor := customerActor(ctx, l.ref.LinkID)
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		prop, err := store.LockRenewalProposal(ctx, tx, es, l.prop.ID)
		if err != nil {
			return err
		}
		now := s.now()
		if err := checkOpen(prop, now); err != nil {
			return err
		}
		if err := s.declineLocked(ctx, tx, actor, es, prop, reason, store.RenewalResponseOnline, now); err != nil {
			return err
		}
		l.prop = prop
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.publicView(ctx, s.d.DB, l)
}

// ---- jobs ----------------------------------------------------------------------

// Sync aligns open proposals with their booking and the deadline, one
// transaction each, and returns how many proposals changed:
//   - booking confirmed (payment webhook, manual driver, credit) ->
//     converted;
//   - booking cancelled, expired or rejected (hold elapsed, staff action)
//     -> expired (season.renewal_expired);
//   - source pass cancelled after the campaign (nothing to renew) -> the
//     booking expires through the booking service and the proposal expires
//     (reason source_pass_cancelled), also before the deadline (INV-68);
//   - deadline passed with the hold elapsed -> the booking expires through
//     the booking service (spot released for general sale) and the
//     proposal expires. A payment hold still running past the deadline is
//     left to finish.
func (s *Service) Sync(ctx context.Context) (int, error) {
	now := s.now()
	refs, err := store.RenewalProposalsToSync(ctx, s.d.DB, now, syncBatch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ref := range refs {
		es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ref.EstablishmentID)
		if err != nil {
			continue
		}
		changed := false
		err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
			var err error
			changed, err = s.syncOne(ctx, tx, es, ref.ID, s.now())
			return err
		})
		if err != nil {
			s.d.Log.ErrorContext(ctx, "renewal sync failed", slog.String("proposal_id", ref.ID), slog.String("error", err.Error()))
			continue
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func (s *Service) syncOne(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, id string, now time.Time) (bool, error) {
	prop, err := store.LockRenewalProposal(ctx, tx, es, id)
	if err != nil {
		return false, err
	}
	if !Open(prop.Status) {
		return false, nil
	}
	b, err := store.LockBooking(ctx, tx, es, prop.BookingID)
	if err != nil {
		return false, err
	}
	actor := audit.SystemActor
	version := prop.Version
	switch b.Status {
	case "confirmed", "checked_in", "completed":
		respond(prop, store.RenewalConverted, store.RenewalResponseSystem, now)
		if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
			return false, err
		}
		return true, s.record(ctx, tx, es, actor, "renewal_proposal.converted", prop, map[string]any{"booking_status": b.Status}, "", "")
	case "held", "pending_payment":
		// The source pass is locked too: a cancellation of the pass and this
		// sync serialise, so a revoked pass never keeps its pre-emption.
		if _, err := store.LockBooking(ctx, tx, es, prop.SourceBookingID); err != nil {
			return false, err
		}
		if err := sourceRenewable(ctx, tx, es, prop); errors.Is(err, ErrSourceCancelled) {
			return true, s.expire(ctx, tx, actor, es, prop, "source_pass_cancelled", now)
		} else if err != nil {
			return false, err
		}
		if prop.DeadlineAt.After(now) || (b.HoldExpiresAt != nil && b.HoldExpiresAt.After(now)) {
			return false, nil
		}
		return true, s.expire(ctx, tx, actor, es, prop, "deadline_passed", now)
	default:
		respond(prop, store.RenewalExpired, store.RenewalResponseSystem, now)
		if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
			return false, err
		}
		return true, s.record(ctx, tx, es, actor, "renewal_proposal.expired", prop, map[string]any{"booking_status": b.Status},
			event.SeasonRenewalExpired, store.RenewalResponseSystem)
	}
}

// Jobs returns the renewal jobs. Registered by internal/app.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobSync, Interval: time.Minute, RunOnStart: true, Timeout: 2 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.Sync(ctx)
		return err
	}}}
}
