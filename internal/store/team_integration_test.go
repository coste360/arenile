//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestTeamStore_MembersGrantsAndScope(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		org, a, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "team-a", "A", t0)
		if err != nil {
			t.Fatal(err)
		}
		a2 := store.NewEstablishmentDefaults(org.ID, "team-a2", "A2", t0)
		if err := store.InsertEstablishment(ctx, db, a2); err != nil {
			t.Fatal(err)
		}
		orgB, b, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "team-b", "B", t0)
		if err != nil {
			t.Fatal(err)
		}
		esA, _ := store.SystemEstablishmentScope(ctx, db, a.ID)
		esA2, _ := store.SystemEstablishmentScope(ctx, db, a2.ID)
		esB, _ := store.SystemEstablishmentScope(ctx, db, b.ID)
		user := func(email string, active bool) *store.User {
			u := &store.User{ID: store.NewID(), Email: email, PasswordHash: "x", Name: email, Locale: "it", Active: active, CreatedAt: t0, UpdatedAt: t0}
			if err := store.InsertUser(ctx, db, u); err != nil {
				t.Fatal(err)
			}
			return u
		}
		member := func(u *store.User, orgID string, est *string, role string) *store.Membership {
			m := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: orgID, EstablishmentID: est, Role: role, CreatedAt: t0, UpdatedAt: t0}
			if err := store.InsertMembership(ctx, db, m); err != nil {
				t.Fatal(err)
			}
			return m
		}
		owner := user("owner@a.it", true)
		member(owner, org.ID, nil, "owner")
		cashier := user("cash@a.it", true)
		cm := member(cashier, org.ID, &a.ID, "cashier")
		member(cashier, org.ID, &a2.ID, "viewer")
		other := user("other@a.it", false)
		member(other, org.ID, &a2.ID, "manager")
		foreign := user("b@b.it", true)
		member(foreign, orgB.ID, nil, "owner")

		page, err := store.ListTeamMembers(ctx, db, esA, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("page 1 %+v %v", page, err)
		}
		page2, err := store.ListTeamMembers(ctx, db, esA, page.NextCursor, 10)
		if err != nil || len(page2.Items) != 1 || page2.NextCursor != "" {
			t.Fatalf("page 2 %+v %v", page2, err)
		}
		for _, p := range [][]store.TeamMember{page.Items, page2.Items} {
			for _, tm := range p {
				if tm.User.ID == other.ID || tm.User.ID == foreign.ID || len(tm.Memberships) != 1 {
					t.Fatalf("unexpected member %+v", tm)
				}
			}
		}
		tm, err := store.GetTeamMember(ctx, db, esA2, cashier.ID)
		if err != nil || len(tm.Memberships) != 1 || tm.Memberships[0].Role != "viewer" {
			t.Fatalf("member in A2 %+v %v", tm, err)
		}
		if _, err := store.GetTeamMember(ctx, db, esB, cashier.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross org %v", err)
		}
		grants, err := store.ListTeamGrants(ctx, db, esA2)
		if err != nil || len(grants) != 3 {
			t.Fatalf("grants %+v %v", grants, err)
		}
		for _, g := range grants {
			if g.UserID == other.ID && g.UserActive {
				t.Fatal("inactive flag lost")
			}
		}
		// Membership mutations are confined to the scope's organization.
		if err := store.TeamUpdateMembershipRole(ctx, db, esB, cm.ID, "manager", t0); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross org role %v", err)
		}
		if err := store.TeamDeleteMembership(ctx, db, esB, cm.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross org delete %v", err)
		}
		if err := store.TeamUpdateMembershipRole(ctx, db, esA, cm.ID, "manager", t0); err != nil {
			t.Fatal(err)
		}
		if err := store.TeamLockOrganization(ctx, db, esA); err != nil {
			t.Fatal(err)
		}
		if err := store.TeamSetUserActive(ctx, db, cashier.ID, false, t0); err != nil {
			t.Fatal(err)
		}
		if u, _ := store.GetUser(ctx, db, cashier.ID); u.Active {
			t.Fatal("still active")
		}
		if err := store.TeamDeleteMembership(ctx, db, esA, cm.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetTeamMember(ctx, db, esA, cashier.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("after delete %v", err)
		}
	})
}

func TestEstablishmentStore_VersionedUpdateAndSlug(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, a, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "ver-a", "A", t0)
		if err != nil {
			t.Fatal(err)
		}
		_, b, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "ver-b", "B", t0)
		if err != nil {
			t.Fatal(err)
		}
		esA, _ := store.SystemEstablishmentScope(ctx, db, a.ID)
		esB, _ := store.SystemEstablishmentScope(ctx, db, b.ID)
		if taken, err := store.EstablishmentSlugTaken(ctx, db, "ver-b", a.ID); err != nil || !taken {
			t.Fatalf("taken %v %v", taken, err)
		}
		if taken, _ := store.EstablishmentSlugTaken(ctx, db, "ver-a", a.ID); taken {
			t.Fatal("own slug reported as taken")
		}
		a.Name, a.Version = "A2", 2
		if ok, err := store.UpdateEstablishmentIfVersion(ctx, db, esA, a, 1); err != nil || !ok {
			t.Fatalf("update %v %v", ok, err)
		}
		if ok, err := store.UpdateEstablishmentIfVersion(ctx, db, esA, a, 1); err != nil || ok {
			t.Fatalf("stale %v %v", ok, err)
		}
		if _, err := store.UpdateEstablishmentIfVersion(ctx, db, esB, a, 2); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross scope %v", err)
		}
		a.Slug, a.Version = "ver-b", 3
		if _, err := store.UpdateEstablishmentIfVersion(ctx, db, esA, a, 2); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("slug conflict %v", err)
		}
	})
}
