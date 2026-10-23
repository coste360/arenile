package apikey

import (
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func TestStatusOf_Scenarios(t *testing.T) {
	now := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)
	tests := []struct {
		name string
		k    store.APIKey
		want Status
	}{
		{"no expiry", store.APIKey{}, StatusActive},
		{"future expiry", store.APIKey{ExpiresAt: &future}, StatusActive},
		{"expires exactly now", store.APIKey{ExpiresAt: &now}, StatusExpired},
		{"past expiry", store.APIKey{ExpiresAt: &past}, StatusExpired},
		{"revoked wins", store.APIKey{ExpiresAt: &past, RevokedAt: &past}, StatusRevoked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusOf(&tt.k, now); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

func TestValidateName_Scenarios(t *testing.T) {
	long := make([]rune, MaxNameRunes+1)
	for i := range long {
		long[i] = 'à'
	}
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"  Gestionale  ", "Gestionale"},
		{"", ""},
		{"   ", ""},
		{string(long[:MaxNameRunes]), string(long[:MaxNameRunes])},
		{string(long), ""},
	} {
		ve := &configerr.ValidationError{}
		if got := validateName(ve, tt.in); got != tt.want || (ve.OrNil() == nil) != (tt.want != "") {
			t.Fatalf("validateName(%q) = %q (%v)", tt.in, got, ve.OrNil())
		}
	}
}

func TestValidateExpiry_Scenarios(t *testing.T) {
	now := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	for _, tt := range []struct {
		at *time.Time
		ok bool
	}{{nil, true}, {&future, true}, {&now, false}, {&past, false}} {
		ve := &configerr.ValidationError{}
		validateExpiry(ve, tt.at, now)
		if (ve.OrNil() == nil) != tt.ok {
			t.Fatalf("expiry %v: %v", tt.at, ve.OrNil())
		}
	}
}

func TestScopes_UsersManageNeverGrantable(t *testing.T) {
	// The API enum of scopes is every permission except users.manage: a
	// key could otherwise create keys and webhooks.
	if !auth.PermUsersManage.Valid() {
		t.Fatal("users.manage must stay a permission")
	}
	ve := &configerr.ValidationError{}
	if got := validateScopes(ve, nil, store.EstablishmentScope{}, []auth.Permission{auth.PermUsersManage}); got != nil || ve.OrNil() == nil {
		t.Fatalf("users.manage accepted: %v", got)
	}
	ve = &configerr.ValidationError{}
	if got := validateScopes(ve, nil, store.EstablishmentScope{}, nil); got != nil || ve.OrNil() == nil {
		t.Fatal("empty scopes accepted")
	}
	ve = &configerr.ValidationError{}
	if got := validateScopes(ve, nil, store.EstablishmentScope{}, []auth.Permission{"bookings.delete"}); got != nil || ve.OrNil() == nil {
		t.Fatal("unknown scope accepted")
	}
}

// TestAuthorize_RefusesKeysAndDevices: neither an API key nor a paired
// device token manages API keys, whatever the role behind it (INV-56,
// INV-62); the check runs before any database access.
func TestAuthorize_RefusesKeysAndDevices(t *testing.T) {
	s := &Service{}
	owner := []auth.Grant{{OrganizationID: "o", Role: auth.RoleOwner}}
	for _, p := range []*auth.Principal{
		{Kind: auth.PrincipalAPIKey, APIKeyID: "k"},
		{Kind: auth.PrincipalUser, UserID: "u", DeviceID: "d", Grants: owner},
	} {
		if _, err := s.authorize(t.Context(), p, "e"); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("%+v: got %v, want ErrForbidden", p, err)
		}
	}
}
