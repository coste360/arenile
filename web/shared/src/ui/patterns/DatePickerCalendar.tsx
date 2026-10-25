import type { DateRange as DayPickerRange, Matcher } from 'react-day-picker';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Calendar } from '@arenile/shared/ui/calendar';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { dateFnsLocale } from '@arenile/shared/lib/dateLocales';
import { dateToIso, formatIsoDate, isoToDate, parseDisplayDate } from '@arenile/shared/lib/dates';

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
  // fixedWeeks: always six rows, so the popover keeps its height (and its side)
  // when the user moves to a month with five weeks.
  return {
    locale,
    disabled,
    weekStartsOn: 1 as const,
    fixedWeeks: true,
    startMonth,
    endMonth,
    minDate,
  };
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
      fixedWeeks={c.fixedWeeks}
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

/** "1/6/2027", "01.06.2027", "01-06-2027" → "2027-06-01"; undefined when not a date. */
function parseTypedDate(text: string): string | undefined {
  const parts = text.trim().split(/[/.-]/);
  if (parts.length !== 3 || parts[2]?.length !== 4) return undefined;
  const [d, m, y] = parts as [string, string, string];
  return parseDisplayDate(`${d.padStart(2, '0')}/${m.padStart(2, '0')}/${y}`);
}

function withinBounds(iso: string, min?: string, max?: string): boolean {
  return (!min || iso >= min) && (!max || iso <= max);
}

/**
 * Typed end of the range (dd/mm/yyyy): applied as soon as it is a complete
 * valid day within the bounds (so nothing is left to apply when the user
 * then clicks the calendar: a commit on that blur would re-render the grid
 * under the click), and on Enter or blur; an invalid date marks the field
 * and leaves the range unchanged.
 */
function TypedDate({
  label,
  value,
  onCommit,
}: {
  label: string;
  value: string | undefined;
  onCommit: (iso: string) => boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  const shown = value ? formatIsoDate(value) : '';
  const [text, setText] = useState(shown);
  const [invalid, setInvalid] = useState(false);
  // A pick on the calendar replaces what was typed.
  const [synced, setSynced] = useState(shown);
  if (synced !== shown) {
    setSynced(shown);
    setText(shown);
    setInvalid(false);
  }
  const commit = () => {
    if (text.trim() === '' || text === shown) {
      setText(shown);
      setInvalid(false);
      return;
    }
    const iso = parseTypedDate(text);
    setInvalid(!(iso && onCommit(iso)));
  };
  return (
    <div className="grid min-w-0 flex-1 gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={text}
        inputMode="numeric"
        autoComplete="off"
        placeholder={t('ui.date.format')}
        aria-invalid={invalid || undefined}
        className="tabular-nums"
        onChange={(e) => {
          setText(e.target.value);
          const iso = parseTypedDate(e.target.value);
          if (iso && onCommit(iso)) setInvalid(false);
        }}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            commit();
          }
        }}
      />
    </div>
  );
}

export function RangeCalendar({
  from,
  to,
  onSelect,
  min,
  max,
  numberOfMonths,
}: RangeCalendarProps) {
  const { t } = useTranslation();
  const fromDate = isoToDate(from);
  const toDate = isoToDate(to);
  const selected: DayPickerRange | undefined = fromDate
    ? { from: fromDate, to: toDate }
    : undefined;
  const c = useCalendarProps(min, max);
  const [month, setMonth] = useState<Date | undefined>(fromDate ?? c.minDate);
  // The calendar mounts on each opening: the first click always starts a new
  // range and the second one ends it (react-day-picker would otherwise move
  // the nearest end of the range already set).
  const [awaitingEnd, setAwaitingEnd] = useState(false);
  // Moves the calendar to a typed date only when it is not on screen: a
  // re-render of the grid between the blur that commits the field and the
  // click on a day would swallow that click.
  const showMonthOf = (iso: string) => {
    const d = isoToDate(iso);
    if (!d || !month) return setMonth(d);
    const first = month.getFullYear() * 12 + month.getMonth();
    const target = d.getFullYear() * 12 + d.getMonth();
    if (target < first || target >= first + (numberOfMonths ?? 1)) setMonth(d);
  };
  const pick = (day: Date) => {
    const iso = dateToIso(day);
    if (!awaitingEnd || !from) {
      setAwaitingEnd(true);
      onSelect({ from: iso });
      return;
    }
    setAwaitingEnd(false);
    onSelect(iso < from ? { from: iso, to: from } : { from, to: iso });
  };
  return (
    <div>
      <div className="flex gap-3 border-b p-3">
        <TypedDate
          label={t('ui.date.from')}
          value={from}
          onCommit={(iso) => {
            if (!withinBounds(iso, min, max)) return false;
            // The next click on the calendar picks the end of the range.
            setAwaitingEnd(true);
            // A start after the end clears the end, to be picked again.
            onSelect({ from: iso, to: to && to >= iso ? to : undefined });
            showMonthOf(iso);
            return true;
          }}
        />
        <TypedDate
          label={t('ui.date.to')}
          value={to}
          onCommit={(iso) => {
            if (!from || iso < from || !withinBounds(iso, min, max)) return false;
            setAwaitingEnd(false);
            onSelect({ from, to: iso });
            return true;
          }}
        />
      </div>
      <Calendar
        mode="range"
        selected={selected}
        month={month}
        onMonthChange={setMonth}
        numberOfMonths={numberOfMonths}
        locale={c.locale}
        weekStartsOn={c.weekStartsOn}
        fixedWeeks={c.fixedWeeks}
        startMonth={c.startMonth}
        endMonth={c.endMonth}
        disabled={c.disabled}
        onSelect={(_, day) => pick(day)}
      />
    </div>
  );
}
