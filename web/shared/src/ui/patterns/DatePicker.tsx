import { CalendarIcon, XIcon } from 'lucide-react';
import { Suspense, lazy, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@arenile/shared/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@arenile/shared/ui/popover';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { cn } from '@arenile/shared/lib/utils';
import { formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';

// The calendar body (react-day-picker, date-fns locales) loads on first open:
// the trigger alone is in the first-paint bundle of the booking page.
const SingleCalendar = lazy(() =>
  import('./DatePickerCalendar').then((m) => ({ default: m.SingleCalendar })),
);
const RangeCalendar = lazy(() =>
  import('./DatePickerCalendar').then((m) => ({ default: m.RangeCalendar })),
);

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

const triggerClass =
  'w-full min-w-0 justify-start gap-2 overflow-hidden bg-transparent px-3 text-left font-normal tabular-nums data-[empty=true]:text-muted-foreground sm:w-56 sm:max-w-full dark:bg-input/30';

/**
 * Naming of the trigger so the name contains what is on screen (WCAG 2.5.3
 * Label in Name; speech users say what they see): `aria-label` becomes
 * "label: visible text", `aria-labelledby` also references the visible text
 * (unless it already points at the trigger itself).
 */
function triggerName(
  props: { id?: string; 'aria-label'?: string; 'aria-labelledby'?: string },
  visible: string,
  valueId: string,
): { 'aria-label'?: string; 'aria-labelledby'?: string } {
  const labelledBy = props['aria-labelledby'];
  if (labelledBy) {
    const ids = labelledBy.split(/\s+/);
    return {
      'aria-labelledby':
        props.id && ids.includes(props.id) ? labelledBy : `${labelledBy} ${valueId}`,
    };
  }
  return props['aria-label'] ? { 'aria-label': `${props['aria-label']}: ${visible}` } : {};
}

function CalendarLoading({ label, wide }: { label: string; wide?: boolean }) {
  return (
    <div role="status" className={cn('space-y-2 p-3', wide ? 'w-[36rem] max-w-[90vw]' : 'w-72')}>
      <span className="sr-only">{label}</span>
      <Skeleton aria-hidden="true" className="h-8 w-full" />
      <Skeleton aria-hidden="true" className="h-56 w-full" />
    </div>
  );
}

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
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
  ...aria
}: DatePickerProps) {
  const { t } = useTranslation();
  const valueId = useId();
  const [open, setOpen] = useState(false);
  const selected = isoToDate(value);
  const visible =
    selected && value ? formatIsoDate(value) : (placeholder ?? t('ui.date.placeholder'));
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
            {...triggerName(
              { id: aria.id, 'aria-label': ariaLabel, 'aria-labelledby': ariaLabelledBy },
              visible,
              valueId,
            )}
          >
            <CalendarIcon aria-hidden="true" className="text-muted-foreground" />
            <span id={valueId}>{visible}</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Suspense fallback={<CalendarLoading label={t('common.loading')} />}>
            <SingleCalendar
              value={value}
              min={min}
              max={max}
              onSelect={(iso) => {
                onChange(iso);
                setOpen(false);
              }}
            />
          </Suspense>
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
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
  ...aria
}: DateRangePickerProps) {
  const { t } = useTranslation();
  const valueId = useId();
  const from = isoToDate(value?.from);
  const label =
    from && value
      ? `${formatIsoDate(value.from)} – ${value.to ? formatIsoDate(value.to) : '…'}`
      : (placeholder ?? t('ui.date.rangePlaceholder'));
  return (
    <div className={cn('flex min-w-0 items-center gap-1', className)}>
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            disabled={disabled}
            data-empty={!from}
            className={cn(triggerClass, 'sm:w-72 sm:max-w-full')}
            {...aria}
            {...triggerName(
              { id: aria.id, 'aria-label': ariaLabel, 'aria-labelledby': ariaLabelledBy },
              label,
              valueId,
            )}
          >
            <CalendarIcon aria-hidden="true" className="text-muted-foreground" />
            <span id={valueId}>{label}</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent
          className="w-auto p-0"
          align="start"
          // Keep the focus off the typed fields: on phones it would open the keyboard over the calendar.
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          <Suspense
            fallback={<CalendarLoading label={t('common.loading')} wide={numberOfMonths > 1} />}
          >
            <RangeCalendar
              from={value?.from}
              to={value?.to}
              min={min}
              max={max}
              numberOfMonths={numberOfMonths}
              onSelect={onChange}
            />
          </Suspense>
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
