package season

import (
	"slices"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

func d(s string) calendar.Date { return calendar.MustParse(s) }

func rg(from, to string) store.DateRange { return store.DateRange{From: d(from), To: d(to)} }

func fieldsOf(ve *configerr.ValidationError) []string {
	out := make([]string, len(ve.Fields))
	for i, f := range ve.Fields {
		out[i] = f.Field
	}
	return out
}

func TestCanTransition_Matrix(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		{StatusDraft, StatusDraft, true},
		{StatusDraft, StatusActive, true},
		{StatusDraft, StatusClosed, false},
		{StatusActive, StatusDraft, true},
		{StatusActive, StatusClosed, true},
		{StatusClosed, StatusActive, true},
		{StatusClosed, StatusDraft, false},
		{StatusClosed, StatusClosed, true},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestValidateSeasonDates_Scenarios(t *testing.T) {
	tests := []struct {
		name          string
		year          int
		opens, closes string
		fields        []string
	}{
		{"summer season", 2027, "2027-05-15", "2027-09-30", nil},
		{"single day", 2027, "2027-08-15", "2027-08-15", nil},
		{"winter season into next year", 2027, "2027-11-01", "2028-03-31", nil},
		{"closes before opening", 2027, "2027-09-30", "2027-05-15", []string{"closes_on"}},
		{"opens in another year", 2027, "2026-12-31", "2027-09-30", []string{"opens_on"}},
		{"too long", 2027, "2027-05-01", "2029-01-01", []string{"closes_on"}},
		{"year out of range", 1999, "1999-05-01", "1999-09-01", []string{"year"}},
		{"missing dates", 2027, "", "", []string{"opens_on", "closes_on"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o, c calendar.Date
			if tt.opens != "" {
				o, c = d(tt.opens), d(tt.closes)
			}
			ve := &configerr.ValidationError{}
			ValidateSeasonDates(tt.year, o, c, ve)
			if !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("fields %v, want %v", fieldsOf(ve), tt.fields)
			}
		})
	}
}

func TestValidateRatePeriod_Scenarios(t *testing.T) {
	sn := store.Season{ID: "s1", Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")}
	others := []store.RatePeriod{
		{ID: "low", SeasonID: "s1", Code: "low", Ranges: []store.DateRange{rg("2027-05-15", "2027-06-14"), rg("2027-09-01", "2027-09-30")}},
		{ID: "high", SeasonID: "s1", Code: "high", Ranges: []store.DateRange{rg("2027-07-01", "2027-08-31")}},
		{ID: "other-season", SeasonID: "s0", Code: "mid", Ranges: []store.DateRange{rg("2027-06-15", "2027-06-30")}},
	}
	name := map[string]string{"it": "Media", "en": "Mid"}
	tests := []struct {
		name   string
		in     RatePeriodDraft
		fields []string
	}{
		{"fills the gap", RatePeriodDraft{Code: "mid", Name: name, Color: "#F2B134", Ranges: []store.DateRange{rg("2027-06-15", "2027-06-30")}}, nil},
		{"update of an existing period keeps its own ranges", RatePeriodDraft{ID: "high", Code: "high", Name: name, Ranges: []store.DateRange{rg("2027-06-25", "2027-08-31")}}, nil},
		{"overlaps another period", RatePeriodDraft{Code: "peak", Name: name, Ranges: []store.DateRange{rg("2027-08-10", "2027-08-20")}}, []string{"ranges[0]"}},
		{"touches the end of another period", RatePeriodDraft{Code: "x", Name: name, Ranges: []store.DateRange{rg("2027-06-14", "2027-06-20")}}, []string{"ranges[0]"}},
		{"overlaps two ranges of the same other period", RatePeriodDraft{Code: "x", Name: name, Ranges: []store.DateRange{rg("2027-06-10", "2027-06-16"), rg("2027-08-31", "2027-09-02")}}, []string{"ranges[0]", "ranges[1]", "ranges[1]"}},
		{"own ranges overlap", RatePeriodDraft{Code: "x", Name: name, Ranges: []store.DateRange{rg("2027-06-15", "2027-06-20"), rg("2027-06-18", "2027-06-25")}}, []string{"ranges"}},
		{"outside the season", RatePeriodDraft{Code: "x", Name: name, Ranges: []store.DateRange{rg("2027-05-01", "2027-05-20")}}, []string{"ranges[0]"}},
		{"inverted range", RatePeriodDraft{Code: "x", Name: name, Ranges: []store.DateRange{rg("2027-06-20", "2027-06-15")}}, []string{"ranges[0].to"}},
		{"no ranges", RatePeriodDraft{Code: "x", Name: name}, []string{"ranges"}},
		{"bad code, colour, missing default locale", RatePeriodDraft{Code: "Alta Stagione", Name: map[string]string{"en": "High"}, Color: "red", Ranges: []store.DateRange{rg("2027-06-15", "2027-06-16")}}, []string{"code", "name.it", "color"}},
		{"unsupported locale", RatePeriodDraft{Code: "x", Name: map[string]string{"it": "x", "es": "x"}, Ranges: []store.DateRange{rg("2027-06-15", "2027-06-16")}}, []string{"name.es"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			ValidateRatePeriod(tt.in, sn, others, "it", ve)
			if !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("fields %v, want %v (%v)", fieldsOf(ve), tt.fields, ve)
			}
		})
	}
}

func TestValidateRatePeriod_Normalizes(t *testing.T) {
	sn := store.Season{ID: "s1", Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")}
	ve := &configerr.ValidationError{}
	got := ValidateRatePeriod(RatePeriodDraft{
		Code: " peak ", Name: map[string]string{"it": " Alta ", "en": "  "}, Color: "#ABCDEF",
		Ranges: []store.DateRange{rg("2027-08-10", "2027-08-20"), rg("2027-07-01", "2027-07-05")},
	}, sn, nil, "it", ve)
	if ve.OrNil() != nil {
		t.Fatal(ve)
	}
	if got.Code != "peak" || got.Color != "#abcdef" || len(got.Name) != 1 || got.Name["it"] != "Alta" {
		t.Fatalf("got %+v", got)
	}
	if !got.Ranges[0].From.Equal(d("2027-07-01")) {
		t.Fatalf("ranges not sorted: %v", got.Ranges)
	}
}

func TestRangesOutside_Scenarios(t *testing.T) {
	periods := []store.RatePeriod{{Code: "low", Ranges: []store.DateRange{rg("2027-05-15", "2027-06-14")}}, {Code: "high", Ranges: []store.DateRange{rg("2027-07-01", "2027-08-31")}}}
	if out := RangesOutside(periods, d("2027-05-01"), d("2027-09-30")); len(out) != 0 {
		t.Fatalf("wider season: %v", out)
	}
	out := RangesOutside(periods, d("2027-06-01"), d("2027-08-15"))
	if len(out) != 2 || !strings.HasPrefix(out[0], "low") || !strings.HasPrefix(out[1], "high") {
		t.Fatalf("narrower season: %v", out)
	}
}

func TestNormalizeSlots_Scenarios(t *testing.T) {
	tests := []struct {
		name   string
		in     []calendar.Slot
		want   []calendar.Slot
		fields []string
	}{
		{"full day", []calendar.Slot{calendar.PM, calendar.AM}, []calendar.Slot{calendar.AM, calendar.PM}, nil},
		{"morning", []calendar.Slot{calendar.AM, calendar.AM}, []calendar.Slot{calendar.AM}, nil},
		{"afternoon", []calendar.Slot{calendar.PM}, []calendar.Slot{calendar.PM}, nil},
		{"empty", []calendar.Slot{}, []calendar.Slot{}, []string{"slots"}},
		{"unknown slot", []calendar.Slot{"FULL"}, []calendar.Slot{}, []string{"slots[0]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := &configerr.ValidationError{}
			got := NormalizeSlots(tt.in, ve)
			if !slices.Equal(got, tt.want) || !slices.Equal(fieldsOf(ve), tt.fields) {
				t.Fatalf("got %v %v", got, fieldsOf(ve))
			}
		})
	}
}

func TestCopyRatePeriods_Scenarios(t *testing.T) {
	src := []store.RatePeriod{
		{
			ID: "a", Code: "low", Name: store.I18nText{"it": "Bassa"}, Color: "#00f", SortOrder: 0,
			Ranges: []store.DateRange{rg("2026-05-16", "2026-06-12"), rg("2026-09-01", "2026-09-30")},
		},
		{ID: "b", Code: "high", Name: store.I18nText{"it": "Alta"}, SortOrder: 1, Ranges: []store.DateRange{rg("2026-06-13", "2026-08-31")}},
	}
	t.Run("same weekday, adjacency kept", func(t *testing.T) {
		target := store.Season{ID: "t", Year: 2027, OpensOn: d("2027-05-01"), ClosesOn: d("2027-09-30")}
		got, warnings := CopyRatePeriods(src, 1, target)
		if len(got) != 2 {
			t.Fatalf("got %+v %v", got, warnings)
		}
		// 2026-05-16 is a Saturday; the copy starts on a Saturday too.
		if got[0].Ranges[0].From.ISOWeekday() != d("2026-05-16").ISOWeekday() {
			t.Fatalf("weekday not kept: %v", got[0].Ranges)
		}
		if got[0].Code != "low" || got[0].Name["it"] != "Bassa" || got[0].Color != "#00f" || got[1].SortOrder != 1 {
			t.Fatalf("attributes not copied: %+v", got)
		}
		assertNoOverlap(t, got, target)
	})
	t.Run("clipped to a shorter season", func(t *testing.T) {
		target := store.Season{ID: "t", Year: 2027, OpensOn: d("2027-06-01"), ClosesOn: d("2027-08-31")}
		got, warnings := CopyRatePeriods(src, 1, target)
		if len(warnings) == 0 {
			t.Fatal("want warnings")
		}
		assertNoOverlap(t, got, target)
		for _, p := range got {
			if p.Code == "low" && len(p.Ranges) != 1 {
				t.Fatalf("september range should be dropped: %v", p.Ranges)
			}
		}
	})
	t.Run("season entirely elsewhere drops periods", func(t *testing.T) {
		target := store.Season{ID: "t", Year: 2027, OpensOn: d("2027-12-01"), ClosesOn: d("2027-12-31")}
		got, warnings := CopyRatePeriods(src, 1, target)
		if len(got) != 0 || len(warnings) < 2 {
			t.Fatalf("got %+v %v", got, warnings)
		}
	})
	t.Run("overlaps created by the shift are trimmed", func(t *testing.T) {
		// Calendar-anchored end (last day of month) next to a weekday-anchored
		// start can collide after the shift.
		ov := []store.RatePeriod{
			{Code: "a", Ranges: []store.DateRange{rg("2026-06-03", "2026-06-30")}},
			{Code: "b", Ranges: []store.DateRange{rg("2026-07-01", "2026-07-10")}},
			{Code: "c", Ranges: []store.DateRange{rg("2026-06-25", "2026-06-29")}},
		}
		target := store.Season{ID: "t", Year: 2027, OpensOn: d("2027-05-01"), ClosesOn: d("2027-09-30")}
		got, _ := CopyRatePeriods(ov, 1, target)
		assertNoOverlap(t, got, target)
	})
}

func assertNoOverlap(t *testing.T, ps []store.RatePeriod, sn store.Season) {
	t.Helper()
	for i, p := range ps {
		for _, r := range p.Ranges {
			if r.From.Before(sn.OpensOn) || r.To.After(sn.ClosesOn) || r.To.Before(r.From) {
				t.Fatalf("range %v of %s outside season", r, p.Code)
			}
		}
		ve := &configerr.ValidationError{}
		others := make([]store.RatePeriod, 0, len(ps))
		for j, o := range ps {
			if j != i {
				o.ID, o.SeasonID = o.Code, sn.ID
				others = append(others, o)
			}
		}
		ValidateRatePeriod(RatePeriodDraft{ID: p.Code, Code: p.Code, Name: map[string]string{"it": "x"}, Ranges: p.Ranges}, sn, others, "it", ve)
		if ve.OrNil() != nil {
			t.Fatalf("copy is not valid: %v", ve)
		}
	}
}
