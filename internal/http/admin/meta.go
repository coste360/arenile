package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/meta"
	"github.com/coste360/arenile/internal/http/api"
)

// Instance metadata (M6/M7, spec 29.2). Public: no establishment or
// personal data.
//
//	GetMeta -> Meta.Get -> api.Meta

// GetMeta implements GET /api/admin/v1/meta.
func (h *Handlers) GetMeta(w http.ResponseWriter, r *http.Request) {
	svc := h.Meta
	if svc == nil {
		svc = meta.NewService(meta.Deps{})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, MetaToAPI(svc.Get(r.Context())))
}

// MetaToAPI maps the domain metadata to the API representation.
func MetaToAPI(m meta.Meta) api.Meta {
	out := api.Meta{
		Product: api.Arenile, Version: m.Version, ApiVersion: m.APIVersion,
		Edition: api.Edition(m.Runtime.Edition), ServerTime: m.ServerTime.UTC(),
		MinClientVersions: api.MinClientVersions{Mobile: m.MinMobileVersion, Desktop: m.MinDesktopVersion},
		Capabilities: api.MetaCapabilities{
			OnlineBooking: m.Capabilities.OnlineBooking, OnlinePayments: m.Capabilities.OnlinePayments,
			PaymentProviders: nonNil(m.Capabilities.PaymentProviders), FiscalDrivers: nonNil(m.Capabilities.FiscalDrivers),
			DevicePairing: m.Capabilities.DevicePairing, FiscalBridge: m.Capabilities.FiscalBridge,
			Realtime: m.Capabilities.Realtime, PushNotifications: m.Capabilities.PushNotifications,
		},
		Features: nonNil(m.Features),
	}
	if d := m.Runtime.Desktop; d != nil {
		rt := &api.DesktopRuntime{Mode: api.DesktopRuntimeMode(d.Mode)}
		if d.LANURL != "" {
			u, fp := d.LANURL, d.LANFingerprint
			rt.LanUrl, rt.LanCertFingerprint = &u, &fp
		}
		out.Desktop = rt
	}
	return out
}
