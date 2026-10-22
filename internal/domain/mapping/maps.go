package mapping

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// ---- reads ------------------------------------------------------------------

// ListMaps returns a page of maps with their spot counts.
func (s *Service) ListMaps(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[MapView], error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return store.Page[MapView]{}, err
	}
	page, err := store.ListMaps(ctx, s.d.DB, es, cursor, limit)
	if err != nil {
		return store.Page[MapView]{}, err
	}
	ids := make([]string, len(page.Items))
	for i, m := range page.Items {
		ids[i] = m.ID
	}
	counts, err := store.MappingSpotCountsByMap(ctx, s.d.DB, es, ids)
	if err != nil {
		return store.Page[MapView]{}, err
	}
	out := store.Page[MapView]{Items: make([]MapView, len(page.Items)), NextCursor: page.NextCursor}
	for i, m := range page.Items {
		out.Items[i] = MapView{Map: m, SpotCount: counts[m.ID]}
	}
	return out, nil
}

// GetMap returns the full map document.
func (s *Service) GetMap(ctx context.Context, p *auth.Principal, establishmentID, mapID string) (*Document, error) {
	es, err := s.view(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	return s.loadDocument(ctx, s.d.DB, es, mapID)
}

func (s *Service) loadDocument(ctx context.Context, db bun.IDB, es store.EstablishmentScope, mapID string) (*Document, error) {
	m, err := store.GetMap(ctx, db, es, mapID)
	if err != nil {
		return nil, notFound(err, ErrMapNotFound)
	}
	spots, err := store.ListMapSpots(ctx, db, es, mapID)
	if err != nil {
		return nil, err
	}
	decs, err := store.ListMapDecorations(ctx, db, es, mapID)
	if err != nil {
		return nil, err
	}
	if spots == nil {
		spots = []store.Spot{}
	}
	if decs == nil {
		decs = []store.MapDecoration{}
	}
	return &Document{MapView: MapView{Map: *m, SpotCount: len(spots)}, Spots: spots, Decorations: decs}, nil
}

// ---- create / delete --------------------------------------------------------

// CreateMap creates an empty map.
func (s *Service) CreateMap(ctx context.Context, p *auth.Principal, establishmentID string, in MapInput) (*Document, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	validateMapInput(ve, "", in)
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	now := s.now()
	m := &store.Map{ID: store.NewID(), Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := applyMapInput(m, in); err != nil {
		return nil, err
	}
	var doc *Document
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertMap(ctx, tx, es, m); err != nil {
			return err
		}
		data := MapChangedData{MapID: m.ID, EntityType: "map", EntityID: m.ID, Action: "created", Version: m.Version}
		if err := s.record(ctx, tx, p, es, "map.created", data, map[string]any{"name": m.Name, "width": m.Width, "height": m.Height}); err != nil {
			return err
		}
		doc, err = s.loadDocument(ctx, tx, es, m.ID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: create map: %w", err)
	}
	return doc, nil
}

// DeleteMap deletes a map with its spots, decorations and zones (with their
// pools). It is refused when a spot has future allocations. expected is the
// optional If-Match version.
func (s *Service) DeleteMap(ctx context.Context, p *auth.Principal, establishmentID, mapID string, expected *int64) error {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		m, err := store.GetMap(ctx, tx, es, mapID)
		if err != nil {
			return notFound(err, ErrMapNotFound)
		}
		if err := configerr.CheckVersion(expected, m.Version); err != nil {
			return err
		}
		spots, err := store.ListMapSpots(ctx, tx, es, mapID)
		if err != nil {
			return err
		}
		spotIDs := make([]string, len(spots))
		var zoneIDs []string
		for i, sp := range spots {
			spotIDs[i] = sp.ID
			zoneIDs = append(zoneIDs, sp.ZoneID)
		}
		if err := s.checkAllocations(ctx, tx, es, spotIDs); err != nil {
			return err
		}
		zones, err := store.ListMapZones(ctx, tx, es, mapID)
		if err != nil {
			return err
		}
		// Spots first (zones are ON DELETE RESTRICT for spots), then the
		// zones of the map (their pools cascade), then the map.
		if err := store.DeleteSpots(ctx, tx, es, mapID, spotIDs); err != nil {
			return err
		}
		deletedZones := make([]string, 0, len(zones))
		for _, z := range zones {
			if err := store.DeleteZone(ctx, tx, es, z.ID); err != nil {
				return err
			}
			deletedZones = append(deletedZones, z.ID)
		}
		// Spots of the map may belong to zones of other maps only through
		// inconsistent data; keep their pools right anyway.
		if _, err := s.syncZonePools(ctx, tx, es, zoneIDs); err != nil {
			return err
		}
		if err := store.DeleteMap(ctx, tx, es, mapID); err != nil {
			return notFound(err, ErrMapNotFound)
		}
		data := MapChangedData{MapID: mapID, EntityType: "map", EntityID: mapID, Action: "deleted", Version: m.Version}
		return s.record(ctx, tx, p, es, "map.deleted", data, map[string]any{
			"name": m.Name, "spots_deleted": len(spotIDs), "zones_deleted": deletedZones,
		})
	})
	if err != nil {
		return fmt.Errorf("mapping: delete map: %w", err)
	}
	return nil
}

