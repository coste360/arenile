package findspot

import (
	"context"
	"log/slog"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// Occupancy loads the candidate spots and their free half-days for a
// search (implemented by internal/domain/inventory).
type Occupancy interface {
	FindSpotCandidates(ctx context.Context, establishmentID string, q Query) ([]availability.Spot, *availability.FreeSpots, error)
}

// Quoter prices a proposal as a booking would be priced (implemented on top
// of the booking pricer).
type Quoter interface {
	QuoteProposal(ctx context.Context, establishmentID string, q Query, p availability.Proposal) (money.Cents, error)
}

// Booker creates the booking of a proposal (implemented by
// *booking.Service: Create with BookingLineInput.segments).
type Booker interface {
	Create(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Detail, error)
}

// Deps are the dependencies of Service.
type Deps struct {
	Clock     clock.Clock
	Occupancy Occupancy
	Quoter    Quoter
	Booker    Booker
	Log       *slog.Logger
}

// Service implements the find-spot search and booking.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// Query is api.FindSpotRequest.
type Query struct {
	StartsOn       calendar.Date
	EndsOn         calendar.Date
	Slots          calendar.Slots
	Formula        *string
	Count          int
	ZoneIDs        []string
	PackID         *string
	SpotKinds      []string
	Attributes     []string
	MaxSpotChanges int
	AgreementID    *string
	CustomerID     *string
	Limit          int
}

// Proposal is a priced availability.Proposal.
type Proposal struct {
	ID               string
	Rank             int
	Proposal         availability.Proposal
	Total            *money.Cents
	AgreementApplied bool
}

// Result is api.FindSpotResult.
type Result struct {
	Proposals []Proposal
	Truncated bool
}

// Search returns the ranked proposals. Requires auth.PermBookingsRead.
func (s *Service) Search(ctx context.Context, p *auth.Principal, establishmentID string, q Query) (*Result, error) {
	_, _, _, _ = ctx, p, establishmentID, q
	return nil, configerr.ErrNotImplemented
}

// Book creates the booking of a proposal (in.Lines carry the segments) and
// refuses it with price_changed when the total differs from expectedTotal.
// Requires auth.PermBookingsWrite.
func (s *Service) Book(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput, expectedTotal *money.Cents) (*booking.Detail, error) {
	_, _, _, _, _ = ctx, p, establishmentID, in, expectedTotal
	return nil, configerr.ErrNotImplemented
}
