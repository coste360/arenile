package admin

import (
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the inventory domain (internal/domain/inventory): holds,
// availability, operations day, planning and spot blocks. Thin: decode,
// call the service (which checks permissions), encode.

// CreateHold implements POST /api/admin/v1/establishments/{establishmentId}/holds.
func (h *Handlers) CreateHold(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateHoldParams) {
	var req api.HoldCreate
	if !decode(w, r, &req) {
		return
	}
	in := inventory.HoldInput{
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots),
		Items: make([]store.HoldItem, len(req.Items)), IP: reqctx.From(r.Context()).ClientIP,
	}
	if req.Channel != nil {
		in.Channel = string(*req.Channel)
	}
	if req.Note != nil {
		in.Note = *req.Note
	}
	if req.TtlSeconds != nil {
		in.TTL = time.Duration(*req.TtlSeconds) * time.Second
	}
	for i, it := range req.Items {
		in.Items[i] = store.HoldItem{SpotID: invDeref(it.SpotId), ZoneID: invDeref(it.ZoneId), PoolID: invDeref(it.PoolId), Quantity: invDerefInt(it.Quantity)}
	}
	hv, err := h.Inventory.CreateHold(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, invHoldToAPI(hv))
}

// GetHold implements GET /api/admin/v1/establishments/{establishmentId}/holds/{holdId}.
func (h *Handlers) GetHold(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, holdID api.HoldID) {
	hv, err := h.Inventory.GetHold(r.Context(), principalOf(r), establishmentID, holdID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, invHoldToAPI(hv))
}

