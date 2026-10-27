package availability

import (
	"github.com/google/uuid"
)

// Find spot (spec 30.6, ADR 0028): the pure search behind "Trova posto".
// It extends the segment algorithm of AutoAssign (spec 7.5) to seat zones
// and to groups: given the free half-days of the candidate spots it returns
// proposals ranked by number of spot changes (0 first), then continuity and
// proximity. No I/O and no clock: internal/domain/findspot loads the
// occupancy, prices the proposals and books the chosen one.

// FindInput is the input of FindSpots.
type FindInput struct {
	// Spots are the active candidate spots (already filtered by zone, kind
	// and pack compatibility by the caller).
	Spots []Spot
	// Free are the free half-days of the candidate spots; not modified.
	Free *FreeSpots
	// Interval is the requested stay.
	Interval Interval
	// Count is the number of spots requested (groups); >= 1.
	Count int
	// MaxSpotChanges is the maximum number of spot changes per line (0..3).
	MaxSpotChanges int
	// RequiredAttributes must all be present on every spot of a proposal.
	RequiredAttributes []string
	// PreviousSpotID is the spot of the same customer in a previous booking
	// (continuity), uuid.Nil if none.
	PreviousSpotID uuid.UUID
	// Limit caps the number of proposals (default 10).
	Limit int
}

// ProposalLine is one requested spot of a proposal: its segments cover the
// stay exactly once, in date order (INV-102).
type ProposalLine struct {
	Segments []Segment
}

// Proposal is a ranked solution of FindSpots.
type Proposal struct {
	Lines []ProposalLine
	// SpotChanges is the total number of changes of the proposal.
	SpotChanges int
	// Adjacent reports that the spots of a group proposal are neighbours.
	Adjacent bool
}

// FindSpots returns the proposals for in, best first. Skeleton: the
// algorithm is delivered by the find-spot builder (owner of this file);
// until then it returns no proposal.
func FindSpots(in FindInput) []Proposal {
	_ = in
	return nil
}
