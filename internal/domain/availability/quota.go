package availability

import (
	"errors"
	"fmt"
)

// Channel is the sales channel asking for availability.
type Channel string

// Channels (same values as policy.Channel and the bookings.channel column).
const (
	ChannelDesk   Channel = "desk"
	ChannelOnline Channel = "online"
	ChannelAPI    Channel = "api"
)

// QuotaType is the kind of online quota of a zone.
type QuotaType string

// Quota types.
const (
	// QuotaPercent: Value is a percentage (0..100) of the zone capacity,
	// rounded down.
	QuotaPercent QuotaType = "percent"
	// QuotaCount: Value is an absolute number of spots/units.
	QuotaCount QuotaType = "count"
)

// ErrInvalidQuota is returned by OnlineQuota.Check.
var ErrInvalidQuota = errors.New("availability: invalid online quota")

// OnlineQuota is zones.online_quota: {"type": "percent"|"count", "value": n}.
type OnlineQuota struct {
	Type  QuotaType `json:"type"`
	Value int       `json:"value"`
}

// Check validates the quota configuration.
func (q OnlineQuota) Check() error {
	switch q.Type {
	case QuotaPercent:
		if q.Value < 0 || q.Value > 100 {
			return fmt.Errorf("%w: percent %d not in 0..100", ErrInvalidQuota, q.Value)
		}
	case QuotaCount:
		if q.Value < 0 {
			return fmt.Errorf("%w: count %d < 0", ErrInvalidQuota, q.Value)
		}
	default:
		return fmt.Errorf("%w: type %q", ErrInvalidQuota, q.Type)
	}
	return nil
}

// Limit returns the number of units sellable online per half-day for a zone of
// the given capacity (active spots, or effective pool capacity).
func (q OnlineQuota) Limit(capacity int) int {
	capacity = max(capacity, 0)
	switch q.Type {
	case QuotaPercent:
		return capacity * min(max(q.Value, 0), 100) / 100
	case QuotaCount:
		return min(max(q.Value, 0), capacity)
	}
	// Unknown type: fail closed for online sales.
	return 0
}

// QuotaState is the occupancy of a zone on one half-day.
type QuotaState struct {
	Capacity   int // active spots / effective pool capacity
	Free       int // free units, all channels considered
	SoldOnline int // units already sold or held through quota-bound channels
}

// OnlineAvailable returns the units the channel may book on one half-day
// (spec 7.6):
//
//	available = min(free, quota - sold_online)
//
// The desk channel ignores the quota. The quota binds every other channel
// (online and api), since both are self-service sales. A nil quota means no
// quota. The result is never negative.
func OnlineAvailable(ch Channel, st QuotaState, q *OnlineQuota) int {
	free := max(st.Free, 0)
	if ch == ChannelDesk || q == nil {
		return free
	}
	return max(min(free, q.Limit(st.Capacity)-st.SoldOnline), 0)
}
