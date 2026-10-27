package findspot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Limits of a search (api.FindSpotRequest).
const (
	MaxCount        = 20
	MaxSpotChanges  = 3
	DefaultLimit    = 10
	MaxLimit        = 50
	MaxDays         = 92
	MaxAttributes   = 10
	maxAttributeLen = 32
)

// Occupancy loads the candidate spots and their free half-days for a
// search. Implemented by StoreOccupancy (the same snapshot as the
// auto-assignment: active spots, every valid allocation, expired holds
// ignored).
type Occupancy interface {
	FindSpotCandidates(ctx context.Context, es store.EstablishmentScope, q Query) (*Candidates, error)
}

// Candidates are the spots a search may propose with their free half-days.
type Candidates struct {
	Spots []availability.Spot
	Free  *availability.FreeSpots
	// Info describes every candidate spot by id (labels and zones for the
	// proposals).
	Info map[string]SpotInfo
	// PreviousSpotID is the spot of the customer's previous booking
	// (continuity), "" if none.
	PreviousSpotID string
}

// SpotInfo describes a proposed spot.
type SpotInfo struct {
	Label         string
	ZoneID        string
	PriceCategory string
}

// Quoter prices a booking request without writing (implemented by
// *booking.Service: the agreement, the customer tags and, for a split
// stay, the price category of each day are applied as for a booking).
type Quoter interface {
	Quote(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Quote, error)
}

// Booker creates the booking of a proposal (implemented by
// *booking.Service: Create with LineInput.Segments).
type Booker interface {
	Create(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput) (*booking.Detail, error)
}

// Deps are the dependencies of Service. Occupancy defaults to
// StoreOccupancy on DB; without a Quoter the proposals have no price.
type Deps struct {
	DB        bun.IDB
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
	if d.Occupancy == nil && d.DB != nil {
		d.Occupancy = &StoreOccupancy{DB: d.DB, Clock: d.Clock}
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
	// Price is the engine result (breakdown and policy violations); nil
	// without a pack or when the price list has no price for it.
	Price *pricingconfig.SimulateResult
}

// Result is api.FindSpotResult.
type Result struct {
	Proposals []Proposal
	Truncated bool
	// Spots describes the spots of the proposals by id.
	Spots map[string]SpotInfo
}

func (q *Query) validate() error {
	v := &configerr.ValidationError{}
	if q.StartsOn.IsZero() {
		v.Add("starts_on", "Campo obbligatorio.")
	}
	switch {
	case q.EndsOn.IsZero():
		v.Add("ends_on", "Campo obbligatorio.")
	case !q.StartsOn.IsZero() && q.EndsOn.Before(q.StartsOn):
		v.Add("ends_on", "Deve essere successiva o uguale a starts_on.")
	case !q.StartsOn.IsZero() && q.StartsOn.DaysUntil(q.EndsOn) >= MaxDays:
		v.Add("ends_on", fmt.Sprintf("Al massimo %d giorni.", MaxDays))
	}
	if !q.Slots.Valid() {
		v.Add("slots", "Usa AM, PM o FULL.")
	}
	if q.Count == 0 {
		q.Count = 1
	}
	if q.Count < 1 || q.Count > MaxCount {
		v.Add("count", fmt.Sprintf("Tra 1 e %d.", MaxCount))
	}
	if q.MaxSpotChanges < 0 || q.MaxSpotChanges > MaxSpotChanges {
		v.Add("max_spot_changes", fmt.Sprintf("Tra 0 e %d.", MaxSpotChanges))
	}
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		v.Add("limit", fmt.Sprintf("Tra 1 e %d.", MaxLimit))
	}
	if len(q.Attributes) > MaxAttributes {
		v.Add("attributes", fmt.Sprintf("Al massimo %d attributi.", MaxAttributes))
	}
	for i, a := range q.Attributes {
		if a = strings.TrimSpace(a); a == "" || len(a) > maxAttributeLen {
			v.Add(fmt.Sprintf("attributes[%d]", i), "Caratteristica non valida.")
		}
		q.Attributes[i] = a
	}
	return v.OrNil()
}

