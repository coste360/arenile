package pricingconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Agreement extensions of M8 (spec 30.2.2, ADR 0028): targets (partner,
// customer category, individual customers), fixed prices per pack and
// scope (pure, in the engine), seasons, quota configuration, requirement,
// channels, register category, versions with history and the agreement
// link. The applied condition stays in the price snapshot (INV-21); a new
// version never changes a booking already priced.

// ErrAgreementNotApplicable: the agreement is inactive, outside its
// validity or seasons, or not sold on the channel of the request
// (booking.ErrAgreementNotApplicable is the same value; 422
// agreement_not_applicable). It is also a configerr.ErrValidation, as the
// inactive agreement was before M8.
var ErrAgreementNotApplicable error = notApplicableError{}

type notApplicableError struct{}

func (notApplicableError) Error() string { return "booking: agreement not applicable" }

// Is makes errors.Is(err, configerr.ErrValidation) succeed.
func (notApplicableError) Is(target error) bool { return target == configerr.ErrValidation }

// Agreement channels (agreements.channels; empty = every channel, the
// behaviour before M8).
const (
	ChannelDesk       = "desk"
	ChannelOnlineCode = "online_code"
	ChannelOnlineLink = "online_link"
)

// DefaultReleaseDaysBefore is the default release of the unused reserved
// quota (ADR 0028, decision 4).
const DefaultReleaseDaysBefore = 3

// Bounds of the quota configuration.
const (
	maxQuotaUnits   = 10000
	maxReleaseDays  = 120
	maxRequirement  = 200
	maxPackPrices   = 100
	maxAgreementIDs = 200
)

var registerCategories = []string{
	store.RegisterResidents, store.RegisterAccommodation, store.RegisterProtected,
	store.RegisterCompanies, store.RegisterAssociations, store.RegisterOther,
}

// AgreementQuota is the quota per half-day (api.AgreementQuota).
type AgreementQuota struct {
	MaxPerDay         *int     `json:"max_per_day,omitempty"`
	ReservedPerDay    *int     `json:"reserved_per_day,omitempty"`
	ReleaseDaysBefore *int     `json:"release_days_before,omitempty"`
	ZoneIDs           []string `json:"zone_ids,omitempty"`
}

// ReleaseDays is release_days_before with its default.
func (q AgreementQuota) ReleaseDays() int {
	if q.ReleaseDaysBefore == nil {
		return DefaultReleaseDaysBefore
	}
	return *q.ReleaseDaysBefore
}

// Reserved is reserved_per_day (0 when unset).
func (q AgreementQuota) Reserved() int {
	if q.ReservedPerDay == nil {
		return 0
	}
	return *q.ReservedPerDay
}

// ReleaseOn is the first day on which the unused reserved capacity of d is
// back on general sale.
func (q AgreementQuota) ReleaseOn(d calendar.Date) calendar.Date {
	return d.AddDays(-q.ReleaseDays())
}

// PartnerRef is the partner of an agreement (badges).
type PartnerRef struct {
	ID   string
	Name string
	Kind string
}

// AgreementM8 holds the M8 fields of an agreement (input and output).
type AgreementM8 struct {
	Target             string // "" = individual customers (before M8)
	PartnerID          *string
	CustomerCategoryID *string
	PackPrices         []pricing.AgreementPackPrice
	Scope              *pricing.AgreementScope
	SeasonIDs          []string
	Quota              *AgreementQuota
	Requirement        string
	Channels           []string
	RegisterCategory   *string

	// Output only.
	Partner    *PartnerRef
	PublicLink string
}

// HasChannel reports whether the agreement is sold on ch (no channels =
// every channel).
func (m AgreementM8) HasChannel(ch string) bool {
	return len(m.Channels) == 0 || slices.Contains(m.Channels, ch)
}

// OnlineChannel reports whether the agreement can be applied online with
// its code (online_code or online_link, or no channel restriction).
func (m AgreementM8) OnlineChannel() bool {
	return m.HasChannel(ChannelOnlineCode) || m.HasChannel(ChannelOnlineLink)
}

// ApplicableTo checks the agreement for a quote: active and valid on today,
// in its seasons and sold on the booking channel (desk and api use desk;
// online uses the code channels). ErrAgreementNotApplicable otherwise.
func (a Agreement) ApplicableTo(today calendar.Date, seasonID string, channel pricing.Channel) error {
	if !a.ValidOn(today) {
		return ErrAgreementNotApplicable
	}
	if len(a.M8.SeasonIDs) > 0 && seasonID != "" && !slices.Contains(a.M8.SeasonIDs, seasonID) {
		return ErrAgreementNotApplicable
	}
	switch channel {
	case pricing.ChannelOnline:
		if !a.M8.OnlineChannel() {
			return ErrAgreementNotApplicable
		}
	default:
		if !a.M8.HasChannel(ChannelDesk) {
			return ErrAgreementNotApplicable
		}
	}
	return nil
}

// decodeAgreementM8 reads the M8 columns of r.
func decodeAgreementM8(r *store.Agreement, m *AgreementM8) error {
	if r.Target != nil {
		m.Target = *r.Target
	}
	m.PartnerID, m.CustomerCategoryID, m.Requirement, m.RegisterCategory = r.PartnerID, r.CustomerCategoryID, r.Requirement, r.RegisterCategory
	for _, c := range []struct {
		col store.JSON
		dst any
	}{
		{r.PackPrices, &m.PackPrices},
		{r.Scope, &m.Scope},
		{r.SeasonIDs, &m.SeasonIDs},
		{r.Quota, &m.Quota},
		{r.Channels, &m.Channels},
	} {
		if len(c.col) == 0 || string(c.col) == "null" {
			continue
		}
		if err := json.Unmarshal(c.col, c.dst); err != nil {
			return fmt.Errorf("pricingconfig: agreement %s: %w", r.ID, err)
		}
	}
	return nil
}

// encodeAgreementM8 writes the M8 columns of row (NULL when empty).
func encodeAgreementM8(row *store.Agreement, m AgreementM8) error {
	enc := func(empty bool, v any) (store.JSON, error) {
		if empty {
			return nil, nil
		}
		b, err := json.Marshal(v)
		return store.JSON(b), err
	}
	var err error
	if row.PackPrices, err = enc(len(m.PackPrices) == 0, m.PackPrices); err != nil {
		return err
	}
	if row.Scope, err = enc(m.Scope == nil, m.Scope); err != nil {
		return err
	}
	if row.SeasonIDs, err = enc(len(m.SeasonIDs) == 0, m.SeasonIDs); err != nil {
		return err
	}
	if row.Quota, err = enc(m.Quota == nil, m.Quota); err != nil {
		return err
	}
	if row.Channels, err = enc(len(m.Channels) == 0, m.Channels); err != nil {
		return err
	}
	row.Target = nil
	if m.Target != "" {
		t := m.Target
		row.Target = &t
	}
	row.PartnerID, row.CustomerCategoryID, row.Requirement, row.RegisterCategory = m.PartnerID, m.CustomerCategoryID, m.Requirement, m.RegisterCategory
	return nil
}

// validateAgreementM8 checks the M8 fields that need no database and
// normalises them.
func validateAgreementM8(v *configerr.ValidationError, in *AgreementInput) {
	m := &in.M8
	m.PartnerID, m.CustomerCategoryID = blankNil(m.PartnerID), blankNil(m.CustomerCategoryID)
	switch m.Target {
	case "", store.AgreementTargetCustomers:
		if m.PartnerID != nil && m.Target != "" {
			v.Add("partner_id", "Solo per le convenzioni con un partner.")
		}
		if m.CustomerCategoryID != nil {
			v.Add("customer_category_id", "Solo per le convenzioni di una categoria di clienti.")
		}
	case store.AgreementTargetPartner:
		if m.PartnerID == nil {
			v.Add("partner_id", "Indica il partner della convenzione.")
		}
		if m.CustomerCategoryID != nil {
			v.Add("customer_category_id", "Solo per le convenzioni di una categoria di clienti.")
		}
	case store.AgreementTargetCategory:
		if m.CustomerCategoryID == nil {
			v.Add("customer_category_id", "Indica la categoria di clienti.")
		}
		if m.PartnerID != nil {
			v.Add("partner_id", "Solo per le convenzioni con un partner.")
		}
	default:
		v.Add("target", "Usa partner, customer_category o individual_customers.")
	}
	m.Requirement = strings.TrimSpace(m.Requirement)
	if len([]rune(m.Requirement)) > maxRequirement {
		v.Add("requirement", fmt.Sprintf("Massimo %d caratteri.", maxRequirement))
	}
	m.Channels = cleanIDs(m.Channels)
	for _, ch := range m.Channels {
		switch ch {
		case ChannelDesk, ChannelOnlineCode, ChannelOnlineLink:
		default:
			v.Add("channels", "Usa desk, online_code o online_link.")
		}
	}
	if (slices.Contains(m.Channels, ChannelOnlineCode) || slices.Contains(m.Channels, ChannelOnlineLink)) && in.Code == nil {
		v.Add("code", "Il codice è obbligatorio per la vendita online della convenzione.")
	}
	if m.RegisterCategory != nil {
		rc := strings.TrimSpace(*m.RegisterCategory)
		switch {
		case rc == "":
			m.RegisterCategory = nil
		case !slices.Contains(registerCategories, rc):
			v.Add("register_category", "Categoria del registro sconosciuta.")
		default:
			m.RegisterCategory = &rc
		}
	}
	m.SeasonIDs = cleanIDs(m.SeasonIDs)
	if len(m.PackPrices) > maxPackPrices {
		v.Add("pack_prices", fmt.Sprintf("Al massimo %d prezzi.", maxPackPrices))
	}
	if s := m.Scope; s != nil {
		s.PackIDs, s.ExtraIDs = cleanIDs(s.PackIDs), cleanIDs(s.ExtraIDs)
	}
	if q := m.Quota; q != nil {
		q.ZoneIDs = cleanIDs(q.ZoneIDs)
		check := func(field string, p *int, lo int) {
			if p != nil && (*p < lo || *p > maxQuotaUnits) {
				v.Add("quota."+field, fmt.Sprintf("Tra %d e %d.", lo, maxQuotaUnits))
			}
		}
		check("max_per_day", q.MaxPerDay, 0)
		check("reserved_per_day", q.ReservedPerDay, 0)
		if q.ReleaseDaysBefore != nil && (*q.ReleaseDaysBefore < 0 || *q.ReleaseDaysBefore > maxReleaseDays) {
			v.Add("quota.release_days_before", fmt.Sprintf("Tra 0 e %d.", maxReleaseDays))
		}
		if q.MaxPerDay != nil && q.ReservedPerDay != nil && *q.ReservedPerDay > *q.MaxPerDay {
			v.Add("quota.reserved_per_day", "Non può superare max_per_day.")
		}
		if q.MaxPerDay == nil && q.Reserved() == 0 {
			m.Quota = nil // nothing to enforce
		}
	}
}

// checkAgreementRefs verifies that the partner, category, packs, extras,
// seasons and zones referenced by m belong to the establishment.
func checkAgreementRefs(ctx context.Context, db bun.IDB, es store.EstablishmentScope, m *AgreementM8) error {
	v := &configerr.ValidationError{}
	bad := func(field string, ids ...string) {
		for _, id := range ids {
			if _, err := uuid.Parse(id); err != nil {
				v.Add(field, "Identificativo non valido.")
				return
			}
		}
	}
	if m.PartnerID != nil {
		bad("partner_id", *m.PartnerID)
	}
	if m.CustomerCategoryID != nil {
		bad("customer_category_id", *m.CustomerCategoryID)
	}
	bad("season_ids", m.SeasonIDs...)
	if m.Quota != nil {
		bad("quota.zone_ids", m.Quota.ZoneIDs...)
	}
	if err := v.OrNil(); err != nil {
		return err
	}
	if m.PartnerID != nil {
		if _, err := store.GetPartner(ctx, db, es, *m.PartnerID); errors.Is(err, store.ErrNotFound) {
			v.Add("partner_id", "Partner inesistente.")
		} else if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
	}
	if m.CustomerCategoryID != nil {
		if _, err := store.GetCustomerCategory(ctx, db, es, *m.CustomerCategoryID); errors.Is(err, store.ErrNotFound) {
			v.Add("customer_category_id", "Categoria inesistente.")
		} else if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
	}
	packs, err := store.ListAllPacks(ctx, db, es)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	packIDs := map[string]bool{}
	for _, p := range packs {
		packIDs[p.ID] = true
	}
	for i, pp := range m.PackPrices {
		if !packIDs[pp.PackID] {
			v.Add(fmt.Sprintf("pack_prices[%d].pack_id", i), "Pack inesistente.")
		}
	}
	if s := m.Scope; s != nil {
		for _, id := range s.PackIDs {
			if !packIDs[id] {
				v.Add("scope.pack_ids", "Pack inesistente.")
				break
			}
		}
		if len(s.ExtraIDs) > 0 {
			xs, err := store.ListAllExtras(ctx, db, es)
			if err != nil {
				return fmt.Errorf("pricingconfig: %w", err)
			}
			ids := map[string]bool{}
			for _, x := range xs {
				ids[x.ID] = true
			}
			for _, id := range s.ExtraIDs {
				if !ids[id] {
					v.Add("scope.extra_ids", "Extra inesistente.")
					break
				}
			}
		}
	}
	for _, id := range m.SeasonIDs {
		if _, err := store.PricingGetSeason(ctx, db, es, id); errors.Is(err, store.ErrNotFound) {
			v.Add("season_ids", "Stagione inesistente.")
			break
		} else if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
	}
	if q := m.Quota; q != nil && len(q.ZoneIDs) > 0 {
		zones, err := store.ListZonesByIDs(ctx, db, es, q.ZoneIDs)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		if len(zones) != len(q.ZoneIDs) {
			v.Add("quota.zone_ids", "Zona inesistente.")
		}
	}
	return v.OrNil()
}

// fillAgreementOutput sets the partner reference and the public link of
// the agreements (one query each).
func fillAgreementOutput(ctx context.Context, db bun.IDB, es store.EstablishmentScope, as []*Agreement) error {
	var ids []string
	for _, a := range as {
		if a.M8.PartnerID != nil {
			ids = append(ids, *a.M8.PartnerID)
		}
	}
	partners, err := store.PartnersByIDs(ctx, db, es, ids)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	slug := ""
	for _, a := range as {
		if a.M8.PartnerID != nil {
			if p, ok := partners[*a.M8.PartnerID]; ok {
				a.M8.Partner = &PartnerRef{ID: p.ID, Name: p.Name, Kind: p.Kind}
			}
		}
		if a.Code == nil || !slices.Contains(a.M8.Channels, ChannelOnlineLink) {
			continue
		}
		if slug == "" {
			est, err := store.GetEstablishment(ctx, db, es)
			if err != nil {
				return fmt.Errorf("pricingconfig: %w", err)
			}
			slug = est.Slug
		}
		a.M8.PublicLink = AgreementLink(slug, *a.Code)
	}
	return nil
}

// AgreementLink is the path of the agreement link on the public page.
func AgreementLink(slug, code string) string {
	return "/b/" + slug + "?c=" + code
}

// ---- versions -------------------------------------------------------------------------

// agreementSnapshot is the JSON of an agreement version (the shape of
// api.AgreementInput).
type agreementSnapshot struct {
	Code               *string                      `json:"code,omitempty"`
	Name               string                       `json:"name"`
	PartnerName        string                       `json:"partner_name,omitempty"`
	Notes              string                       `json:"notes,omitempty"`
	Action             pricing.Action               `json:"action"`
	Conditions         pricing.Conditions           `json:"conditions"`
	ValidFrom          *calendar.Date               `json:"valid_from,omitempty"`
	ValidTo            *calendar.Date               `json:"valid_to,omitempty"`
	Stackable          bool                         `json:"stackable"`
	Active             bool                         `json:"active"`
	Target             string                       `json:"target,omitempty"`
	PartnerID          *string                      `json:"partner_id,omitempty"`
	CustomerCategoryID *string                      `json:"customer_category_id,omitempty"`
	PackPrices         []pricing.AgreementPackPrice `json:"pack_prices,omitempty"`
	Scope              *pricing.AgreementScope      `json:"scope,omitempty"`
	SeasonIDs          []string                     `json:"season_ids,omitempty"`
	Quota              *AgreementQuota              `json:"quota,omitempty"`
	Requirement        string                       `json:"requirement,omitempty"`
	Channels           []string                     `json:"channels,omitempty"`
	RegisterCategory   *string                      `json:"register_category,omitempty"`
}

func snapshotOf(a Agreement) agreementSnapshot {
	m := a.M8
	return agreementSnapshot{
		Code: a.Code, Name: a.Name, PartnerName: a.PartnerName, Notes: a.Notes, Action: a.Action, Conditions: a.Conditions,
		ValidFrom: a.ValidFrom, ValidTo: a.ValidTo, Stackable: a.Stackable, Active: a.Active, Target: m.Target,
		PartnerID: m.PartnerID, CustomerCategoryID: m.CustomerCategoryID, PackPrices: m.PackPrices, Scope: m.Scope,
		SeasonIDs: m.SeasonIDs, Quota: m.Quota, Requirement: m.Requirement, Channels: m.Channels, RegisterCategory: m.RegisterCategory,
	}
}

func (x agreementSnapshot) input() AgreementInput {
	return AgreementInput{
		Code: x.Code, Name: x.Name, PartnerName: x.PartnerName, Notes: x.Notes, Action: x.Action, Conditions: x.Conditions,
		ValidFrom: x.ValidFrom, ValidTo: x.ValidTo, Stackable: x.Stackable, Active: x.Active,
		M8: AgreementM8{
			Target: x.Target, PartnerID: x.PartnerID, CustomerCategoryID: x.CustomerCategoryID, PackPrices: x.PackPrices,
			Scope: x.Scope, SeasonIDs: x.SeasonIDs, Quota: x.Quota, Requirement: x.Requirement, Channels: x.Channels,
			RegisterCategory: x.RegisterCategory,
		},
	}
}

// recordAgreementVersion stores the immutable snapshot of a at its version.
func recordAgreementVersion(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, p *auth.Principal, a Agreement, now time.Time) error {
	b, err := json.Marshal(snapshotOf(a))
	if err != nil {
		return fmt.Errorf("pricingconfig: agreement version: %w", err)
	}
	row := &store.AgreementVersion{
		ID: store.NewID(), AgreementID: a.ID, Version: a.Version, Snapshot: store.JSON(b), ChangedAt: now,
	}
	if p != nil && p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		row.ChangedByUserID = store.ExistingUser(ctx, tx, &u)
	}
	if err := store.InsertAgreementVersion(ctx, tx, es, row); err != nil {
		return fmt.Errorf("pricingconfig: agreement version: %w", err)
	}
	return nil
}

