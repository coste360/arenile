package establishment

import (
	"context"
	"errors"
	"testing"
)

func TestValidateSlug_Reserved(t *testing.T) {
	for _, s := range ReservedSlugs {
		if err := ValidateSlug(s); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("%q: want ErrInvalidSlug, got %v", s, err)
		}
	}
	for _, s := range []string{"lido-azzurro", "bagni-42", "a"} {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "-lido", "lido-", "Lido Azzurro", "lido_azzurro"} {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("%q: want error", s)
		}
	}
}

// TestEmbedOrigins_ReservedSlugSkipsDatabase: requests for the public bundles
// (/b/assets/...) must not query the database. The service has no database
// here, so any query would panic.
func TestEmbedOrigins_ReservedSlugSkipsDatabase(t *testing.T) {
	s := &Service{}
	for _, slug := range append([]string{"ASSETS", "not a slug", ""}, ReservedSlugs...) {
		got, err := s.EmbedOrigins(context.Background(), slug)
		if err != nil || got != nil {
			t.Fatalf("%q: %v %v", slug, got, err)
		}
	}
}
