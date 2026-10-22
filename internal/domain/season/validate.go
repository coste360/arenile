package season

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Status is the lifecycle state of a season.
type Status string

// Season statuses (spec 6.1).
const (
	StatusDraft  Status = "draft"
	StatusActive Status = "active"
	StatusClosed Status = "closed"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool { return s == StatusDraft || s == StatusActive || s == StatusClosed }

// transitions lists the allowed status changes. A draft season is being
// prepared; active is open for sales; closed is over (it can be reopened).
// draft -> closed is not allowed (a season that never opened is deleted
// instead) and closed -> draft is not allowed (it may hold history).
var transitions = map[Status][]Status{
	StatusDraft:  {StatusActive},
	StatusActive: {StatusDraft, StatusClosed},
	StatusClosed: {StatusActive},
}

// CanTransition reports whether a season may move from one status to another.
// Staying in the same status is always allowed.
func CanTransition(from, to Status) bool {
	return from == to || slices.Contains(transitions[from], to)
}

// Validation limits.
const (
	MinYear            = 2000
	MaxYear            = 2100
	MaxRangesPerPeriod = 50
	MaxNameLength      = 100
	MaxReasonLength    = 500
)

// SupportedLocales are the languages accepted in localized texts (spec 20).
var SupportedLocales = []string{"it", "en", "de", "fr"}

var (
	codeRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

// ValidateSeasonDates checks year and opening dates: the season opens in its
// year, closes on or after the opening and at the latest at the end of the
// following year (winter openings).
func ValidateSeasonDates(year int, opensOn, closesOn calendar.Date, ve *configerr.ValidationError) {
	if year < MinYear || year > MaxYear {
		ve.Add("year", fmt.Sprintf("Anno tra %d e %d.", MinYear, MaxYear))
	}
	if opensOn.IsZero() {
		ve.Add("opens_on", "Obbligatoria.")
	}
	if closesOn.IsZero() {
		ve.Add("closes_on", "Obbligatoria.")
	}
	if opensOn.IsZero() || closesOn.IsZero() {
		return
	}
	if opensOn.Year() != year {
		ve.Add("opens_on", "La stagione deve aprire nel suo anno.")
	}
	if closesOn.Before(opensOn) {
		ve.Add("closes_on", "La chiusura non può precedere l'apertura.")
	} else if closesOn.Year() > year+1 {
		ve.Add("closes_on", "La stagione deve chiudere entro la fine dell'anno seguente.")
	}
}

// RatePeriodDraft is a rate period being validated.
type RatePeriodDraft struct {
	ID     string // empty for a new period
	Code   string
	Name   map[string]string
	Color  string
	Ranges []store.DateRange
}

// ValidateRatePeriod checks a rate period against its season and the other
// periods of the same season (spec 6.1): code format, localized name with
// the establishment default locale, colour, ranges inside the season, no
// overlap between its own ranges nor with any range of another period. It
// returns the period normalized (trimmed, ranges sorted by start).
// Duplicate codes are reported separately by the caller as a conflict.
func ValidateRatePeriod(in RatePeriodDraft, season store.Season, others []store.RatePeriod, defaultLocale string, ve *configerr.ValidationError) RatePeriodDraft {
	in.Code = strings.TrimSpace(in.Code)
	if !codeRe.MatchString(in.Code) {
		ve.Add("code", "Usa lettere minuscole, cifre, - e _ (al massimo 32 caratteri).")
	}
	in.Name = ValidateI18n("name", in.Name, defaultLocale, true, ve)
	in.Color = strings.ToLower(strings.TrimSpace(in.Color))
	if in.Color != "" && !hexColorRe.MatchString(in.Color) {
		ve.Add("color", "Colore esadecimale non valido (es. #f2b134).")
	}
	switch {
	case len(in.Ranges) == 0:
		ve.Add("ranges", "Indica almeno un intervallo di date.")
		return in
	case len(in.Ranges) > MaxRangesPerPeriod:
		ve.Add("ranges", fmt.Sprintf("Al massimo %d intervalli.", MaxRangesPerPeriod))
		return in
	}
	valid := true
	for i, r := range in.Ranges {
		field := fmt.Sprintf("ranges[%d]", i)
		switch {
		case r.From.IsZero() || r.To.IsZero():
			ve.Add(field, "Date obbligatorie.")
			valid = false
		case r.To.Before(r.From):
			ve.Add(field+".to", "La fine non può precedere l'inizio.")
			valid = false
		case r.From.Before(season.OpensOn) || r.To.After(season.ClosesOn):
			ve.Add(field, fmt.Sprintf("L'intervallo deve rientrare nella stagione (%s – %s).", season.OpensOn, season.ClosesOn))
			valid = false
		}
	}
	if !valid {
		return in
	}
	in.Ranges = sortRanges(in.Ranges)
	for i := 1; i < len(in.Ranges); i++ {
		if !in.Ranges[i].From.After(in.Ranges[i-1].To) {
			ve.Add("ranges", fmt.Sprintf("Gli intervalli %s – %s e %s – %s si sovrappongono.",
				in.Ranges[i-1].From, in.Ranges[i-1].To, in.Ranges[i].From, in.Ranges[i].To))
		}
	}
	for _, o := range others {
		if o.ID == in.ID || o.SeasonID != season.ID {
			continue
		}
		for i, r := range in.Ranges {
			for _, or := range o.Ranges {
				if overlaps(r, or) {
					ve.Add(fmt.Sprintf("ranges[%d]", i), fmt.Sprintf("Si sovrappone al periodo %q (%s – %s).", o.Code, or.From, or.To))
				}
			}
		}
	}
	return in
}

// RangesOutside returns the ranges of periods that would fall outside a
// season opening from opensOn to closesOn (used when the season dates change).
func RangesOutside(periods []store.RatePeriod, opensOn, closesOn calendar.Date) []string {
	var out []string
	for _, p := range periods {
		for _, r := range p.Ranges {
			if r.From.Before(opensOn) || r.To.After(closesOn) {
				out = append(out, fmt.Sprintf("%s (%s – %s)", p.Code, r.From, r.To))
			}
		}
	}
	return out
}

// ValidateI18n checks a localized text: keys are supported locales, values
// are trimmed, non-empty and at most MaxNameLength characters; when required
// the default locale must be present (it is the fallback, spec 20).
func ValidateI18n(field string, t map[string]string, defaultLocale string, required bool, ve *configerr.ValidationError) map[string]string {
	out := make(map[string]string, len(t))
	for k, v := range t {
		v = strings.TrimSpace(v)
		switch {
		case !slices.Contains(SupportedLocales, k):
			ve.Add(field+"."+k, "Lingua non supportata (it, en, de, fr).")
		case v == "":
			// An empty translation is the same as a missing one.
		case utf8.RuneCountInString(v) > MaxNameLength:
			ve.Add(field+"."+k, fmt.Sprintf("Al massimo %d caratteri.", MaxNameLength))
		default:
			out[k] = v
		}
	}
	if required && out[defaultLocale] == "" {
		ve.Add(field+"."+defaultLocale, "Obbligatorio nella lingua predefinita dello stabilimento.")
	}
	return out
}

// NormalizeSlots validates the slots of a closure day: a non-empty subset of
// AM/PM, returned deduplicated in day order.
func NormalizeSlots(slots []calendar.Slot, ve *configerr.ValidationError) []calendar.Slot {
	var am, pm bool
	for i, s := range slots {
		switch s {
		case calendar.AM:
			am = true
		case calendar.PM:
			pm = true
		default:
			ve.Add(fmt.Sprintf("slots[%d]", i), "Fascia non valida (AM o PM).")
		}
	}
	out := make([]calendar.Slot, 0, 2)
	if am {
		out = append(out, calendar.AM)
	}
	if pm {
		out = append(out, calendar.PM)
	}
	if len(slots) == 0 {
		ve.Add("slots", "Seleziona almeno una fascia.")
	}
	return out
}

func overlaps(a, b store.DateRange) bool { return !a.From.After(b.To) && !b.From.After(a.To) }

func sortRanges(rs []store.DateRange) []store.DateRange {
	out := slices.Clone(rs)
	slices.SortFunc(out, func(a, b store.DateRange) int {
		if c := strings.Compare(a.From.String(), b.From.String()); c != 0 {
			return c
		}
		return strings.Compare(a.To.String(), b.To.String())
	})
	return out
}
