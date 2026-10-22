import { Button, Input } from '@arenile/shared/ui';
import { Link } from '@tanstack/react-router';
import {
  useFieldArray,
  useWatch,
  type Control,
  type FieldErrors,
  type FieldValues,
  type Path,
  type UseFormRegister,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { RatePeriod, Season } from '../configuration/api/resources';
import { Field, Select } from '../configuration/components/fields';
import { MultiCheck } from '../configuration/components/MultiCheck';
import { errorAt, fieldAria } from '../configuration/lib/aria';
import {
  CHANNELS,
  FORMULAS,
  SLOTS,
  WEEKDAYS,
  type ActionType,
  type ConditionValues,
} from './rules.schema';

export function SeasonSelect({
  seasons: items,
  season,
  onSelect,
}: {
  seasons: Season[];
  season: Season | undefined;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  if (items.length === 0) return null;
  return (
    <label className="flex items-center gap-2 text-sm">
      <span className="font-medium">{t('season')}</span>
      <Select className="w-44" value={season?.id ?? ''} onChange={(e) => onSelect(e.target.value)}>
        {items.map((s) => (
          <option key={s.id} value={s.id}>
            {s.year} · {t(`seasonStatus.${s.status}`)}
          </option>
        ))}
      </Select>
    </label>
  );
}

/** Links between the pricing pages (rules, simulator, matrix, coverage). */
export function PricingNav({
  establishmentId,
  seasonId,
}: {
  establishmentId: string;
  seasonId?: string;
}) {
  const { t } = useTranslation('pricing');
  const search = seasonId ? { seasonId } : {};
  const cls =
    'rounded-md px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100 [&.active]:bg-slate-900 [&.active]:text-white';
  const links = [
    { to: '/e/$establishmentId/pricing', label: t('nav.rules') },
    { to: '/e/$establishmentId/pricing/simulator', label: t('nav.simulator') },
    { to: '/e/$establishmentId/pricing/matrix', label: t('nav.matrix') },
    { to: '/e/$establishmentId/pricing/coverage', label: t('nav.coverage') },
  ] as const;
  return (
    <nav aria-label={t('nav.label')} className="flex flex-wrap gap-1">
      {links.map((l) => (
        <Link
          key={l.to}
          to={l.to}
          params={{ establishmentId }}
          search={search}
          activeOptions={{ exact: true, includeSearch: false }}
          className={cls}
        >
          {l.label}
        </Link>
      ))}
    </nav>
  );
}

/** Action editor (type + the fields it needs), bound to the flat action form fields. */
export function ActionFields<T extends FieldValues>({
  control,
  register,
  errors,
  allowed,
  idPrefix,
}: {
  control: Control<T>;
  register: UseFormRegister<T>;
  errors: FieldErrors<T>;
  allowed: readonly ActionType[];
  idPrefix: string;
}) {
  const { t } = useTranslation('pricing');
  const type = useWatch({ control, name: 'action_type' as Path<T> }) as ActionType;
  const err = (name: string) => errorAt(errors, name);
  const input = (name: string, label: string, hint?: string) => (
    <Field id={`${idPrefix}-${name}`} label={label} error={err(name)} hint={hint}>
      <Input
        {...fieldAria(`${idPrefix}-${name}`, err(name), !!hint)}
        inputMode="decimal"
        {...register(name as Path<T>)}
      />
    </Field>
  );
  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <Field id={`${idPrefix}-action_type`} label={t('fields.action')} error={err('action_type')}>
        <Select
          {...fieldAria(`${idPrefix}-action_type`, err('action_type'))}
          {...register('action_type' as Path<T>)}
        >
          {allowed.map((a) => (
            <option key={a} value={a}>
              {t(`actions.${a}`)}
            </option>
          ))}
        </Select>
      </Field>
      {(type === 'set' || type === 'add' || type === 'amount_off') &&
        input('amount', t('fields.amount'), type === 'add' ? t('hints.signedAmount') : undefined)}
      {type === 'amount_off' && (
        <Field id={`${idPrefix}-per`} label={t('fields.per')} error={err('per')}>
          <Select {...fieldAria(`${idPrefix}-per`, err('per'))} {...register('per' as Path<T>)}>
            <option value="">{t('per.default')}</option>
            {(['booking', 'line', 'day'] as const).map((p) => (
              <option key={p} value={p}>
                {t(`per.${p}`)}
              </option>
            ))}
          </Select>
        </Field>
      )}
      {type === 'percent_off' && input('percent', t('fields.percent'))}
      {type === 'multiply' && input('factor', t('fields.factor'), t('hints.factor'))}
      {type === 'free_days' && (
        <>
          {input('every', t('fields.every'))}
          {input('free', t('fields.free'))}
        </>
      )}
    </div>
  );
}

/** Conditions editor (spec 8.2): every dimension optional, AND between them. */
export function ConditionFields<T extends FieldValues>({
  control,
  register,
  errors,
  periods,
  zones,
  agreements,
  idPrefix,
}: {
  control: Control<T>;
  register: UseFormRegister<T>;
  errors: FieldErrors<T>;
  periods: RatePeriod[];
  zones: { id: string; name: string }[];
  /** Offered as a condition only for rules (not inside promo codes or agreements). */
  agreements?: { id: string; name: string }[];
  idPrefix: string;
}) {
  const { t } = useTranslation('pricing');
  // The forms extend ConditionValues; the editor only touches those fields.
  const ranges = useFieldArray({
    control: control as unknown as Control<ConditionValues>,
    name: 'date_ranges',
  });
  const occupancy = useWatch({
    control: control as unknown as Control<ConditionValues>,
    name: ['occupancy_min', 'occupancy_max'],
  });
  const err = (name: string) => errorAt(errors, name);
  const text = (
    name: string,
    label: string,
    opts: { hint?: string; type?: string; numeric?: boolean } = {},
  ) => (
    <Field
      id={`${idPrefix}-${name.replace(/\./g, '-')}`}
      label={label}
      error={err(name)}
      hint={opts.hint}
    >
      <Input
        {...fieldAria(`${idPrefix}-${name.replace(/\./g, '-')}`, err(name), !!opts.hint)}
        type={opts.type ?? 'text'}
        inputMode={opts.numeric ? 'numeric' : undefined}
        {...register(name as Path<T>)}
      />
    </Field>
  );
  return (
    <fieldset className="space-y-4 rounded-md border border-slate-200 p-3">
      <legend className="px-1 text-sm font-medium text-slate-800">{t('fields.conditions')}</legend>
      <p className="text-xs text-slate-600">{t('hints.conditions')}</p>
      {periods.length > 0 && (
        <MultiCheck
          control={control}
          name={'rate_periods' as Path<T>}
          id={`${idPrefix}-rate_periods`}
          legend={t('fields.rate_periods')}
          options={periods.map((p) => ({ value: p.code, label: p.code }))}
        />
      )}
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-slate-800">{t('fields.date_ranges')}</legend>
        {ranges.fields.length === 0 && (
          <p className="text-xs text-slate-600">{t('hints.noDateRanges')}</p>
        )}
        {ranges.fields.map((f, i) => (
          <div key={f.id} className="grid grid-cols-[1fr_1fr_auto] items-end gap-2">
            {text(`date_ranges.${i}.from`, t('fields.date_from'), { type: 'date' })}
            {text(`date_ranges.${i}.to`, t('fields.date_to'), { type: 'date' })}
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('removeDateRange', { n: i + 1 })}
              onClick={() => ranges.remove(i)}
            >
              {t('remove')}
            </Button>
          </div>
        ))}
        <Button variant="outline" size="sm" onClick={() => ranges.append({ from: '', to: '' })}>
          {t('addDateRange')}
        </Button>
      </fieldset>
      <MultiCheck
        control={control}
        name={'weekdays' as Path<T>}
        id={`${idPrefix}-weekdays`}
        legend={t('fields.weekdays')}
        options={WEEKDAYS.map((d) => ({ value: d, label: t(`weekdaysShort.${d}`) }))}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <MultiCheck
          control={control}
          name={'slots' as Path<T>}
          id={`${idPrefix}-slots`}
          legend={t('fields.slots')}
          options={SLOTS.map((s) => ({ value: s, label: t(`slots.${s}`) }))}
        />
        <MultiCheck
          control={control}
          name={'channels' as Path<T>}
          id={`${idPrefix}-channels`}
          legend={t('fields.channels')}
          options={CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
        />
      </div>
      <MultiCheck
        control={control}
        name={'formulas' as Path<T>}
        id={`${idPrefix}-formulas`}
        legend={t('fields.formulas')}
        options={FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) }))}
      />
      {zones.length > 0 && (
        <MultiCheck
          control={control}
          name={'zone_ids' as Path<T>}
          id={`${idPrefix}-zone_ids`}
          legend={t('fields.zones')}
          options={zones.map((z) => ({ value: z.id, label: z.name }))}
        />
      )}
      {agreements && agreements.length > 0 && (
        <MultiCheck
          control={control}
          name={'agreement_ids' as Path<T>}
          id={`${idPrefix}-agreement_ids`}
          legend={t('fields.agreements')}
          options={agreements.map((a) => ({ value: a.id, label: a.name }))}
        />
      )}
      <div className="grid gap-4 sm:grid-cols-3">
        {text('price_categories', t('fields.price_categories'), {
          hint: t('hints.priceCategories'),
        })}
        {text('spot_attributes', t('fields.spot_attributes'), { hint: t('hints.spotAttributes') })}
        {text('customer_tags', t('fields.customer_tags'), { hint: t('hints.customerTags') })}
        <Field id={`${idPrefix}-holidays`} label={t('fields.holidays')} error={err('holidays')}>
          <Select
            {...fieldAria(`${idPrefix}-holidays`, err('holidays'))}
            {...register('holidays' as Path<T>)}
          >
            {(['any', 'only', 'exclude'] as const).map((h) => (
              <option key={h} value={h}>
                {t(`holidays.${h}`)}
              </option>
            ))}
          </Select>
        </Field>
        {text('min_days', t('fields.min_days'), { numeric: true })}
        {text('max_days', t('fields.max_days'), { numeric: true })}
        {text('lead_min', t('fields.lead_min'), { numeric: true, hint: t('hints.leadTime') })}
        {text('lead_max', t('fields.lead_max'), { numeric: true })}
      </div>
      <details
        className="rounded-md border border-slate-200 p-3"
        open={occupancy.some((v) => v !== '' && v !== undefined)}
      >
        <summary className="cursor-pointer text-sm font-medium text-slate-800">
          {t('advanced')}
        </summary>
        <p className="mt-2 text-xs text-slate-600">{t('hints.occupancy')}</p>
        <div className="mt-2 grid gap-4 sm:grid-cols-3">
          {text('occupancy_min', t('fields.occupancy_min'), { numeric: true })}
          {text('occupancy_max', t('fields.occupancy_max'), { numeric: true })}
        </div>
      </details>
    </fieldset>
  );
}