// Search returns the ranked proposals: the pure availability.FindSpots on
// the occupancy read now, each proposal priced as the booking would be
// (with the agreement; a split stay priced day by day, ADR 0028). Advisory
// only: Book is decided by the database constraints. Requires
// auth.PermBookingsRead.
func (s *Service) Search(ctx context.Context, p *auth.Principal, establishmentID string, q Query) (*Result, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	c, err := s.d.Occupancy.FindSpotCandidates(ctx, es, q)
	if err != nil {
		return nil, err
	}
	in := availability.FindInput{
		Spots: c.Spots, Free: c.Free, Interval: availability.Interval{From: q.StartsOn, To: q.EndsOn, Slots: q.Slots},
		Count: q.Count, MaxSpotChanges: q.MaxSpotChanges, RequiredAttributes: q.Attributes, Limit: q.Limit + 1,
	}
	if id, err := uuid.Parse(c.PreviousSpotID); err == nil {
		in.PreviousSpotID = id
	}
	found := availability.FindSpots(in)
	out := &Result{Proposals: []Proposal{}, Spots: map[string]SpotInfo{}}
	if len(found) > q.Limit {
		found, out.Truncated = found[:q.Limit], true
	}
	for i, fp := range found {
		pr := Proposal{ID: ProposalID(fp), Rank: i + 1, Proposal: fp}
		for _, id := range fp.SpotIDs() {
			out.Spots[id.String()] = c.Info[id.String()]
		}
		if q.PackID != nil && *q.PackID != "" && s.d.Quoter != nil {
			s.price(ctx, p, establishmentID, q, &pr)
		}
		out.Proposals = append(out.Proposals, pr)
	}
	return out, nil
}

// price quotes a proposal; a proposal the price list cannot price keeps a
// nil total (the search never fails because of one proposal).
func (s *Service) price(ctx context.Context, p *auth.Principal, establishmentID string, q Query, pr *Proposal) {
	in := booking.CreateInput{
		StartsOn: q.StartsOn, EndsOn: q.EndsOn, Slots: q.Slots, Formula: formulaOf(q),
		Lines: ProposalLines(pr.Proposal, *q.PackID),
	}
	if q.CustomerID != nil {
		in.CustomerID = *q.CustomerID
	}
	if q.AgreementID != nil {
		in.AgreementID = *q.AgreementID
	}
	res, err := s.d.Quoter.Quote(ctx, p, establishmentID, in)
	if err != nil {
		s.d.Log.DebugContext(ctx, "findspot: proposal not priced", "proposal", pr.ID, "error", err)
		return
	}
	total := res.Result.Breakdown.Total
	pr.Total, pr.Price = &total, res.Result
	for _, d := range res.Result.Breakdown.Discounts {
		if strings.HasPrefix(d.Source, "agreement:") {
			pr.AgreementApplied = true
		}
	}
}

// formulaOf is the requested formula or the one of the dates (day,
// half_day, multi_day).
func formulaOf(q Query) string {
	if q.Formula != nil && *q.Formula != "" {
		return *q.Formula
	}
	switch {
	case !q.StartsOn.Equal(q.EndsOn):
		return "multi_day"
	case q.Slots != calendar.SlotsFull:
		return "half_day"
	}
	return "day"
}

// ProposalLines are the booking lines of a proposal for pack: one line per
// proposed spot, with its segments when it changes spot.
func ProposalLines(p availability.Proposal, packID string) []booking.LineInput {
	out := make([]booking.LineInput, 0, len(p.Lines))
	for _, l := range p.Lines {
		li := booking.LineInput{PackID: packID, SpotID: l.Segments[0].SpotID.String()}
		if len(l.Segments) > 1 {
			for _, sg := range l.Segments {
				li.Segments = append(li.Segments, booking.SegmentInput{SpotID: sg.SpotID.String(), StartsOn: sg.Interval.From, EndsOn: sg.Interval.To})
			}
		}
		out = append(out, li)
	}
	return out
}

// ProposalID is the stable identifier of a proposal: a hash of its segments.
func ProposalID(p availability.Proposal) string {
	sum := sha256.Sum256([]byte(p.Key()))
	return hex.EncodeToString(sum[:8])
}

// Book creates the booking of a proposal (in.Lines carry the segments) in
// one transaction through the booking service: one allocation per segment,
// so a half-day taken meanwhile is refused by the database (409 with the
// conflicts, the panel searches again; INV-01, INV-102). It refuses the
// creation with price_changed when the total differs from expectedTotal.
// Requires auth.PermBookingsWrite (checked by the booking service).
func (s *Service) Book(ctx context.Context, p *auth.Principal, establishmentID string, in booking.CreateInput, expectedTotal *money.Cents) (*booking.Detail, error) {
	if len(in.Lines) == 0 {
		return nil, configerr.Invalid("booking.lines", "Almeno una riga.")
	}
	for i, l := range in.Lines {
		if l.SpotID == "" && len(l.Segments) == 0 {
			return nil, configerr.Invalid(fmt.Sprintf("booking.lines[%d].segments", i), "Ogni riga della proposta ha una postazione.")
		}
	}
	in.ExpectedTotal = expectedTotal
	d, err := s.d.Booker.Create(ctx, p, establishmentID, in)
	if errors.Is(err, booking.ErrTotalChanged) {
		return nil, fmt.Errorf("findspot: %w", ticketing.ErrPriceChanged)
	}
	return d, err
}
