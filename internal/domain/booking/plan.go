package booking

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
)

// Pure decisions of the booking use cases (functional core): no database, no
// clock. The service (imperative shell) loads the data, calls these and
// writes the outcome in one transaction. Each function has table tests in
// plan_test.go.

// InitialState is the outcome of DecideInitialState.
type InitialState struct {
	Status Status
	// Kind is the allocation kind; empty means no allocation (draft).
	Kind inventory.Kind
	// HoldTTL is the hold duration when Kind is KindHold (zero otherwise).
	HoldTTL time.Duration
	// Event is the outbox event written besides booking.created ("" for none).
	Event string
}

// StateRequest are the inputs of DecideInitialState.
type StateRequest struct {
	Requested        Status // draft | held | confirmed ("" = confirmed)
	ApprovalRequired bool   // policy or pack requires the manager's approval
	SeasonPass       bool
	Kind             inventory.Kind // explicit allocation kind (internal callers)
	HoldTTL          time.Duration  // TTL of held bookings
	ApprovalTTL      time.Duration  // TTL of the long approval hold
}

// DecideInitialState decides the status and the allocation kind of a new
// booking (spec 10.1, 10.2): a panel booking defaults to confirmed (manual
// driver: the desk collects the money at the till); approval turns it into
// awaiting_approval with a long hold; held occupies with a hold of HoldTTL;
// draft occupies nothing. Season passes allocate with kind season.
func DecideInitialState(r StateRequest) (InitialState, error) {
	switch r.Requested {
	case StatusDraft:
		return InitialState{Status: StatusDraft}, nil
	case StatusHeld:
		return InitialState{Status: StatusHeld, Kind: inventory.KindHold, HoldTTL: r.HoldTTL, Event: eventHeld}, nil
	case "", StatusConfirmed:
		if r.ApprovalRequired {
			return InitialState{Status: StatusAwaitingApproval, Kind: inventory.KindHold, HoldTTL: r.ApprovalTTL, Event: eventApprovalRequested}, nil
		}
		kind := inventory.KindBooking
		if r.SeasonPass {
			kind = inventory.KindSeason
		}
		if r.Kind == inventory.KindSeason || r.Kind == inventory.KindBooking {
			kind = r.Kind
		}
		return InitialState{Status: StatusConfirmed, Kind: kind, Event: eventConfirmed}, nil
	}
	return InitialState{}, fmt.Errorf("%w: initial status %q", ErrInvalidInitialStatus, r.Requested)
}

// ---- booking codes -------------------------------------------------------------

