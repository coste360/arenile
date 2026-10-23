package renewal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// ---- campaigns ----------------------------------------------------------------

// ListCampaigns lists campaigns, newest first (bookings.read).
func (s *Service) ListCampaigns(ctx context.Context, p *auth.Principal, establishmentID, targetSeasonID, status, cursor string, limit int) (store.Page[Campaign], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[Campaign]{}, err
	}
	page, err := store.ListRenewalCampaigns(ctx, s.d.DB, es, store.RenewalCampaignFilter{TargetSeasonID: targetSeasonID, Status: status}, cursor, limit)
	if err != nil {
		return store.Page[Campaign]{}, err
	}
	items, err := s.campaignViews(ctx, s.d.DB, es, page.Items)
	if err != nil {
		return store.Page[Campaign]{}, err
	}
	return store.Page[Campaign]{Items: items, NextCursor: page.NextCursor}, nil
}

// GetCampaign returns a campaign with counts (bookings.read).
func (s *Service) GetCampaign(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Campaign, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	c, err := store.GetRenewalCampaign(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.campaignView(ctx, s.d.DB, es, c)
}

// candidate is a source pass with what the proposal needs.
type candidate struct {
	pass  store.SeasonPassView
	input booking.CreateInput
}

// CreateCampaign previews (in.DryRun) or creates a campaign and its
// proposals (bookings.write, spec 10.4). For every renewable pass of the
// source season (or in.PassIDs) a held season pass booking of the target
// season is created through the booking service, same spot, pack, slots
// and extras, priced by the target season, held until the deadline: the
// spot stays reserved by the allocation constraints. Passes that cannot be
// proposed are reported in Skipped. One campaign per (source, target):
// ErrCampaignExists. Everything is one transaction (one savepoint per
// proposal, so a skipped pass leaves no trace); the preview writes nothing.
func (s *Service) CreateCampaign(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*CreateResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	src, target, err := s.validateCreate(ctx, es, &in)
	if err != nil {
		return nil, err
	}
	switch _, err := store.FindRenewalCampaign(ctx, s.d.DB, es, src.ID, target.ID); {
	case err == nil:
		return nil, ErrCampaignExists
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	cands, skipped, err := s.candidates(ctx, s.d.DB, es, src, target, in)
	if err != nil {
		return nil, err
	}
	now := s.now()
	res := &CreateResult{DryRun: in.DryRun, Proposals: []Proposal{}, Skipped: skipped}
	if in.DryRun {
		for _, c := range cands {
			q, err := s.d.Bookings.Quote(ctx, p, establishmentID, c.input)
			if err == nil && len(q.Conflicts) > 0 {
				err = fmt.Errorf("%w: spot taken", errSpotTaken)
			}
			if err != nil {
				if reason, ok := skipReason(err); ok {
					res.Skipped = append(res.Skipped, skip(c.pass, reason, err))
					continue
				}
				return nil, err
			}
			res.Proposals = append(res.Proposals, previewOf(c, q.Result.Breakdown.Total, q.Result.DepositDue, in.DeadlineAt))
		}
		res.Campaign = &Campaign{RenewalCampaign: store.RenewalCampaign{
			Name: in.Name, Status: store.RenewalCampaignOpen, SourceSeasonID: src.ID, TargetSeasonID: target.ID,
			DeadlineAt: in.DeadlineAt, Notes: in.Notes, Version: 1, CreatedAt: now, UpdatedAt: now,
		}}
		for _, pr := range res.Proposals {
			res.Campaign.TotalAmount += pr.TotalAmount
		}
		res.Campaign.Counts.Proposed = len(res.Proposals)
		return res, nil
	}

	actor := p.Actor(in.IP)
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		c := &store.RenewalCampaign{
			ID: store.NewID(), Name: in.Name, Status: store.RenewalCampaignOpen, SourceSeasonID: src.ID, TargetSeasonID: target.ID,
			DeadlineAt: in.DeadlineAt, Notes: in.Notes, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if p.Kind == auth.PrincipalUser && p.UserID != "" {
			u := p.UserID
			c.CreatedByUserID = &u
		}
		if err := store.InsertRenewalCampaign(ctx, tx, es, c); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrCampaignExists
			}
			return err
		}
		for _, cand := range cands {
			prop, err := s.propose(ctx, tx, p, es, c, cand, now)
			if err != nil {
				if reason, ok := skipReason(err); ok {
					res.Skipped = append(res.Skipped, skip(cand.pass, reason, err))
					continue
				}
				return err
			}
			res.Proposals = append(res.Proposals, *prop)
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: actor, Action: "renewal_campaign.created", EntityType: "renewal_campaign", EntityID: c.ID,
			Diff: map[string]any{
				"source_season_id": src.ID, "target_season_id": target.ID, "deadline_at": c.DeadlineAt.Format(time.RFC3339),
				"proposals": len(res.Proposals), "skipped": len(res.Skipped),
			},
		}); err != nil {
			return fmt.Errorf("renewal: audit: %w", err)
		}
		cv, err := s.campaignView(ctx, tx, es, c)
		if err != nil {
			return err
		}
		res.Campaign = cv
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.d.Log.InfoContext(ctx, "renewal campaign created", slog.String("establishment_id", es.ID()),
		slog.String("campaign_id", res.Campaign.ID), slog.Int("proposals", len(res.Proposals)), slog.Int("skipped", len(res.Skipped)))
	return res, nil
}

