package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository of the online configuration (M4, spec 12, 13.1, 19): the
// online_settings row of an establishment, the immutable terms versions,
// the payment provider settings and the online columns of establishments
// (booking_enabled, allowed_embed_origins, theme). Owned by
// internal/domain/online, which validates every value before writing.

// GetOnlineSettings returns the online settings row of es (ErrNotFound
// when the establishment never saved one).
func GetOnlineSettings(ctx context.Context, db bun.IDB, es EstablishmentScope) (*OnlineSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(OnlineSettings)
	err := db.NewSelect().Model(row).Where("os.establishment_id = ?", es.ID()).Scan(ctx)
	return row, wrapRead("get online settings", err)
}

// EnsureOnlineSettings returns the online settings row of es, inserting the
// defaults (version 1) on first use. Concurrent first uses converge on the
// same row (INSERT ... ON CONFLICT DO NOTHING on the primary key).
func EnsureOnlineSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, config, emailConfig JSON, now time.Time) (*OnlineSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := &OnlineSettings{
		EstablishmentID: es.ID(), Config: config, EmailConfig: emailConfig, Version: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if _, err := db.NewInsert().Model(row).On("CONFLICT (establishment_id) DO NOTHING").Exec(ctx); err != nil {
		return nil, wrapWrite("ensure online settings", err)
	}
	return GetOnlineSettings(ctx, db, es)
}

// UpdateOnlineSettingsVersioned writes config, email config and current
// terms of row when the stored version is still oldVersion and sets
// row.Version to oldVersion+1. It reports false when no row matched.
func UpdateOnlineSettingsVersioned(ctx context.Context, db bun.IDB, es EstablishmentScope, row *OnlineSettings, oldVersion int64) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	row.Version = oldVersion + 1
	res, err := db.NewUpdate().Model(row).
		Column("config", "email_config", "current_terms_version_id", "version", "updated_at").
		Where("establishment_id = ?", es.ID()).Where("version = ?", oldVersion).
		Exec(ctx)
	if err != nil {
		row.Version = oldVersion
		return false, wrapWrite("update online settings", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update online settings: %w", err)
	}
	if n != 1 {
		row.Version = oldVersion
		return false, nil
	}
	return true, nil
}

// UpdateEstablishmentOnline writes the online columns of the establishment
// of es (booking_enabled, allowed_embed_origins, theme) and bumps its
// configuration version, in the caller's transaction.
func UpdateEstablishmentOnline(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingEnabled bool, origins []string, theme Theme, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	if origins == nil {
		origins = []string{}
	}
	e := &Establishment{BookingEnabled: bookingEnabled, AllowedEmbedOrigins: origins, Theme: theme, UpdatedAt: now.UTC()}
	res, err := db.NewUpdate().Model(e).
		Column("booking_enabled", "allowed_embed_origins", "theme", "updated_at").
		Set("version = version + 1").
		Where("id = ?", es.ID()).
		Exec(ctx)
	if err != nil {
		return wrapWrite("update establishment online", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// ---- terms versions -------------------------------------------------------------

// InsertTermsVersion inserts an immutable terms version (ErrConflict when
// the label is already published for the establishment).
func InsertTermsVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, tv *TermsVersion) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	tv.EstablishmentID = es.ID()
	if tv.Texts == nil {
		tv.Texts = map[string]TermsTexts{}
	}
	_, err := db.NewInsert().Model(tv).Exec(ctx)
	return wrapWrite("insert terms version", err)
}

// GetTermsVersion returns one terms version of es.
func GetTermsVersion(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*TermsVersion, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	tv := new(TermsVersion)
	err := db.NewSelect().Model(tv).Where("tv.id = ?", id).Where("tv.establishment_id = ?", es.ID()).Scan(ctx)
	return tv, wrapRead("get terms version", err)
}

// ListTermsVersions lists the terms versions of es, newest first (at most
// limit rows; the list is short by nature: one row per publication).
func ListTermsVersions(ctx context.Context, db bun.IDB, es EstablishmentScope, limit int) ([]TermsVersion, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	var items []TermsVersion
	err := db.NewSelect().Model(&items).Where("tv.establishment_id = ?", es.ID()).
		OrderExpr("tv.published_at DESC, tv.id DESC").Limit(int64(limit)).Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list terms versions: %w", err)
	}
	if items == nil {
		items = []TermsVersion{}
	}
	return items, nil
}

// ---- payment provider settings ----------------------------------------------------

// GetPaymentProviderSettings returns the settings of provider and mode for
// es (ErrNotFound when never configured).
func GetPaymentProviderSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, provider, mode string) (*PaymentProviderSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(PaymentProviderSettings)
	err := db.NewSelect().Model(row).Where("pps.establishment_id = ?", es.ID()).
		Where("pps.provider = ?", provider).Where("pps.mode = ?", mode).Scan(ctx)
	return row, wrapRead("get payment provider settings", err)
}