// ---- transactional save -----------------------------------------------------

// SaveMap replaces the whole map document (spec 11.2) if the stored version
// is still expected (If-Match): it updates the map, inserts, updates and
// deletes spots and decorations, resyncs the implicit zone pools, bumps the
// version, writes the audit log and emits config.map_changed, all in one
// transaction. Deleting a spot with future allocations is refused
// (configerr.SpotsAllocatedError); deactivating it is always allowed.
func (s *Service) SaveMap(ctx context.Context, p *auth.Principal, establishmentID, mapID string, expected int64, in SaveInput) (*Document, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validateDocument(&in).OrNil(); err != nil {
		return nil, err
	}
	var doc *Document
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		cur, err := store.GetMap(ctx, tx, es, mapID)
		if err != nil {
			return notFound(err, ErrMapNotFound)
		}
		if err := configerr.CheckVersion(&expected, cur.Version); err != nil {
			return err
		}
		doc, err = s.applyDocument(ctx, tx, p, es, cur, in, "saved")
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: save map: %w", err)
	}
	return doc, nil
}

// saveSummary is the audit diff of a document save.
type saveSummary struct {
	FromVersion        int64    `json:"from_version"`
	ToVersion          int64    `json:"to_version"`
	SpotsInserted      []string `json:"spots_inserted,omitempty"`
	SpotsUpdated       []string `json:"spots_updated,omitempty"`
	SpotsDeleted       []string `json:"spots_deleted,omitempty"`
	SpotsDeactivated   []string `json:"spots_deactivated,omitempty"`
	DecorationsChanged int      `json:"decorations_changed"`
	PoolsSynced        []string `json:"pools_synced,omitempty"`
}

