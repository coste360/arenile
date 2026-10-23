//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type customerHTTPEnv struct {
	h          *admin.Handlers
	estA, estB *store.Establishment
	cashier    *auth.Principal
	viewer     *auth.Principal
	ownerB     *auth.Principal
}

func newCustomerHTTPEnv(t *testing.T, db *store.DB) *customerHTTPEnv {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(now)
	svc := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	user := func(org, est string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
	}
	return &customerHTTPEnv{
		h:       &admin.Handlers{Customers: svc, Log: slog.New(slog.DiscardHandler)},
		estA:    estA,
		estB:    estB,
		cashier: user(orgA.ID, estA.ID, auth.RoleCashier),
		viewer:  user(orgA.ID, estA.ID, auth.RoleViewer),
		ownerB:  user(orgB.ID, "", auth.RoleOwner),
	}
}

// callTarget is call with a request target (path and query).
func callTarget(t *testing.T, p *auth.Principal, method, target string, body any, fn func(w http.ResponseWriter, r *http.Request)) (int, http.Header, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, target, &buf)
	if p != nil {
		r = r.WithContext(auth.WithPrincipal(r.Context(), p))
	}
	w := httptest.NewRecorder()
	fn(w, r)
	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %q: %v", w.Body.String(), err)
		}
	}
	return w.Code, w.Header(), out
}

func TestCustomersHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newCustomerHTTPEnv(t, db)
		a := e.estA.ID
		create := func(p *auth.Principal, target string, in api.CustomerInput) (int, http.Header, map[string]any) {
			return callTarget(t, p, http.MethodPost, target, in, func(w http.ResponseWriter, r *http.Request) {
				e.h.CreateCustomer(w, r, a, api.CreateCustomerParams{})
			})
		}
		email := "Mario.Rossi@Example.it"
		in := api.CustomerInput{LastName: "Rossi", FirstName: strp("Mario"), Email: (*openapi_types.Email)(&email), Phone: strp("333 1234567"), Tags: &[]string{"VIP"}}
		status, hdr, body := create(e.cashier, "/", in)
		if status != 201 || hdr.Get("ETag") != `"1"` || hdr.Get("Cache-Control") != "no-store" ||
			body["email"] != "mario.rossi@example.it" || body["phone"] != "+393331234567" || body["display_name"] != "Rossi Mario" {
			t.Fatalf("create: %d %v %v", status, hdr, body)
		}
		id := body["id"].(string)

		// Dedupe hint: 409 with the existing id, force=true creates anyway.
		status, _, body = create(e.cashier, "/", api.CustomerInput{LastName: "Rossi", Phone: strp("+39 333 123 4567")})
		if status != 409 || body["type"] != "conflict" || !strings.Contains(body["detail"].(string), id) {
			t.Fatalf("duplicate: %d %v", status, body)
		}
		status, _, _ = create(e.cashier, "/?force=true", api.CustomerInput{LastName: "Rossi", FirstName: strp("Lucia"), Phone: strp("+39 333 123 4567")})
		if status != 201 {
			t.Fatalf("forced create: %d", status)
		}
		status, _, body = create(e.cashier, "/", api.CustomerInput{LastName: "", Phone: strp("12")})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("validation: %d %v", status, body)
		}
		status, _, body = create(e.viewer, "/", api.CustomerInput{LastName: "X"})
		if status != 403 || body["type"] != "forbidden" {
			t.Fatalf("viewer create: %d %v", status, body)
		}

		status, _, body = callTarget(t, e.viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListCustomers(w, r, a, api.ListCustomersParams{Q: strp("333 123"), Tag: strp("vip")})
		})
		if items := body["items"].([]any); status != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != id {
			t.Fatalf("list: %d %v", status, body)
		}

		status, hdr, body = callTarget(t, e.viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetCustomer(w, r, a, id)
		})
		if status != 200 || hdr.Get("ETag") != `"1"` || body["customer"].(map[string]any)["id"] != id ||
			body["credit_balance"].(float64) != 0 || body["stats"].(map[string]any)["bookings_count"].(float64) != 0 {
			t.Fatalf("get: %d %v", status, body)
		}
		status, _, body = callTarget(t, e.ownerB, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetCustomer(w, r, a, id)
		})
		if status != 404 || body["type"] != "not_found" {
			t.Fatalf("cross-tenant get: %d %v", status, body)
		}

		upd := func(ifMatch string, in api.CustomerInput) (int, http.Header, map[string]any) {
			return callTarget(t, e.cashier, http.MethodPut, "/", in, func(w http.ResponseWriter, r *http.Request) {
				e.h.UpdateCustomer(w, r, a, id, api.UpdateCustomerParams{IfMatch: strp(ifMatch)})
			})
		}
		status, hdr, _ = upd(`"1"`, api.CustomerInput{LastName: "Rossi", FirstName: strp("Mario"), Notes: strp("ombrellone vicino al bar")})
		if status != 200 || hdr.Get("ETag") != `"2"` {
			t.Fatalf("update: %d", status)
		}
		status, _, body = upd(`"1"`, api.CustomerInput{LastName: "Rossi"})
		if status != 412 || body["type"] != "version_conflict" || body["current_version"].(float64) != 2 {
			t.Fatalf("stale update: %d %v", status, body)
		}

		status, hdr, body = callTarget(t, e.cashier, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ExportCustomer(w, r, a, id)
		})
		if status != 200 || !strings.Contains(hdr.Get("Content-Disposition"), id) || hdr.Get("Cache-Control") != "no-store" ||
			body["customer"].(map[string]any)["notes"] != "ombrellone vicino al bar" || body["exported_at"] == nil {
			t.Fatalf("export: %d %v %v", status, hdr, body)
		}

		status, hdr, body = callTarget(t, e.cashier, http.MethodPost, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.AnonymizeCustomer(w, r, a, id, api.AnonymizeCustomerParams{IfMatch: strp(`"2"`)})
		})
		if status != 200 || hdr.Get("ETag") != `"3"` || body["anonymized_at"] == nil || body["email"] != nil || body["last_name"] != customer.AnonymizedLastName {
			t.Fatalf("anonymize: %d %v", status, body)
		}
		status, _, body = upd(`"3"`, api.CustomerInput{LastName: "Rossi"})
		if status != 409 || body["type"] != "conflict" {
			t.Fatalf("update anonymized: %d %v", status, body)
		}
	})
}