// GetActivePaymentProvider returns the active provider settings of es
// (ErrNotFound when no provider is active: the manual driver applies).
func GetActivePaymentProvider(ctx context.Context, db bun.IDB, es EstablishmentScope) (*PaymentProviderSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(PaymentProviderSettings)
	err := db.NewSelect().Model(row).Where("pps.establishment_id = ?", es.ID()).Where("pps.active = ?", true).
		Limit(1).Scan(ctx)
	return row, wrapRead("get active payment provider", err)
}

// LatestPaymentProviderSettings returns the most recently updated settings
// of provider for es in any mode (webhooks of a provider must verify with
// the configuration that created the payment). ErrNotFound when none.
func LatestPaymentProviderSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, provider string) (*PaymentProviderSettings, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	row := new(PaymentProviderSettings)
	err := db.NewSelect().Model(row).Where("pps.establishment_id = ?", es.ID()).Where("pps.provider = ?", provider).
		OrderExpr("pps.active DESC, pps.updated_at DESC, pps.id DESC").Limit(1).Scan(ctx)
	return row, wrapRead("get payment provider settings", err)
}

// DeactivatePaymentProviders clears the active flag of every provider row
// of es (before activating another one: the partial unique index allows a
// single active row).
func DeactivatePaymentProviders(ctx context.Context, db bun.IDB, es EstablishmentScope, now time.Time) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	_, err := db.NewUpdate().Model((*PaymentProviderSettings)(nil)).
		Set("active = ?", false).Set("updated_at = ?", now.UTC()).Set("version = version + 1").
		Where("establishment_id = ?", es.ID()).Where("active = ?", true).
		Exec(ctx)
	return wrapWrite("deactivate payment providers", err)
}

// SavePaymentProviderSettings inserts row or, when the (provider, mode)
// row exists, rewrites it (credentials, public config, active flag,
// verification) bumping its version.
func SavePaymentProviderSettings(ctx context.Context, db bun.IDB, es EstablishmentScope, row *PaymentProviderSettings) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	row.EstablishmentID = es.ID()
	if row.PublicConfig == nil {
		row.PublicConfig = map[string]any{}
	}
	cur, err := GetPaymentProviderSettings(ctx, db, es, row.Provider, row.Mode)
	switch {
	case err == nil:
		row.ID, row.CreatedAt, row.Version = cur.ID, cur.CreatedAt, cur.Version+1
		_, err = db.NewUpdate().Model(row).
			Column("active", "credentials", "public_config", "verified_at", "version", "updated_at").
			Where("id = ?", cur.ID).Where("establishment_id = ?", es.ID()).
			Exec(ctx)
		return wrapWrite("update payment provider settings", err)
	case errors.Is(err, ErrNotFound):
		if row.ID == "" {
			row.ID = NewID()
		}
		if row.Version == 0 {
			row.Version = 1
		}
		_, err = db.NewInsert().Model(row).Exec(ctx)
		return wrapWrite("insert payment provider settings", err)
	default:
		return err
	}
}

// MarkPaymentProviderVerified records a successful credentials check.
func MarkPaymentProviderVerified(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time, label string) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	row, err := getPaymentProviderSettingsByID(ctx, db, es, id)
	if err != nil {
		return err
	}
	t := at.UTC()
	row.VerifiedAt = &t
	if row.PublicConfig == nil {
		row.PublicConfig = map[string]any{}
	}
	if label != "" {
		row.PublicConfig["account_label"] = label
	}
	_, err = db.NewUpdate().Model(row).Column("verified_at", "public_config").
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return wrapWrite("verify payment provider settings", err)
}

func getPaymentProviderSettingsByID(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*PaymentProviderSettings, error) {
	row := new(PaymentProviderSettings)
	err := db.NewSelect().Model(row).Where("pps.id = ?", id).Where("pps.establishment_id = ?", es.ID()).Scan(ctx)
	return row, wrapRead("get payment provider settings", err)
}
