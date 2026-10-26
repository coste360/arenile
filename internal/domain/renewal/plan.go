package renewal

import (
	"errors"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Pure decisions of the renewals (functional core): no database, no clock
// (time is passed in). Table tests in plan_test.go.

// Skip reasons (api.RenewalSkipReason).
const (
	SkipSpotUnavailable    = "spot_unavailable"
	SkipSpotInactive       = "spot_inactive"
	SkipNoPrice            = "no_price"
	SkipAlreadyRenewed     = "already_renewed"
	SkipCustomerAnonymized = "customer_anonymized"
	SkipNotConfirmed       = "not_confirmed"
)

// Bulk actions (api.RenewalBulkAction).
const (
	ActionSend           = "send"
	ActionAccept         = "accept"
	ActionDecline        = "decline"
	ActionExtendDeadline = "extend_deadline"
	ActionRelease        = "release"
)

// Payment choices of the public acceptance.
const (
	PaymentDeposit = "deposit"
	PaymentFull    = "full"
)

// PassInterval maps the period of a source pass onto the target season. A
// pass that ran until the end of the source season is a full-season pass
// (its start is only moved to the purchase day when sold late, see
// pass.passInterval) and covers the whole target season. A pass that ended
// earlier keeps its shape: both ends move by the year difference with
// pricing.ShiftDate (weekday kept where it makes sense), a start on the
// opening day stays on the opening day, and the result is clamped to the
// target season. ok is false when nothing is left.
func PassInterval(src, target store.Season, from, to calendar.Date) (calendar.Date, calendar.Date, bool) {
	if !to.Before(src.ClosesOn) {
		return target.OpensOn, target.ClosesOn, true
	}
	years := target.Year - src.Year
	f, t := pricing.ShiftDate(from, years, false), pricing.ShiftDate(to, years, true)
	if !from.After(src.OpensOn) || f.Before(target.OpensOn) {
		f = target.OpensOn
	}
	if t.After(target.ClosesOn) {
		t = target.ClosesOn
	}
	if t.Before(f) {
		return calendar.Date{}, calendar.Date{}, false
	}
	return f, t, true
}

// ValidateDeadline checks a pre-emption deadline: in the future and before
// the first day of the target season (opensAt is its midnight in the
// establishment timezone).
func ValidateDeadline(field string, deadline, now, opensAt time.Time) error {
	switch {
	case deadline.IsZero():
		return configerr.Invalid(field, "Indica la scadenza della prelazione.")
	case !deadline.After(now):
		return configerr.Invalid(field, "La scadenza della prelazione deve essere futura.")
	case !deadline.Before(opensAt):
		return configerr.Invalid(field, "La scadenza della prelazione deve precedere l'apertura della nuova stagione.")
	}
	return nil
}

// SkipReasonOf classifies the error of the creation of a proposal booking:
// the spot taken (allocation conflict) or no price in the target season
// skip the pass; any other error aborts the campaign.
func SkipReasonOf(err error) (string, bool) {
	var ce *availability.ConflictError
	var pve *policy.ViolationError
	switch {
	case errors.As(err, &ce), errors.Is(err, availability.ErrSpotUnavailable), errors.Is(err, availability.ErrPoolExhausted):
		return SkipSpotUnavailable, true
	case errors.Is(err, pricing.ErrNoPrice):
		return SkipNoPrice, true
	case errors.As(err, &pve), errors.Is(err, configerr.ErrValidation):
		// The pack or the policy of the target season refuses the request
		// (formula no longer sold, catalog changed): no price to propose.
		return SkipNoPrice, true
	}
	return "", false
}

// Open reports whether a proposal can still be accepted or declined.
func Open(status string) bool {
	return status == store.RenewalProposed || status == store.RenewalAccepted
}

// Actions are what the customer may do from the magic link.
type Actions struct {
	Accept, Decline, PayDeposit, PayFull, UseCredit bool
}

// ActionsInput are the inputs of PublicActions.
type ActionsInput struct {
	Status        string // proposal status
	BookingStatus string
	Deadline      time.Time
	HoldExpiresAt *time.Time
	Now           time.Time
	Total, Paid   money.Cents
	Deposit       money.Cents
	Online        bool // the establishment takes online payments
	Credit        money.Cents
	CanApply      bool // a credit applier is configured
}

// PublicActions decides the actions of the public renewal page: open
// proposals with a running hold (proposed before the deadline, accepted
// while the payment hold runs) can be accepted or declined; the deposit is
// offered when the policy asks for one below the balance; credit when the
// customer has some and it can be applied.
func PublicActions(in ActionsInput) Actions {
	if !Open(in.Status) || (in.BookingStatus != "held" && in.BookingStatus != "pending_payment") {
		return Actions{}
	}
	if in.HoldExpiresAt == nil || !in.HoldExpiresAt.After(in.Now) {
		return Actions{}
	}
	if in.Status == store.RenewalProposed && !in.Deadline.After(in.Now) {
		return Actions{}
	}
	balance := in.Total - in.Paid
	return Actions{
		Accept:     true,
		Decline:    true,
		PayDeposit: in.Online && in.Deposit > in.Paid && in.Deposit < in.Total,
		PayFull:    in.Online && balance > 0,
		UseCredit:  in.CanApply && in.Credit > 0 && balance > 0,
	}
}

// AcceptRefusal is the error of an acceptance the public actions refuse
// (PublicActions(in).Accept false): a closed proposal, or a booking already
// confirmed (by the desk, the payment webhook or a credit payment, before
// renewals.sync converts the proposal), is no longer open; a passed deadline
// or hold, or a booking closed otherwise, is an expiry, as sync records it.
func AcceptRefusal(in ActionsInput) error {
	switch {
	case in.Status == store.RenewalExpired:
		return ErrExpired
	case !Open(in.Status):
		return ErrNotOpen
	}
	switch in.BookingStatus {
	case "confirmed", "checked_in", "completed", "no_show":
		return ErrNotOpen
	}
	return ErrExpired
}

// AmountDue is what the customer pays online for the chosen payment after
// what is already paid (deposit: up to the deposit; full: the balance),
// never negative.
func AmountDue(payment string, total, paid, deposit money.Cents) money.Cents {
	target := total
	if payment == PaymentDeposit && deposit > 0 && deposit < total {
		target = deposit
	}
	return max(target-paid, 0)
}
