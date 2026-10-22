import type { DateRange as DayPickerRange, Matcher } from 'react-day-picker';
import { useTranslation } from 'react-i18next';
import { Calendar } from '@arenile/shared/ui/calendar';
import { dateFnsLocale } from '@arenile/shared/lib/dateLocales';
import { dateToIso, isoToDate } from '@arenile/shared/lib/dates';

/*
 * Calendar body of DatePicker / DateRangePicker, loaded on first open:
 * react-day-picker and the date-fns locales stay out of the first-paint
 * bundle of the booking page (spec 12.6, 150 KB budget).
 */

function useCalendarProps(min?: string, max?: string) {
  const { i18n } = useTranslation();
  const locale = dateFnsLocale(i18n.language);
  const disabled: Matcher[] = [];
  const minDate = isoToDate(min);
  const maxDate = isoToDate(max);
  if (minDate) disabled.push({ before: minDate });
  if (maxDate) disabled.push({ after: maxDate });
  // The dropdown caption only lists the months between startMonth and endMonth
  // (react-day-picker defaults to the current year): allow ten years around today.
  const year = new Date().getFullYear();
  const startMonth = minDate ?? new Date(year - 10, 0, 1);
  const endMonth = maxDate ?? new Date(year + 10, 11, 31);
  return { locale, disabled, weekStartsOn: 1 as const, startMonth, endMonth, minDate };
}

export interface SingleCalendarProps {
  value: string | null | undefined;
  onSelect: (iso: string | undefined) => void;
  min?: string;
  max?: string;
}

export function SingleCalendar({ value, onSelect, min, max }: SingleCalendarProps) {
  const selected = isoToDate(value);
  const c = useCalendarProps(min, max);
  return (
    <Calendar
      mode="single"
      selected={selected}
      // A future season opens on its first month, not on an all-disabled current month.
      defaultMonth={selected ?? c.minDate}
      captionLayout="dropdown"
      locale={c.locale}
      weekStartsOn={c.weekStartsOn}
      startMonth={c.startMonth}
      endMonth={c.endMonth}
      disabled={c.disabled}
      onSelect={(d) => onSelect(d ? dateToIso(d) : undefined)}
    />
  );
}

export interface RangeCalendarProps {
  from: string | undefined;
  to: string | undefined;
  onSelect: (range: { from: string; to?: string } | undefined) => void;
  min?: string;
  max?: string;
  numberOfMonths?: number;
}

export function RangeCalendar({
  from,
  to,
  onSelect,
  min,
  max,
  numberOfMonths,
}: RangeCalendarProps) {
  const fromDate = isoToDate(from);
  const toDate = isoToDate(to);
  const selected: DayPickerRange | undefined = fromDate
    ? { from: fromDate, to: toDate }
    : undefined;
  const c = useCalendarProps(min, max);
  return (
    <Calendar
      mode="range"
      selected={selected}
      defaultMonth={fromDate ?? c.minDate}
      numberOfMonths={numberOfMonths}
      locale={c.locale}
      weekStartsOn={c.weekStartsOn}
      startMonth={c.startMonth}
      endMonth={c.endMonth}
      disabled={c.disabled}
      onSelect={(r) =>
        onSelect(
          r?.from ? { from: dateToIso(r.from), to: r.to ? dateToIso(r.to) : undefined } : undefined,
        )
      }
    />
  );
}
