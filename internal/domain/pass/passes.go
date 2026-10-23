package pass

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
)

// Detail is a season pass with its releases (api.SeasonPassDetail).
type Detail struct {
	Pass store.SeasonPassView
	ReleaseList
}

// ReleaseList is the releases of a pass with the bookings that bought the
// resold half-days.
type ReleaseList struct {
	Releases []store.SeasonRelease
	// ResoldBookingIDs maps resold_booking_line_id to the resale booking id.
	ResoldBookingIDs map[string]string
}

// releaseList loads the releases of a pass and their resale bookings.
func releaseList(ctx context.Context, db bun.IDB, es store.EstablishmentScope, passID string) (ReleaseList, error) {
	rel, err := store.ListSeasonReleases(ctx, db, es, passID)
	if err != nil {
		return ReleaseList{}, fmt.Errorf("pass: releases: %w", err)
	}
	var lines []string
	for i := range rel {
		if l := rel[i].ResoldBookingLineID; l != nil {
			lines = append(lines, *l)
		}
	}
	ids, err := store.PassLineBookingIDs(ctx, db, es, lines)
	if err != nil {
		return ReleaseList{}, fmt.Errorf("pass: releases: %w", err)
	}
	return ReleaseList{Releases: rel, ResoldBookingIDs: ids}, nil
}

// ListFilter selects season passes (empty fields = any).
type ListFilter = store.SeasonPassFilter

// CreateInput is a new season pass (api.SeasonPassCreate): a customer on a
// spot for the season (or a sub-range) with a pack.
type CreateInput struct {
	CustomerID string
	SeasonID   string
	SpotID     string
	PackID     string
	// StartsOn/EndsOn nil: the season bounds (start moved to today when the
	// season already started).
	StartsOn *calendar.Date
	EndsOn   *calendar.Date
	// Slots empty means FULL.
	Slots              calendar.Slots
	Extras             []booking.ExtraInput
	NotesInternal      string
	RenewalOfBookingID string
}

// UpdateInput changes the notes of a pass (nil = unchanged). Spot and
// dates change through the booking service (PATCH /bookings, move).
type UpdateInput struct {
	NotesCustomer *string
	NotesInternal *string
}

// List returns a page of season passes.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f ListFilter, cursor string, limit int) (store.Page[store.SeasonPassView], error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return store.Page[store.SeasonPassView]{}, err
	}
	page, err := store.ListSeasonPasses(ctx, s.d.DB, es, f, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("pass: list: %w", err)
	}
	return page, err
}

// Get returns a season pass with its releases.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, s.d.DB, es, id)
}

func (s *Service) detail(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*Detail, error) {
	v, err := loadPass(ctx, db, es, id)
	if err != nil {
		return nil, err
	}
	rl, err := releaseList(ctx, db, es, id)
	if err != nil {
		return nil, err
	}
	return &Detail{Pass: *v, ReleaseList: rl}, nil
}

// Create creates a season pass: a booking with is_season_pass, formula
// season and season allocations for every day and slot of the period,
// priced by the pricing engine (fixed season price or period rules), in one
// transaction (Bookings.CreateInTx: customer, pricing, policy, allocation,
// audit, booking.created). A spot already taken on some half-days fails
// with *availability.ConflictError listing them (409 spot_unavailable).
func (s *Service) Create(ctx context.Context, p *auth.Principal, ip, establishmentID string, in CreateInput) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		req, err := s.prepareCreate(ctx, tx, es, &in)
		if err != nil {
			return err
		}
		d, err := s.d.Bookings.CreateInTx(ctx, tx, p, es, req)
		if err != nil {
			return err
		}
		out, err = s.detail(ctx, tx, es, d.Booking.ID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, p, ip, es, "season_pass.created", d.Booking.ID, map[string]any{
			"season_id": in.SeasonID, "spot_id": in.SpotID, "pack_id": in.PackID,
			"starts_on": req.StartsOn.String(), "ends_on": req.EndsOn.String(), "slots": req.Slots,
			"total_amount": d.Booking.TotalAmount, "renewal_of_booking_id": in.RenewalOfBookingID,
		}, "", nil)
	})
	return out, err
}

