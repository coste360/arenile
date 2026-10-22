import { format } from 'date-fns';
import { CalendarIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import type { DateRange as DayPickerRange, Matcher } from 'react-day-picker';
import { useTranslation } from 'react-i18next';
import { Button } from '@arenile/shared/ui/button';
import { Calendar } from '@arenile/shared/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@arenile/shared/ui/popover';
import { cn } from '@arenile/shared/lib/utils';
import {
  DISPLAY_DATE_FORMAT,
  dateFnsLocale,
  dateToIso,
  isoToDate,
} from '@arenile/shared/lib/dates';

interface CommonProps {
  id?: string;
  placeholder?: string;
  disabled?: boolean;
  /** Show a clear button when a value is set (default false). */
  clearable?: boolean;
  /** Earliest/latest selectable day, "YYYY-MM-DD". */
  min?: string;
  max?: string;
  className?: string;
  'aria-invalid'?: boolean | 'true' | 'false';
  'aria-describedby'?: string;
  'aria-label'?: string;
  /** Lets FormControl wire the label (forwarded to the trigger button). */
  'aria-labelledby'?: string;
}

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
  return { locale, disabled, weekStartsOn: 1 as const, startMonth, endMonth };
}

const triggerClass =
  'w-full min-w-0 justify-start gap-2 overflow-hidden px-3 text-left font-normal tabular-nums data-[empty=true]:text-muted-foreground sm:w-56';

export interface DatePickerProps extends CommonProps {
  /** Calendar date "YYYY-MM-DD" (API format); undefined/empty when unset. */
  value: string | null | undefined;
  onChange: (value: string | undefined) => void;
}

/**
 * Date field (never a native <input type="date">): shows dd/MM/yyyy, picks
 * with the shadcn Calendar in the UI language (week starts on Monday) and
 * returns "YYYY-MM-DD".
 */
export function DatePicker({
  value,
  onChange,
  placeholder,
  clearable,
  min,
  max,
  className,
  disabled,
  ...aria
}: DatePickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selected = isoToDate(value);
  const calendar = useCalendarProps(min, max);
  return (
    <div className={cn('flex min-w-0 items-center gap-1', className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            disabled={disabled}
            data-empty={!selected}
            className={triggerClass}
            {...aria}
          >
            <CalendarIcon aria-hidden="true" className="text-muted-foreground" />
            {selected
              ? format(selected, DISPLAY_DATE_FORMAT)
              : (placeholder ?? t('ui.date.placeholder'))}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Calendar
            mode="single"
            selected={selected}
            defaultMonth={selected}
            captionLayout="dropdown"
            locale={calendar.locale}
            weekStartsOn={calendar.weekStartsOn}
            startMonth={calendar.startMonth}
            endMonth={calendar.endMonth}
            disabled={calendar.disabled}
            onSelect={(d) => {
              onChange(d ? dateToIso(d) : undefined);
              setOpen(false);
            }}
          />
        </PopoverContent>
      </Popover>
      {clearable && selected && !disabled && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('ui.date.clear')}
          onClick={() => onChange(undefined)}
        >
          <XIcon aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}

/** Inclusive range of calendar dates, "YYYY-MM-DD". */
export interface IsoDateRange {
  from: string;
  to?: string;
}

export interface DateRangePickerProps extends CommonProps {
  value: IsoDateRange | null | undefined;
  onChange: (value: IsoDateRange | undefined) => void;
  /** Months shown side by side on wide screens (default 2). */
  numberOfMonths?: number;
}

/** Period field: "14/07/2026 – 21/07/2026", two-month calendar, ISO in/out. */
export function DateRangePicker({
  value,
  onChange,
  placeholder,
  clearable,
  min,
  max,
  numberOfMonths = 2,
  className,
  disabled,
  ...aria
}: DateRangePickerProps) {
  const { t } = useTranslation();
  const from = isoToDate(value?.from);
  const to = isoToDate(value?.to);
  const selected: DayPickerRange | undefined = from ? { from, to } : undefined;
  const calendar = useCalendarProps(min, max);
  const label = from
    ? `${format(from, DISPLAY_DATE_FORMAT)} – ${to ? format(to, DISPLAY_DATE_FORMAT) : '…'}`
    : (placeholder ?? t('ui.date.rangePlaceholder'));
  return (
    <div className={cn('flex min-w-0 items-center gap-1', className)}>
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            disabled={disabled}
            data-empty={!from}
            className={cn(triggerClass, 'sm:w-72')}
            {...aria}
          >
            <CalendarIcon aria-hidden="true" className="text-muted-foreground" />
            {label}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Calendar
            mode="range"
            selected={selected}
            defaultMonth={from}
            numberOfMonths={numberOfMonths}
            locale={calendar.locale}
            weekStartsOn={calendar.weekStartsOn}
            startMonth={calendar.startMonth}
            endMonth={calendar.endMonth}
            disabled={calendar.disabled}
            onSelect={(r) =>
              onChange(
                r?.from
                  ? { from: dateToIso(r.from), to: r.to ? dateToIso(r.to) : undefined }
                  : undefined,
              )
            }
          />
        </PopoverContent>
      </Popover>
      {clearable && from && !disabled && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('ui.date.clear')}
          onClick={() => onChange(undefined)}
        >
          <XIcon aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
