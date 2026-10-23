package auth

import (
	"reflect"
	"testing"
)

func TestNarrowGrants_Scenarios(t *testing.T) {
	tests := []struct {
		name   string
		grants []Grant
		want   []Grant
	}{
		{"none", nil, nil},
		{
			"organization grant becomes establishment grant",
			[]Grant{{OrganizationID: "o1", Role: RoleManager}},
			[]Grant{{OrganizationID: "o1", EstablishmentID: "e1", Role: RoleManager}},
		},
		{
			"other establishment and organization dropped",
			[]Grant{
				{OrganizationID: "o1", EstablishmentID: "e2", Role: RoleOwner},
				{OrganizationID: "o2", Role: RoleOwner},
				{OrganizationID: "o1", EstablishmentID: "e1", Role: RoleCashier},
			},
			[]Grant{{OrganizationID: "o1", EstablishmentID: "e1", Role: RoleCashier}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NarrowGrants(tt.grants, "o1", "e1"); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NarrowGrants = %+v, want %+v", got, tt.want)
			}
		})
	}
}
