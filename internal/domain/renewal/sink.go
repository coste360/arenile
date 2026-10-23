package renewal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
)

// SinkName is the name of the renewal event sink.
const SinkName = "renewal"

// Name implements ext.EventSink.
func (s *Service) Name() string { return SinkName }

// Handle implements ext.EventSink: when the held booking of an accepted
// proposal is confirmed (payment webhook or manual driver, INV-19), the
// proposal becomes converted right away instead of at the next
// renewals.sync run, so the customer coming back from the payment page
// sees the renewal confirmed. Same transition as the job (syncOne), so a
// redelivered event or a concurrent job run changes nothing more.
func (s *Service) Handle(ctx context.Context, ev ext.Event) error {
	if ev.Type != event.BookingConfirmed {
		return nil
	}
	var data event.BookingChange
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return fmt.Errorf("renewal: decode %s: %w", ev.Type, err)
	}
	if data.BookingID == "" || !data.IsSeasonPass {
		return nil
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, ev.EstablishmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("renewal: %w", err)
	}
	prop, err := store.GetRenewalProposalByBooking(ctx, s.d.DB, es, data.BookingID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("renewal: %w", err)
	}
	if !Open(prop.Status) {
		return nil
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		_, err := s.syncOne(ctx, tx, es, prop.ID, s.now())
		return err
	})
}