// AgreementVersionView is one stored version (api.AgreementVersion).
type AgreementVersionView struct {
	Version         int64
	ChangedAt       time.Time
	ChangedByUserID *string
	ChangedByName   string
	Input           AgreementInput
}

// ListAgreementVersions returns the history of an agreement, newest first
// (view permission). An agreement saved before M8 has no history.
func (s *Service) ListAgreementVersions(ctx context.Context, p *auth.Principal, establishmentID, id string) ([]AgreementVersionView, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if _, err := store.GetAgreement(ctx, s.d.DB, es, id); err != nil {
		return nil, mapStoreErr(err, nil)
	}
	rows, err := store.ListAgreementVersions(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	var uids []string
	for _, r := range rows {
		if r.ChangedByUserID != nil {
			uids = append(uids, *r.ChangedByUserID)
		}
	}
	names := map[string]string{}
	if len(uids) > 0 {
		if names, err = store.UserNames(ctx, s.d.DB, uids); err != nil {
			return nil, fmt.Errorf("pricingconfig: %w", err)
		}
	}
	out := make([]AgreementVersionView, 0, len(rows))
	for _, r := range rows {
		var snap agreementSnapshot
		if err := json.Unmarshal(r.Snapshot, &snap); err != nil {
			return nil, fmt.Errorf("pricingconfig: agreement version %s: %w", r.ID, err)
		}
		v := AgreementVersionView{Version: r.Version, ChangedAt: r.ChangedAt.UTC(), ChangedByUserID: r.ChangedByUserID, Input: snap.input()}
		if r.ChangedByUserID != nil {
			v.ChangedByName = names[*r.ChangedByUserID]
		}
		out = append(out, v)
	}
	return out, nil
}

// ---- quota usage -------------------------------------------------------------------------

// QuotaDay is the quota of one half-day (api.AgreementQuotaDay).
type QuotaDay struct {
	Date      calendar.Date
	Slot      calendar.Slot
	Reserved  int
	Used      int
	Max       *int
	ReleaseOn calendar.Date
	Released  bool
}

// QuotaUsage is api.AgreementQuotaUsage.
type QuotaUsage struct {
	Quota *AgreementQuota
	Days  []QuotaDay
}

// AgreementQuotaUsage returns the quota of an agreement per half-day in
// [from, to] (view permission): the configured reserve and cap with the
// units sold (agreement_quota_usage) and whether the unused reserve is
// already back on general sale (ADR 0028).
func (s *Service) AgreementQuotaUsage(ctx context.Context, p *auth.Principal, establishmentID, id string, from, to calendar.Date) (*QuotaUsage, error) {
	es, err := authorize(ctx, s.d.DB, p, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	row, err := store.GetAgreement(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	a, err := agreementFromRow(row)
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, es)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: timezone: %w", err)
	}
	today := calendar.In(s.d.Clock.Now(), loc)
	used, err := store.AgreementQuotaUsageRange(ctx, s.d.DB, es, id, from, to)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	out := &QuotaUsage{Quota: a.M8.Quota, Days: []QuotaDay{}}
	byKey := map[string]store.AgreementQuotaUsage{}
	for _, u := range used {
		byKey[u.Date.String()+string(u.Slot)] = u
	}
	q := a.M8.Quota
	for _, d := range calendar.Range(from, to) {
		for _, sl := range []calendar.Slot{calendar.AM, calendar.PM} {
			u, ok := byKey[d.String()+string(sl)]
			if q == nil && !ok {
				continue
			}
			day := QuotaDay{Date: d, Slot: sl, Used: u.Used}
			if q != nil {
				day.Reserved, day.Max, day.ReleaseOn = q.Reserved(), q.MaxPerDay, q.ReleaseOn(d)
			} else {
				day.Reserved, day.Max, day.ReleaseOn = u.Reserved, u.MaxUnits, u.ReleaseOn
			}
			day.Released = !today.Before(day.ReleaseOn)
			out.Days = append(out.Days, day)
		}
	}
	return out, nil
}

func blankNil(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}
