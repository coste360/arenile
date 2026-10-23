package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/http/api"
)

func TestValidateFirstRun_Scenarios(t *testing.T) {
	ok := api.FirstRunRequest{
		SetupToken: "tok", Email: "anna@lido-test.it", Name: "Anna", Password: "spiaggia-sicura-2027", OrganizationName: "Lido Test",
	}
	if fe := validateFirstRun(ok); len(fe) != 0 {
		t.Fatalf("valid request rejected: %+v", fe)
	}
	tests := map[string]func(r *api.FirstRunRequest){
		"setup_token":       func(r *api.FirstRunRequest) { r.SetupToken = " " },
		"email":             func(r *api.FirstRunRequest) { r.Email = "anna@localhost" },
		"name":              func(r *api.FirstRunRequest) { r.Name = "" },
		"organization_name": func(r *api.FirstRunRequest) { r.OrganizationName = "  " },
		"password":          func(r *api.FirstRunRequest) { r.Password = "corta" },
	}
	for field, mut := range tests {
		r := ok
		mut(&r)
		fe := validateFirstRun(r)
		if len(fe) != 1 || fe[0].Field != field {
			t.Errorf("%s: %+v", field, fe)
		}
	}
}

func TestHandlers_ServerEditionNotFound(t *testing.T) {
	var h *Handlers
	for name, call := range map[string]func(w http.ResponseWriter, r *http.Request){
		"status":   (&Handlers{}).GetFirstRunStatus,
		"create":   h.CompleteFirstRun,
		"settings": h.GetDesktopSettings,
		"backups":  h.ListDesktopBackups,
	} {
		w := httptest.NewRecorder()
		call(w, httptest.NewRequest(http.MethodGet, "/api/desktop/v1/x", nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
}