// maxCreateHold is the longest hold the booking service accepts at
// creation (booking.validateCreate).
const maxCreateHold = 24 * time.Hour

var (
	// errSpotTaken marks a preview whose allocation check found conflicts.
	errSpotTaken = errors.New("renewal: spot taken")
	// errAlreadyRenewed: the pass got an open proposal meanwhile.
	errAlreadyRenewed = errors.New("renewal: pass already proposed")
)

func skipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, errSpotTaken):
		return SkipSpotUnavailable, true
	case errors.Is(err, errAlreadyRenewed):
		return SkipAlreadyRenewed, true
	}
	return SkipReasonOf(err)
}

func skip(v store.SeasonPassView, reason string, err error) Skipped {
	sk := Skipped{SourcePassID: v.Booking.ID, CustomerDisplayName: v.CustomerDisplayName, SpotLabel: v.SpotLabel, Reason: reason}
	if err != nil {
		sk.Detail = err.Error()
	}
	return sk
}

// previewOf is the proposal a candidate would become.
func previewOf(c candidate, total, deposit money.Cents, deadline time.Time) Proposal {
	return Proposal{
		RenewalProposal: store.RenewalProposal{
			SourceBookingID: c.pass.Booking.ID, CustomerID: c.pass.Booking.CustomerID, SpotID: c.pass.SpotID, PackID: c.pass.PackID,
			Status: store.RenewalProposed, PreviousAmount: c.pass.Booking.TotalAmount, TotalAmount: total, DepositDue: deposit,
			DeadlineAt: deadline,
		},
		CustomerDisplayName: c.pass.CustomerDisplayName, SpotLabel: c.pass.SpotLabel,
		StartsOn: c.input.StartsOn.String(), EndsOn: c.input.EndsOn.String(), Slots: string(c.input.Slots),
	}
}

// validateCreate checks the request and loads the two seasons.
func (s *Service) validateCreate(ctx context.Context, es store.EstablishmentScope, in *CreateInput) (*store.Season, *store.Season, error) {
	v := &configerr.ValidationError{}
	in.Name, in.Notes = strings.TrimSpace(in.Name), strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Name) > maxName {
		v.Add("name", fmt.Sprintf("Massimo %d caratteri.", maxName))
	}
	if utf8.RuneCountInString(in.Notes) > maxNotes {
		v.Add("notes", fmt.Sprintf("Massimo %d caratteri.", maxNotes))
	}
	if len(in.PassIDs) > maxPassIDs {
		v.Add("pass_ids", fmt.Sprintf("Massimo %d abbonamenti per richiesta.", maxPassIDs))
	}
	if in.SourceSeasonID == "" {
		v.Add("source_season_id", "Indica la stagione di origine.")
	}
	if in.TargetSeasonID == "" {
		v.Add("target_season_id", "Indica la stagione di destinazione.")
	}
	if in.SourceSeasonID != "" && in.SourceSeasonID == in.TargetSeasonID {
		v.Add("target_season_id", "Deve essere diversa dalla stagione di origine.")
	}
	if err := v.OrNil(); err != nil {
		return nil, nil, err
	}
	src, err := store.GetSeason(ctx, s.d.DB, es, in.SourceSeasonID)
	if errors.Is(err, store.ErrNotFound) {
		v.Add("source_season_id", "Stagione inesistente in questo stabilimento.")
	} else if err != nil {
		return nil, nil, err
	}
	target, err := store.GetSeason(ctx, s.d.DB, es, in.TargetSeasonID)
	if errors.Is(err, store.ErrNotFound) {
		v.Add("target_season_id", "Stagione inesistente in questo stabilimento.")
	} else if err != nil {
		return nil, nil, err
	}
	if err := v.OrNil(); err != nil {
		return nil, nil, err
	}
	if !target.OpensOn.After(src.ClosesOn) {
		return nil, nil, configerr.Invalid("target_season_id", "La stagione di destinazione deve seguire quella di origine.")
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, nil, err
	}
	in.DeadlineAt = in.DeadlineAt.UTC()
	if err := ValidateDeadline("deadline_at", in.DeadlineAt, s.now(), midnight(target.OpensOn, location(est))); err != nil {
		return nil, nil, err
	}
	if in.Name == "" {
		in.Name = "Rinnovi " + strconv.Itoa(target.Year)
	}
	return src, target, nil
}

