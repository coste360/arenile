//go:build integration

package admin_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestMapsHTTP_Reorient(t *testing.T) {
	db := storetest.SQLite(t)
	e := newMapsEnv(t, db)
	a := e.estA.ID
	mapIn := api.MapInput{Name: api.I18nText{"it": "Spiaggia"}, Width: 1200, Height: 800, GridSize: 10, SeaSide: "bottom"}
	status, _, body := mapsCall(t, e.owner, http.MethodPost, mapIn, func(w http.ResponseWriter, r *http.Request) {
		e.h.CreateMap(w, r, a, api.CreateMapParams{})
	})
	if status != 201 {
		t.Fatalf("create map: %d %s", status, body)
	}
	mapID := mapsDecode[api.MapDocument](t, body).Map.Id
	status, _, body = mapsCall(t, e.owner, http.MethodPost, api.ZoneInput{MapId: mapsPtr(mapID), Code: "row1", Name: api.I18nText{"it": "Fila 1"}, AssignmentMode: "seat", OnlineVisible: mapsPtr(true), PriceCategory: "row1"},
		func(w http.ResponseWriter, r *http.Request) { e.h.CreateZone(w, r, a, api.CreateZoneParams{}) })
	if status != 201 {
		t.Fatalf("create zone: %d %s", status, body)
	}
	zone := mapsDecode[api.Zone](t, body)
	save := api.MapSaveRequest{Map: mapIn, Spots: []api.SpotInput{{ZoneId: zone.Id, Label: "A1", Kind: "umbrella", X: 100, Y: 700, Shape: api.Shape{Type: "circle", W: 40, H: 40}}}}
	if status, _, body = mapsCall(t, e.owner, http.MethodPut, save, func(w http.ResponseWriter, r *http.Request) {
		e.h.SaveMap(w, r, a, mapID, api.SaveMapParams{IfMatch: `"1"`})
	}); status != 200 {
		t.Fatalf("save: %d %s", status, body)
	}
	reorient := func(ifMatch string, req api.MapReorientRequest) (int, http.Header, []byte) {
		return mapsCall(t, e.owner, http.MethodPost, req, func(w http.ResponseWriter, r *http.Request) {
			e.h.ReorientMap(w, r, a, mapID, api.ReorientMapParams{IfMatch: ifMatch})
		})
	}

	// If-Match is required.
	if status, _, body = reorient("", api.MapReorientRequest{SeaSide: "top"}); status != 428 {
		t.Fatalf("missing If-Match: %d %s", status, body)
	}
	// Dry run: preview, ETag of the current version.
	status, hdr, body := reorient(`"2"`, api.MapReorientRequest{SeaSide: "top", DryRun: mapsPtr(true)})
	res := mapsDecode[api.MapReorientResult](t, body)
	if status != 200 || res.Applied || hdr.Get("ETag") != `"2"` || res.Transform != api.MapTransformMirrorY || res.Document.Spots[0].Y != 100 {
		t.Fatalf("dry run: %d %s %s", status, hdr.Get("ETag"), body)
	}
	// Stale version.
	if status, _, body = reorient(`"1"`, api.MapReorientRequest{SeaSide: "top"}); status != 412 {
		t.Fatalf("stale: %d %s", status, body)
	}
	// Invalid side.
	if status, _, body = reorient(`"2"`, api.MapReorientRequest{SeaSide: "north"}); status != 400 && status != 422 {
		t.Fatalf("invalid side: %d %s", status, body)
	}
	// Viewer: forbidden.
	if status, _, body = mapsCall(t, e.viewer, http.MethodPost, api.MapReorientRequest{SeaSide: "top"}, func(w http.ResponseWriter, r *http.Request) {
		e.h.ReorientMap(w, r, a, mapID, api.ReorientMapParams{IfMatch: "*"})
	}); status != 403 {
		t.Fatalf("viewer: %d %s", status, body)
	}
	// Applied with If-Match "*": new ETag, sea at the top, A1 near it.
	status, hdr, body = reorient("*", api.MapReorientRequest{SeaSide: "top"})
	res = mapsDecode[api.MapReorientResult](t, body)
	if status != 200 || !res.Applied || hdr.Get("ETag") != `"3"` || res.Document.Map.SeaSide != "top" || res.FromSeaSide != "bottom" || res.ToSeaSide != "top" {
		t.Fatalf("apply: %d %s %s", status, hdr.Get("ETag"), body)
	}
	if res.Document.Spots[0].Y != 100 || res.Document.Map.Version != 3 {
		t.Fatalf("applied document: %s", body)
	}
}
