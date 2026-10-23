package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// Season pass renewal emails (spec 10.4, 19; M5B, sender internal/domain/
// renewal): the proposal and its reminders. They are booking emails of the
// held booking of the proposal and carry its manage link (issued when the
// message is composed, revoked if the transport fails), which opens the
// renewal page. The payload holds only the deadline (RFC 3339).

// renewalTemplates are the renewal template codes.
var renewalTemplates = map[string]bool{TemplateRenewalProposal: true, TemplateRenewalReminder: true}

// RenewalVars are the variables of the renewal templates besides the
// booking ones.
var RenewalVars = []string{"renewal.deadline"}

// renewalLink issues the manage link of a renewal email; a proposal whose
// booking is no longer held or pending payment is not sent (errSkip).
func (s *Service) renewalLink(ctx context.Context, es store.EstablishmentScope, d *bookingData) (*MagicLink, error) {
	if d.Booking.Status != statusHeld && d.Booking.Status != statusPendingPayment {
		return nil, fmt.Errorf("%w: renewal booking is %s", errSkip, d.Booking.Status)
	}
	return s.issueForEmail(ctx, es, d.Booking.ID, nil)
}

// renewalVars formats the deadline of the payload in the establishment
// timezone ("" when absent).
func renewalVars(m *store.Message, est *store.Establishment, cat catalog) map[string]string {
	out := map[string]string{"renewal.deadline": ""}
	raw, _ := m.Payload["renewal_deadline"].(string)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		out["renewal.deadline"] = formatDeadline(cat, t.In(estLocation(est)))
	}
	return out
}

// formatDeadline is the localized date and the time (HH:MM) of t.
func formatDeadline(cat catalog, t time.Time) string {
	return formatDate(cat, calendar.In(t, t.Location())) + ", " + t.Format("15:04")
}

// The renewal templates exist in every locale (templates/<lang>/
// renewal_*.md): registered here, next to their compose step.
func init() {
	Templates = append(Templates, TemplateRenewalProposal, TemplateRenewalReminder)
}