// candidates classifies the source passes: the proposable ones with their
// booking request, and the skipped ones.
func (s *Service) candidates(ctx context.Context, db bun.IDB, es store.EstablishmentScope, src, target *store.Season, in CreateInput) ([]candidate, []Skipped, error) {
	passes, err := store.RenewalCandidates(ctx, db, es, src.ID, in.PassIDs)
	if err != nil {
		return nil, nil, err
	}
	if len(in.PassIDs) > 0 {
		found := map[string]bool{}
		for _, v := range passes {
			found[v.Booking.ID] = true
		}
		for _, id := range in.PassIDs {
			if !found[id] {
				return nil, nil, configerr.Invalid("pass_ids", "Uno degli abbonamenti indicati non appartiene alla stagione di origine.")
			}
		}
	}
	ids := make([]string, len(passes))
	for i := range passes {
		ids[i] = passes[i].Booking.ID
	}
	open, err := store.OpenRenewalSources(ctx, db, es, ids)
	if err != nil {
		return nil, nil, err
	}
	renewable := map[string]bool{}
	for _, st := range store.RenewableBookingStatuses {
		renewable[st] = true
	}
	// The booking service bounds the hold of a new booking (24 hours, the
	// panel limit): the proposal is created with at most that and its hold
	// is then moved to the deadline in the same transaction (proposeIn).
	ttl := min(in.DeadlineAt.Sub(s.now()), maxCreateHold)
	var out []candidate
	skipped := []Skipped{}
	for _, v := range passes {
		b := v.Booking
		if !renewable[b.Status] {
			skipped = append(skipped, skip(v, SkipNotConfirmed, nil))
			continue
		}
		if open[b.ID] {
			skipped = append(skipped, skip(v, SkipAlreadyRenewed, nil))
			continue
		}
		cu, err := store.GetCustomer(ctx, db, es, b.CustomerID)
		if err != nil {
			return nil, nil, err
		}
		if cu.AnonymizedAt != nil {
			skipped = append(skipped, skip(v, SkipCustomerAnonymized, nil))
			continue
		}
		if v.SpotID == "" {
			skipped = append(skipped, skip(v, SkipSpotUnavailable, errors.New("the pass has no assigned spot")))
			continue
		}
		spot, err := store.PassGetSpot(ctx, db, es, v.SpotID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && !spot.Active) {
			skipped = append(skipped, skip(v, SkipSpotInactive, nil))
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		from, to, ok := PassInterval(*src, *target, b.StartsOn, b.EndsOn)
		if !ok {
			skipped = append(skipped, skip(v, SkipNoPrice, errors.New("the pass period has no match in the target season")))
			continue
		}
		extras, err := store.LineExtrasOf(ctx, db, es, v.LineID)
		if err != nil {
			return nil, nil, err
		}
		line := booking.LineInput{PackID: v.PackID, ZoneID: spot.ZoneID, SpotID: spot.ID, Quantity: 1}
		for _, x := range extras {
			line.Extras = append(line.Extras, booking.ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity})
		}
		out = append(out, candidate{pass: v, input: booking.CreateInput{
			CustomerID: b.CustomerID, Channel: store.ChannelDesk, StartsOn: from, EndsOn: to, Slots: b.Slots, Formula: "season",
			Lines: []booking.LineInput{line}, Status: booking.StatusHeld, HoldTTL: ttl,
			// The renewal is the manager's offer: the booking windows of
			// the target policy (advance limits) do not apply to it; a
			// forced creation records the violations in the audit log.
			Force:         true,
			NotesInternal: "Rinnovo dell'abbonamento " + b.Code, SeasonID: target.ID, IsSeasonPass: true,
			RenewalOfBookingID: b.ID,
		}})
	}
	return out, skipped, nil
}