// applyDocument writes in over the current map cur (already version
// checked) inside tx. in has passed validateDocument.
func (s *Service) applyDocument(ctx context.Context, tx bun.Tx, p *auth.Principal, es store.EstablishmentScope, cur *store.Map, in SaveInput, action string) (*Document, error) {
	oldSpots, err := store.ListMapSpots(ctx, tx, es, cur.ID)
	if err != nil {
		return nil, err
	}
	oldDecs, err := store.ListMapDecorations(ctx, tx, es, cur.ID)
	if err != nil {
		return nil, err
	}
	oldSpotByID := make(map[string]*store.Spot, len(oldSpots))
	for i := range oldSpots {
		oldSpotByID[oldSpots[i].ID] = &oldSpots[i]
	}
	oldDecByID := make(map[string]*store.MapDecoration, len(oldDecs))
	for i := range oldDecs {
		oldDecByID[oldDecs[i].ID] = &oldDecs[i]
	}

	// Checks needing the database: ids belong to this map, zones belong to
	// the establishment and to this map and have spots (not quantity).
	ve := &configerr.ValidationError{}
	zoneIDs := make([]string, 0, len(in.Spots))
	for _, sp := range in.Spots {
		zoneIDs = append(zoneIDs, sp.ZoneID)
	}
	zones, err := store.ListZonesByIDs(ctx, tx, es, uniqueSorted(zoneIDs))
	if err != nil {
		return nil, err
	}
	zoneByID := make(map[string]*store.Zone, len(zones))
	for i := range zones {
		zoneByID[zones[i].ID] = &zones[i]
	}
	for i, sp := range in.Spots {
		field := fmt.Sprintf("spots[%d]", i)
		if sp.ID != "" && oldSpotByID[sp.ID] == nil {
			ve.Add(field+".id", "Postazione inesistente su questa mappa.")
		}
		z := zoneByID[sp.ZoneID]
		switch {
		case z == nil:
			ve.Add(field+".zone_id", "Zona inesistente.")
		case z.MapID == nil || *z.MapID != cur.ID:
			ve.Add(field+".zone_id", "La zona non appartiene a questa mappa.")
		case z.AssignmentMode == ModeQuantity:
			ve.Add(field+".zone_id", "Le zone a quantità non hanno postazioni.")
		}
	}
	for i, d := range in.Decorations {
		if d.ID != "" && oldDecByID[d.ID] == nil {
			ve.Add(fmt.Sprintf("decorations[%d].id", i), "Elemento inesistente su questa mappa.")
		}
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}

	// Diff.
	now := s.now()
	sum := saveSummary{FromVersion: cur.Version, ToVersion: cur.Version + 1}
	kept := make(map[string]bool, len(in.Spots))
	var inserts []store.Spot
	var updates []store.Spot
	var renamed []string
	for _, sp := range in.Spots {
		if sp.ID == "" {
			row := spotRow(sp, store.NewID(), cur.ID, now)
			row.CreatedAt = now
			inserts = append(inserts, row)
			sum.SpotsInserted = append(sum.SpotsInserted, row.ID)
			continue
		}
		kept[sp.ID] = true
		old := oldSpotByID[sp.ID]
		row := spotRow(sp, sp.ID, cur.ID, now)
		row.CreatedAt = old.CreatedAt
		if spotEqual(old, &row) {
			continue
		}
		if old.Active && !row.Active {
			sum.SpotsDeactivated = append(sum.SpotsDeactivated, row.ID)
		}
		if old.Label != row.Label {
			renamed = append(renamed, row.ID)
		}
		updates = append(updates, row)
		sum.SpotsUpdated = append(sum.SpotsUpdated, row.ID)
	}
	var deleted []string
	affectedZones := slices.Clone(zoneIDs)
	for _, old := range oldSpots {
		if !kept[old.ID] {
			deleted = append(deleted, old.ID)
			affectedZones = append(affectedZones, old.ZoneID)
		} else if z := old.ZoneID; z != "" {
			affectedZones = append(affectedZones, z)
		}
	}
	sum.SpotsDeleted = deleted
	if err := s.checkAllocations(ctx, tx, es, deleted); err != nil {
		return nil, err
	}

	// Bump the version first: concurrent saves serialize on the map row and
	// the loser fails the version predicate.
	next := *cur
	if err := applyMapInput(&next, in.Map); err != nil {
		return nil, err
	}
	next.UpdatedAt = now
	ok, err := store.UpdateMapVersioned(ctx, tx, es, &next, cur.Version)
	if err != nil {
		return nil, err
	}
	if !ok {
		latest, err := store.GetMap(ctx, tx, es, cur.ID)
		if err != nil {
			return nil, notFound(err, ErrMapNotFound)
		}
		return nil, &configerr.VersionConflictError{Current: latest.Version}
	}

	// Spots: delete, free renamed labels, update, insert.
	if err := store.DeleteSpots(ctx, tx, es, cur.ID, deleted); err != nil {
		return nil, err
	}
	for _, id := range renamed {
		if err := store.SetSpotLabel(ctx, tx, es, cur.ID, id, "~"+id, now); err != nil {
			return nil, err
		}
	}
	for i := range updates {
		if err := store.UpdateSpot(ctx, tx, es, &updates[i]); err != nil {
			return nil, err
		}
	}
	if err := store.InsertSpots(ctx, tx, es, inserts); err != nil {
		return nil, err
	}

	// Decorations.
	keptDec := make(map[string]bool, len(in.Decorations))
	var decInserts []store.MapDecoration
	for _, d := range in.Decorations {
		if d.ID == "" {
			row := decorationRow(d, store.NewID(), cur.ID, now)
			row.CreatedAt = now
			decInserts = append(decInserts, row)
			sum.DecorationsChanged++
			continue
		}
		keptDec[d.ID] = true
		old := oldDecByID[d.ID]
		row := decorationRow(d, d.ID, cur.ID, now)
		row.CreatedAt = old.CreatedAt
		if decorationEqual(old, &row) {
			continue
		}
		if err := store.UpdateDecoration(ctx, tx, es, &row); err != nil {
			return nil, err
		}
		sum.DecorationsChanged++
	}
	var decDeleted []string
	for _, old := range oldDecs {
		if !keptDec[old.ID] {
			decDeleted = append(decDeleted, old.ID)
		}
	}
	sum.DecorationsChanged += len(decDeleted)
	if err := store.DeleteDecorations(ctx, tx, es, cur.ID, decDeleted); err != nil {
		return nil, err
	}
	if err := store.InsertDecorations(ctx, tx, es, decInserts); err != nil {
		return nil, err
	}

	// Implicit pools of the zones touched by the save (spec 7.5).
	sum.PoolsSynced, err = s.syncZonePools(ctx, tx, es, affectedZones)
	if err != nil {
		return nil, err
	}

	data := MapChangedData{MapID: cur.ID, EntityType: "map", EntityID: cur.ID, Action: action, Version: next.Version}
	if err := s.record(ctx, tx, p, es, "map."+action, data, sum); err != nil {
		return nil, err
	}
	return s.loadDocument(ctx, tx, es, cur.ID)
}

