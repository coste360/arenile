package checkout

import (
	"context"
	"fmt"
	"slices"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/store"
)

// Profile is the online flow configuration published with the public
// profile (api.PublicOnlineConfig): what the page offers and how it pays.
type Profile struct {
	Establishment *online.PublicEstablishment
	// Formulas offered: the configured list, or every formula allowed by an
	// online pack (sorted).
	Formulas    []string
	Slots       []string
	HoldMinutes int
	// Maps holding at least one zone visible online, Zones visible online
	// (display order).
	Maps  []store.Map
	Zones []store.Zone
	// MinDate is today; MaxDate the earliest of the booking window limit,
	// the online maximum lead time (policies) and the end of the last season.
	MinDate, MaxDate *calendar.Date
}

// Profile returns the online configuration of slug (ErrBookingDisabled
// when online booking is off: the profile is then published without it).
func (s *Service) Profile(ctx context.Context, slug string) (*Profile, error) {
	pe, err := s.establishment(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	es := pe.Scope
	cat, err := s.loadCatalog(ctx, s.d.DB, es, nil)
	if err != nil {
		return nil, err
	}
	out := &Profile{Establishment: pe, Slots: pe.Flow.EffectiveSlots(), HoldMinutes: int(s.d.HoldTTL.Minutes()), Zones: cat.onlineZones("")}
	if out.HoldMinutes < 1 {
		out.HoldMinutes = 1
	}
	out.Formulas = slices.Clone(pe.Flow.Formulas)
	if len(out.Formulas) == 0 {
		for _, p := range cat.onlinePacks() {
			for _, f := range p.AllowedFormulas {
				if !slices.Contains(out.Formulas, f) {
					out.Formulas = append(out.Formulas, f)
				}
			}
		}
		slices.Sort(out.Formulas)
	}
	mapIDs := []string{}
	for _, z := range out.Zones {
		if z.MapID != nil && !slices.Contains(mapIDs, *z.MapID) {
			mapIDs = append(mapIDs, *z.MapID)
		}
	}
	for _, id := range mapIDs {
		m, err := store.GetMap(ctx, s.d.DB, es, id)
		if err != nil {
			return nil, fmt.Errorf("checkout: map: %w", err)
		}
		out.Maps = append(out.Maps, *m)
	}
	slices.SortFunc(out.Maps, func(a, b store.Map) int { return a.SortOrder - b.SortOrder })
	today := s.today(pe.Establishment)
	out.MinDate = &today
	seasons, err := store.SeasonsOverlapping(ctx, s.d.DB, es, today, today.AddDays(online.MaxDaysAheadLimit))
	if err != nil {
		return nil, fmt.Errorf("checkout: seasons: %w", err)
	}
	var maxDate *calendar.Date
	for _, sn := range seasons {
		c := sn.ClosesOn
		if maxDate == nil || c.After(*maxDate) {
			maxDate = &c
		}
	}
	if pe.Flow.MaxDaysAhead > 0 {
		lim := today.AddDays(pe.Flow.MaxDaysAhead)
		if maxDate == nil || lim.Before(*maxDate) {
			maxDate = &lim
		}
	}
	lead, err := s.onlineMaxLeadDays(ctx, es, pe.Establishment, seasons)
	if err != nil {
		return nil, err
	}
	if lead != nil {
		lim := today.AddDays(*lead)
		if maxDate == nil || lim.Before(*maxDate) {
			maxDate = &lim
		}
	}
	out.MaxDate = maxDate
	return out, nil
}

// onlineMaxLeadDays is the most permissive maximum lead time (days) that an
// online booking can have: the establishment default overlaid by every
// active policy of the given seasons whose scope admits the online channel.
// nil means unlimited. The calendar uses it as an upper bound so that the
// page never offers dates the policy engine would refuse
// (lead_time_too_long); the exact rule is still enforced at quote/checkout.
func (s *Service) onlineMaxLeadDays(ctx context.Context, es store.EstablishmentScope, est *store.Establishment, seasons []store.Season) (*int, error) {
	settings, err := establishment.ParseSettings(est.Settings, est.Currency)
	if err != nil {
		return nil, fmt.Errorf("checkout: settings: %w", err)
	}
	best := settings.DefaultPolicy.MaxLeadTimeDays
	if best == nil {
		return nil, nil
	}
	for _, sn := range seasons {
		rows, err := store.ListSeasonBookingPolicies(ctx, s.d.DB, es, sn.ID)
		if err != nil {
			return nil, fmt.Errorf("checkout: policies: %w", err)
		}
		for i := range rows {
			if !rows[i].Active {
				continue
			}
			var scope policy.Scope
			var rules policy.Rules
			if rows[i].Scope.Unmarshal(&scope) != nil || rows[i].Rules.Unmarshal(&rules) != nil {
				continue
			}
			if len(scope.Channels) > 0 && !slices.Contains(scope.Channels, policy.ChannelOnline) {
				continue
			}
			if rules.MaxLeadTimeDays == nil {
				continue // inherits the default
			}
			if *rules.MaxLeadTimeDays > *best {
				best = rules.MaxLeadTimeDays
			}
		}
	}
	return best, nil
}
