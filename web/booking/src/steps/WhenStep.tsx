import { daysInclusive } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Field, FieldDescription, FieldLabel } from '@arenile/shared/ui/field';
import { CircleCheckIcon, InfoIcon, TriangleAlertIcon } from 'lucide-react';
import { useId, type Dispatch } from 'react';
import { useTranslation } from 'react-i18next';
import { usePublicCalendar } from '../api/public';
import { ChoiceGroup } from '../components/ChoiceGroup';
import { useBookingEnv } from '../flow/context';
import {
  bookableWindow,
  compatibleFormulas,
  formulaChoices,
  type FlowAction,
  type FlowState,
  type Formula,
  type Slots,
  type StayDates,
} from '../flow/selection';
import { PUBLIC_NS } from '../i18n';
import { AvailabilityRangePicker } from './when/AvailabilityRangePicker';

export interface WhenStepProps {
  state: FlowState;
  dispatch: Dispatch<FlowAction>;
}

type RangeStatus = 'available' | 'limited' | 'full' | null;

/** Worst availability of the chosen days, from the (cached) month calendars. */
function useRangeStatus(slug: string, dates: StayDates | undefined, slots: Slots): RangeStatus {
  const fromMonth = dates?.from.slice(0, 7) ?? '';
  const toMonth = dates?.to?.slice(0, 7) ?? fromMonth;
  const complete = !!dates?.to;
  const a = usePublicCalendar(slug, fromMonth, { slots, enabled: complete });
  const b = usePublicCalendar(slug, toMonth, { slots, enabled: complete && toMonth !== fromMonth });
  if (!dates?.to) return null;
  const days = [
    ...(a.data?.month === fromMonth ? a.data.days : []),
    ...(toMonth !== fromMonth && b.data?.month === toMonth ? b.data.days : []),
  ].filter((d) => d.date >= dates.from && d.date <= (dates.to as string));
  if (days.length === 0) return null;
  if (days.some((d) => d.status !== 'available' && d.status !== 'limited')) return 'full';
  if (days.some((d) => d.status === 'limited')) return 'limited';
  return 'available';
}

/**
 * "Quando" (spec 12.3 passo 1): stay dates on the availability calendar
 * (limited to the season and the online window), the slot and the
 * compatible formula. The first compatible formula is chosen automatically;
 * "stagione intera" sets the dates to the whole season.
 */
export function WhenStep({ state, dispatch }: WhenStepProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { slug, establishment, online, today } = useBookingEnv();
  const id = useId();
  const season = establishment.season ?? null;
  const window = bookableWindow(today, online, season);
  const slotOptions: Slots[] = online.slots.length > 0 ? online.slots : ['FULL'];
  const choices = formulaChoices(state.dates, state.slots, online.formulas, season);
  const rangeStatus = useRangeStatus(slug, state.dates, state.slots);
  const days = state.dates
    ? daysInclusive(state.dates.from, state.dates.to ?? state.dates.from)
    : 0;

  const formulaFor = (dates: StayDates | undefined, slots: Slots): Formula | null =>
    compatibleFormulas(dates, slots, online.formulas, season)[0] ?? null;

  const onFormula = (formula: Formula) => {
    if (formula === 'season' && season) {
      dispatch({
        type: 'formula',
        formula,
        dates: { from: season.opens_on, to: season.closes_on },
      });
    } else {
      dispatch({ type: 'formula', formula });
    }
  };

  return (
    <div className="space-y-6">
      <Field>
        <FieldLabel id={`${id}-label`} htmlFor={`${id}-trigger`}>
          {tc('booking.when.dates')}
        </FieldLabel>
        <AvailabilityRangePicker
          id={`${id}-trigger`}
          slug={slug}
          value={state.dates}
          min={window.min}
          max={window.max}
          slots={state.slots}
          showRatePeriods={online.show_rate_periods}
          onChange={(dates) =>
            dispatch({ type: 'dates', dates, formula: formulaFor(dates, state.slots) })
          }
          aria-labelledby={`${id}-label ${id}-trigger`}
          aria-describedby={`${id}-hint`}
        />
        <FieldDescription id={`${id}-hint`} aria-live="polite">
          {days > 0 ? tc('booking.when.days', { count: days }) : tc('booking.when.pickHint')}
        </FieldDescription>
      </Field>

      {slotOptions.length > 1 && (
        <div className="space-y-2">
          <p id={`${id}-slot`} className="text-sm font-medium">
            {t('when.slot')}
          </p>
          <ChoiceGroup
            labelledBy={`${id}-slot`}
            value={state.slots}
            options={slotOptions.map((s) => ({ value: s, label: t(`when.slots.${s}`) }))}
            onChange={(slots) =>
              dispatch({ type: 'slots', slots, formula: formulaFor(state.dates, slots) })
            }
          />
        </div>
      )}

      {state.dates?.to && (
        <div className="space-y-2">
          <p id={`${id}-formula`} className="text-sm font-medium">
            {t('when.formula')}
          </p>
          {choices.length > 0 ? (
            <ChoiceGroup
              labelledBy={`${id}-formula`}
              value={state.formula}
              options={choices.map((f) => ({ value: f, label: t(`when.formulas.${f}`) }))}
              onChange={onFormula}
            />
          ) : (
            <p className="text-sm text-muted-foreground">{t('when.noFormula')}</p>
          )}
        </div>
      )}

      {rangeStatus && (
        <Alert
          role="status"
          className={rangeStatus === 'full' ? 'border-destructive/40' : undefined}
        >
          {rangeStatus === 'available' ? (
            <CircleCheckIcon aria-hidden="true" />
          ) : rangeStatus === 'limited' ? (
            <InfoIcon aria-hidden="true" />
          ) : (
            <TriangleAlertIcon aria-hidden="true" />
          )}
          <AlertDescription>
            {t(
              rangeStatus === 'available'
                ? 'when.rangeAvailable'
                : rangeStatus === 'limited'
                  ? 'when.rangeLimited'
                  : 'when.rangeFull',
            )}
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