// propose creates the held booking and the proposal of one candidate in a
// savepoint of tx (rolled back on error so a skipped pass leaves nothing).
func (s *Service) propose(ctx context.Context, tx bun.Tx, p *auth.Principal, es store.EstablishmentScope, c *store.RenewalCampaign, cand candidate, now time.Time) (*Proposal, error) {
	sp, err := tx.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("renewal: savepoint: %w", err)
	}
	prop, err := s.proposeIn(ctx, sp, p, es, c, cand, now)
	if err != nil {
		_ = sp.Rollback()
		return nil, err
	}
	if err := sp.Commit(); err != nil {
		return nil, fmt.Errorf("renewal: release savepoint: %w", err)
	}
	return prop, nil
}

func (s *Service) proposeIn(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, c *store.RenewalCampaign, cand candidate, now time.Time) (*Proposal, error) {
	d, err := s.d.Bookings.CreateInTx(ctx, tx, p, es, cand.input)
	if err != nil {
		return nil, err
	}
	b := d.Booking
	if b.HoldExpiresAt == nil || !b.HoldExpiresAt.Equal(c.DeadlineAt) {
		if err := s.d.Bookings.ExtendHold(ctx, tx, p.Actor(reqctx.From(ctx).ClientIP), es, b.ID, c.DeadlineAt); err != nil {
			return nil, err
		}
	}
	prop := &store.RenewalProposal{
		ID: store.NewID(), CampaignID: c.ID, SourceBookingID: cand.pass.Booking.ID, BookingID: b.ID,
		CustomerID: b.CustomerID, SpotID: cand.pass.SpotID, PackID: cand.pass.PackID, Status: store.RenewalProposed,
		PreviousAmount: max(cand.pass.Booking.TotalAmount, 0), TotalAmount: b.TotalAmount, DepositDue: b.DepositDue,
		DeadlineAt: c.DeadlineAt, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertRenewalProposal(ctx, tx, es, prop); err != nil {
		if errors.Is(err, store.ErrConflict) {
			// Another campaign proposed this pass meanwhile (one open
			// proposal per source pass, unique index).
			return nil, errAlreadyRenewed
		}
		return nil, err
	}
	if err := s.record(ctx, tx, es, p.Actor(reqctx.From(ctx).ClientIP), "renewal_proposal.created", prop, map[string]any{
		"source_pass_id": prop.SourceBookingID, "total_amount": int64(prop.TotalAmount), "previous_amount": int64(prop.PreviousAmount),
		"deposit_due": int64(prop.DepositDue), "deadline_at": prop.DeadlineAt.Format(time.RFC3339),
	}, "", ""); err != nil {
		return nil, err
	}
	out := Proposal{
		RenewalProposal: *prop, BookingCode: b.Code, BookingStatus: b.Status, CustomerDisplayName: cand.pass.CustomerDisplayName,
		SpotLabel: cand.pass.SpotLabel, StartsOn: b.StartsOn.String(), EndsOn: b.EndsOn.String(), Slots: string(b.Slots),
	}
	return &out, nil
}

// UpdateCampaign renames, moves the deadline of or closes a campaign
// (bookings.write, optimistic concurrency on version). A new deadline
// applies to the proposals still proposed with the previous deadline (their
// holds move in the same transaction); closing expires every open proposal
// (spot released, season.renewal_expired).
func (s *Service) UpdateCampaign(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in UpdateInput) (*Campaign, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	v := &configerr.ValidationError{}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || utf8.RuneCountInString(n) > maxName {
			v.Add("name", fmt.Sprintf("Da 1 a %d caratteri.", maxName))
		}
		in.Name = &n
	}
	if in.Notes != nil {
		n := strings.TrimSpace(*in.Notes)
		if utf8.RuneCountInString(n) > maxNotes {
			v.Add("notes", fmt.Sprintf("Massimo %d caratteri.", maxNotes))
		}
		in.Notes = &n
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	actor := p.Actor(in.IP)
	var out *Campaign
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		c, err := store.LockRenewalCampaign(ctx, tx, es, id)
		if err != nil {
			return notFound(err)
		}
		if err := configerr.CheckVersion(expected, c.Version); err != nil {
			return err
		}
		version, now := c.Version, s.now()
		diff := map[string]any{}
		if in.Name != nil && *in.Name != c.Name {
			diff["name"] = map[string]any{"before": c.Name, "after": *in.Name}
			c.Name = *in.Name
		}
		if in.Notes != nil && *in.Notes != c.Notes {
			diff["notes"] = true
			c.Notes = *in.Notes
		}
		if (in.DeadlineAt != nil || in.Close) && c.Status != store.RenewalCampaignOpen {
			return ErrNotOpen
		}
		if in.DeadlineAt != nil && !in.DeadlineAt.Equal(c.DeadlineAt) {
			moved, err := s.moveDeadline(ctx, tx, actor, es, c, in.DeadlineAt.UTC(), now)
			if err != nil {
				return err
			}
			diff["deadline_at"] = map[string]any{"before": c.DeadlineAt.Format(time.RFC3339), "after": in.DeadlineAt.UTC().Format(time.RFC3339)}
			diff["proposals_moved"] = moved
			c.DeadlineAt = in.DeadlineAt.UTC()
		}
		action := "renewal_campaign.updated"
		if in.Close {
			n, err := s.closeOpen(ctx, tx, actor, es, c, now)
			if err != nil {
				return err
			}
			c.Status, c.ClosedAt = store.RenewalCampaignClosed, &now
			diff["proposals_expired"] = n
			action = "renewal_campaign.closed"
		}
		c.UpdatedAt = now
		ok, err := store.UpdateRenewalCampaignVersioned(ctx, tx, es, c, version)
		if err != nil {
			return err
		}
		if !ok {
			return &configerr.VersionConflictError{Current: version}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "renewal_campaign", EntityID: c.ID, Diff: diff,
		}); err != nil {
			return fmt.Errorf("renewal: audit: %w", err)
		}
		out, err = s.campaignView(ctx, tx, es, c)
		return err
	})
	return out, err
}

