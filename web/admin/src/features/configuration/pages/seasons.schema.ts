import type { Establishment } from '@arenile/shared/api';
import { z } from 'zod';
import type { EstablishmentUpdate } from '../api/establishment';
import type { RatePeriod, RatePeriodInput, Season, SeasonInput } from '../api/resources';
import { daysBetween, rangesOverlap, type DateRange } from '../lib/dates';
import { compactI18n, i18nFormValue, type I18nText } from '../lib/i18nText';
import {
  MONTH_DAY_RE,
  V,
  codeField,
  hexColor,
  i18nTextField,
  intField,
  isCalendarDate,
  isoDate,
  optionalInt,
} from '../lib/validation';

// ---- Season ---------------------------------------------------------------------

export const SEASON_STATUSES = ['draft', 'active', 'closed'] as const;

export const seasonSchema = z
  .object({
    year: intField({ min: 2000, max: 2100 }),
    opens_on: isoDate,
    closes_on: isoDate,
    status: z.enum(SEASON_STATUSES),
  })
  .superRefine((v, ctx) => checkSeasonDates(v.year, v.opens_on, v.closes_on, ctx));

/**
 * Season dates against its year, as the server checks them (spec 5.1): the
 * season opens in its year and closes by the end of the following one, never
 * before it opens.
 */
export function checkSeasonDates(
  year: string,
  opensOn: string,
  closesOn: string,
  ctx: z.RefinementCtx,
  paths: { opens: string; closes: string } = { opens: 'opens_on', closes: 'closes_on' },
) {
  if (!isCalendarDate(opensOn) || !isCalendarDate(closesOn)) return;
  if (opensOn > closesOn) {
    ctx.addIssue({ code: 'custom', message: V.dateOrder, path: [paths.closes] });
  }
  const y = Number(year);
  if (!/^\d{4}$/.test(year.trim())) return;
  if (Number(opensOn.slice(0, 4)) !== y) {
    ctx.addIssue({ code: 'custom', message: V.seasonOpensInYear, path: [paths.opens] });
  } else if (Number(closesOn.slice(0, 4)) > y + 1) {
    ctx.addIssue({ code: 'custom', message: V.seasonClosesBy, path: [paths.closes] });
  }
}

/** Same day and month in another year; 29 February becomes the 28th. */
function inYear(iso: string, year: number): string {
  const md = iso.slice(5);
  const leap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
  return `${year}-${md === '02-29' && !leap ? '02-28' : md}`;
}

/**
 * The season dates moved to a new year, keeping day and month (the closing
 * date keeps its distance in years from the opening), or null when the year
 * or the dates are not complete yet.
 */
export function moveSeasonDates(
  year: string,
  opensOn: string,
  closesOn: string,
): { opens_on: string; closes_on: string } | null {
  if (!/^\d{4}$/.test(year.trim()) || !isCalendarDate(opensOn) || !isCalendarDate(closesOn)) {
    return null;
  }
  const y = Number(year);
  const from = Number(opensOn.slice(0, 4));
  if (y === from || y < 2000 || y > 2100) return null;
  return {
    opens_on: inYear(opensOn, y),
    closes_on: inYear(closesOn, y + Number(closesOn.slice(0, 4)) - from),
  };
}

export type SeasonValues = z.infer<typeof seasonSchema>;

/** Next summer: the current year until August, the following one afterwards. */
export function defaultSeasonYear(today = new Date()): number {
  return today.getMonth() >= 8 ? today.getFullYear() + 1 : today.getFullYear();
}

export function toSeasonValues(s?: Season, today = new Date()): SeasonValues {
  if (s) {
    return { year: String(s.year), opens_on: s.opens_on, closes_on: s.closes_on, status: s.status };
  }
  const y = defaultSeasonYear(today);
  return { year: String(y), opens_on: `${y}-05-01`, closes_on: `${y}-09-30`, status: 'draft' };
}

export function toSeasonInput(v: SeasonValues): SeasonInput {
  return { year: Number(v.year), opens_on: v.opens_on, closes_on: v.closes_on, status: v.status };
}

// ---- Rate period ----------------------------------------------------------------

export const PERIOD_PRESETS = [
  { code: 'low', color: '#7dd3fc' },
  { code: 'mid', color: '#facc15' },
  { code: 'high', color: '#fb923c' },
  { code: 'peak', color: '#ef4444' },
] as const;

export interface OtherPeriod {
  code: string;
  ranges: DateRange[];
}

export interface RatePeriodContext {
  defaultLocale: string;
  season: Pick<Season, 'opens_on' | 'closes_on'>;
  /** Other periods of the same season (overlaps are rejected). */
  others: OtherPeriod[];
}

/**
 * Rate period form. Overlaps are checked here for immediate feedback; the
 * server enforces the same rule (spec 6.1) and its message is shown if it
 * still rejects the save (e.g. a concurrent edit).
 */
