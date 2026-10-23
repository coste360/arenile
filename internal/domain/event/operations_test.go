package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

func TestTopic_Mapping(t *testing.T) {
	cases := []struct {
		typ        string
		hasBooking bool
		want       string
	}{
		{BookingConfirmed, true, TopicBooking},
		{BookingCancelled, false, TopicBooking},
		{PaymentSucceeded, true, TopicBooking},
		{PaymentSucceeded, false, TopicCash},
		{AllocationReleased, false, TopicAllocation},
		{SeasonDayReleased, true, TopicAllocation},
		{SpotBlockCreated, false, TopicAllocation},
		{HoldCreated, false, TopicHold},
		{CashSessionClosed, false, TopicCash},
		{CashMovementRecorded, false, TopicCash},
		{CustomerAnonymized, false, TopicCustomer},
		{ConfigMapChanged, false, TopicConfig},
		{"something.else", false, TopicOther},
	}
	for _, c := range cases {
		if got := Topic(c.typ, c.hasBooking); got != c.want {
			t.Errorf("Topic(%q, %v) = %q, want %q", c.typ, c.hasBooking, got, c.want)
		}
	}
}

func TestToStreamMessage_BookingTopicWithoutPII(t *testing.T) {
	data, _ := json.Marshal(BookingChange{
		BookingID: "b1", Code: "AR-1", Status: "confirmed", CustomerID: "c1",
		SpotIDs: []string{"s1"}, Dates: []string{"2027-07-01"},
	})
	m := ToStreamMessage(ext.Event{ID: "evt_1", Type: BookingConfirmed, EstablishmentID: "e", OccurredAt: time.Unix(0, 0), Data: data})
	if m.Topic != TopicBooking || m.BookingID != "b1" || len(m.SpotIDs) != 1 {
		t.Fatalf("%+v", m)
	}
	pay, _ := json.Marshal(PaymentChange{PaymentID: "p1", Method: "cash", Direction: "in", Amount: 100})
	if m := ToStreamMessage(ext.Event{Type: PaymentSucceeded, Data: pay}); m.Topic != TopicCash {
		t.Fatalf("counter sale topic %q", m.Topic)
	}
}
