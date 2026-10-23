package inventory

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func d(s string) calendar.Date { return calendar.MustParse(s) }

var t0 = time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)

func testSnapshot() *snapshot {
	zoneID := "z-zone"
	qtyID := "z-qty"
	return &snapshot{
		zones: map[string]*store.Zone{
			"z-seat": {ID: "z-seat", AssignmentMode: modeSeat, OnlineQuota: &store.OnlineQuota{Type: "percent", Value: 50}},
			"z-zone": {ID: "z-zone", AssignmentMode: modeZone},
			"z-qty":  {ID: "z-qty", AssignmentMode: modeQuantity, OnlineQuota: &store.OnlineQuota{Type: "count", Value: 2}},
		},
		spots: map[string]*store.Spot{
			"s1":  {ID: "s1", ZoneID: "z-seat", Active: true},
			"s2":  {ID: "s2", ZoneID: "z-seat", Active: false},
			"zs1": {ID: "zs1", ZoneID: "z-zone", Active: true},
		},
		pools: map[string]*store.Pool{
			"p-zone":   {ID: "p-zone", ZoneID: &zoneID, Capacity: 3, SystemManaged: true},
			"p-qty":    {ID: "p-qty", ZoneID: &qtyID, Capacity: 4, CapacityOverrides: []store.CapacityOverride{{From: d("2027-08-10"), To: d("2027-08-20"), Capacity: 1}}},
			"p-sunbed": {ID: "p-sunbed", Capacity: 10},
		},
		zonePool:      map[string]string{"z-zone": "p-zone", "z-qty": "p-qty"},
		componentPool: map[string]string{"sunbed": "p-sunbed", "qtyunit": "p-qty"},
		closed:        closedSet{{Date: d("2027-07-02"), Slot: calendar.PM}: true},
	}
}

func seqID() func() string {
	n := 0
	return func() string { n++; return "id" + strconv.Itoa(n) }
}

