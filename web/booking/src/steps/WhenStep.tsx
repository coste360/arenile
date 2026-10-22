import { daysInclusive, dateToIso, maxIso } from '@arenile/shared/lib/dates';
import { Field, FieldDescription, FieldLabel } from '@arenile/shared/ui/field';
import { DateRangePicker, type IsoDateRange } from '@arenile/shared/ui/patterns/DatePicker';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

export interface WhenStepProps {
  value: IsoDateRange | undefined;
  onChange: (value: IsoDateRange | undefined) => void;
  /** Bookable window of the active season ("YYYY-MM-DD"); without it, today onwards. */
  season?: { opens_on: string; closes_on: string };
  /** Today, injectable for tests ("YYYY-MM-DD"). */
  today?: string;
}

/**
 * "Quando" step content: period of the stay with the shared shadcn date range
 * picker (dd/MM/yyyy, week starting on Monday), limited to the active season
 * and never in the past. The calendar itself loads when the field opens.
 */
export function WhenStep({
  value,
  onChange,
  season,
  today = dateToIso(new Date()),
}: WhenStepProps) {
  const { t } = useTranslation();
  const id = useId();
  const days = value ? daysInclusive(value.from, value.to ?? value.from) : 0;
  const min = season ? maxIso(today, season.opens_on) : today;
  const max = season?.closes_on;

  return (
    <Field>
      <FieldLabel id={`${id}-label`} htmlFor={`${id}-trigger`}>
        {t('booking.when.dates')}
      </FieldLabel>
      <DateRangePicker
        value={value}
        onChange={onChange}
        min={min}
        max={max}
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
