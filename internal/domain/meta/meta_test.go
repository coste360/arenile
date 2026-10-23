package meta

import (
	"os"
	"regexp"
	"slices"
	"testing"
	"time"
)

func TestBuild_Editions(t *testing.T) {
	now := time.Date(2027, 7, 1, 10, 0, 0, 0, time.FixedZone("CEST", 7200))
	tests := []struct {
		name           string
		rt             Runtime
		onlineBooking  bool
		onlinePayments bool
		pairing        bool
		bridge         bool
		providers      []string
		hasFeature     string
	}{
		{"server", Runtime{}, true, true, true, true, []string{"manual", "stripe"}, FeaturePublicPage},
		{"desktop local", Runtime{Edition: EditionDesktop, Desktop: &Desktop{Mode: DesktopLocal}}, false, false, false, false, []string{"manual"}, FeatureDesktopBackups},
		{"desktop lan", Runtime{Edition: EditionDesktop, Desktop: &Desktop{Mode: DesktopLAN, LANURL: "https://192.168.1.2:8443"}}, false, false, true, false, []string{"manual"}, FeatureDevicePairing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Build(Input{Version: "v1.2.3", Runtime: tt.rt, Now: now, PaymentProviders: []string{"manual", "stripe"}, FiscalDrivers: []string{"none"}})
			c := m.Capabilities
			if c.OnlineBooking != tt.onlineBooking || c.OnlinePayments != tt.onlinePayments || c.DevicePairing != tt.pairing || c.FiscalBridge != tt.bridge {
				t.Fatalf("capabilities %+v", c)
			}
			if !slices.Equal(c.PaymentProviders, tt.providers) || !slices.Contains(m.Features, tt.hasFeature) {
				t.Fatalf("providers %v features %v", c.PaymentProviders, m.Features)
			}
			if m.ServerTime.Location() != time.UTC || m.APIVersion != APIVersion || m.Runtime.Edition == "" {
				t.Fatalf("meta %+v", m)
			}
			if (m.Runtime.Edition == EditionDesktop) != (m.Runtime.Desktop != nil) {
				t.Fatalf("desktop block %+v", m.Runtime)
			}
		})
	}
}

func TestBuild_ServerWithoutOnlineProvider(t *testing.T) {
	m := Build(Input{PaymentProviders: []string{"manual"}})
	if m.Capabilities.OnlinePayments || slices.Contains(m.Features, FeatureOnlinePayments) {
		t.Fatal("manual only is not online payments")
	}
	if m.Capabilities.FiscalDrivers == nil {
		t.Fatal("nil slices must be empty")
	}
}

func TestAPIVersion_MatchesOpenAPI(t *testing.T) {
	b, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^info:\n(?:  .*\n)*?  version: (\S+)`).FindSubmatch(b)
	if len(m) < 2 || string(m[1]) != APIVersion {
		t.Fatalf("api/openapi.yaml info.version = %q, meta.APIVersion = %q", m, APIVersion)
	}
}