// ReleaseHold implements DELETE /api/admin/v1/establishments/{establishmentId}/holds/{holdId}.
func (h *Handlers) ReleaseHold(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, holdID api.HoldID, _ api.ReleaseHoldParams) {
	if err := h.Inventory.ReleaseHold(r.Context(), principalOf(r), establishmentID, holdID, reqctx.From(r.Context()).ClientIP); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAvailability implements GET /api/admin/v1/establishments/{establishmentId}/availability.
func (h *Handlers) GetAvailability(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetAvailabilityParams) {
	q := inventory.AvailabilityQuery{From: params.From, To: params.To, MapID: invDeref(params.MapId), ZoneID: invDeref(params.ZoneId), PackID: invDeref(params.PackId)}
	if params.Slots != nil {
		q.Slots = calendar.Slots(*params.Slots)
	}
	if params.Channel != nil {
		q.Channel = string(*params.Channel)
	}
	res, err := h.Inventory.GetAvailability(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.AvailabilityResult{
		Channel: api.Channel(res.Channel), From: res.From, To: res.To, Slots: api.Slots(res.Slots),
		Zones: make([]api.ZoneAvailability, len(res.Zones)), Pools: invPoolsToAPI(res.Pools),
	}
	if len(res.ClosedDates) > 0 {
		cd := res.ClosedDates
		out.ClosedDates = &cd
	}
	for i, z := range res.Zones {
		za := api.ZoneAvailability{
			ZoneId: z.ZoneID, Code: z.Code, AssignmentMode: api.AssignmentMode(z.Mode), Capacity: z.Capacity, Available: z.Available,
		}
		if z.Spots != nil {
			spots := make([]api.SpotAvailability, len(z.Spots))
			for k, sp := range z.Spots {
				st := api.SpotAvailabilityStatusUnavailable
				if sp.Available {
					st = api.SpotAvailabilityStatusAvailable
				}
				spots[k] = api.SpotAvailability{SpotId: sp.SpotID, Label: sp.Label, Status: st}
				spotAvailabilityDays(&z.Spots[k], &spots[k])
				if len(sp.Conflicts) > 0 {
					cs := invConflictsToAPI(sp.Conflicts)
					spots[k].Conflicts = &cs
				}
			}
			za.Spots = &spots
		}
		out.Zones[i] = za
	}
	writeJSON(w, http.StatusOK, out)
}

// GetOperationsDay implements GET /api/admin/v1/establishments/{establishmentId}/operations/day.
func (h *Handlers) GetOperationsDay(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetOperationsDayParams) {
	q := inventory.OperationsQuery{Date: params.Date, MapID: invDeref(params.MapId)}
	if params.Slot != nil {
		q.Slot = calendar.Slots(*params.Slot)
	}
	day, err := h.Inventory.GetOperationsDay(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.OperationsDay{
		Date: day.Date, Slot: api.Slots(day.Slot), Closed: day.Closed,
		Spots:              make([]api.OperationalSpot, len(day.Spots)),
		PendingAssignments: make([]api.PendingAssignment, len(day.PendingAssignments)),
		Pools:              invPoolsToAPI(day.Pools),
		Totals: api.OperationsTotals{
			Spots: day.Totals.Spots, Occupied: day.Totals.Occupied, Free: day.Totals.Free, Arrivals: day.Totals.Arrivals,
			Departures: day.Totals.Departures, CheckedIn: day.Totals.CheckedIn, PendingAssignments: day.Totals.PendingAssignments,
			BalanceDueAmount: int64(day.Totals.BalanceDue),
		},
	}
	for i, sp := range day.Spots {
		os := api.OperationalSpot{
			SpotId: sp.SpotID, Label: sp.Label, MapId: sp.MapID, ZoneId: sp.ZoneID,
			Status: api.OperationalSpotStatus(sp.Status), HoldExpiresAt: sp.HoldExpiresAt,
			HoldId: invOpt(sp.HoldID), BlockId: invOpt(sp.BlockID), ReleaseId: invOpt(sp.ReleaseID),
		}
		if sp.BlockID != "" {
			os.BlockReason = &day.Spots[i].BlockReason
		}
		if sp.AMStatus != "" {
			am, pm := api.OperationalSpotStatus(sp.AMStatus), api.OperationalSpotStatus(sp.PMStatus)
			os.AmStatus, os.PmStatus = &am, &pm
		}
		if b := sp.Booking; b != nil {
			os.Booking = &api.OperationalBooking{
				BookingId: b.BookingID, Code: b.Code, Status: api.BookingStatus(b.Status), Channel: api.Channel(b.Channel),
				CustomerId: b.CustomerID, CustomerDisplayName: b.CustomerDisplayName, CustomerTags: b.CustomerTags,
				OccupantLabel: b.OccupantLabel, CustomerAnonymized: b.CustomerAnonymized,
				StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots), IsSeasonPass: b.IsSeasonPass,
				LineId: invOpt(b.LineID), PackCodes: b.PackCodes, TotalAmount: int64(b.TotalAmount),
				PaidAmount: int64(b.PaidAmount), BalanceDue: int64(b.BalanceDue), HasExtras: b.HasExtras, HasNotes: b.HasNotes,
				Version: b.Version, Ticket: opTicketToAPI(b.Ticket),
			}
			if b.Test {
				os.Booking.Test = &b.Test
			}
			operationalMarksToAPI(&b.OperationalMarks, os.Booking)
		}
		out.Spots[i] = os
	}
	for i, pa := range day.PendingAssignments {
		out.PendingAssignments[i] = api.PendingAssignment{
			BookingId: pa.BookingID, LineId: pa.LineID, Code: pa.Code, CustomerDisplayName: pa.CustomerDisplayName,
			ZoneId: pa.ZoneID, StartsOn: pa.StartsOn, EndsOn: pa.EndsOn, Slots: api.Slots(pa.Slots),
			Ticket: opTicketToAPI(pa.Ticket),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetPlanning implements GET /api/admin/v1/establishments/{establishmentId}/planning.
func (h *Handlers) GetPlanning(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetPlanningParams) {
	pl, err := h.Inventory.GetPlanning(r.Context(), principalOf(r), establishmentID, inventory.PlanningQuery{
		From: params.From, To: params.To, MapID: invDeref(params.MapId), ZoneID: invDeref(params.ZoneId),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.Planning{
		From: pl.From, To: pl.To, Days: pl.Days, ClosedDays: pl.ClosedDays,
		Spots:   make([]api.PlanningSpot, len(pl.Spots)),
		Entries: make([]api.PlanningEntry, len(pl.Entries)),
		Rows:    make([]api.PlanningRow, len(pl.Rows)),
	}
	for i, sp := range pl.Spots {
		out.Spots[i] = api.PlanningSpot{SpotId: sp.SpotID, Label: sp.Label, MapId: sp.MapID, ZoneId: sp.ZoneID, RowLabel: invOpt(sp.RowLabel)}
	}
	for i, e := range pl.Entries {
		pe := api.PlanningEntry{
			Kind: api.PlanningEntryKind(e.Kind), Label: e.Label, BookingId: invOpt(e.BookingID),
			HoldId: invOpt(e.HoldID), BlockId: invOpt(e.BlockID), Code: invOpt(e.Code), BalanceDue: e.BalanceDue,
		}
		if e.Status != "" {
			st := api.BookingStatus(e.Status)
			pe.Status = &st
		}
		if e.Channel != "" {
			ch := api.Channel(e.Channel)
			pe.Channel = &ch
		}
		out.Entries[i] = pe
	}
	for i, row := range pl.Rows {
		segs := make([]api.PlanningSegment, len(row.Segments))
		for k, sg := range row.Segments {
			segs[k] = api.PlanningSegment{Entry: sg.Entry, From: sg.From, To: sg.To, Slots: api.Slots(sg.Slots)}
		}
		out.Rows[i] = api.PlanningRow{Spot: row.Spot, Segments: segs}
	}
	writeJSON(w, http.StatusOK, out)
}

// ListSpotBlocks implements GET /api/admin/v1/establishments/{establishmentId}/spot-blocks.
func (h *Handlers) ListSpotBlocks(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListSpotBlocksParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := store.SpotBlockFilter{SpotID: invDeref(params.SpotId)}
	if params.From != nil {
		f.From = *params.From
	}
	if params.To != nil {
		f.To = *params.To
	}
	page, err := h.Inventory.ListSpotBlocks(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.SpotBlockList{Items: invBlocksToAPI(page.Items), NextCursor: nextCursor(page.NextCursor)})
}

// CreateSpotBlock implements POST /api/admin/v1/establishments/{establishmentId}/spot-blocks.
func (h *Handlers) CreateSpotBlock(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateSpotBlockParams) {
	var req api.SpotBlockCreate
	if !decode(w, r, &req) {
		return
	}
	blocks, err := h.Inventory.CreateSpotBlocks(r.Context(), principalOf(r), establishmentID, inventory.SpotBlockInput{
		SpotIDs: req.SpotIds, StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots),
		Reason: invDeref(req.Reason), IP: reqctx.From(r.Context()).ClientIP,
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.SpotBlockList{Items: invBlocksToAPI(blocks)})
}

// DeleteSpotBlock implements DELETE /api/admin/v1/establishments/{establishmentId}/spot-blocks/{blockId}.
func (h *Handlers) DeleteSpotBlock(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, blockID api.SpotBlockID, _ api.DeleteSpotBlockParams) {
	if err := h.Inventory.DeleteSpotBlock(r.Context(), principalOf(r), establishmentID, blockID, reqctx.From(r.Context()).ClientIP); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- mapping helpers -------------------------------------------------------

func invHoldToAPI(hv *inventory.HoldView) api.Hold {
	out := api.Hold{
		Id: hv.ID, Channel: api.Channel(hv.Channel), StartsOn: hv.StartsOn, EndsOn: hv.EndsOn, Slots: api.Slots(hv.Slots),
		ExpiresAt: hv.ExpiresAt, CreatedAt: hv.CreatedAt, Status: api.HoldStatus(hv.Status),
		ConvertedBookingId: hv.ConvertedBookingID, Note: invOpt(hv.Note), Items: make([]api.HoldItem, len(hv.Items)),
	}
	for i, it := range hv.Items {
		out.Items[i] = api.HoldItem{SpotId: invOpt(it.SpotID), ZoneId: invOpt(it.ZoneID), PoolId: invOpt(it.PoolID)}
		if it.Quantity > 0 {
			q := it.Quantity
			out.Items[i].Quantity = &q
		}
	}
	return out
}

func invPoolsToAPI(pools []inventory.PoolAvailability) []api.PoolAvailability {
	out := make([]api.PoolAvailability, len(pools))
	for i, p := range pools {
		days := make([]api.PoolDay, len(p.Days))
		for k, d := range p.Days {
			days[k] = api.PoolDay{Date: d.Date, Slot: api.Slot(d.Slot), Capacity: d.Capacity, Used: d.Used, Held: d.Held, Available: d.Available}
		}
		out[i] = api.PoolAvailability{PoolId: p.PoolID, ZoneId: p.ZoneID, ComponentCode: p.ComponentCode, Capacity: p.Capacity, Available: p.Available, Days: days}
	}
	return out
}

func invBlocksToAPI(blocks []store.SpotBlock) []api.SpotBlock {
	out := make([]api.SpotBlock, len(blocks))
	for i, b := range blocks {
		out[i] = api.SpotBlock{
			Id: b.ID, SpotId: b.SpotID, StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots),
			Reason: b.Reason, CreatedAt: b.CreatedAt,
		}
	}
	return out
}

func invConflictsToAPI(cs []availability.Conflict) []api.AllocationConflict {
	out := make([]api.AllocationConflict, len(cs))
	for i, c := range cs {
		out[i] = api.AllocationConflict{
			Date: c.Date, Slot: api.Slot(c.Slot), Reason: api.AllocationConflictReason(c.Reason),
			SpotId: invOpt(c.SpotID), PoolId: invOpt(c.PoolID), ZoneId: invOpt(c.ZoneID),
		}
	}
	return out
}

func invDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func invDerefInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func invOpt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
