package renewal

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// JobSync is the name of the proposal/booking synchronisation job.
const JobSync = "renewals.sync"

// Deps are the dependencies of the service.
type Deps struct {
	DB       bun.IDB
	Clock    clock.Clock
	Audit    *audit.Writer
	Outbox   *event.Outbox
	Log      *slog.Logger
	Bookings Bookings
	Checkout Checkout
	Notify   Notifier
}

// Service implements the renewal use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil && d.Clock != nil {
		d.Outbox = event.NewOutbox(d.Clock)
	}
	return &Service{d: d}
}

// Counts are the proposals per status of a campaign.
type Counts struct {
	Proposed, Accepted, Declined, Expired, Converted, Sent int
}

// Campaign is api.RenewalCampaign.
type Campaign struct {
	store.RenewalCampaign
	Counts          Counts
	TotalAmount     money.Cents
	ConvertedAmount money.Cents
}

// Proposal is api.RenewalProposal (row + display fields).
type Proposal struct {
	store.RenewalProposal
	BookingCode         string
	BookingStatus       string
	CustomerDisplayName string
	CustomerHasEmail    bool
	SpotLabel           string
	ZoneID              string
	StartsOn, EndsOn    string // YYYY-MM-DD of the target booking
	Slots               string
	PaidAmount          money.Cents
}

// CreateInput is api.RenewalCampaignCreate.
type CreateInput struct {
	Name           string
	SourceSeasonID string
	TargetSeasonID string
	DeadlineAt     time.Time
	PassIDs        []string
	Notes          string
	DryRun         bool
	IP             string
}

// Skipped is a pass that cannot be proposed (api.RenewalSkipped).
type Skipped struct {
	SourcePassID        string
	CustomerDisplayName string
	SpotLabel           string
	Reason              string // api.RenewalSkipReason
	Detail              string
}

// CreateResult is api.RenewalCampaignResult.
type CreateResult struct {
	DryRun    bool
	Campaign  *Campaign
	Proposals []Proposal
	Skipped   []Skipped
}

// UpdateInput is api.RenewalCampaignUpdate (nil = unchanged).
type UpdateInput struct {
	Name       *string
	Notes      *string
	DeadlineAt *time.Time
	Close      bool
	IP         string
}

// ProposalFilter filters proposals of a campaign.
type ProposalFilter struct {
	Status string
	Sent   *bool
	ZoneID string
	Query  string
}

// BulkInput is api.RenewalBulkRequest.
type BulkInput struct {
	Action      string // send | accept | decline | extend_deadline | release
	ProposalIDs []string
	Status      string
	OnlyUnsent  bool
	DeadlineAt  *time.Time
	Confirm     bool
	Force       bool
	IP          string
}

// BulkFailure is the error of one proposal in a bulk action.
type BulkFailure struct {
	ProposalID string
	Err        error // mapped to a ProblemType by the handler
}

// BulkResult is api.RenewalBulkResult.
type BulkResult struct {
	Action                      string
	Matched, Succeeded, Skipped int
	Failed                      []BulkFailure
}

// AcceptInput is api.RenewalStaffAcceptRequest.
type AcceptInput struct {
	Confirm bool
	Note    string
	IP      string
}

// PublicProposal is api.PublicRenewalProposal.
type PublicProposal struct {
	Proposal        Proposal
	SeasonYear      int
	ZoneName        string
	PackName        string
	CreditAvailable money.Cents
	TermsVersion    string
	CanAccept       bool
	CanDecline      bool
	CanPayDeposit   bool
	CanPayFull      bool
	CanUseCredit    bool
}

// PublicAcceptInput is api.PublicRenewalAcceptRequest.
type PublicAcceptInput struct {
	Payment            string // deposit | full
	UseCredit          bool
	AcceptTermsVersion string
	IdempotencyKey     string
	IP                 string
}

// PublicAcceptResult is api.PublicRenewalAcceptResult.
type PublicAcceptResult struct {
	Proposal      PublicProposal
	CreditApplied money.Cents
	Pay           checkout.PayOutcome
}

// ---- panel ------------------------------------------------------------------

// ListCampaigns lists campaigns, newest first (bookings.read).
func (s *Service) ListCampaigns(ctx context.Context, p *auth.Principal, establishmentID, targetSeasonID, status, cursor string, limit int) (store.Page[Campaign], error) {
	return store.Page[Campaign]{}, configerr.ErrNotImplemented
}

// CreateCampaign previews or creates a campaign and its proposals
// (bookings.write). Idempotent per (source, target): ErrCampaignExists.
func (s *Service) CreateCampaign(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*CreateResult, error) {
	return nil, configerr.ErrNotImplemented
}

// GetCampaign returns a campaign with counts (bookings.read).
func (s *Service) GetCampaign(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Campaign, error) {
	return nil, configerr.ErrNotImplemented
}

// UpdateCampaign renames, extends or closes a campaign (bookings.write,
// optimistic concurrency on version).
func (s *Service) UpdateCampaign(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in UpdateInput) (*Campaign, error) {
	return nil, configerr.ErrNotImplemented
}

// ListProposals lists the proposals of a campaign (bookings.read).
func (s *Service) ListProposals(ctx context.Context, p *auth.Principal, establishmentID, campaignID string, f ProposalFilter, cursor string, limit int) (store.Page[Proposal], error) {
	return store.Page[Proposal]{}, configerr.ErrNotImplemented
}

// Bulk applies an action to many proposals, one transaction each.
func (s *Service) Bulk(ctx context.Context, p *auth.Principal, establishmentID, campaignID string, in BulkInput) (*BulkResult, error) {
	return nil, configerr.ErrNotImplemented
}

// GetProposal returns one proposal (bookings.read).
func (s *Service) GetProposal(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Proposal, error) {
	return nil, configerr.ErrNotImplemented
}

// Accept accepts on behalf of the customer (bookings.write).
func (s *Service) Accept(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in AcceptInput) (*Proposal, error) {
	return nil, configerr.ErrNotImplemented
}

// Decline records a decline and releases the spot (bookings.write).
func (s *Service) Decline(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, reason, ip string) (*Proposal, error) {
	return nil, configerr.ErrNotImplemented
}

// Send emails (or re-emails) the proposal with a fresh magic link.
func (s *Service) Send(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) (*Proposal, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- customer (magic link) ----------------------------------------------------

// GetByToken returns the proposal of the magic link (ErrNotRenewal if the
// booking is not a proposal).
func (s *Service) GetByToken(ctx context.Context, slug, token string) (*PublicProposal, error) {
	return nil, configerr.ErrNotImplemented
}

// AcceptByToken accepts and starts the payment (ErrExpired after deadline).
func (s *Service) AcceptByToken(ctx context.Context, slug, token string, in PublicAcceptInput) (*PublicAcceptResult, error) {
	return nil, configerr.ErrNotImplemented
}

// DeclineByToken declines and releases the spot.
func (s *Service) DeclineByToken(ctx context.Context, slug, token, reason string) (*PublicProposal, error) {
	return nil, configerr.ErrNotImplemented
}

// ---- jobs ----------------------------------------------------------------------

// Sync aligns proposals with their booking (converted / expired) and
// returns how many proposals changed.
func (s *Service) Sync(ctx context.Context) (int, error) {
	return 0, configerr.ErrNotImplemented
}

// Jobs returns the renewal jobs. Registered by internal/app once implemented.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobSync, Interval: time.Minute, RunOnStart: true, Timeout: 2 * time.Minute, Run: func(ctx context.Context) error {
		_, err := s.Sync(ctx)
		return err
	}}}
}
