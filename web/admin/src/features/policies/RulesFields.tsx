import { Button, Input } from '@arenile/shared/ui';
import {
  useFieldArray,
  useWatch,
  type Control,
  type FieldErrors,
  type UseFormRegister,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Checkbox, Field, Select } from '../configuration/components/fields';
import { MultiCheck } from '../configuration/components/MultiCheck';
import { errorAt, fieldAria } from '../configuration/lib/aria';
import { nextTier, sortTiers, type RulesValues } from './policy.schema';

const FORMULAS = ['half_day', 'day', 'multi_day', 'week', 'fortnight', 'month', 'season'] as const;

/**
 * Editor of the booking rules (spec 9). It is used with forms whose values
 * extend RulesValues; "inherit" / empty leaves a rule to the default policy.
 */
export function RulesFields({
  control,
  register,
  errors,
  idPrefix,
  isDefault,
}: {
  // The forms extend RulesValues; the editor only touches those fields.
  control: Control<RulesValues>;
  register: UseFormRegister<RulesValues>;
  errors: FieldErrors<RulesValues>;
  idPrefix: string;
  /** Editing the default policy: "inherit" means the built-in default. */
  isDefault?: boolean;
}) {
  const { t } = useTranslation('policies');
  const tiers = useFieldArray({ control, name: 'cancellation' });
  const tierValues = useWatch({ control, name: 'cancellation' });
  const [paymentMode, sameDay, balanceDue, cancellationSet, releaseCredit, depositType] = useWatch({
    control,
    name: [
      'payment_mode',
      'same_day',
      'balance_due',
      'cancellation_set',
      'release_credit',
      'deposit_type',
    ],
  });
  const id = (name: string) => `${idPrefix}-${name.replace(/\./g, '-')}`;
  const err = (name: string) => errorAt(errors, name);
  const text = (
    name: keyof RulesValues,
    label: string,
    opts: { hint?: string; type?: string } = {},
  ) => (
    <Field id={id(name)} label={label} error={err(name)} hint={opts.hint}>
      <Input
        {...fieldAria(id(name), err(name), !!opts.hint)}
        type={opts.type ?? 'text'}
        inputMode={opts.type ? undefined : 'numeric'}
        {...register(name)}
      />
    </Field>
  );
  const choice = (
    name: keyof RulesValues,
    label: string,
    options: { value: string; label: string }[],
  ) => (
    <Field id={id(name)} label={label} error={err(name)}>
      <Select {...fieldAria(id(name), err(name))} {...register(name)}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </Select>
    </Field>
  );
  const inherit = isDefault ? t('inheritBuiltin') : t('inherit');
  const triOptions = [
    { value: 'inherit', label: inherit },
    { value: 'yes', label: t('yes') },
    { value: 'no', label: t('no') },
  ];

  return (
    <div className="space-y-4">
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('sections.timing')}</legend>
        <div className="grid gap-4 sm:grid-cols-3">
          {text('min_lead_days', t('fields.min_lead_days'))}
          {text('cutoff_time', t('fields.cutoff_time'), { type: 'time', hint: t('hints.cutoff') })}
          {text('max_lead_time_days', t('fields.max_lead_time_days'))}
          {choice('same_day', t('fields.same_day'), triOptions)}
          {sameDay === 'yes' &&
            text('same_day_until', t('fields.same_day_until'), { type: 'time' })}
        </div>
      </fieldset>
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('sections.duration')}</legend>
        <MultiCheck
          control={control}
          name="allowed_formulas"
          id={id('allowed_formulas')}
          legend={t('fields.allowed_formulas')}
          hint={t('hints.allFormulas')}
          options={FORMULAS.map((f) => ({ value: f, label: t(`pricing:formulas.${f}`) }))}
        />
        <div className="grid gap-4 sm:grid-cols-3">
          {text('min_days', t('fields.min_days'))}
          {text('max_days', t('fields.max_days'))}
          {choice('fixed_start_weekday', t('fields.fixed_start_weekday'), [
            { value: '', label: inherit },
            { value: '0', label: t('anyWeekday') },
            ...['1', '2', '3', '4', '5', '6', '7'].map((d) => ({
              value: d,
              label: t(`pricing:weekdaysShort.${d}`),
            })),
          ])}
        </div>
      </fieldset>
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('sections.payment')}</legend>
        <div className="grid gap-4 sm:grid-cols-3">
          {choice('payment_mode', t('fields.payment_mode'), [
            { value: '', label: inherit },
            { value: 'full', label: t('paymentModes.full') },
            { value: 'deposit', label: t('paymentModes.deposit') },
            { value: 'none', label: t('paymentModes.none') },
          ])}
          {paymentMode === 'deposit' && (
            <>
              {choice('deposit_type', t('fields.deposit_type'), [
                { value: 'percent', label: t('depositTypes.percent') },
                { value: 'amount', label: t('depositTypes.amount') },
              ])}
              {text(
                'deposit_value',
                depositType === 'amount' ? t('fields.deposit_amount') : t('fields.deposit_percent'),
              )}
            </>
          )}
          {paymentMode !== '' && paymentMode !== 'full' && (
            <>
              {choice('balance_due', t('fields.balance_due'), [
                { value: '', label: inherit },
                { value: 'on_arrival', label: t('balance.on_arrival') },
                { value: 'days_before', label: t('balance.days_before') },
              ])}
              {balanceDue === 'days_before' && text('balance_days', t('fields.balance_days'))}
            </>
          )}
        </div>
      </fieldset>
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('sections.cancellation')}</legend>
        <Checkbox
          id={id('cancellation_set')}
          label={t('fields.cancellation_set')}
          {...register('cancellation_set')}
        />
        {cancellationSet && (
          <div className="space-y-2">
            {tiers.fields.length === 0 && <p className="text-sm text-slate-600">{t('noRefund')}</p>}
            {tiers.fields.map((f, i) => (
              <div
                key={f.id}
                data-testid="cancellation-tier"
                className="grid grid-cols-[1fr_1fr_1fr_auto] items-end gap-2"
              >
                <Field
                  id={id(`cancellation.${i}.until_days_before`)}
                  label={t('fields.until_days_before')}
                  error={err(`cancellation.${i}.until_days_before`)}
                >
                  <Input
                    {...fieldAria(
                      id(`cancellation.${i}.until_days_before`),
                      err(`cancellation.${i}.until_days_before`),
                    )}
                    inputMode="numeric"
                    {...register(`cancellation.${i}.until_days_before`)}
                  />
                </Field>
                <Field
                  id={id(`cancellation.${i}.refund_percent`)}
                  label={t('fields.refund_percent')}
                  error={err(`cancellation.${i}.refund_percent`)}
                >
                  <Input
                    {...fieldAria(
                      id(`cancellation.${i}.refund_percent`),
                      err(`cancellation.${i}.refund_percent`),
                    )}
                    inputMode="numeric"
                    {...register(`cancellation.${i}.refund_percent`)}
                  />
                </Field>
                <Field id={id(`cancellation.${i}.as`)} label={t('fields.refund_as')}>
                  <Select id={id(`cancellation.${i}.as`)} {...register(`cancellation.${i}.as`)}>
                    <option value="refund">{t('refundAs.refund')}</option>
                    <option value="credit">{t('refundAs.credit')}</option>
                  </Select>
                </Field>
                <div className="flex gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t('moveTierUp', { n: i + 1 })}
                    disabled={i === 0}
                    onClick={() => tiers.move(i, i - 1)}
                  >
                    ↑
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t('moveTierDown', { n: i + 1 })}
                    disabled={i === tiers.fields.length - 1}
                    onClick={() => tiers.move(i, i + 1)}
                  >
                    ↓
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t('removeTier', { n: i + 1 })}
                    onClick={() => tiers.remove(i)}
                  >
                    {t('remove')}
                  </Button>
                </div>
              </div>
            ))}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => tiers.append(nextTier(tierValues ?? []))}
              >
                {t('addTier')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={tiers.fields.length < 2}
                onClick={() => tiers.replace(sortTiers(tierValues ?? []))}
              >
                {t('sortTiers')}
              </Button>
            </div>
            <p className="text-xs text-slate-600">{t('hints.tiers')}</p>
          </div>
        )}
        <div className="grid gap-4 sm:grid-cols-3">
          {text('modification_until', t('fields.modification_until'))}
          {text('modification_fee', t('fields.modification_fee'))}
          {choice('weather', t('fields.weather'), [
            { value: 'inherit', label: inherit },
            { value: 'off', label: t('weather.off') },
            { value: 'refund', label: t('weather.refund') },
            { value: 'credit', label: t('weather.credit') },
          ])}
        </div>
      </fieldset>
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('sections.other')}</legend>
        <div className="grid gap-4 sm:grid-cols-3">
          {choice('approval_required', t('fields.approval_required'), triOptions)}
          {text('max_spots_per_booking', t('fields.max_spots_per_booking'))}
          {choice('allow_spot_change', t('fields.allow_spot_change'), triOptions)}
          {choice('customer_can_release', t('fields.customer_can_release'), triOptions)}
          {choice('release_credit', t('fields.release_credit'), [
            { value: 'inherit', label: inherit },
            { value: 'off', label: t('no') },
            { value: 'on', label: t('yes') },
          ])}
          {releaseCredit === 'on' &&
            text('release_credit_percent', t('fields.release_credit_percent'))}
        </div>
      </fieldset>
    </div>
  );
}
