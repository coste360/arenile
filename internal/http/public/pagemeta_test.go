package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

type fakeProfiles map[string]*establishment.Public

func (f fakeProfiles) GetPublic(_ context.Context, slug string) (*establishment.Public, error) {
	if e, ok := f[slug]; ok {
		return e, nil
	}
	return nil, establishment.ErrNotFound
}

func TestPageMetaOf_Scenarios(t *testing.T) {
	long := strings.Repeat("mare ", 60)
	tests := []struct {
		name                 string
		e                    establishment.Public
		wantTitle, wantDesc  string
		wantImage            string
		wantWidth            int
		wantLocale, wantLang string
	}{
		{
			name:      "no logo: Arenile card, Italian defaults",
			e:         establishment.Public{Slug: "lido-sole", Name: "Lido Sole", DefaultLocale: "it"},
			wantTitle: "Lido Sole – Prenota il tuo posto in spiaggia", wantDesc: "Prenota online il tuo posto in spiaggia, senza commissioni.",
			wantImage: "https://lidi.test/b/og-image.png", wantWidth: 1200, wantLocale: "it_IT", wantLang: "it",
		},
		{
			name: "local logo and custom intro in German",
			e: establishment.Public{Slug: "strand", Name: "Strandbad", DefaultLocale: "de", Theme: store.Theme{
				LogoURL: "/uploads/logo.png", CustomTexts: map[string]map[string]string{"de": {"intro": "  Sonne\n und  Meer "}, "it": {"intro": "Sole"}},
			}},
			wantTitle: "Strandbad – Buchen Sie Ihren Platz am Strand", wantDesc: "Sonne und Meer",
			wantImage: "https://lidi.test/uploads/logo.png", wantLocale: "de_DE", wantLang: "de",
		},
		{
			name: "https logo, long intro truncated, unknown locale falls back to it",
			e: establishment.Public{Slug: "x", Name: "X", DefaultLocale: "es", Theme: store.Theme{
				LogoURL: "https://cdn.test/l.png", CustomTexts: map[string]map[string]string{"it": {"intro": long}},
			}},
			wantTitle: "X – Prenota il tuo posto in spiaggia", wantDesc: strings.TrimSpace(long[:199]) + "…",
			wantImage: "https://cdn.test/l.png", wantLocale: "it_IT", wantLang: "it",
		},
		{
			name:      "unsafe logo ignored",
			e:         establishment.Public{Slug: "y", Name: "Y", DefaultLocale: "en", Theme: store.Theme{LogoURL: "javascript:alert(1)"}},
			wantTitle: "Y – Book your spot on the beach", wantDesc: "Book your beach spot online, commission-free.",
			wantImage: "https://lidi.test/b/og-image.png", wantWidth: 1200, wantLocale: "en_GB", wantLang: "en",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := PageMetaOf(&tt.e, "https://lidi.test")
			if m.Title != tt.wantTitle || m.Description != tt.wantDesc || m.Image != tt.wantImage || m.ImageWidth != tt.wantWidth ||
				m.Locale != tt.wantLocale || m.Lang != tt.wantLang || m.URL != "https://lidi.test/b/"+tt.e.Slug {
				t.Fatalf("meta %+v", m)
			}
		})
	}
}

func TestBookingPageMeta_Resolver(t *testing.T) {
	f := BookingPageMeta(fakeProfiles{"lido-sole": {Slug: "lido-sole", Name: "Lido Sole", DefaultLocale: "it"}}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/b/lido-sole", nil)
	req.Host = "prenota.lido.test"
	req = req.WithContext(reqctx.With(req.Context(), &reqctx.Info{HTTPS: true}))
	m, ok := f(req, "lido-sole")
	if !ok || m.URL != "https://prenota.lido.test/b/lido-sole" || m.Image != "https://prenota.lido.test/b/og-image.png" {
		t.Fatalf("meta %+v ok %v", m, ok)
	}
	if _, ok := f(req, "missing"); ok {
		t.Fatal("unknown slug resolved")
	}
}
