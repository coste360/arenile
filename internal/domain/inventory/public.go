package inventory

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// PublicAvailability is the availability of the public page (M4, spec 7.8,
// 12.3): always channel online (online quota applied, ADR 0011), no
// permission check (the caller resolved es from a public slug), expired
// holds ignored. Implements checkout.Inventory.
func (s *Service) PublicAvailability(ctx context.Context, es store.EstablishmentScope, q AvailabilityQuery) (*AvailabilityResult, error) {
	if q.Slots == "" {
		q.Slots = calendar.SlotsFull
	}
	q.Channel = store.ChannelOnline
	if err := validateRange(q.From, q.To, MaxAvailabilityDays); err != nil {
		return nil, err
	}
	if !q.Slots.Valid() {
		return nil, configerr.Invalid("slots", "Fasce non valide.")
	}
	res, err := s.availability(ctx, es, q)
	return res, wrap("public availability", err)
}
