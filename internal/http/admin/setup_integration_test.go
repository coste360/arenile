//go:build integration

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type setupEnv struct {
	h     *Handlers
	estID string
	orgID string
}

func newSetupEnv(t *testing.T) *setupEnv {
	t.Helper()
	db := storetest.SQLite(t)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	c := clock.NewFake(now)
	org, est, err := store.CreateOrganizationWithEstablishment(context.Background(), db, "Org", "lido-http", "Lido", now)
	if err != nil {
		t.Fatal(err)
	}
	svc := setup.NewService(setup.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	return &setupEnv{h: &Handlers{Setup: svc, Log: slog.New(slog.DiscardHandler)}, estID: est.ID, orgID: org.ID}
}

func (e *setupEnv) request(t *testing.T, role auth.Role, method string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, "/", &buf)
	p := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: e.orgID, Role: role}}}
	return r.WithContext(auth.WithPrincipal(r.Context(), p))
}

func TestSetupHandlers_Flow(t *testing.T) {
	e := newSetupEnv(t)

	// Templates: visible to every role, with i18n names.
	w := httptest.NewRecorder()
	e.h.ListSetupTemplates(w, e.request(t, auth.RoleViewer, http.MethodGet, nil), e.estID)
	var list api.SetupTemplateList
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Items) != 4 {
		t.Fatalf("templates: %d %s", w.Code, w.Body)
	}
	if list.Items[0].Code != api.ClassicRows || list.Items[0].Rows != 8 || list.Items[0].SpotsPerRow != 15 ||
		list.Items[0].Name["de"] == "" || len(list.Items[0].RatePeriods) != 4 {
		t.Fatalf("classic: %+v", list.Items[0])
	}

	// Apply: forbidden for a cashier, created for a manager.
	rows, per := 8, 15
	req := api.ApplyTemplateRequest{Template: api.ClassicRows, Rows: &rows, SpotsPerRow: &per}
	w = httptest.NewRecorder()
	e.h.ApplySetupTemplate(w, e.request(t, auth.RoleCashier, http.MethodPost, req), e.estID, api.ApplySetupTemplateParams{})
	if w.Code != http.StatusForbidden {
		t.Fatalf("cashier apply: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	e.h.ApplySetupTemplate(w, e.request(t, auth.RoleManager, http.MethodPost, req), e.estID, api.ApplySetupTemplateParams{})
	var applied api.ApplyTemplateResult
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &applied) != nil {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if applied.SpotsCreated != 124 || applied.SeasonId == nil || applied.MapId == nil || len(applied.ZoneIds) != 4 {
		t.Fatalf("applied: %+v", applied)
	}
	// Second application: 409 conflict.
	w = httptest.NewRecorder()
	e.h.ApplySetupTemplate(w, e.request(t, auth.RoleOwner, http.MethodPost, req), e.estID, api.ApplySetupTemplateParams{})
	if w.Code != http.StatusConflict {
		t.Fatalf("second apply: %d %s", w.Code, w.Body)
	}
	// Validation: 400 with the field.
	zero := 0
	w = httptest.NewRecorder()
	e.h.ApplySetupTemplate(w, e.request(t, auth.RoleOwner, http.MethodPost, api.ApplyTemplateRequest{Template: api.LidoWithPool, Rows: &zero}), e.estID, api.ApplySetupTemplateParams{})
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte(`"rows"`)) {
		t.Fatalf("invalid apply: %d %s", w.Code, w.Body)
	}

	// Quick price list: incomplete -> 400; complete -> 201; again -> 409; replace -> 201.
	var prices []api.QuickPrice
	for _, c := range []string{"row1", "row2", "standard", "gazebo"} {
		for _, p := range []string{"low", "mid", "high", "peak"} {
			prices = append(prices, api.QuickPrice{PriceCategory: c, RatePeriodCode: p, Amount: 2500})
		}
	}
	quick := func(role auth.Role, body api.QuickPricelistRequest) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		e.h.CreateQuickPricelist(w, e.request(t, role, http.MethodPost, body), e.estID, api.CreateQuickPricelistParams{})
		return w
	}
	if w := quick(auth.RoleOwner, api.QuickPricelistRequest{SeasonId: *applied.SeasonId, Prices: prices[:15]}); w.Code != http.StatusBadRequest {
		t.Fatalf("incomplete: %d %s", w.Code, w.Body)
	}
	if w := quick(auth.RoleViewer, api.QuickPricelistRequest{SeasonId: *applied.SeasonId, Prices: prices}); w.Code != http.StatusForbidden {
		t.Fatalf("viewer quick: %d %s", w.Code, w.Body)
	}
	if w := quick(auth.RoleOwner, api.QuickPricelistRequest{SeasonId: store.NewID(), Prices: prices}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown season: %d %s", w.Code, w.Body)
	}
	w = quick(auth.RoleOwner, api.QuickPricelistRequest{SeasonId: *applied.SeasonId, Prices: prices})
	var qr api.QuickPricelistResult
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &qr) != nil || len(qr.Rules) != 16 || qr.Replaced != 0 {
		t.Fatalf("quick: %d %s", w.Code, w.Body)
	}
	r0 := qr.Rules[0]
	if r0.Stage != api.PriceStage("base") || r0.Action.Amount == nil || *r0.Action.Amount != 2500 || r0.Conditions.RatePeriods == nil ||
		r0.Conditions.PriceCategories == nil || r0.TargetIds != nil || r0.Version != 1 {
		t.Fatalf("rule: %+v", r0)
	}
	if w := quick(auth.RoleOwner, api.QuickPricelistRequest{SeasonId: *applied.SeasonId, Prices: prices}); w.Code != http.StatusConflict {
		t.Fatalf("again: %d %s", w.Code, w.Body)
	}
	replace := true
	w = quick(auth.RoleManager, api.QuickPricelistRequest{SeasonId: *applied.SeasonId, Prices: prices, ReplaceExisting: &replace})
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &qr) != nil || qr.Replaced != 16 {
		t.Fatalf("replace: %d %s", w.Code, w.Body)
	}

	// Status.
	w = httptest.NewRecorder()
	e.h.GetSetupStatus(w, e.request(t, auth.RoleBeachStaff, http.MethodGet, nil), e.estID)
	var st api.SetupStatus
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &st) != nil || !st.HasMap || st.SpotCount != 124 || !st.HasPrices {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}

	// Malformed body: 400.
	w = httptest.NewRecorder()
	bad := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{"))
	e.h.CreateQuickPricelist(w, bad.WithContext(e.request(t, auth.RoleOwner, http.MethodPost, nil).Context()), e.estID, api.CreateQuickPricelistParams{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", w.Code)
	}
}

func TestSetupHandlers_Unauthenticated(t *testing.T) {
	e := newSetupEnv(t)
	w := httptest.NewRecorder()
	e.h.GetSetupStatus(w, httptest.NewRequest(http.MethodGet, "/", nil), e.estID)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", w.Code)
	}
}
