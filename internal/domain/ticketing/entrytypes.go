package ticketing

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Entry ticket types (Adulto, Ridotto, Bambino, ...): each one is a pack,
// priced by the pricing engine per rate period, sold on a quantity-mode
// zone whose pool is the daily capacity shared with the online channel.

var entryCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// maxEntryText bounds each translation of name and description.
const maxEntryText = 200

// EntryTypeInput is api.EntryTicketTypeInput (create) or the merged update.
type EntryTypeInput struct {
	Code        string
	Name        store.I18nText
	Description store.I18nText
	PackID      string
	ZoneID      string
	SortOrder   int
	Active      bool
}

// EntryTypePatch is api.EntryTicketTypeUpdate (nil fields unchanged).
type EntryTypePatch struct {
	Name            *store.I18nText
	Description     *store.I18nText
	PackID          *string
	ZoneID          *string
	SortOrder       *int
	Active          *bool
	ExpectedVersion *int64
}

// ListEntryTypes returns the entry ticket types (auth.PermBookingsRead).
func (s *Service) ListEntryTypes(ctx context.Context, p *auth.Principal, establishmentID string) ([]store.EntryTicketType, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	out, err := store.ListEntryTicketTypes(ctx, s.d.DB, es)
	if out == nil {
		out = []store.EntryTicketType{}
	}
	return out, err
}

// GetEntryType returns one entry ticket type (auth.PermBookingsRead).
func (s *Service) GetEntryType(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.EntryTicketType, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	t, err := store.GetEntryTicketType(ctx, s.d.DB, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrEntryTypeNotFound
	}
	return t, err
}

// validateEntryType checks in against the catalog: an existing pack and a
// quantity-mode zone of es.
func validateEntryType(ctx context.Context, db bun.IDB, es store.EstablishmentScope, in *EntryTypeInput, create bool) error {
	ve := &configerr.ValidationError{}
	if create && !entryCodeRe.MatchString(in.Code) {
		ve.Add("code", "Minuscole, cifre, - e _ (massimo 40 caratteri).")
	}
	if len(in.Name) == 0 {
		ve.Add("name", "Indica il nome (almeno in italiano).")
	}
	for lang, v := range in.Name {
		if v == "" || len([]rune(v)) > maxEntryText {
			ve.Add("name."+lang, "Nome vuoto o troppo lungo.")
		}
	}
	for lang, v := range in.Description {
		if len([]rune(v)) > maxEntryText {
			ve.Add("description."+lang, "Massimo 200 caratteri.")
		}
	}
	if in.Description == nil {
		in.Description = store.I18nText{}
	}
	if in.PackID == "" {
		ve.Add("pack_id", "Campo obbligatorio.")
	} else if _, err := store.GetPack(ctx, db, es, in.PackID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		ve.Add("pack_id", "Pack inesistente.")
	}
	if in.ZoneID == "" {
		ve.Add("zone_id", "Campo obbligatorio.")
	} else {
		zones, err := store.InventoryZones(ctx, db, es, []string{in.ZoneID})
		if err != nil {
			return err
		}
		switch {
		case len(zones) == 0:
			ve.Add("zone_id", "Zona inesistente.")
		case zones[0].AssignmentMode != "quantity":
			ve.Add("zone_id", "Serve una zona a quantità (la sua capienza giornaliera è il pool della zona).")
		}
	}
	return ve.OrNil()
}

// CreateEntryType creates an entry ticket type (auth.PermEstablishmentConfigure).
func (s *Service) CreateEntryType(ctx context.Context, p *auth.Principal, establishmentID string, in EntryTypeInput) (*store.EntryTicketType, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if err := validateEntryType(ctx, s.d.DB, es, &in, true); err != nil {
		return nil, err
	}
	now := s.now()
	t := &store.EntryTicketType{
		ID: store.NewID(), Code: in.Code, Name: in.Name, Description: in.Description, PackID: in.PackID, ZoneID: in.ZoneID,
		SortOrder: in.SortOrder, Active: in.Active, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertEntryTicketType(ctx, tx, es, t); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return fmt.Errorf("%w: esiste già un tipo con questo codice o con questo pack", configerr.ErrConflict)
			}
			return err
		}
		return s.recordEntryType(ctx, tx, p, es, "entry_ticket_type.created", t)
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateEntryType modifies an entry ticket type (auth.PermEstablishmentConfigure,
// If-Match required by the handler).
func (s *Service) UpdateEntryType(ctx context.Context, p *auth.Principal, establishmentID, id string, in EntryTypePatch) (*store.EntryTicketType, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	var out *store.EntryTicketType
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetEntryTicketType(ctx, tx, es, id)
		if errors.Is(err, store.ErrNotFound) {
			return ErrEntryTypeNotFound
		}
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.ExpectedVersion, cur.Version); err != nil {
			return err
		}
		m := EntryTypeInput{
			Code: cur.Code, Name: cur.Name, Description: cur.Description, PackID: cur.PackID, ZoneID: cur.ZoneID,
			SortOrder: cur.SortOrder, Active: cur.Active,
		}
		if in.Name != nil {
			m.Name = *in.Name
		}
		if in.Description != nil {
			m.Description = *in.Description
		}
		if in.PackID != nil {
			m.PackID = *in.PackID
		}
		if in.ZoneID != nil {
			m.ZoneID = *in.ZoneID
		}
		if in.SortOrder != nil {
			m.SortOrder = *in.SortOrder
		}
		if in.Active != nil {
			m.Active = *in.Active
		}
		if err := validateEntryType(ctx, tx, es, &m, false); err != nil {
			return err
		}
		next := *cur
		next.Name, next.Description, next.PackID, next.ZoneID, next.SortOrder, next.Active, next.UpdatedAt =
			m.Name, m.Description, m.PackID, m.ZoneID, m.SortOrder, m.Active, s.now()
		ok, err := store.UpdateEntryTicketTypeVersioned(ctx, tx, es, &next, cur.Version)
		if errors.Is(err, store.ErrConflict) {
			return fmt.Errorf("%w: esiste già un tipo con questo pack", configerr.ErrConflict)
		}
		if err != nil {
			return err
		}
		if !ok {
			return &configerr.VersionConflictError{Current: cur.Version}
		}
		out = &next
		return s.recordEntryType(ctx, tx, p, es, "entry_ticket_type.updated", &next)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteEntryType deletes an entry ticket type never sold
// (auth.PermEstablishmentConfigure); a sold one is a conflict: deactivate it.
func (s *Service) DeleteEntryType(ctx context.Context, p *auth.Principal, establishmentID, id string) error {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetEntryTicketType(ctx, tx, es, id)
		if errors.Is(err, store.ErrNotFound) {
			return ErrEntryTypeNotFound
		}
		if err != nil {
			return err
		}
		sold, err := store.PackSold(ctx, tx, es, cur.PackID)
		if err != nil {
			return err
		}
		if sold {
			return fmt.Errorf("%w: il tipo ha già biglietti venduti, disattivalo invece di eliminarlo", configerr.ErrConflict)
		}
		if err := store.DeleteEntryTicketType(ctx, tx, es, id); err != nil {
			return err
		}
		return s.recordEntryType(ctx, tx, p, es, "entry_ticket_type.deleted", cur)
	})
}

func (s *Service) recordEntryType(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, action string, t *store.EntryTicketType) error {
	return s.d.Audit.Record(ctx, tx, auditEntry(ctx, p, es, action, "entry_ticket_type", t.ID, map[string]any{
		"code": t.Code, "pack_id": t.PackID, "zone_id": t.ZoneID, "active": t.Active, "sort_order": t.SortOrder,
	}))
}
