package checkout

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
)

func TestEmbedReturnURL_Scenarios(t *testing.T) {
	allowed := []string{"https://www.lido.it", "http://127.0.0.1:8091"}
	const id = "0190f2a4-0000-7000-8000-000000000001"
	tests := []struct {
		name, raw, want string
	}{
		{"host page with query and fragment", "https://www.lido.it/prenota?x=1#widget", "https://www.lido.it/prenota?arenile_checkout=" + id + "&x=1"},
		{"replaces a previous return parameter", "https://www.lido.it/?arenile_checkout=old", "https://www.lido.it/?arenile_checkout=" + id},
		{"loopback http (development)", "http://127.0.0.1:8091/host.html", "http://127.0.0.1:8091/host.html?arenile_checkout=" + id},
		{"default port normalised", "https://www.lido.it:443/p", "https://www.lido.it:443/p?arenile_checkout=" + id},
		{"origin not allowed (open redirect)", "https://evil.example/p", ""},
		{"lookalike host", "https://www.lido.it.evil.example/", ""},
		{"credentials", "https://user@www.lido.it/", ""},
		{"relative", "/prenota", ""},
		{"javascript", "javascript:alert(1)", ""},
		{"plain http on a public host", "http://www.lido.it/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := embedReturnURL(tt.raw, allowed, id)
			if tt.want == "" {
				if !errors.Is(err, configerr.ErrValidation) {
					t.Fatalf("want validation error, got %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