func TestBuildPlan_Scenarios(t *testing.T) {
	exp := t0.Add(10 * time.Minute)
	base := AllocateRequest{BookingID: "b1", Channel: store.ChannelDesk, StartsOn: d("2027-07-01"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsFull, Kind: KindBooking}
	with := func(f func(*AllocateRequest)) AllocateRequest {
		r := base
		f(&r)
		return r
	}
	tests := []struct {
		name          string
		req           AllocateRequest
		wantErr       error
		wantSpotRows  int
		wantDemands   map[string]int // pool -> quantity
		wantStatus    []string
		wantConflicts int
		wantSeatQuota int
	}{
		{
			name: "seat full day skips closed PM", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}} }),
			wantSpotRows: 3, wantStatus: []string{store.AssignmentAssigned},
		},
		{
			name: "seat AM only", req: with(func(r *AllocateRequest) {
				r.Slots = calendar.SlotsAM
				r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1", Components: map[string]int{"sunbed": 2}}}
			}),
			wantSpotRows: 2, wantDemands: map[string]int{"p-sunbed": 2}, wantStatus: []string{store.AssignmentAssigned},
		},
		{
			name: "zone mode pending consumes implicit pool", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", ZoneID: "z-zone"}} }),
			wantDemands: map[string]int{"p-zone": 1}, wantStatus: []string{store.AssignmentPending},
		},
		{
			name: "zone mode assigned also writes spot rows", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", SpotID: "zs1"}} }),
			wantSpotRows: 3, wantDemands: map[string]int{"p-zone": 1}, wantStatus: []string{store.AssignmentAssigned},
		},
		{
			name: "quantity zone and component on same pool take max", req: with(func(r *AllocateRequest) {
				r.Lines = []LineRequest{{LineID: "l1", ZoneID: "z-qty", Quantity: 2, Components: map[string]int{"qtyunit": 2, "sunbed": 1}}}
			}),
			wantDemands: map[string]int{"p-qty": 2, "p-sunbed": 1}, wantStatus: []string{store.AssignmentNotApplicable},
		},
		{
			name: "panel hold on a pool", req: AllocateRequest{
				HoldID: "h1", Channel: store.ChannelDesk, StartsOn: d("2027-07-01"), EndsOn: d("2027-07-01"), Slots: calendar.SlotsAM,
				Kind: KindHold, ExpiresAt: &exp, Lines: []LineRequest{{PoolID: "p-sunbed", Quantity: 3}},
			},
			wantDemands: map[string]int{"p-sunbed": 3}, wantStatus: []string{store.AssignmentNotApplicable},
		},
		{
			name: "online seat with quota is tracked", req: with(func(r *AllocateRequest) {
				r.Channel = store.ChannelOnline
				r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}}
			}),
			wantSpotRows: 3, wantStatus: []string{store.AssignmentAssigned}, wantSeatQuota: 1,
		},
		{
			name: "inactive spot is a conflict", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", SpotID: "s2"}} }),
			wantSpotRows: 3, wantStatus: []string{store.AssignmentAssigned}, wantConflicts: 3,
		},
		{name: "seat without spot", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", ZoneID: "z-seat"}} }), wantErr: configerr.ErrValidation},
		{name: "spot of another zone", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", ZoneID: "z-zone", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
		{name: "unknown spot", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", SpotID: "nope"}} }), wantErr: configerr.ErrValidation},
		{name: "hold without expiry", req: with(func(r *AllocateRequest) { r.Kind = KindHold; r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
		{name: "booking with expiry", req: with(func(r *AllocateRequest) { r.ExpiresAt = &exp; r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
		{name: "two owners", req: with(func(r *AllocateRequest) { r.HoldID = "h"; r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
		{name: "no lines", req: base, wantErr: configerr.ErrValidation},
		{name: "reversed dates", req: with(func(r *AllocateRequest) {
			r.EndsOn = d("2027-06-01")
			r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}}
		}), wantErr: configerr.ErrValidation},
		{name: "bad channel", req: with(func(r *AllocateRequest) { r.Channel = "fax"; r.Lines = []LineRequest{{LineID: "l1", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
		{name: "quantity zone with spot", req: with(func(r *AllocateRequest) { r.Lines = []LineRequest{{LineID: "l1", ZoneID: "z-qty", SpotID: "s1"}} }), wantErr: configerr.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, conflicts, err := buildPlan(tt.req, testSnapshot(), seqID(), t0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(p.spotRows) != tt.wantSpotRows {
				t.Fatalf("spot rows = %d, want %d", len(p.spotRows), tt.wantSpotRows)
			}
			got := map[string]int{}
			for _, dm := range p.demands {
				got[dm.poolID] += dm.quantity
			}
			if len(got) != len(tt.wantDemands) {
				t.Fatalf("demands = %v, want %v", got, tt.wantDemands)
			}
			for k, v := range tt.wantDemands {
				if got[k] != v {
					t.Fatalf("demands = %v, want %v", got, tt.wantDemands)
				}
			}
			for i, st := range tt.wantStatus {
				if p.lines[i].AssignmentStatus != st {
					t.Fatalf("line %d status = %s, want %s", i, p.lines[i].AssignmentStatus, st)
				}
			}
			if len(conflicts) != tt.wantConflicts {
				t.Fatalf("conflicts = %v", conflicts)
			}
			if len(p.seatQuotaZones) != tt.wantSeatQuota {
				t.Fatalf("seat quota zones = %v", p.seatQuotaZones)
			}
			for _, r := range p.spotRows {
				if (r.Kind == store.AllocationHold) != (r.ExpiresAt != nil) {
					t.Fatalf("INV-06 violated: %+v", r)
				}
				if r.Date.Equal(d("2027-07-02")) && r.Slot == calendar.PM {
					t.Fatalf("closed half-day allocated: %+v", r)
				}
			}
		})
	}
}

func TestEffectiveCapacity_Overrides(t *testing.T) {
	pl := testSnapshot().pools["p-qty"]
	tests := []struct {
		date string
		want int
	}{{"2027-08-09", 4}, {"2027-08-10", 1}, {"2027-08-20", 1}, {"2027-08-21", 4}}
	for _, tt := range tests {
		if got := effectiveCapacity(pl, d(tt.date)); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.date, got, tt.want)
		}
	}
}

func TestOnlineLimit_Scenarios(t *testing.T) {
	snap := testSnapshot()
	if onlineLimit(snap, "p-qty", store.ChannelDesk, d("2027-07-01")) != nil {
		t.Fatal("desk ignores quota")
	}
	if l := onlineLimit(snap, "p-qty", store.ChannelOnline, d("2027-07-01")); l == nil || *l != 2 {
		t.Fatalf("online limit = %v", l)
	}
	if l := onlineLimit(snap, "p-qty", store.ChannelAPI, d("2027-08-15")); l == nil || *l != 1 {
		t.Fatalf("api limit with override = %v", l)
	}
	if onlineLimit(snap, "p-sunbed", store.ChannelOnline, d("2027-07-01")) != nil {
		t.Fatal("pool without zone has no quota")
	}
}

func TestPoolDeltas_Release(t *testing.T) {
	k := func(pool string) []store.PoolAllocation {
		return []store.PoolAllocation{
			{PoolID: pool, Date: d("2027-07-01"), Slot: calendar.AM, Kind: store.AllocationBooking, Channel: store.ChannelDesk, Quantity: 2},
			{PoolID: pool, Date: d("2027-07-01"), Slot: calendar.AM, Kind: store.AllocationHold, Channel: store.ChannelOnline, Quantity: 1},
			{PoolID: pool, Date: d("2027-07-01"), Slot: calendar.PM, Kind: store.AllocationBooking, Channel: store.ChannelAPI, Quantity: 3},
		}
	}
	got := poolDeltas(k("p"), -1)
	if len(got) != 2 {
		t.Fatalf("deltas = %+v", got)
	}
	if got[0].Key.Slot != calendar.AM || got[0].Used != -2 || got[0].Held != -1 || got[0].Online != -1 {
		t.Fatalf("AM delta = %+v", got[0])
	}
	if got[1].Used != -3 || got[1].Held != 0 || got[1].Online != -3 {
		t.Fatalf("PM delta = %+v", got[1])
	}
}

func TestSegments_Folding(t *testing.T) {
	type c struct{ am, pm int }
	tests := []struct {
		name  string
		cells []c
		want  []PlanningSegment
	}{
		{"empty", []c{{-1, -1}, {-1, -1}}, nil},
		{"full run", []c{{0, 0}, {0, 0}, {-1, -1}}, []PlanningSegment{{Entry: 0, From: 0, To: 1, Slots: calendar.SlotsFull}}},
		{"am and pm different entries", []c{{0, 1}, {0, 1}}, []PlanningSegment{
			{Entry: 0, From: 0, To: 1, Slots: calendar.SlotsAM}, {Entry: 1, From: 0, To: 1, Slots: calendar.SlotsPM},
		}},
		{"gap splits", []c{{2, 2}, {-1, -1}, {2, 2}}, []PlanningSegment{
			{Entry: 2, From: 0, To: 0, Slots: calendar.SlotsFull}, {Entry: 2, From: 2, To: 2, Slots: calendar.SlotsFull},
		}},
		{"full then am only", []c{{0, 0}, {0, -1}}, []PlanningSegment{
			{Entry: 0, From: 0, To: 0, Slots: calendar.SlotsFull}, {Entry: 0, From: 1, To: 1, Slots: calendar.SlotsAM},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := segments(func(i int) (int, int) { return tt.cells[i].am, tt.cells[i].pm }, len(tt.cells))
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestDisplayName_Scenarios(t *testing.T) {
	tests := []struct{ first, last, want string }{
		{"Mario", "Rossi", "Rossi M."}, {"", "Rossi", "Rossi"}, {"Élise", "Dupont", "Dupont É."}, {" ", " Bianchi ", "Bianchi"},
	}
	for _, tt := range tests {
		if got := DisplayName(tt.first, tt.last); got != tt.want {
			t.Errorf("DisplayName(%q,%q) = %q, want %q", tt.first, tt.last, got, tt.want)
		}
	}
}

func TestCombineStatus_FullDay(t *testing.T) {
	got := combineStatus([]halfDayState{{status: SpotFree}, {status: SpotBooked}})
	if got != SpotBooked {
		t.Fatalf("got %s", got)
	}
	if !occupied(SpotHeld) || occupied(SpotReleased) {
		t.Fatal("occupied mapping")
	}
	_ = availability.DateSlot{}
}
