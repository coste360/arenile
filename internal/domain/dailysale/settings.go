package dailysale

import (
	"context"
	"errors"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// maxFooter bounds each translation of the receipt footer.
const maxFooter = 300

// SettingsInput is api.DailySaleSettingsUpdate.
type SettingsInput struct {
	PrintFormat      string
	AutoPrint        bool
	ZoneFirstEnabled bool
	FooterText       store.I18nText
	// ExpectedVersion is the If-Match version (nil: no precondition).
	ExpectedVersion *int64
}

// defaultSettings are the settings of an establishment that never saved
// them (version 1, the version of the row EnsureDailySaleSettings writes).
func defaultSettings(estID string) *store.DailySaleSettings {
	return &store.DailySaleSettings{
		EstablishmentID: estID, PrintFormat: store.PrintThermal80, AutoPrint: true, ZoneFirstEnabled: true,
		FooterText: store.I18nText{}, Version: 1,
	}
}

// Settings returns the daily sale settings (auth.PermBookingsRead); the
// defaults when never saved (no write).
func (s *Service) Settings(ctx context.Context, p *auth.Principal, establishmentID string) (*store.DailySaleSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	st, err := store.GetDailySaleSettings(ctx, s.d.DB, es)
	if errors.Is(err, store.ErrNotFound) {
		return defaultSettings(es.ID()), nil
	}
	return st, err
}

// UpdateSettings saves the daily sale settings (auth.PermEstablishmentConfigure)
// with optimistic concurrency on the version.
func (s *Service) UpdateSettings(ctx context.Context, p *auth.Principal, establishmentID string, in SettingsInput) (*store.DailySaleSettings, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	switch in.PrintFormat {
	case store.PrintThermal80, store.PrintThermal58, store.PrintA4, store.PrintA5:
	default:
		ve.Add("print_format", "Usa thermal_80, thermal_58, a4 o a5.")
	}
	footer := store.I18nText{}
	for lang, text := range in.FooterText {
		text = strings.TrimSpace(text)
		if len([]rune(text)) > maxFooter {
			ve.Add("footer_text."+lang, "Massimo 300 caratteri.")
		}
		if text != "" {
			footer[lang] = text
		}
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	var out *store.DailySaleSettings
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		now := s.now()
		if err := store.EnsureDailySaleSettings(ctx, tx, es, now); err != nil {
			return err
		}
		st, err := store.LockDailySaleSettings(ctx, tx, es)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(in.ExpectedVersion, st.Version); err != nil {
			return err
		}
		before := map[string]any{"print_format": st.PrintFormat, "auto_print": st.AutoPrint, "zone_first_enabled": st.ZoneFirstEnabled}
		st.PrintFormat, st.AutoPrint, st.ZoneFirstEnabled, st.FooterText, st.UpdatedAt = in.PrintFormat, in.AutoPrint, in.ZoneFirstEnabled, footer, now
		ok, err := store.UpdateDailySaleSettingsVersioned(ctx, tx, es, st, st.Version)
		if err != nil {
			return err
		}
		if !ok {
			return &configerr.VersionConflictError{Current: st.Version}
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(reqctx.From(ctx).ClientIP), Action: "daily_sale.settings_updated",
			EntityType: "daily_sale_settings", EntityID: es.ID(),
			Diff: map[string]any{"before": before, "print_format": st.PrintFormat, "auto_print": st.AutoPrint, "zone_first_enabled": st.ZoneFirstEnabled},
		}); err != nil {
			return err
		}
		out = st
		return nil
	})
	return out, err
}
