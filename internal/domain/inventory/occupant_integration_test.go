//go:build integration

package inventory_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestOperationsDay_OccupantLabel: the operational map gets the short name
// of the occupant (spec 11.3) for every role that may view the map and the
// bookings (spec 16.1: viewer included), never across tenants, never for
// anonymised customers, and never email or phone.
func TestOperationsDay_OccupantLabel(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		email, phone := "anna.verdi@example.it", "+393330000042"
		anna := &store.Customer{
			ID: store.NewID(), EstablishmentID: f.est.ID, FirstName: "Anna", LastName: "", Email: &email, Phone: &phone,
			Locale: "it", Tags: []string{}, SearchText: "anna", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		_, err := db.NewInsert().Model(anna).Exec(ctx)
		must(t, err)
		// A1: Rossi Mario (the fixture customer); A2: Anna, no surname.
		_, err = f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		tb, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		_, err = db.NewUpdate().Table("bookings").Set("customer_id = ?", anna.ID).Where("id = ?", tb.id).Exec(ctx)
		must(t, err)

		q := inventory.OperationsQuery{Date: d("2027-07-02"), MapID: f.mapID}
		labels := func(p *auth.Principal) map[string]*inventory.OperationalBooking {
			t.Helper()
			day, err := f.svc.GetOperationsDay(ctx, p, f.est.ID, q)
			must(t, err)
			out := map[string]*inventory.OperationalBooking{}
			for _, sp := range day.Spots {
				out[sp.Label] = sp.Booking
			}
			return out
		}
		for name, p := range map[string]*auth.Principal{"viewer": f.viewer, "cashier": f.cashier, "owner": f.owner} {
			got := labels(p)
			if got["A1"] == nil || got["A1"].OccupantLabel != "Rossi" || got["A1"].CustomerAnonymized {
				t.Fatalf("%s A1 = %+v", name, got["A1"])
			}
			if got["A2"] == nil || got["A2"].OccupantLabel != "Anna" {
				t.Fatalf("%s A2 = %+v", name, got["A2"])
			}
			if got["A3"] != nil {
				t.Fatalf("%s A3 free, got %+v", name, got["A3"])
			}
		}

		// Cross tenant: the owner of another organisation gets nothing.
		if _, err := f.svc.GetOperationsDay(ctx, f.ownerB, f.est.ID, q); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		// An API key of this establishment without bookings.read: forbidden.
		reports := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: f.est.ID, APIKeyOrganizationID: f.est.OrganizationID, Scopes: []auth.Permission{auth.PermReportsRead}}
		if _, err := f.svc.GetOperationsDay(ctx, reports, f.est.ID, q); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("reports-only api key: %v", err)
		}

		// The HTTP payload carries the label but never email or phone.
		h := &admin.Handlers{Inventory: f.svc, Log: slog.New(slog.DiscardHandler)}
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, f.viewer))
		w := httptest.NewRecorder()
		h.GetOperationsDay(w, r, f.est.ID, api.GetOperationsDayParams{Date: d("2027-07-02"), MapId: &f.mapID})
		if w.Code != http.StatusOK {
			t.Fatalf("http: %d %s", w.Code, w.Body)
		}
		body := w.Body.String()
		if strings.Contains(body, email) || strings.Contains(body, phone) {
			t.Fatalf("personal contact data in the payload: %s", body)
		}
		var od api.OperationsDay
		must(t, json.Unmarshal(w.Body.Bytes(), &od))
		byLabel := map[string]*api.OperationalBooking{}
		for _, sp := range od.Spots {
			byLabel[sp.Label] = sp.Booking
		}
		if b := byLabel["A1"]; b == nil || b.OccupantLabel != "Rossi" || b.CustomerAnonymized {
			t.Fatalf("http A1 = %+v", b)
		}

		// GDPR anonymisation (spec 23): no name, the flag tells the client.
		_, err = db.NewUpdate().Table("customers").Set("first_name = ''").Set("last_name = 'Anonimizzato'").
			Set("email = NULL").Set("phone = NULL").Set("anonymized_at = ?", t0).Where("id = ?", anna.ID).Exec(ctx)
		must(t, err)
		if got := labels(f.cashier)["A2"]; got == nil || got.OccupantLabel != "" || !got.CustomerAnonymized {
			t.Fatalf("anonymised A2 = %+v", got)
		}
	})
}
