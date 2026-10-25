package meta

import (
	"context"
	"slices"
	"time"

	"github.com/coste360/arenile/internal/clock"
)

// APIVersion is info.version of api/openapi.yaml (kept equal by a test).
const APIVersion = "0.1.0"

// Minimum client versions supported by this server (semver).
const (
	MinMobileVersion  = "1.0.0"
	MinDesktopVersion = "0.1.0"
)

// Edition of the running binary.
type Edition string

// Editions (api.Edition).
const (
	EditionServer  Edition = "server"
	EditionDesktop Edition = "desktop"
)

// DesktopMode is the reachability of the desktop edition server.
type DesktopMode string

// Desktop modes (api.DesktopRuntimeMode). The desktop "remote" mode is a
// client of another instance and never reported by a server.
const (
	DesktopLocal DesktopMode = "local"
	DesktopLAN   DesktopMode = "lan"
)

// Desktop is the state of the desktop edition.
type Desktop struct {
	Mode           DesktopMode
	LANURL         string
	LANFingerprint string
	// LANCertRenewsAt is when the self-signed certificate will be
	// regenerated (fingerprint change); zero when unknown or off.
	LANCertRenewsAt time.Time
}

// Runtime is the edition and, for the desktop edition, its state.
type Runtime struct {
	Edition Edition
	Desktop *Desktop
}

// RuntimeProvider returns the current runtime (may change while running).
type RuntimeProvider interface {
	Runtime() Runtime
}

// ServerRuntime is the RuntimeProvider of the server binary.
type ServerRuntime struct{}

// Runtime implements RuntimeProvider.
func (ServerRuntime) Runtime() Runtime { return Runtime{Edition: EditionServer} }

// Drivers lists the registered extension codes (app.Registry).
type Drivers interface {
	PaymentProviderCodes() []string
	FiscalDriverCodes() []string
}

// Capabilities mirror api.MetaCapabilities.
type Capabilities struct {
	OnlineBooking     bool
	OnlinePayments    bool
	PaymentProviders  []string
	FiscalDrivers     []string
	DevicePairing     bool
	FiscalBridge      bool
	Realtime          bool
	PushNotifications bool
}

// Meta mirrors api.Meta.
type Meta struct {
	Version           string
	APIVersion        string
	Runtime           Runtime
	ServerTime        time.Time
	MinMobileVersion  string
	MinDesktopVersion string
	Capabilities      Capabilities
	Features          []string
}

// Input is everything Build needs.
type Input struct {
	Version           string
	Runtime           Runtime
	Now               time.Time
	PaymentProviders  []string
	FiscalDrivers     []string
	PushNotifications bool
}

// Feature slugs (api.Meta.features).
const (
	FeatureBookings       = "bookings"
	FeatureCash           = "cash"
	FeatureSeasonPasses   = "season_passes"
	FeaturePublicPage     = "public_page"
	FeatureOnlinePayments = "online_payments"
	FeatureDevicePairing  = "device_pairing"
	FeatureFiscalBridge   = "fiscal_bridge"
	FeatureDesktopBackups = "desktop_backups"
	FeatureLANMode        = "lan_mode"
)

// Build computes the metadata (pure).
func Build(in Input) Meta {
	desktop := in.Runtime.Edition == EditionDesktop
	lan := desktop && in.Runtime.Desktop != nil && in.Runtime.Desktop.Mode == DesktopLAN
	caps := Capabilities{
		OnlineBooking:     !desktop,
		OnlinePayments:    !desktop && slices.ContainsFunc(in.PaymentProviders, func(c string) bool { return c != "manual" }),
		PaymentProviders:  nonNil(in.PaymentProviders),
		FiscalDrivers:     nonNil(in.FiscalDrivers),
		DevicePairing:     !desktop || lan,
		FiscalBridge:      !desktop,
		Realtime:          true,
		PushNotifications: in.PushNotifications && !desktop,
	}
	if desktop {
		caps.PaymentProviders = []string{"manual"}
	}
	features := []string{FeatureBookings, FeatureCash, FeatureSeasonPasses}
	if caps.OnlineBooking {
		features = append(features, FeaturePublicPage)
	}
	if caps.OnlinePayments {
		features = append(features, FeatureOnlinePayments)
	}
	if caps.DevicePairing {
		features = append(features, FeatureDevicePairing)
	}
	if caps.FiscalBridge {
		features = append(features, FeatureFiscalBridge)
	}
	if desktop {
		features = append(features, FeatureDesktopBackups, FeatureLANMode)
	}
	rt := in.Runtime
	if rt.Edition == "" {
		rt.Edition = EditionServer
	}
	if !desktop {
		rt.Desktop = nil
	}
	return Meta{
		Version: in.Version, APIVersion: APIVersion, Runtime: rt, ServerTime: in.Now.UTC(),
		MinMobileVersion: MinMobileVersion, MinDesktopVersion: MinDesktopVersion,
		Capabilities: caps, Features: features,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return slices.Clone(s)
}

// Deps are the dependencies of Service.
type Deps struct {
	Version string
	Clock   clock.Clock
	Runtime RuntimeProvider
	Drivers Drivers
	// PushNotifications: a push relay key is configured (ADR 0027). Build
	// still reports false on the desktop edition.
	PushNotifications bool
}

// Service gathers the input of Build at request time.
type Service struct{ d Deps }

// NewService returns a Service (nil Runtime = ServerRuntime).
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Runtime == nil {
		d.Runtime = ServerRuntime{}
	}
	return &Service{d: d}
}

// Get returns the current metadata.
func (s *Service) Get(_ context.Context) Meta {
	in := Input{Version: s.d.Version, Runtime: s.d.Runtime.Runtime(), Now: s.d.Clock.Now(), PushNotifications: s.d.PushNotifications}
	if s.d.Drivers != nil {
		in.PaymentProviders = s.d.Drivers.PaymentProviderCodes()
		in.FiscalDrivers = s.d.Drivers.FiscalDriverCodes()
	}
	return Build(in)
}

// Edition reports the current edition (used by handlers that exist only on
// the desktop edition).
func (s *Service) Edition() Edition { return s.d.Runtime.Runtime().Edition }
