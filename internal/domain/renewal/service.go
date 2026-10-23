package renewal

import (
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// JobSync is the name of the proposal/booking synchronisation job.
const JobSync = "renewals.sync"

// Limits of the inputs.
const (
	maxName    = 200
	maxNotes   = 2000
	maxReason  = 500
	maxNote    = 1000
	maxPassIDs = 2000
	syncBatch  = 200
)

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
	// Credits and CreditApplier are optional (nil: no credit on the
	// renewal page; see ports.go).
	Credits       Credits
	CreditApplier CreditApplier
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
