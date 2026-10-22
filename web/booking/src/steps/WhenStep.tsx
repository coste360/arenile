import { isoToDate, dateToIso } from '@arenile/shared/lib/dates';
import { Field, FieldDescription, FieldLabel } from '@arenile/shared/ui/field';
import { DateRangePicker, type IsoDateRange } from '@arenile/shared/ui/patterns/DatePicker';
import { differenceInCalendarDays } from 'date-fns';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

export interface WhenStepProps {
  value: IsoDateRange | undefined;
  onChange: (value: IsoDateRange | undefined) => void;
  /** Today, injectable for tests ("YYYY-MM-DD"). */
  today?: string;
}

/**
 * "Quando" step content: period of the stay with the shared shadcn date range
 * picker (dd/MM/yyyy, week starting on Monday). Loaded lazily so the calendar
 * (react-day-picker, date-fns, Radix Popover) stays out of the initial
 * bundle (spec 12.6, 150 KB budget).
 */
export default function WhenStep({
  value,
  onChange,
  today = dateToIso(new Date()),
}: WhenStepProps) {
  const { t } = useTranslation();
  const id = useId();
  const from = isoToDate(value?.from);
  const to = isoToDate(value?.to ?? value?.from);
  const days = from && to ? differenceInCalendarDays(to, from) + 1 : 0;

  return (
    <Field>
      <FieldLabel id={`${id}-label`} htmlFor={`${id}-trigger`}>
        {t('booking.when.dates')}
      </FieldLabel>
      <DateRangePicker
        value={value}
        onChange={onChange}
        min={today}
        clearable
        numberOfMonths={1}
        id={`${id}-trigger`}
        aria-labelledby={`${id}-label ${id}-trigger`}
        aria-describedby={`${id}-hint`}
      />
      <FieldDescription id={`${id}-hint`} aria-live="polite">
        {days > 0 ? t('booking.when.days', { count: days }) : t('booking.when.pickHint')}
      </FieldDescription>
    </Field>
  );
}