export function ratePeriodSchema(ctx: RatePeriodContext) {
  return z
    .object({
      code: codeField,
      name: i18nTextField(ctx.defaultLocale),
      color: hexColor,
      sort_order: intField({ optional: true, max: 1000 }),
      ranges: z.array(z.object({ from: isoDate, to: isoDate })).min(1, V.required),
    })
    .superRefine((v, issue) => {
      v.ranges.forEach((r, i) => {
        if (!isCalendarDate(r.from) || !isCalendarDate(r.to)) return;
        if (r.from > r.to) {
          issue.addIssue({ code: 'custom', message: V.dateOrder, path: ['ranges', i, 'to'] });
          return;
        }
        if (r.from < ctx.season.opens_on || r.to > ctx.season.closes_on) {
          issue.addIssue({ code: 'custom', message: V.outsideSeason, path: ['ranges', i, 'from'] });
        }
        const clash =
          v.ranges.some((o, j) => j !== i && o.from <= o.to && rangesOverlap(r, o)) ||
          ctx.others.some((p) => p.ranges.some((o) => rangesOverlap(r, o)));
        if (clash) {
          issue.addIssue({ code: 'custom', message: V.overlap, path: ['ranges', i, 'from'] });
        }
      });
    });
}

export type RatePeriodValues = z.infer<ReturnType<typeof ratePeriodSchema>>;

export function toRatePeriodValues(
  p: RatePeriod | undefined,
  locales: string[],
  season: Pick<Season, 'opens_on' | 'closes_on'>,
  existing: number,
): RatePeriodValues {
  if (p) {
    return {
      code: p.code,
      name: i18nFormValue(p.name, locales),
      color: p.color || '#7dd3fc',
      sort_order: String(p.sort_order ?? ''),
      ranges: p.ranges.map((r) => ({ from: r.from, to: r.to })),
    };
  }
  const preset = PERIOD_PRESETS[existing % PERIOD_PRESETS.length] ?? PERIOD_PRESETS[0];
  return {
    code: existing < PERIOD_PRESETS.length ? preset.code : '',
    name: i18nFormValue({}, locales),
    color: preset.color,
    sort_order: String(existing),
    ranges: [{ from: season.opens_on, to: season.closes_on }],
  };
}

export function toRatePeriodInput(v: RatePeriodValues, seasonId: string): RatePeriodInput {
  return {
    season_id: seasonId,
    code: v.code,
    name: compactI18n(v.name as I18nText),
    color: v.color.toLowerCase(),
    ranges: v.ranges,
    sort_order: optionalInt(v.sort_order),
  };
}

// ---- Timeline -------------------------------------------------------------------

export interface TimelineSegment {
  periodId: string;
  code: string;
  color: string;
  from: string;
  to: string;
  /** Percent offset and width over the season. */
  left: number;
  width: number;
}

/** Positions of each range of each period on a season-long horizontal bar. */
export function timelineSegments(
  season: Pick<Season, 'opens_on' | 'closes_on'>,
  periods: Pick<RatePeriod, 'id' | 'code' | 'color' | 'ranges'>[],
): TimelineSegment[] {
  const total = daysBetween(season.opens_on, season.closes_on) + 1;
  if (total <= 0) return [];
  const out: TimelineSegment[] = [];
  for (const p of periods) {
    for (const r of p.ranges) {
      const from = r.from < season.opens_on ? season.opens_on : r.from;
      const to = r.to > season.closes_on ? season.closes_on : r.to;
      if (from > to) continue;
      const left = (daysBetween(season.opens_on, from) / total) * 100;
      const width = ((daysBetween(from, to) + 1) / total) * 100;
      out.push({ periodId: p.id, code: p.code, color: p.color, from, to, left, width });
    }
  }
  return out.sort((a, b) => a.left - b.left);
}

/** Days of the season covered by no period (uncovered days have no base price). */
export function uncoveredDays(
  season: Pick<Season, 'opens_on' | 'closes_on'>,
  periods: Pick<RatePeriod, 'ranges'>[],
): number {
  const total = daysBetween(season.opens_on, season.closes_on) + 1;
  const covered = new Set<number>();
  for (const p of periods) {
    for (const r of p.ranges) {
      const from = r.from < season.opens_on ? season.opens_on : r.from;
      const to = r.to > season.closes_on ? season.closes_on : r.to;
      for (let d = daysBetween(season.opens_on, from); d <= daysBetween(season.opens_on, to); d++) {
        covered.add(d);
      }
    }
  }
  return Math.max(0, total - covered.size);
}

// ---- Local holidays (settings.pricing) --------------------------------------------

export const holidaysSchema = z.object({
  local_holidays: z.array(z.object({ value: isoDate })),
  recurring_local_holidays: z.array(
    z.object({ value: z.string().trim().regex(MONTH_DAY_RE, V.monthDay) }),
  ),
});

export type HolidaysValues = z.infer<typeof holidaysSchema>;

export function toHolidaysValues(e: Establishment): HolidaysValues {
  return {
    local_holidays: (e.settings.pricing.local_holidays ?? []).map((value) => ({ value })),
    recurring_local_holidays: (e.settings.pricing.recurring_local_holidays ?? []).map((value) => ({
      value,
    })),
  };
}

export function holidaysPatch(v: HolidaysValues, current: Establishment): EstablishmentUpdate {
  const dates = [...new Set(v.local_holidays.map((h) => h.value))].sort();
  const recurring = [...new Set(v.recurring_local_holidays.map((h) => h.value))].sort();
  return {
    settings: {
      ...current.settings,
      pricing: {
        ...current.settings.pricing,
        local_holidays: dates.length ? dates : undefined,
        recurring_local_holidays: recurring.length ? recurring : undefined,
      },
    },
  };
}

export const holidaysAliases = {
  'settings.pricing.local_holidays': 'local_holidays',
  'settings.pricing.recurring_local_holidays': 'recurring_local_holidays',
};