// moveDeadline validates the new deadline and moves the proposals still
// proposed with the previous one (and still running) with their holds.
func (s *Service) moveDeadline(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, c *store.RenewalCampaign, deadline, now time.Time) (int, error) {
	target, err := store.GetSeason(ctx, tx, es, c.TargetSeasonID)
	if err != nil {
		return 0, err
	}
	est, err := store.GetEstablishment(ctx, tx, es)
	if err != nil {
		return 0, err
	}
	if err := ValidateDeadline("deadline_at", deadline, now, midnight(target.OpensOn, location(est))); err != nil {
		return 0, err
	}
	ids, err := store.ListRenewalProposalIDs(ctx, tx, es, c.ID, store.RenewalProposalFilter{Status: store.RenewalProposed})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		prop, err := store.LockRenewalProposal(ctx, tx, es, id)
		if err != nil {
			return 0, err
		}
		if !prop.DeadlineAt.Equal(c.DeadlineAt) || !prop.DeadlineAt.After(now) {
			continue
		}
		if err := s.extend(ctx, tx, actor, es, prop, deadline, now); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

// extend moves the deadline (and the booking hold) of a proposed proposal.
func (s *Service) extend(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, deadline, now time.Time) error {
	if err := s.d.Bookings.ExtendHold(ctx, tx, actor, es, prop.BookingID, deadline); err != nil {
		return err
	}
	prev := prop.DeadlineAt
	version := prop.Version
	prop.DeadlineAt = deadline
	if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
		return err
	}
	return s.record(ctx, tx, es, actor, "renewal_proposal.deadline_moved", prop, map[string]any{
		"deadline_at": deadline.Format(time.RFC3339), "previous_deadline_at": prev.Format(time.RFC3339),
	}, "", "")
}

// closeOpen expires every open proposal of a campaign (campaign closed).
func (s *Service) closeOpen(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, c *store.RenewalCampaign, now time.Time) (int, error) {
	n := 0
	for _, st := range []string{store.RenewalProposed, store.RenewalAccepted} {
		ids, err := store.ListRenewalProposalIDs(ctx, tx, es, c.ID, store.RenewalProposalFilter{Status: st})
		if err != nil {
			return 0, err
		}
		for _, id := range ids {
			prop, err := store.LockRenewalProposal(ctx, tx, es, id)
			if err != nil {
				return 0, err
			}
			if !Open(prop.Status) {
				continue
			}
			if err := s.expire(ctx, tx, actor, es, prop, "campaign_closed", now); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

// expire releases the booking of an open proposal and marks it expired
// (season.renewal_expired), in tx.
func (s *Service) expire(ctx context.Context, tx bun.IDB, actor audit.Actor, es store.EstablishmentScope, prop *store.RenewalProposal, why string, now time.Time) error {
	if _, err := s.d.Bookings.EndRenewal(ctx, tx, actor, es, prop.BookingID, false, ""); err != nil {
		return err
	}
	version := prop.Version
	respond(prop, store.RenewalExpired, store.RenewalResponseSystem, now)
	if err := saveProposal(ctx, tx, es, prop, version, now); err != nil {
		return err
	}
	return s.record(ctx, tx, es, actor, "renewal_proposal.expired", prop, map[string]any{"reason": why},
		event.SeasonRenewalExpired, store.RenewalResponseSystem)
}
