//go:build integration

package admin_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestCreditsHTTP_LedgerAndGrant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
		if err != nil {
			t.Fatal(err)
		}
		c := clock.NewFake(now)
		h := &admin.Handlers{Credits: credit.NewService(credit.Deps{DB: db, Clock: c}), Log: slog.New(slog.DiscardHandler)}
		owner := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: auth.RoleOwner}}}
		viewer := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, EstablishmentID: est.ID, Role: auth.RoleViewer}}}
		for _, p := range []*auth.Principal{owner, viewer} {
			if err := store.InsertUser(ctx, db, &store.User{ID: p.UserID, Email: p.UserID + "@example.org", PasswordHash: "x", Name: "Test", Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
		cust := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, FirstName: "Mario", LastName: "Rossi", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
		if _, err := db.NewInsert().Model(cust).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		target := "/api/admin/v1/establishments/" + est.ID + "/customers/" + cust.ID + "/credits"
		post := func(p *auth.Principal, in api.CustomerCreditCreate) (int, map[string]any) {
			code, _, out := callTarget(t, p, http.MethodPost, target, in, func(w http.ResponseWriter, r *http.Request) {
				h.CreateCustomerCredit(w, r, est.ID, cust.ID, api.CreateCustomerCreditParams{})
			})
			return code, out
		}
		code, out := post(owner, api.CustomerCreditCreate{Amount: 2500, Source: "goodwill"})
		if code != http.StatusCreated || out["source"] != "goodwill" || out["amount"].(float64) != 2500 {
			t.Fatalf("grant %d %v", code, out)
		}
		code, out = post(owner, api.CustomerCreditCreate{Amount: -3000, Source: "adjustment"})
		if code != http.StatusUnprocessableEntity || out["type"] == nil {
			t.Fatalf("overdraft %d %v", code, out)
		}
		if code, _ := post(viewer, api.CustomerCreditCreate{Amount: 100, Source: "goodwill"}); code != http.StatusForbidden {
			t.Fatalf("viewer grant %d", code)
		}
		code, _, out = callTarget(t, viewer, http.MethodGet, target, nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetCustomerCredits(w, r, est.ID, cust.ID, api.GetCustomerCreditsParams{})
		})
		items, _ := out["items"].([]any)
		if code != http.StatusOK || out["balance"].(float64) != 2500 || out["available"].(float64) != 2500 || len(items) != 1 {
			t.Fatalf("ledger %d %v", code, out)
		}
	})
}