// prepareCreate validates the references of in and builds the booking request.
func (s *Service) prepareCreate(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, in *CreateInput) (booking.CreateInput, error) {
	ve := &configerr.ValidationError{}
	if in.CustomerID == "" {
		ve.Add("customer_id", "Il cliente è obbligatorio.")
	}
	slots := in.Slots
	if slots == "" {
		slots = calendar.SlotsFull
	}
	if !slots.Valid() {
		ve.Add("slots", "Fascia non valida (AM, PM o FULL).")
	}
	season, err := store.GetSeason(ctx, tx, es, in.SeasonID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		ve.Add("season_id", "Stagione inesistente in questo stabilimento.")
	case err != nil:
		return booking.CreateInput{}, fmt.Errorf("pass: create: %w", err)
	}
	spot, err := store.PassGetSpot(ctx, tx, es, in.SpotID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		ve.Add("spot_id", "Postazione inesistente in questo stabilimento.")
	case err != nil:
		return booking.CreateInput{}, fmt.Errorf("pass: create: %w", err)
	case !spot.Active:
		ve.Add("spot_id", "La postazione è disattivata.")
	}
	if _, err := store.GetPack(ctx, tx, es, in.PackID); errors.Is(err, store.ErrNotFound) {
		ve.Add("pack_id", "Pack inesistente in questo stabilimento.")
	} else if err != nil {
		return booking.CreateInput{}, fmt.Errorf("pass: create: %w", err)
	}
	if in.RenewalOfBookingID != "" {
		prev, err := store.PassGetBooking(ctx, tx, es, in.RenewalOfBookingID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && !prev.IsSeasonPass) {
			ve.Add("renewal_of_booking_id", "L'abbonamento da rinnovare non esiste.")
		} else if err != nil {
			return booking.CreateInput{}, fmt.Errorf("pass: create: %w", err)
		}
	}
	if err := ve.OrNil(); err != nil {
		return booking.CreateInput{}, err
	}
	today, err := s.today(ctx, tx, es)
	if err != nil {
		return booking.CreateInput{}, err
	}
	iv, err := passInterval(season, in.StartsOn, in.EndsOn, today)
	if err != nil {
		return booking.CreateInput{}, err
	}
	return booking.CreateInput{
		CustomerID: in.CustomerID,
		Channel:    store.ChannelDesk,
		StartsOn:   iv.from,
		EndsOn:     iv.to,
		Slots:      slots,
		Formula:    "season",
		Lines: []booking.LineInput{{
			PackID: in.PackID, ZoneID: spot.ZoneID, SpotID: spot.ID, Quantity: 1, Extras: in.Extras,
		}},
		NotesInternal:      in.NotesInternal,
		SeasonID:           season.ID,
		IsSeasonPass:       true,
		AllocationKind:     inventory.KindSeason,
		RenewalOfBookingID: in.RenewalOfBookingID,
	}, nil
}

// Update changes the notes of a season pass (optimistic: ifMatch nil skips
// the version check) and emits booking.modified.
func (s *Service) Update(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in UpdateInput) (*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	var out *Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		v, err := loadPass(ctx, tx, es, id)
		if err != nil {
			return err
		}
		b := v.Booking
		if err := configerr.CheckVersion(ifMatch, b.Version); err != nil {
			return err
		}
		nc, ni := b.NotesCustomer, b.NotesInternal
		if in.NotesCustomer != nil {
			nc = *in.NotesCustomer
		}
		if in.NotesInternal != nil {
			ni = *in.NotesInternal
		}
		if len([]rune(nc)) > 4000 || len([]rune(ni)) > 4000 {
			return configerr.Invalid("notes_internal", "Le note possono avere al massimo 4000 caratteri.")
		}
		if nc == b.NotesCustomer && ni == b.NotesInternal {
			out, err = s.detail(ctx, tx, es, id)
			return err
		}
		ok, err := store.UpdateSeasonPassNotes(ctx, tx, es, id, nc, ni, b.Version, s.now())
		if err != nil {
			return fmt.Errorf("pass: update: %w", err)
		}
		if !ok {
			cur, err := loadPass(ctx, tx, es, id)
			if err != nil {
				return err
			}
			return &configerr.VersionConflictError{Current: cur.Booking.Version}
		}
		if out, err = s.detail(ctx, tx, es, id); err != nil {
			return err
		}
		var fields []string
		if nc != b.NotesCustomer {
			fields = append(fields, "notes_customer")
		}
		if ni != b.NotesInternal {
			fields = append(fields, "notes_internal")
		}
		nb := out.Pass.Booking
		return s.record(ctx, tx, p, ip, es, "season_pass.updated", id, map[string]any{"fields": fields},
			event.BookingModified, bookingChange(&nb, out.Pass.SpotID))
	})
	return out, err
}

// bookingChange is the booking.* payload of a pass (no personal data).
func bookingChange(b *store.Booking, spotID string) event.BookingChange {
	c := event.BookingChange{
		BookingID: b.ID, Code: b.Code, Status: b.Status, Channel: b.Channel, CustomerID: b.CustomerID,
		StartsOn: b.StartsOn.String(), EndsOn: b.EndsOn.String(), TotalAmount: int64(b.TotalAmount),
		PaidAmount: int64(b.PaidAmount), IsSeasonPass: true, Version: b.Version,
	}
	if spotID != "" {
		c.SpotIDs = []string{spotID}
	}
	return c
}
