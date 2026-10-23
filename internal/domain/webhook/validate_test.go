package webhook

import (
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

func TestValidateURL_Scenarios(t *testing.T) {
	strict, lan := event.EgressPolicy{}, event.EgressPolicy{AllowPrivate: true}
	tests := []struct {
		name   string
		raw    string
		policy event.EgressPolicy
		want   string // "" = invalid
	}{
		{"https", " https://hooks.example.com/arenile?x=1 ", strict, "https://hooks.example.com/arenile?x=1"},
		{"https with port", "https://hooks.example.com:8443/h", strict, "https://hooks.example.com:8443/h"},
		{"http to localhost", "http://localhost:9000/h", strict, "http://localhost:9000/h"},
		{"http to 127.0.0.1 on LAN", "http://127.0.0.1:9000/h", lan, "http://127.0.0.1:9000/h"},
		{"http to [::1] on LAN", "http://[::1]:9000/h", lan, "http://[::1]:9000/h"},
		{"http elsewhere", "http://hooks.example.com/h", strict, ""},
		{"ftp", "ftp://hooks.example.com/h", strict, ""},
		{"relative", "/hooks", strict, ""},
		{"no host", "https:///h", strict, ""},
		{"opaque", "https:hooks.example.com", strict, ""},
		{"userinfo", "https://user:pw@hooks.example.com/h", strict, ""},
		{"fragment", "https://hooks.example.com/h#x", strict, ""},
		{"empty", "  ", strict, ""},
		{"too long", "https://hooks.example.com/" + strings.Repeat("a", MaxURLBytes), strict, ""},
		{"private literal", "https://10.0.0.5/h", strict, ""},
		{"private literal on LAN", "https://10.0.0.5/h", lan, "https://10.0.0.5/h"},
		{"loopback literal over http strict", "http://127.0.0.1:9000/h", strict, ""},
		{"metadata literal on LAN", "https://169.254.169.254/latest", lan, ""},
		{"mapped v6 literal", "https://[::ffff:192.168.1.1]/h", strict, ""},
		{"public literal", "https://93.184.216.34/h", strict, "https://93.184.216.34/h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			got := ValidateURL(ve, tt.raw, tt.policy)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if (ve.OrNil() == nil) != (tt.want != "") {
				t.Fatalf("validation error mismatch: %v", ve.OrNil())
			}
		})
	}
}

func TestValidateEvents_Scenarios(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string // nil = invalid
	}{
		{"types deduped", []string{"booking.created", "payment.failed", "booking.created"}, []string{"booking.created", "payment.failed"}},
		{"all", []string{"*"}, []string{"*"}},
		{"group", []string{"booking.*", "config.*"}, []string{"booking.*", "config.*"}},
		{"cash group", []string{"cash_session.*"}, []string{"cash_session.*"}},
		{"empty", []string{}, nil},
		{"nil", nil, nil},
		{"unknown type", []string{"booking.exploded"}, nil},
		{"unknown group", []string{"invoice.*"}, nil},
		{"test event not subscribable", []string{TestEventType}, nil},
		{"bare star group", []string{".*"}, nil},
		{"partial prefix", []string{"book.*"}, nil},
		{"too many", make([]string, MaxEventFilters+1), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			got := ValidateEvents(ve, tt.in)
			if tt.want == nil {
				if ve.OrNil() == nil || got != nil {
					t.Fatalf("want invalid, got %v", got)
				}
				return
			}
			if ve.OrNil() != nil || strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("got %v (%v) want %v", got, ve.OrNil(), tt.want)
			}
		})
	}
}

func TestStatusOf_Derivation(t *testing.T) {
	now := time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC)
	if s := EndpointStatusOf(&store.WebhookEndpoint{Active: true}); s != EndpointActive {
		t.Fatal(s)
	}
	if s := EndpointStatusOf(&store.WebhookEndpoint{}); s != EndpointPaused {
		t.Fatal(s)
	}
	if s := EndpointStatusOf(&store.WebhookEndpoint{DisabledAt: &now}); s != EndpointDisabled {
		t.Fatal(s)
	}
	if s := DeliveryStatusOf(&store.WebhookDelivery{}); s != DeliveryPending {
		t.Fatal(s)
	}
	if s := DeliveryStatusOf(&store.WebhookDelivery{DeliveredAt: &now}); s != DeliveryDelivered {
		t.Fatal(s)
	}
	if s := DeliveryStatusOf(&store.WebhookDelivery{FailedAt: &now}); s != DeliveryFailed {
		t.Fatal(s)
	}
}

// TestEventTypes_MatchStore: every subscribable type is matched by its own
// filter, by its group and by "*" (store.WebhookEndpoint.Matches), and the
// test event by none but itself.
func TestEventTypes_MatchStore(t *testing.T) {
	for _, typ := range EventTypes {
		prefix, _, _ := strings.Cut(typ, ".")
		group := prefix + ".*"
		for _, f := range []string{typ, group, "*"} {
			ep := store.WebhookEndpoint{Events: []string{f}}
			if !ep.Matches(typ) {
				t.Fatalf("%s does not match filter %s", typ, f)
			}
		}
	}
}
