package webhook

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
)

// TestAuthorize_RefusesKeysAndDevices: neither an API key nor a paired
// device token manages outgoing webhooks, whatever the role behind it
// (INV-56, INV-62); the check runs before any database access.
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