// CodePrefix derives the code prefix of an establishment from its slug: the
// initials of its first two words ("lido-delfino" -> "LD"), or the first two
// letters of a single word; "AR" when nothing usable is left.
func CodePrefix(slug string) string {
	words := strings.FieldsFunc(strings.ToUpper(slug), func(r rune) bool {
		return r > unicode.MaxASCII || !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var b strings.Builder
	switch {
	case len(words) >= 2:
		b.WriteByte(words[0][0])
		b.WriteByte(words[1][0])
	case len(words) == 1 && len(words[0]) >= 2:
		b.WriteString(words[0][:2])
	default:
		return "AR"
	}
	return b.String()
}

// CodeSeries is the part of the code shared by one establishment and year,
// e.g. "LD-2027-".
func CodeSeries(prefix string, year int) string { return fmt.Sprintf("%s-%04d-", prefix, year) }

// NextCode returns the code following last within series (last "" or of
// another series starts at 1): "LD-2027-000123" -> "LD-2027-000124".
func NextCode(series, last string) string {
	n := 0
	if rest, ok := strings.CutPrefix(last, series); ok {
		if v, err := strconv.Atoi(rest); err == nil && v > 0 {
			n = v
		}
	}
	return fmt.Sprintf("%s%06d", series, n+1)
}

// ---- pricing ---------------------------------------------------------------------

// PricedLine is one requested line with its resolved zone and spot.
type PricedLine struct {
	PackID   string
	ZoneID   string
	SpotID   string
	Quantity int
	Extras   []ExtraInput
	// Segments of a split stay: each day is priced with the spot occupied
	// that day (ADR 0028).
	Segments []SegmentInput
}

// PricingLines expands the requested lines into the engine lines: a line
// with quantity q (quantity zones) is priced as q packs, its extras are
// attached to the first unit. It returns the engine lines and, for each
// engine line, the index of the requested line.
func PricingLines(lines []PricedLine) ([]pricing.LineInput, []int) {
	var out []pricing.LineInput
	var owner []int
	for i, l := range lines {
		q := max(l.Quantity, 1)
		for u := range q {
			in := pricing.LineInput{PackID: l.PackID, ZoneID: l.ZoneID, SpotID: l.SpotID, Segments: EngineSegments(l.Segments)}
			if u == 0 {
				for _, x := range l.Extras {
					in.Extras = append(in.Extras, pricing.ExtraInput{ExtraID: x.ExtraID, Quantity: x.Quantity})
				}
			}
			out = append(out, in)
			owner = append(owner, i)
		}
	}
	return out, owner
}

// LineTotals sums the engine line totals back into the requested lines and
// returns, for each requested line, its total and the totals of its extras
// (in request order). Extras are matched to the breakdown by position.
func LineTotals(bd pricing.PriceBreakdown, owner []int, n int) (totals []money.Cents, extraTotals [][]money.Cents) {
	totals = make([]money.Cents, n)
	extraTotals = make([][]money.Cents, n)
	seen := make([]bool, n)
	for k, lb := range bd.Lines {
		if k >= len(owner) {
			break
		}
		i := owner[k]
		totals[i] += lb.Total
		if !seen[i] {
			seen[i] = true
			for _, x := range lb.Extras {
				extraTotals[i] = append(extraTotals[i], x.Total)
			}
		}
	}
	return totals, extraTotals
}

// ---- cancellation ------------------------------------------------------------------

// Settlement is how a cancellation settles the money already paid.
type Settlement string

// Settlements (api.BookingCancelRequestSettlement).
const (
	SettlePolicy Settlement = "policy" // as the policy snapshot says (default)
	SettleRefund Settlement = "refund" // force money back
	SettleCredit Settlement = "credit" // force customer credit
	SettleNone   Settlement = "none"   // nothing returned
)

// RefundPreview is the money returned by a cancellation.
type RefundPreview struct {
	Amount        money.Cents
	As            Settlement // refund | credit (none when Amount is zero by choice)
	PaidAmount    money.Cents
	RefundPercent int
	// Tier is the cancellation tier applied (nil when none).
	Tier *policy.CancellationTier
}

// CancelRequest are the inputs of ComputeRefund.
type CancelRequest struct {
	Settlement Settlement
	Weather    bool
	// Override replaces the computed amount (panel), capped at paid.
	Override *money.Cents
}

// ComputeRefund computes the refund of a cancellation of a booking with
// rules (the policy snapshot), paid already paid, at daysBefore days before
// the start (negative once started). The amount never exceeds paid (INV-24).
func ComputeRefund(rules policy.Rules, paid money.Cents, daysBefore int, req CancelRequest) RefundPreview {
	out := RefundPreview{PaidAmount: max(paid, 0), As: SettleRefund}
	if req.Settlement == SettleNone {
		out.As = SettleNone
		return out
	}
	switch {
	case req.Weather:
		w := policy.WeatherOutcome(out.PaidAmount, rules)
		out.Amount = w.RefundAmount + w.CreditAmount
		if w.CreditAmount > 0 {
			out.As = SettleCredit
		}
		if w.Applies {
			out.RefundPercent = 100
		}
	default:
		tier := tierFor(rules.Cancellation, daysBefore)
		if tier != nil {
			t := *tier
			out.Tier = &t
			out.RefundPercent = t.RefundPercent
			out.Amount = out.PaidAmount.MulRat(int64(t.RefundPercent), 100)
			if t.As == policy.AsCredit {
				out.As = SettleCredit
			}
		}
	}
	switch req.Settlement {
	case SettleRefund:
		out.As = SettleRefund
	case SettleCredit:
		out.As = SettleCredit
	}
	if req.Override != nil {
		out.Amount = *req.Override
	}
	out.Amount = min(max(out.Amount, 0), out.PaidAmount)
	return out
}

// tierFor is the tier with the largest until_days_before not greater than
// daysBefore (the rule of policy.CancellationOutcome); nil once started.
func tierFor(tiers []policy.CancellationTier, daysBefore int) *policy.CancellationTier {
	if daysBefore < 0 {
		return nil
	}
	var best *policy.CancellationTier
	for i := range tiers {
		t := &tiers[i]
		if daysBefore >= t.UntilDaysBefore && (best == nil || t.UntilDaysBefore > best.UntilDaysBefore) {
			best = t
		}
	}
	return best
}
