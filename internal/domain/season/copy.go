package season

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// CopyInput describes the season copy of spec 8.7 (the season part: the
// target season and its rate periods). Price rules, fixed prices and
// policies are copied by pricingconfig, which owns those tables.
type CopyInput struct {
	TargetYear int
	// OpensOn/ClosesOn override the shifted dates of the source season.
	OpensOn  *calendar.Date
	ClosesOn *calendar.Date
	// IncludeRatePeriods copies the rate periods (default true in the API).
	IncludeRatePeriods bool
	IP                 string
}

// CopyResult is the outcome of CopySeason.
type CopyResult struct {
	Source      store.Season
	Season      *store.Season
	RatePeriods []store.RatePeriod
	// Warnings are Italian, user-facing notes (adjusted or dropped ranges).
	Warnings []string
}

// CopySeason creates the season of TargetYear from a source season
// (configuration): dates shift with pricing.ShiftDate (same weekday where it
// makes sense, calendar anchors for month boundaries and holidays), the new
// season is a draft, and rate periods are copied with their ranges shifted,
// clipped to the new season and de-overlapped. It runs in one transaction;
// to copy the price list in the same transaction, the caller (the
// pricingconfig CopySeason use case) runs it through WithDB(tx).
func (s *Service) CopySeason(ctx context.Context, p *auth.Principal, establishmentID, sourceID string, in CopyInput) (*CopyResult, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	res := &CopyResult{Warnings: []string{}}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		src, err := store.GetSeason(ctx, tx, es, sourceID)
		if err != nil {
			return err
		}
		res.Source = *src
		years := in.TargetYear - src.Year
		ve := &configerr.ValidationError{}
		if years == 0 {
			ve.Add("target_year", "Deve essere diverso dall'anno della stagione di origine.")
		}
		opens, closes := pricing.ShiftDate(src.OpensOn, years, false), pricing.ShiftDate(src.ClosesOn, years, true)
		if in.OpensOn != nil {
			opens = *in.OpensOn
		}
		if in.ClosesOn != nil {
			closes = *in.ClosesOn
		}
		ValidateSeasonDates(in.TargetYear, opens, closes, ve)
		if err := ve.OrNil(); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		sn := &store.Season{
			ID: store.NewID(), Year: in.TargetYear, OpensOn: opens, ClosesOn: closes, Status: string(StatusDraft),
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.insertSeason(ctx, tx, p, es, sn, in.IP, ActionSeasonCopied, map[string]any{"source_season_id": src.ID}); err != nil {
			if errors.Is(err, ErrSeasonYearTaken) {
				return ErrTargetYearTaken
			}
			return err
		}
		res.Season = sn
		if !in.IncludeRatePeriods {
			return nil
		}
		periods, err := store.ListSeasonRatePeriods(ctx, tx, es, src.ID)
		if err != nil {
			return err
		}
		copies, warnings := CopyRatePeriods(periods, years, *sn)
		res.Warnings = append(res.Warnings, warnings...)
		for i := range copies {
			copies[i].ID = store.NewID()
			copies[i].SeasonID = sn.ID
			copies[i].Version = 1
			copies[i].CreatedAt, copies[i].UpdatedAt = now, now
			if err := s.insertRatePeriod(ctx, tx, p, es, &copies[i], in.IP); err != nil {
				return err
			}
		}
		res.RatePeriods = copies
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSeasonYearTaken) {
			err = ErrTargetYearTaken
		}
		return nil, wrap("copy season", err)
	}
	return res, nil
}

// CopyRatePeriods shifts the rate periods of a season by years (pure). Ranges
// are clipped to the target season; when two shifted ranges overlap, the
// later one starts the day after the earlier one ends; ranges left empty are
// dropped, and periods without ranges are not copied. Every adjustment is
// reported as a warning. IDs and timestamps are left to the caller.
func CopyRatePeriods(periods []store.RatePeriod, years int, target store.Season) ([]store.RatePeriod, []string) {
	in := make([]pricing.RatePeriod, len(periods))
	for i, p := range periods {
		rs := make([]pricing.DateRange, len(p.Ranges))
		for j, r := range p.Ranges {
			rs[j] = pricing.DateRange{From: r.From, To: r.To}
		}
		in[i] = pricing.RatePeriod{ID: p.ID, Code: p.Code, Ranges: rs}
	}
	shifted, err := pricing.CopySeason(pricing.CopySeasonInput{FromYear: target.Year - years, ToYear: target.Year, RatePeriods: in})
	warnings := []string{}
	if err != nil || len(shifted.RatePeriods) != len(periods) {
		return nil, []string{"Periodi tariffari non copiati: anni non validi."}
	}
	type span struct {
		period int
		r      store.DateRange
	}
	var spans []span
	for i, p := range shifted.RatePeriods {
		for _, r := range p.Ranges {
			from, to := r.From, r.To
			if from.Before(target.OpensOn) {
				from = target.OpensOn
			}
			if to.After(target.ClosesOn) {
				to = target.ClosesOn
			}
			if to.Before(from) {
				warnings = append(warnings, fmt.Sprintf("Periodo %q: l'intervallo %s – %s è fuori dalla nuova stagione ed è stato escluso.", p.Code, r.From, r.To))
				continue
			}
			if !from.Equal(r.From) || !to.Equal(r.To) {
				warnings = append(warnings, fmt.Sprintf("Periodo %q: intervallo adattato alla nuova stagione (%s – %s).", p.Code, from, to))
			}
			spans = append(spans, span{i, store.DateRange{From: from, To: to}})
		}
	}
	slices.SortStableFunc(spans, func(a, b span) int { return strings.Compare(a.r.From.String(), b.r.From.String()) })
	kept := spans[:0]
	for _, sp := range spans {
		if n := len(kept); n > 0 && !sp.r.From.After(kept[n-1].r.To) {
			prev := kept[n-1]
			from := prev.r.To.AddDays(1)
			if sp.r.To.Before(from) {
				warnings = append(warnings, fmt.Sprintf("Periodo %q: l'intervallo %s – %s si sovrapponeva a %q ed è stato escluso.",
					periods[sp.period].Code, sp.r.From, sp.r.To, periods[prev.period].Code))
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Periodo %q: l'intervallo ora inizia il %s per non sovrapporsi a %q.",
				periods[sp.period].Code, from, periods[prev.period].Code))
			sp.r.From = from
		}
		kept = append(kept, sp)
	}
	byPeriod := make([][]store.DateRange, len(periods))
	for _, sp := range kept {
		byPeriod[sp.period] = append(byPeriod[sp.period], sp.r)
	}
	out := make([]store.RatePeriod, 0, len(periods))
	for i, p := range periods {
		if len(byPeriod[i]) == 0 {
			warnings = append(warnings, fmt.Sprintf("Periodo %q non copiato: nessun intervallo nella nuova stagione.", p.Code))
			continue
		}
		out = append(out, store.RatePeriod{
			Code: p.Code, Name: p.Name, Color: p.Color, Ranges: sortRanges(byPeriod[i]), SortOrder: p.SortOrder,
		})
	}
	return out, warnings
}
