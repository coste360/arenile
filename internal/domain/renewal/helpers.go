package renewal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	return p.Authorize(ctx, s.d.DB, establishmentID, perm)
}

// location is the timezone of the establishment (UTC when unknown).
func location(est *store.Establishment) *time.Location {
	if est != nil {
		if loc, err := time.LoadLocation(est.Timezone); err == nil {
			return loc
		}
	}
	return time.UTC
}

// midnight is the first instant of d in loc.
func midnight(d calendar.Date, loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).UTC()
}

// toProposal maps a store row to the domain view.
func toProposal(r store.RenewalProposalRow) Proposal {
	return Proposal{
		RenewalProposal: r.Proposal, BookingCode: r.BookingCode, BookingStatus: r.BookingStatus,
		CustomerDisplayName: r.CustomerDisplayName, CustomerHasEmail: r.CustomerHasEmail, SpotLabel: r.SpotLabel,
		ZoneID: r.ZoneID, StartsOn: r.StartsOn.String(), EndsOn: r.EndsOn.String(), Slots: string(r.Slots), PaidAmount: r.PaidAmount,
	}
}

// proposalView loads the display fields of one proposal.
func (s *Service) proposalView(ctx context.Context, db bun.IDB, es store.EstablishmentScope, p *store.RenewalProposal) (*Proposal, error) {
	rows, err := store.RenewalProposalRows(ctx, db, es, []store.RenewalProposal{*p})
	if err != nil {
		return nil, err
	}
	out := toProposal(rows[0])
	return &out, nil
}

// campaignViews adds the counts and amounts to campaigns.
func (s *Service) campaignViews(ctx context.Context, db bun.IDB, es store.EstablishmentScope, cs []store.RenewalCampaign) ([]Campaign, error) {
	ids := make([]string, len(cs))
	for i := range cs {
		ids[i] = cs[i].ID
	}
	stats, err := store.RenewalCampaignStatsOf(ctx, db, es, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Campaign, len(cs))
	for i := range cs {
		st := stats[cs[i].ID]
		out[i] = Campaign{RenewalCampaign: cs[i]}
		if st == nil {
			continue
		}
		out[i].Counts = Counts{
			Proposed: st.ByStatus[store.RenewalProposed], Accepted: st.ByStatus[store.RenewalAccepted],
			Declined: st.ByStatus[store.RenewalDeclined], Expired: st.ByStatus[store.RenewalExpired],
			Converted: st.ByStatus[store.RenewalConverted], Sent: st.Sent,
		}
		out[i].TotalAmount, out[i].ConvertedAmount = st.TotalAmount, st.ConvertedAmount
	}
	return out, nil
}

func (s *Service) campaignView(ctx context.Context, db bun.IDB, es store.EstablishmentScope, c *store.RenewalCampaign) (*Campaign, error) {
	vs, err := s.campaignViews(ctx, db, es, []store.RenewalCampaign{*c})
	if err != nil {
		return nil, err
	}
	return &vs[0], nil
}

// saveProposal writes p with a conditional update on version.
func saveProposal(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, p *store.RenewalProposal, version int64, now time.Time) error {
	p.UpdatedAt = now
	ok, err := store.UpdateRenewalProposalVersioned(ctx, tx, es, p, version)
	if err != nil {
		return fmt.Errorf("renewal: %w", err)
	}
	if !ok {
		return &configerr.VersionConflictError{Current: version}
	}
	return nil
}

// respond moves p to status with the response instant and channel. The
// first response (acceptance) is kept when the proposal converts later.
func respond(p *store.RenewalProposal, status, channel string, now time.Time) {
	keep := status == store.RenewalConverted && p.RespondedAt != nil
	p.Status = status
	if !keep {
		ch := channel
		p.RespondedAt, p.ResponseChannel = &now, &ch
	}
}

// record writes the audit entry of a proposal change and, when eventType is
// set, its outbox event (ids and channel only, no personal data), in tx.
func (s *Service) record(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, actor audit.Actor, action string, p *store.RenewalProposal, diff map[string]any, eventType, channel string) error {
	if diff == nil {
		diff = map[string]any{}
	}
	diff["status"] = p.Status
	diff["campaign_id"] = p.CampaignID
	diff["booking_id"] = p.BookingID
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "renewal_proposal", EntityID: p.ID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("renewal: audit: %w", err)
	}
	if eventType == "" {
		return nil
	}
	data := map[string]any{
		"proposal_id": p.ID, "campaign_id": p.CampaignID, "booking_id": p.BookingID, "source_pass_id": p.SourceBookingID,
		"customer_id": p.CustomerID, "spot_id": p.SpotID, "status": p.Status, "deadline_at": p.DeadlineAt.Format(time.RFC3339),
	}
	if channel != "" {
		data["channel"] = channel
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, data); err != nil {
		return fmt.Errorf("renewal: outbox: %w", err)
	}
	return nil
}

// sourceRenewable checks that the source pass of prop can still be renewed
// (confirmed, checked in or completed: store.RenewableBookingStatuses). A
// pass cancelled after the campaign was created is ErrSourceCancelled.
func sourceRenewable(ctx context.Context, db bun.IDB, es store.EstablishmentScope, prop *store.RenewalProposal) error {
	src, err := store.GetBooking(ctx, db, es, prop.SourceBookingID)
	if err != nil {
		return fmt.Errorf("renewal: source pass: %w", err)
	}
	if !slices.Contains(store.RenewableBookingStatuses, src.Status) {
		return ErrSourceCancelled
	}
	return nil
}

// notFound maps a missing row to store.ErrNotFound (404).
func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return store.ErrNotFound
	}
	return err
}