// ---- row generator ----------------------------------------------------------

// GenerateRows runs the row generator on a map. Without apply it is a
// preview and writes nothing. With apply the spots are appended to the map
// through a versioned save: expected (If-Match) is then required, and labels
// already on the map are refused with ErrLabelsTaken.
func (s *Service) GenerateRows(ctx context.Context, p *auth.Principal, establishmentID, mapID string, params GenerateParams, apply bool, expected *int64) (*GenerateResult, error) {
	es, err := s.configure(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if apply && expected == nil {
		return nil, configerr.ErrPreconditionRequired
	}
	if !apply {
		return s.previewRows(ctx, s.d.DB, es, mapID, params)
	}
	var res *GenerateResult
	err = s.inTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		res, err = s.previewRows(ctx, tx, es, mapID, params)
		if err != nil {
			return err
		}
		cur, err := store.GetMap(ctx, tx, es, mapID)
		if err != nil {
			return notFound(err, ErrMapNotFound)
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		if len(res.ConflictingLabels) > 0 {
			return fmt.Errorf("%w: %v", ErrLabelsTaken, res.ConflictingLabels)
		}
		in, err := s.currentInput(ctx, tx, es, cur)
		if err != nil {
			return err
		}
		in.Spots = append(in.Spots, slices.Clone(res.Spots)...)
		if err := validateDocument(&in).OrNil(); err != nil {
			return err
		}
		res.Document, err = s.applyDocument(ctx, tx, p, es, cur, in, "rows_generated")
		res.Applied = err == nil
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("mapping: generate rows: %w", err)
	}
	return res, nil
}

func (s *Service) previewRows(ctx context.Context, db bun.IDB, es store.EstablishmentScope, mapID string, params GenerateParams) (*GenerateResult, error) {
	m, err := store.GetMap(ctx, db, es, mapID)
	if err != nil {
		return nil, notFound(err, ErrMapNotFound)
	}
	params.SeaSide = m.SeaSide
	spots, err := Generate(params)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	z, err := store.GetZone(ctx, db, es, params.ZoneID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		ve.Add("zone_id", "Zona inesistente.")
	case err != nil:
		return nil, err
	case z.MapID == nil || *z.MapID != m.ID:
		ve.Add("zone_id", "La zona non appartiene a questa mappa.")
	case z.AssignmentMode == ModeQuantity:
		ve.Add("zone_id", "Le zone a quantità non hanno postazioni.")
	}
	mi := mapInputOf(m)
	for i, sp := range spots {
		inner := &configerr.ValidationError{}
		validateSpotFields(inner, "spot", sp, mi)
		if len(inner.Fields) > 0 {
			ve.Add("spots", fmt.Sprintf("La postazione %s (n. %d) non è valida: %s. Controlla origine, distanze e forma.",
				sp.Label, i+1, inner.Fields[0].Message))
			break
		}
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	existing, err := store.ListMapSpots(ctx, db, es, m.ID)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]bool, len(existing))
	for _, e := range existing {
		labels[e.Label] = true
	}
	conflicts := []string{}
	for _, sp := range spots {
		if labels[sp.Label] {
			conflicts = append(conflicts, sp.Label)
		}
	}
	return &GenerateResult{Spots: spots, ConflictingLabels: conflicts}, nil
}

