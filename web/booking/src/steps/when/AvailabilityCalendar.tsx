import { dateFnsLocale } from '@arenile/shared/lib/dateLocales';
import { dateToIso, isoToDate } from '@arenile/shared/lib/dates';
import { Calendar } from '@arenile/shared/ui/calendar';
import { useMemo, useState } from 'react';
import { labelDayButton, type DateRange, type Matcher } from 'react-day-picker';
import { useTranslation } from 'react-i18next';
import { usePublicCalendar, type PublicCalendar } from '../../api/public';
import type { Slots, StayDates } from '../../flow/selection';
import { PUBLIC_NS } from '../../i18n';

export interface AvailabilityCalendarProps {
  slug: string;
  value: StayDates | undefined;
  onSelect: (value: StayDates | undefined) => void;
  min: string;
  max?: string;
  slots: Slots;
  /** Highlight rate periods (online flow option `show_rate_periods`). */
  showRatePeriods: boolean;
}

/** Background tokens of the rate periods, cycled in order (no establishment colours as inline CSS). */
const PERIOD_CLASSES = ['bg-primary/10', 'bg-highlight/25', 'bg-muted'];

const monthKey = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;

function datesWith(cal: PublicCalendar | undefined, statuses: readonly string[]): Date[] {
  return (cal?.days ?? [])
    .filter((d) => statuses.includes(d.status))
    .map((d) => isoToDate(d.date))
    .filter((d): d is Date => !!d);
}

/**
 * Calendar of the "Quando" step (spec 12.3 passo 1), loaded when the date
 * field opens: one month of availability at a time from `GET calendar`
 * (online channel and quota), full / closed / non-bookable days disabled and
 * never inside a range, "ultimi posti" marked with a dot and in the day's
 * accessible name, rate periods tinted when the manager enables them.
 */
export default function AvailabilityCalendar({
  slug,
  value,
  onSelect,
  min,
  max,
  slots,
  showRatePeriods,
}: AvailabilityCalendarProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const minDate = isoToDate(min);
  const maxDate = isoToDate(max);
  const [month, setMonth] = useState<Date>(() => isoToDate(value?.from) ?? minDate ?? new Date());
  const cal = usePublicCalendar(slug, monthKey(month), { slots });
  const data = cal.data?.month === monthKey(month) ? cal.data : undefined;

  const { disabled, modifiers, classNames, statusOf, blockedBetween } = useMemo(() => {
    const blocked = datesWith(data, ['full', 'closed', 'unavailable']);
    const limited = datesWith(data, ['limited']);
    const dis: Matcher[] = [...blocked];
    const lo = isoToDate(min);
    const hi = isoToDate(max);
    if (lo) dis.push({ before: lo });
    if (hi) dis.push({ after: hi });
    // Full, closed and not bookable days are struck through, not only greyed
    // out: the state never relies on colour alone (WCAG 1.4.1).
    const mods: Record<string, Date[]> = { limited, blocked };
    const cls: Record<string, string> = {
      blocked: 'line-through',
      limited:
        'relative after:pointer-events-none after:absolute after:bottom-1 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-highlight',
    };
    if (showRatePeriods) {
      const codes = (data?.rate_periods ?? []).map((p) => p.code);
      codes.forEach((code, i) => {
        mods[`period-${i}`] = (data?.days ?? [])
          .filter((d) => d.rate_period_code === code)
          .map((d) => isoToDate(d.date))
          .filter((d): d is Date => !!d);
        cls[`period-${i}`] = PERIOD_CLASSES[i % PERIOD_CLASSES.length] as string;
      });
    }
    const byDate = new Map((data?.days ?? []).map((d) => [d.date, d.status]));
    return {
      disabled: dis,
      modifiers: mods,
      classNames: cls,
      statusOf: (d: Date) => byDate.get(dateToIso(d)),
      blockedBetween: (a: string, b: string) =>
        (data?.days ?? []).some(
          (d) =>
            d.date >= a &&
            d.date <= b &&
            (d.status === 'full' || d.status === 'closed' || d.status === 'unavailable'),
        ),
    };
  }, [data, min, max, showRatePeriods]);

  const selected: DateRange | undefined = value
    ? { from: isoToDate(value.from), to: isoToDate(value.to) }
    : undefined;

  return (
    <div className="space-y-2" aria-busy={cal.isFetching || undefined}>
      <Calendar
        mode="range"
        selected={selected}
        month={month}
        onMonthChange={setMonth}
        startMonth={minDate}
        endMonth={maxDate}
        weekStartsOn={1}
        locale={dateFnsLocale(i18n.language)}
        disabled={disabled}
        modifiers={modifiers}
        modifiersClassNames={classNames}
        labels={{
          labelDayButton: (date, mods, options, lib) => {
            const base = labelDayButton(date, mods, options, lib);
            const status = statusOf(date);
            if (status === 'limited') return `${base}, ${t('when.dayLimited')}`;
            if (status === 'full') return `${base}, ${t('when.dayFull')}`;
            if (status === 'closed') return `${base}, ${t('when.dayClosed')}`;
            if (status === 'unavailable') return `${base}, ${t('when.dayUnavailable')}`;
            return base;
          },
        }}
        onSelect={(_range, day) => {
          // Two taps make a range (the same day twice = one day); a range
          // never spans a blocked day of the loaded month.
          const iso = dateToIso(day);
          if (value?.from && !value.to) {
            const [a, b] = iso < value.from ? [iso, value.from] : [value.from, iso];
            onSelect(blockedBetween(a, b) ? { from: iso } : { from: a, to: b });
          } else {
            onSelect({ from: iso });
          }
        }}
      />
      <ul
        aria-label={t('when.calendarLegend')}
        className="flex flex-wrap gap-x-4 gap-y-1 px-3 pb-3 text-xs text-muted-foreground"
      >
        <li className="flex items-center gap-1.5">
          <span aria-hidden="true" className="size-1.5 rounded-full bg-highlight" />
          {t('when.limited')}
        </li>
        <li className="flex items-center gap-1.5">
          <span aria-hidden="true" className="text-muted-foreground line-through">
            12
          </span>
          {t('when.full')}
        </li>
        {showRatePeriods &&
          (data?.rate_periods ?? []).map((p, i) => (
            <li key={p.code} className="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className={`size-3 rounded-sm ${PERIOD_CLASSES[i % PERIOD_CLASSES.length] ?? ''}`}
              />
              {p.name[i18n.language] ?? p.name.it ?? p.code}
            </li>
          ))}
      </ul>
    </div>
  );
}
