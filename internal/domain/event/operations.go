package event

import "strings"

// M3 event types that complete spec 18.1 for the panel operations (holds,
// spot blocks, cash). They follow the same outbox rules: written in the
// transaction of the change, payload version 1, no personal data.
const (
	BookingRejected           = "booking.rejected"
	BookingExpired            = "booking.expired"
	HoldCreated               = "hold.created"
	HoldReleased              = "hold.released"
	HoldConverted             = "hold.converted"
	SpotBlockCreated          = "spot_block.created"
	SpotBlockDeleted          = "spot_block.deleted"
	SeasonDayReleaseRevoked   = "season.day_release_revoked"
	CustomerUpdated           = "customer.updated"
	CashSessionOpened         = "cash_session.opened"
	CashMovementRecorded      = "cash_movement.recorded"
	FiscalDocumentRecorded    = "fiscal_document.recorded"
	AllocationAssigned        = "allocation.assigned"
	allocationTopicPrefixSpot = "spot_block."
)

// Payloads (version 1). Every M3 payload carries the top-level hints
// spot_ids and dates when resources are involved, so that the SSE stream
// (ToStreamMessage) can tell the panel which spots and days to refresh.
// They never contain names, emails, phones or notes (spec 21.1, 23).

// BookingChange is the payload of booking.* events.
type BookingChange struct {
	BookingID      string   `json:"booking_id"`
	Code           string   `json:"code"`
	Status         string   `json:"status"`
	PreviousStatus string   `json:"previous_status,omitempty"`
	Channel        string   `json:"channel"`
	CustomerID     string   `json:"customer_id"`
	StartsOn       string   `json:"starts_on"`
	EndsOn         string   `json:"ends_on"`
	TotalAmount    int64    `json:"total_amount"`
	PaidAmount     int64    `json:"paid_amount"`
	IsSeasonPass   bool     `json:"is_season_pass,omitempty"`
	Version        int64    `json:"version"`
	SpotIDs        []string `json:"spot_ids,omitempty"`
	Dates          []string `json:"dates,omitempty"`
}

// AllocationChange is the payload of allocation.*, season.*, hold.* and
// spot_block.* events.
type AllocationChange struct {
	Reason     string   `json:"reason"` // released | assigned | hold_created | hold_released | hold_converted | blocked | unblocked | day_released | release_revoked
	BookingID  string   `json:"booking_id,omitempty"`
	HoldID     string   `json:"hold_id,omitempty"`
	BlockID    string   `json:"block_id,omitempty"`
	ReleaseIDs []string `json:"release_ids,omitempty"`
	PoolIDs    []string `json:"pool_ids,omitempty"`
	SpotIDs    []string `json:"spot_ids,omitempty"`
	Dates      []string `json:"dates,omitempty"`
}

// PaymentChange is the payload of payment.* events.
type PaymentChange struct {
	PaymentID     string `json:"payment_id"`
	BookingID     string `json:"booking_id,omitempty"`
	CashSessionID string `json:"cash_session_id,omitempty"`
	Provider      string `json:"provider"`
	Method        string `json:"method"`
	Direction     string `json:"direction"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
}

// CashChange is the payload of cash_session.* and cash_movement.* events.
type CashChange struct {
	CashSessionID string           `json:"cash_session_id"`
	Register      string           `json:"register"`
	Status        string           `json:"status"` // open | closed
	MovementID    string           `json:"movement_id,omitempty"`
	Difference    *int64           `json:"difference,omitempty"`
	Expected      map[string]int64 `json:"expected_totals,omitempty"`
	Counted       map[string]int64 `json:"counted_totals,omitempty"`
}

// CustomerChange is the payload of customer.* events (ids only).
type CustomerChange struct {
	CustomerID string `json:"customer_id"`
	Version    int64  `json:"version,omitempty"`
}

// Stream topics: the SSE event names of the admin stream (spec 21.1). The
// panel subscribes to topics, not to every domain event type.
const (
	TopicBooking    = "booking.changed"
	TopicAllocation = "allocation.changed"
	TopicHold       = "hold.changed"
	TopicCash       = "cash.changed"
	TopicCustomer   = "customer.changed"
	TopicConfig     = "config.changed"
	TopicOther      = "event"
)

// Topic maps a domain event type to its stream topic. hasBooking reports
// whether the event refers to a booking (payments on a booking refresh the
// booking, counter sales only the cash desk).
func Topic(eventType string, hasBooking bool) string {
	prefix, _, _ := strings.Cut(eventType, ".")
	switch prefix + "." {
	case "booking.":
		return TopicBooking
	case "payment.":
		if hasBooking {
			return TopicBooking
		}
		return TopicCash
	case "allocation.", "season.", allocationTopicPrefixSpot:
		return TopicAllocation
	case "hold.":
		return TopicHold
	case "cash_session.", "cash_movement.", "fiscal_document.":
		return TopicCash
	case "customer.":
		return TopicCustomer
	case "config.":
		return TopicConfig
	}
	return TopicOther
}