// currentInput rebuilds the SaveInput of the stored document.
func (s *Service) currentInput(ctx context.Context, db bun.IDB, es store.EstablishmentScope, m *store.Map) (SaveInput, error) {
	doc, err := s.loadDocument(ctx, db, es, m.ID)
	if err != nil {
		return SaveInput{}, err
	}
	in := SaveInput{Map: mapInputOf(&doc.Map)}
	for _, sp := range doc.Spots {
		in.Spots = append(in.Spots, SpotInput{
			ID: sp.ID, ZoneID: sp.ZoneID, Label: sp.Label, RowLabel: sp.RowLabel, Number: sp.Number, Kind: sp.Kind,
			X: sp.X, Y: sp.Y, Rotation: sp.Rotation, Shape: sp.Shape, Capacity: sp.Capacity, MaxExtra: sp.MaxExtra,
			PriceCategoryOverride: sp.PriceCategoryOverride, Attributes: sp.Attributes, Active: sp.Active,
		})
	}
	for _, d := range doc.Decorations {
		in.Decorations = append(in.Decorations, DecorationInput{
			ID: d.ID, Kind: d.Kind, X: d.X, Y: d.Y, Rotation: d.Rotation, Shape: d.Shape, Label: d.Label,
		})
	}
	return in, nil
}

// ---- conversions ------------------------------------------------------------

func applyMapInput(m *store.Map, in MapInput) error {
	m.Name = cleanI18n(in.Name)
	m.Width, m.Height, m.GridSize = in.Width, in.Height, in.GridSize
	m.SeaSide = in.SeaSide
	m.SortOrder = in.SortOrder
	m.Background = nil
	if in.Background != nil {
		b, err := json.Marshal(in.Background)
		if err != nil {
			return fmt.Errorf("mapping: encode background: %w", err)
		}
		m.Background = store.JSON(b)
	}
	return nil
}

// mapInputOf is the MapInput of a stored map.
func mapInputOf(m *store.Map) MapInput {
	in := MapInput{Name: m.Name, Width: m.Width, Height: m.Height, GridSize: m.GridSize, SeaSide: m.SeaSide, SortOrder: m.SortOrder}
	if b := ParseBackground(m.Background); b != nil {
		in.Background = b
	}
	return in
}

// ParseBackground decodes a stored background (nil when absent or malformed).
func ParseBackground(raw store.JSON) *Background {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	b := new(Background)
	if err := json.Unmarshal(raw, b); err != nil {
		return nil
	}
	return b
}

func spotRow(in SpotInput, id, mapID string, now time.Time) store.Spot {
	return store.Spot{
		ID: id, MapID: mapID, ZoneID: in.ZoneID, Label: in.Label, RowLabel: in.RowLabel, Number: in.Number,
		Kind: in.Kind, X: in.X, Y: in.Y, Rotation: in.Rotation, Shape: in.Shape,
		Capacity: nonNilMap(in.Capacity), MaxExtra: nonNilMap(in.MaxExtra),
		PriceCategoryOverride: in.PriceCategoryOverride, Attributes: nonNilSlice(in.Attributes),
		Active: in.Active, UpdatedAt: now,
	}
}

func spotEqual(a, b *store.Spot) bool {
	return a.ZoneID == b.ZoneID && a.Label == b.Label && a.RowLabel == b.RowLabel && a.Number == b.Number &&
		a.Kind == b.Kind && a.X == b.X && a.Y == b.Y && a.Rotation == b.Rotation && a.Shape == b.Shape &&
		maps.Equal(a.Capacity, b.Capacity) && maps.Equal(a.MaxExtra, b.MaxExtra) &&
		equalPtr(a.PriceCategoryOverride, b.PriceCategoryOverride) && slices.Equal(a.Attributes, b.Attributes) &&
		a.Active == b.Active
}

func decorationRow(in DecorationInput, id, mapID string, now time.Time) store.MapDecoration {
	return store.MapDecoration{
		ID: id, MapID: mapID, Kind: in.Kind, X: in.X, Y: in.Y, Rotation: in.Rotation, Shape: in.Shape,
		Label: in.Label, UpdatedAt: now,
	}
}

func decorationEqual(a, b *store.MapDecoration) bool {
	return a.Kind == b.Kind && a.X == b.X && a.Y == b.Y && a.Rotation == b.Rotation && a.Shape == b.Shape &&
		maps.Equal(a.Label, b.Label)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func nonNilMap(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
