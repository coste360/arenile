package team

import (
	"errors"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

func TestEffective_Scenarios(t *testing.T) {
	est := "e1"
	tests := []struct {
		name  string
		ms    []store.Membership
		role  auth.Role
		scope Scope
		ok    bool
	}{
		{"none", nil, "", "", false},
		{"organization viewer", []store.Membership{{Role: "viewer"}}, auth.RoleViewer, ScopeOrganization, true},
		{"establishment manager beats organization viewer", []store.Membership{{Role: "viewer"}, {Role: "manager", EstablishmentID: &est}}, auth.RoleManager, ScopeEstablishment, true},
		{"organization owner beats establishment cashier", []store.Membership{{Role: "cashier", EstablishmentID: &est}, {Role: "owner"}}, auth.RoleOwner, ScopeOrganization, true},
		{"tie prefers organization", []store.Membership{{Role: "manager", EstablishmentID: &est}, {Role: "manager"}}, auth.RoleManager, ScopeOrganization, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, scope, ok := Effective(tt.ms)
			if role != tt.role || scope != tt.scope || ok != tt.ok {
				t.Fatalf("got %s %s %v", role, scope, ok)
			}
		})
	}
}

func TestCheckLastOwner_Scenarios(t *testing.T) {
	orgOwner := func(u string, active bool) Grant {
		return Grant{UserID: u, OrgWide: true, Role: auth.RoleOwner, Active: active}
	}
	estOwner := func(u string) Grant { return Grant{UserID: u, Role: auth.RoleOwner, Active: true} }
	manager := func(u string) Grant { return Grant{UserID: u, OrgWide: true, Role: auth.RoleManager, Active: true} }
	two := []Grant{orgOwner("a", true), orgOwner("b", true), manager("m")}
	one := []Grant{orgOwner("a", true), manager("m")}
	tests := []struct {
		name          string
		before, after []Grant
		wantErr       bool
	}{
		{"demote one of two owners", two, replaceUser(two, "b", manager("b")), false},
		{"demote the last owner", one, replaceUser(one, "a", manager("a")), true},
		{"remove the last owner", one, replaceUser(one, "a"), true},
		{"deactivate the last owner", one, withActive(one, "a", false), true},
		{"deactivate one of two owners", two, withActive(two, "a", false), false},
		{"only owner is already inactive: other changes allowed", []Grant{orgOwner("a", false), manager("m")}, []Grant{orgOwner("a", false)}, false},
		{"move last org owner to establishment scope", one, replaceUser(one, "a", estOwner("a")), true},
		{"establishment owner removed while an org owner remains", append(one, estOwner("e")), replaceUser(append(one, estOwner("e")), "e"), false},
		{"same owner with two memberships, one removed", []Grant{orgOwner("a", true), estOwner("a")}, []Grant{orgOwner("a", true)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckLastOwner(tt.before, tt.after)
			if (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, ErrLastOwner)) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestGeneratePassword_PolicyAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != GeneratedPasswordLength || auth.ValidatePassword(pw) != nil {
			t.Fatalf("password %q does not meet the policy", pw)
		}
		if strings.Trim(pw, passwordAlphabet) != "" {
			t.Fatalf("unexpected characters in %q", pw)
		}
		if seen[pw] {
			t.Fatal("duplicate password")
		}
		seen[pw] = true
	}
}

func TestScope_Valid(t *testing.T) {
	if !ScopeEstablishment.Valid() || !ScopeOrganization.Valid() || Scope("global").Valid() {
		t.Fatal("scope validity")
	}
}
