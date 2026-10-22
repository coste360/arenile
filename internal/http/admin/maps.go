package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of maps, zones, pools and the row generator (owned by the mapping
// builder, domain in internal/domain/mapping). Thin: decode, call the
// service, encode. Permissions are checked by the service.

// ---- maps -------------------------------------------------------------------

// ListMaps implements GET /establishments/{establishmentId}/maps.
func (h *Handlers) ListMaps(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListMapsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Mapping.ListMaps(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.MapList{Items: make([]api.Map, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items[i] = mapToAPI(page.Items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateMap implements POST /establishments/{establishmentId}/maps.
func (h *Handlers) CreateMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateMapParams) {
	var req api.MapInput
	if !decode(w, r, &req) {
		return
	}
	doc, err := h.Mapping.CreateMap(r.Context(), principalOf(r), establishmentID, mapInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, doc.Map.Version, documentToAPI(doc))
}

// DeleteMap implements DELETE /establishments/{establishmentId}/maps/{mapId}.
func (h *Handlers) DeleteMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID, params api.DeleteMapParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Mapping.DeleteMap(r.Context(), principalOf(r), establishmentID, mapID, expected); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetMap implements GET /establishments/{establishmentId}/maps/{mapId}.
func (h *Handlers) GetMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID) {
	doc, err := h.Mapping.GetMap(r.Context(), principalOf(r), establishmentID, mapID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, doc.Map.Version, documentToAPI(doc))
}

// SaveMap implements PUT /establishments/{establishmentId}/maps/{mapId}
// (If-Match required: 428 when missing, 412 when stale).
func (h *Handlers) SaveMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID, params api.SaveMapParams) {
	ifMatch := params.IfMatch
	m, err := RequireIfMatch(&ifMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.MapSaveRequest
	if !decode(w, r, &req) {
		return
	}
	expected, err := h.mapVersionFor(r, establishmentID, mapID, m)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	in := mapping.SaveInput{
		Map:         mapInputFromAPI(req.Map),
		Spots:       make([]mapping.SpotInput, len(req.Spots)),
		Decorations: make([]mapping.DecorationInput, len(req.Decorations)),
	}
	for i, s := range req.Spots {
		in.Spots[i] = spotInputFromAPI(s)
	}
	for i, d := range req.Decorations {
		in.Decorations[i] = decorationInputFromAPI(d)
	}
	doc, err := h.Mapping.SaveMap(r.Context(), principalOf(r), establishmentID, mapID, expected, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, doc.Map.Version, documentToAPI(doc))
}

// mapVersionFor resolves the version a mandatory If-Match stands for:
// the tag's version, or the current one for "*" (any version of an existing
// map; the versioned save still fails with 412 if it changes meanwhile).
func (h *Handlers) mapVersionFor(r *http.Request, establishmentID, mapID string, m IfMatch) (int64, error) {
	if !m.Any {
		return m.Version, nil
	}
	doc, err := h.Mapping.GetMap(r.Context(), principalOf(r), establishmentID, mapID)
	if err != nil {
		return 0, err
	}
	return doc.Map.Version, nil
}

// GenerateMapRows implements POST /establishments/{establishmentId}/maps/{mapId}/generate-rows:
// a preview, or (apply=true, If-Match required) an append to the map.
func (h *Handlers) GenerateMapRows(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID, params api.GenerateMapRowsParams) {
	var req api.RowGeneratorRequest
	if !decode(w, r, &req) {
		return
	}
	apply := req.Apply != nil && *req.Apply
	var expected *int64
	if apply {
		m, err := RequireIfMatch(params.IfMatch)
		if err != nil {
			problem.Error(w, r, h.Log, err)
			return
		}
		v, err := h.mapVersionFor(r, establishmentID, mapID, m)
		if err != nil {
			problem.Error(w, r, h.Log, err)
			return
		}
		expected = &v
	}
	res, err := h.Mapping.GenerateRows(r.Context(), principalOf(r), establishmentID, mapID, generateParamsFromAPI(req), apply, expected)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RowGeneratorResult{
		Spots:             make([]api.SpotInput, len(res.Spots)),
		ConflictingLabels: res.ConflictingLabels,
		Applied:           res.Applied,
	}
	if out.ConflictingLabels == nil {
		out.ConflictingLabels = []string{}
	}
	for i, s := range res.Spots {
		out.Spots[i] = spotInputToAPI(s)
	}
	if res.Document != nil {
		d := documentToAPI(res.Document)
		out.Document = &d
		w.Header().Set("ETag", ETag(res.Document.Map.Version))
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- zones ------------------------------------------------------------------

// ListZones implements GET /establishments/{establishmentId}/zones.
func (h *Handlers) ListZones(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListZonesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Mapping.ListZones(r.Context(), principalOf(r), establishmentID, params.MapId, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ZoneList{Items: make([]api.Zone, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items[i] = zoneToAPI(&page.Items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateZone implements POST /establishments/{establishmentId}/zones.
func (h *Handlers) CreateZone(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateZoneParams) {
	var req api.ZoneInput
	if !decode(w, r, &req) {
		return
	}
	z, err := h.Mapping.CreateZone(r.Context(), principalOf(r), establishmentID, zoneInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, z.Zone.Version, zoneToAPI(z))
}

// DeleteZone implements DELETE /establishments/{establishmentId}/zones/{zoneId}.
func (h *Handlers) DeleteZone(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, zoneID api.ZoneID, params api.DeleteZoneParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Mapping.DeleteZone(r.Context(), principalOf(r), establishmentID, zoneID, expected); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetZone implements GET /establishments/{establishmentId}/zones/{zoneId}.
func (h *Handlers) GetZone(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, zoneID api.ZoneID) {
	z, err := h.Mapping.GetZone(r.Context(), principalOf(r), establishmentID, zoneID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, z.Zone.Version, zoneToAPI(z))
}

// UpdateZone implements PUT /establishments/{establishmentId}/zones/{zoneId}.
func (h *Handlers) UpdateZone(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, zoneID api.ZoneID, params api.UpdateZoneParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ZoneInput
	if !decode(w, r, &req) {
		return
	}
	z, err := h.Mapping.UpdateZone(r.Context(), principalOf(r), establishmentID, zoneID, expected, zoneInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, z.Zone.Version, zoneToAPI(z))
}

// ---- pools ------------------------------------------------------------------

// ListPools implements GET /establishments/{establishmentId}/pools.
func (h *Handlers) ListPools(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPoolsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Mapping.ListPools(r.Context(), principalOf(r), establishmentID, params.ZoneId, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PoolList{Items: make([]api.Pool, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items[i] = poolToAPI(&page.Items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePool implements POST /establishments/{establishmentId}/pools.
func (h *Handlers) CreatePool(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePoolParams) {
	var req api.PoolInput
	if !decode(w, r, &req) {
		return
	}
	pl, err := h.Mapping.CreatePool(r.Context(), principalOf(r), establishmentID, poolInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, pl.Version, poolToAPI(pl))
}

// DeletePool implements DELETE /establishments/{establishmentId}/pools/{poolId}.
func (h *Handlers) DeletePool(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, poolID api.PoolID, params api.DeletePoolParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Mapping.DeletePool(r.Context(), principalOf(r), establishmentID, poolID, expected); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPool implements GET /establishments/{establishmentId}/pools/{poolId}.
func (h *Handlers) GetPool(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, poolID api.PoolID) {
	pl, err := h.Mapping.GetPool(r.Context(), principalOf(r), establishmentID, poolID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, pl.Version, poolToAPI(pl))
}

// UpdatePool implements PUT /establishments/{establishmentId}/pools/{poolId}.
func (h *Handlers) UpdatePool(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, poolID api.PoolID, params api.UpdatePoolParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.PoolInput
	if !decode(w, r, &req) {
		return
	}
	pl, err := h.Mapping.UpdatePool(r.Context(), principalOf(r), establishmentID, poolID, expected, poolInputFromAPI(req))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, pl.Version, poolToAPI(pl))
}

// ---- conversions ------------------------------------------------------------

func mapInputFromAPI(in api.MapInput) mapping.MapInput {
	out := mapping.MapInput{
		Name: store.I18nText(in.Name), Width: in.Width, Height: in.Height, GridSize: in.GridSize,
		SeaSide: string(in.SeaSide), SortOrder: mapsDerefOr(in.SortOrder, 0),
	}
	if in.Background != nil {
		b := &mapping.Background{}
		// Same JSON shape (api.MapBackground <-> mapping.Background).
		if err := ConvertJSON(in.Background, b); err == nil {
			out.Background = b
		} else {
			out.Background = &mapping.Background{Type: string(in.Background.Type)}
		}
	}
	return out
}

func mapToAPI(v mapping.MapView) api.Map {
	m := v.Map
	out := api.Map{
		Id: m.ID, Name: api.I18nText(mapsNonNilText(m.Name)), Width: m.Width, Height: m.Height, GridSize: m.GridSize,
		SeaSide: api.SeaSide(m.SeaSide), SortOrder: m.SortOrder, SpotCount: v.SpotCount, Version: m.Version,
		CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
	}
	if b := mapping.ParseBackground(m.Background); b != nil {
		var ab api.MapBackground
		if err := ConvertJSON(b, &ab); err == nil {
			out.Background = &ab
		}
	}
	return out
}

func documentToAPI(d *mapping.Document) api.MapDocument {
	out := api.MapDocument{
		Map:         mapToAPI(d.MapView),
		Spots:       make([]api.Spot, len(d.Spots)),
		Decorations: make([]api.Decoration, len(d.Decorations)),
	}
	for i := range d.Spots {
		out.Spots[i] = spotToAPI(&d.Spots[i])
	}
	for i := range d.Decorations {
		out.Decorations[i] = decorationToAPI(&d.Decorations[i])
	}
	return out
}

func spotToAPI(s *store.Spot) api.Spot {
	return api.Spot{
		Id: s.ID, MapId: s.MapID, ZoneId: s.ZoneID, Label: s.Label, RowLabel: s.RowLabel, Number: s.Number,
		Kind: api.SpotKind(s.Kind), X: s.X, Y: s.Y, Rotation: s.Rotation, Shape: mapsShapeToAPI(s.Shape),
		Capacity: api.ComponentQuantities(mapsNonNilQty(s.Capacity)), MaxExtra: api.ComponentQuantities(mapsNonNilQty(s.MaxExtra)),
		PriceCategoryOverride: s.PriceCategoryOverride, Attributes: nonNil(s.Attributes), Active: s.Active,
	}
}

func decorationToAPI(d *store.MapDecoration) api.Decoration {
	out := api.Decoration{
		Id: d.ID, MapId: d.MapID, Kind: api.DecorationKind(d.Kind), X: d.X, Y: d.Y, Rotation: d.Rotation,
		Shape: mapsShapeToAPI(d.Shape),
	}
	if len(d.Label) > 0 {
		l := api.I18nText(d.Label)
		out.Label = &l
	}
	return out
}

func spotInputFromAPI(s api.SpotInput) mapping.SpotInput {
	out := mapping.SpotInput{
		ID: mapsDerefOr(s.Id, ""), ZoneID: s.ZoneId, Label: s.Label, RowLabel: mapsDerefOr(s.RowLabel, ""),
		Number: mapsDerefOr(s.Number, 0), Kind: string(s.Kind), X: s.X, Y: s.Y, Rotation: mapsDerefOr(s.Rotation, 0),
		Shape: mapsShapeFromAPI(s.Shape), PriceCategoryOverride: s.PriceCategoryOverride,
		Active: mapsDerefOr(s.Active, true),
	}
	if s.Capacity != nil {
		out.Capacity = map[string]int(*s.Capacity)
	}
	if s.MaxExtra != nil {
		out.MaxExtra = map[string]int(*s.MaxExtra)
	}
	if s.Attributes != nil {
		out.Attributes = *s.Attributes
	}
	return out
}

func spotInputToAPI(s mapping.SpotInput) api.SpotInput {
	capacity := api.ComponentQuantities(mapsNonNilQty(s.Capacity))
	maxExtra := api.ComponentQuantities(mapsNonNilQty(s.MaxExtra))
	attrs := nonNil(s.Attributes)
	out := api.SpotInput{
		ZoneId: s.ZoneID, Label: s.Label, RowLabel: &s.RowLabel, Number: &s.Number, Kind: api.SpotKind(s.Kind),
		X: s.X, Y: s.Y, Rotation: &s.Rotation, Shape: mapsShapeToAPI(s.Shape), Capacity: &capacity, MaxExtra: &maxExtra,
		PriceCategoryOverride: s.PriceCategoryOverride, Attributes: &attrs, Active: &s.Active,
	}
	if s.ID != "" {
		id := s.ID
		out.Id = &id
	}
	return out
}

func decorationInputFromAPI(d api.DecorationInput) mapping.DecorationInput {
	out := mapping.DecorationInput{
		ID: mapsDerefOr(d.Id, ""), Kind: string(d.Kind), X: d.X, Y: d.Y, Rotation: mapsDerefOr(d.Rotation, 0),
		Shape: mapsShapeFromAPI(d.Shape),
	}
	if d.Label != nil {
		out.Label = store.I18nText(*d.Label)
	}
	return out
}

func generateParamsFromAPI(req api.RowGeneratorRequest) mapping.GenerateParams {
	p := mapping.GenerateParams{
		ZoneID: req.ZoneId, Rows: req.Rows, SpotsPerRow: req.SpotsPerRow,
		OriginX: mapsDerefOr(req.OriginX, 0), OriginY: mapsDerefOr(req.OriginY, 0),
		SpacingX: req.SpacingX, SpacingY: req.SpacingY,
		FirstRowLabel: mapsDerefOr(req.FirstRowLabel, ""), NumberStart: req.NumberStart,
		LabelFormat: mapsDerefOr(req.LabelFormat, ""), Rotation: req.Rotation,
	}
	if req.RowLabelStyle != nil {
		p.RowLabelStyle = string(*req.RowLabelStyle)
	}
	if req.Numbering != nil {
		p.Numbering = string(*req.Numbering)
	}
	if req.Direction != nil {
		p.Direction = string(*req.Direction)
	}
	if req.Kind != nil {
		p.Kind = string(*req.Kind)
	}
	if req.Shape != nil {
		sh := mapsShapeFromAPI(*req.Shape)
		p.Shape = &sh
	}
	if req.Capacity != nil {
		p.Capacity = map[string]int(*req.Capacity)
	}
	return p
}

func zoneInputFromAPI(in api.ZoneInput) mapping.ZoneInput {
	out := mapping.ZoneInput{
		MapID: in.MapId, Code: in.Code, Name: store.I18nText(in.Name), AssignmentMode: string(in.AssignmentMode),
		OnlineVisible: mapsDerefOr(in.OnlineVisible, true), PriceCategory: in.PriceCategory,
		AutoAssign: mapsDerefOr(in.AutoAssign, false), SortOrder: mapsDerefOr(in.SortOrder, 0),
	}
	if in.OnlineQuota != nil {
		out.OnlineQuota = &store.OnlineQuota{Type: string(in.OnlineQuota.Type), Value: in.OnlineQuota.Value}
	}
	return out
}

func zoneToAPI(v *mapping.ZoneView) api.Zone {
	z := v.Zone
	out := api.Zone{
		Id: z.ID, MapId: z.MapID, Code: z.Code, Name: api.I18nText(mapsNonNilText(z.Name)),
		AssignmentMode: api.AssignmentMode(z.AssignmentMode), OnlineVisible: z.OnlineVisible,
		PriceCategory: z.PriceCategory, AutoAssign: z.AutoAssign, SortOrder: z.SortOrder,
		SpotCount: v.SpotCount, PoolId: v.PoolID, Version: z.Version,
		CreatedAt: z.CreatedAt.UTC(), UpdatedAt: z.UpdatedAt.UTC(),
	}
	if z.OnlineQuota != nil {
		out.OnlineQuota = &api.OnlineQuota{Type: api.OnlineQuotaType(z.OnlineQuota.Type), Value: z.OnlineQuota.Value}
	}
	return out
}

func poolInputFromAPI(in api.PoolInput) mapping.PoolInput {
	out := mapping.PoolInput{
		ZoneID: in.ZoneId, ComponentCode: in.ComponentCode, Name: store.I18nText(in.Name), Capacity: in.Capacity,
	}
	if in.CapacityOverrides != nil {
		out.CapacityOverrides = make([]store.CapacityOverride, len(*in.CapacityOverrides))
		for i, o := range *in.CapacityOverrides {
			out.CapacityOverrides[i] = store.CapacityOverride{From: o.From, To: o.To, Capacity: o.Capacity}
		}
	}
	return out
}

func poolToAPI(pl *store.Pool) api.Pool {
	out := api.Pool{
		Id: pl.ID, ZoneId: pl.ZoneID, ComponentCode: pl.ComponentCode, Name: api.I18nText(mapsNonNilText(pl.Name)),
		Capacity: pl.Capacity, CapacityOverrides: make([]api.CapacityOverride, len(pl.CapacityOverrides)),
		SystemManaged: pl.SystemManaged, Version: pl.Version,
		CreatedAt: pl.CreatedAt.UTC(), UpdatedAt: pl.UpdatedAt.UTC(),
	}
	for i, o := range pl.CapacityOverrides {
		out.CapacityOverrides[i] = api.CapacityOverride{From: o.From, To: o.To, Capacity: o.Capacity}
	}
	return out
}

func mapsShapeToAPI(s store.Shape) api.Shape {
	return api.Shape{Type: api.ShapeType(s.Type), W: s.W, H: s.H}
}

func mapsShapeFromAPI(s api.Shape) store.Shape {
	return store.Shape{Type: string(s.Type), W: s.W, H: s.H}
}

func mapsDerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

func mapsNonNilText(t store.I18nText) store.I18nText {
	if t == nil {
		return store.I18nText{}
	}
	return t
}

func mapsNonNilQty(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}
