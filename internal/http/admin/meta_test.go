package admin

import (
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/meta"
)

func TestMetaToAPI_DesktopModes(t *testing.T) {
	tests := []struct {
		name    string
		desktop *meta.Desktop
		wantQR  bool
		wantURL string
	}{
		{name: "server edition", desktop: nil},
		{name: "desktop local", desktop: &meta.Desktop{Mode: meta.DesktopLocal}},
		{
			name:    "desktop lan",
			desktop: &meta.Desktop{Mode: meta.DesktopLAN, LANURL: "https://192.168.1.20:8443", LANFingerprint: "AB:CD"},
			wantQR:  true, wantURL: "https://192.168.1.20:8443",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := MetaToAPI(meta.Meta{Runtime: meta.Runtime{Edition: meta.EditionDesktop, Desktop: tc.desktop}})
			if tc.desktop == nil {
				if out.Desktop != nil {
					t.Fatalf("desktop runtime on server edition: %+v", out.Desktop)
				}
				return
			}
			if got := out.Desktop.LanQrSvg != nil; got != tc.wantQR {
				t.Fatalf("lan_qr_svg present = %v, want %v", got, tc.wantQR)
			}
			if tc.wantQR {
				if *out.Desktop.LanUrl != tc.wantURL || !strings.HasPrefix(*out.Desktop.LanQrSvg, "<svg") {
					t.Fatalf("lan fields: %v %.40q", *out.Desktop.LanUrl, *out.Desktop.LanQrSvg)
				}
			}
		})
	}
}

func TestLANPanelAddress(t *testing.T) {
	for in, want := range map[string]string{
		"https://192.168.1.20:8443":  "https://192.168.1.20:8443/admin/",
		"https://192.168.1.20:8443/": "https://192.168.1.20:8443/admin/",
	} {
		if got := LANPanelAddress(in); got != want {
			t.Errorf("LANPanelAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
