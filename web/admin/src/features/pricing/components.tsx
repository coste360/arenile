import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
  Button,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Separator,
} from '@arenile/shared/ui';
import {
  DateRangePicker,
  MultiSelect,
  type Option,
  OptionSelect,
} from '@arenile/shared/ui/patterns';
import { CalendarPlusIcon, XIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import {
  useFieldArray,
  useFormState,
  useWatch,
  type Control,
  type FieldValues,
  type Path,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { RatePeriod } from '../configuration/api/resources';
import { errorAt } from '../configuration/lib/aria';
import {
  EditorSection,
  EuroTextInput,
  MultiToggle,
  PercentTextInput,
  SingleToggle,
  TextRow,
} from './kit';
import {
  CHANNELS,
  FORMULAS,
  PERS,
  SLOTS,
  WEEKDAYS,
  type ActionType,
  type ConditionValues,
} from './rules.schema';

export { PricingNav, SeasonSelect } from './kit';

/** Generic field: label, control (render prop on the RHF field), hint, translated error. */
function Row<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  children,
}: {
  control: Control<T>;
  name: string;
  label: ReactNode;
  description?: ReactNode;
  className?: string;
  children: (field: {
    value: unknown;
    onChange: (v: unknown) => void;
    onBlur: () => void;
  }) => ReactNode;
}) {
  return (
    <FormField
      control={control}
      name={name as Path<T>}
      render={({ field }) => (
        <FormItem className={className}>
          <FormLabel>{label}</FormLabel>
          <FormControl>{children(field)}</FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

/** Action editor (type + the fields it needs), bound to the flat action form fields. */
export function ActionFields<T extends FieldValues>({
  control,
  allowed,
}: {
  control: Control<T>;
  allowed: readonly ActionType[];
}) {
  const { t } = useTranslation('pricing');
  const type = useWatch({ control, name: 'action_type' as Path<T> }) as ActionType;
  const numeric = { inputMode: 'numeric' as const, className: 'max-w-32' };
  return (
    <EditorSection title={t('fields.action')}>
      <div className="grid gap-4 sm:grid-cols-3">
        <Row control={control} name="action_type" label={t('fields.action')}>
          {(f) => (
            <OptionSelect
              value={f.value as string}
              onChange={f.onChange}
              options={allowed.map((a) => ({ value: a, label: t(`actions.${a}`) }))}
            />
          )}
        </Row>
        {(type === 'set' || type === 'add' || type === 'amount_off') && (
          <Row
            control={control}
            name="amount"
            label={t('fields.amount')}
            description={type === 'add' ? t('hints.signedAmount') : undefined}
          >
            {(f) => (
              <EuroTextInput
                value={f.value as string}
                onChange={f.onChange}
                onBlur={f.onBlur}
                allowNegative={type === 'add'}
              />
            )}
          </Row>
        )}
        {type === 'amount_off' && (
          <Row control={control} name="per" label={t('fields.per')}>
            {(f) => (
              <OptionSelect
                value={f.value as string}
                onChange={f.onChange}
                options={[
                  { value: '', label: t('per.default') },
                  ...PERS.map((p) => ({ value: p, label: t(`per.${p}`) })),
                ]}
              />
            )}
          </Row>
        )}
        {type === 'percent_off' && (
          <Row control={control} name="percent" label={t('fields.percent')}>
            {(f) => (
              <PercentTextInput value={f.value as string} onChange={f.onChange} onBlur={f.onBlur} />
            )}
          </Row>
        )}
        {type === 'multiply' && (
          <TextRow
            control={control}
            name={'factor' as Path<T>}
            label={t('fields.factor')}
            description={t('hints.factor')}
            inputProps={{ inputMode: 'decimal', className: 'max-w-32' }}
          />
        )}
        {type === 'free_days' && (
          <>
            <TextRow
              control={control}
              name={'every' as Path<T>}
              label={t('fields.every')}
              inputProps={numeric}
            />
            <TextRow
              control={control}
              name={'free' as Path<T>}
              label={t('fields.free')}
              inputProps={numeric}
            />
          </>
        )}
      </div>
    </EditorSection>
  );
}

/** Conditions editor (spec 8.2): every dimension optional, AND between them. */
export function ConditionFields<T extends FieldValues>({
  control,
  periods,
  zones,
  agreements,
}: {
  control: Control<T>;
  periods: RatePeriod[];
  zones: { id: string; name: string }[];
  /** Offered as a condition only for rules (not inside promo codes or agreements). */
  agreements?: { id: string; name: string }[];
}) {
  const { t } = useTranslation('pricing');
  // The forms extend ConditionValues; the editor only touches those fields.
  const c = control as unknown as Control<ConditionValues>;
  const ranges = useFieldArray({ control: c, name: 'date_ranges' });
  const occupancy = useWatch({ control: c, name: ['occupancy_min', 'occupancy_max'] });
  const { errors } = useFormState({ control: c });
  const numeric = { inputMode: 'numeric' as const, className: 'max-w-32' };
  const multi = (name: keyof ConditionValues, label: string, options: Option[]) => (
    <Row control={c} name={name} label={label}>
      {(f) => <MultiSelect value={f.value as string[]} onChange={f.onChange} options={options} />}
    </Row>
  );
  const toggles = (name: keyof ConditionValues, label: string, options: Option[]) => (
    <Row control={c} name={name} label={label}>
      {(f) => (
        <MultiToggle
          value={f.value as string[]}
          onChange={f.onChange}
          options={options}
          label={label}
        />
      )}
    </Row>
  );
  return (
    <EditorSection title={t('fields.conditions')} description={t('hints.conditions')}>
      <div className="space-y-5 rounded-lg border bg-muted/20 p-4">
        <div className="grid gap-4 sm:grid-cols-2">
          {periods.length > 0 &&
            multi(
              'rate_periods',
              t('fields.rate_periods'),
              periods.map((p) => ({ value: p.code, label: p.code })),
            )}
          {multi(
            'formulas',
            t('fields.formulas'),
            FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) })),
          )}
          {zones.length > 0 &&
            multi(
              'zone_ids',
              t('fields.zones'),
              zones.map((z) => ({ value: z.id, label: z.name })),
            )}
          {agreements &&
            agreements.length > 0 &&
            multi(
              'agreement_ids',
              t('fields.agreements'),
              agreements.map((a) => ({ value: a.id, label: a.name })),
            )}
        </div>

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">{t('fields.date_ranges')}</legend>
          {ranges.fields.length === 0 && (
            <p className="text-sm text-muted-foreground">{t('hints.noDateRanges')}</p>
          )}
          {ranges.fields.map((f, i) => {
            const error =
              errorAt(errors, `date_ranges.${i}.from`) ?? errorAt(errors, `date_ranges.${i}.to`);
            return (
              <FormField
                key={f.id}
                control={c}
                name={`date_ranges.${i}`}
                render={({ field }) => (
                  <div className="space-y-1">
                    <div className="flex items-center gap-1">
                      <DateRangePicker
                        aria-label={t('fields.date_ranges')}
                        aria-invalid={error ? true : undefined}
                        value={
                          field.value.from
                            ? { from: field.value.from, to: field.value.to || undefined }
                            : undefined
                        }
                        onChange={(r) => field.onChange({ from: r?.from ?? '', to: r?.to ?? '' })}
                      />
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('removeDateRange', { n: i + 1 })}
                        onClick={() => ranges.remove(i)}
                      >
                        <XIcon aria-hidden="true" />
                      </Button>
                    </div>
                    {error?.message && (
                      <p className="text-sm text-destructive">
                        {error.type === 'server' ? error.message : t(error.message)}
                      </p>
                    )}
                  </div>
                )}
              />
            );
          })}
          <Button variant="outline" size="sm" onClick={() => ranges.append({ from: '', to: '' })}>
            <CalendarPlusIcon aria-hidden="true" />
            {t('addDateRange')}
          </Button>
        </fieldset>

        <Separator />
        {toggles(
          'weekdays',
          t('fields.weekdays'),
          WEEKDAYS.map((d) => ({ value: d, label: t(`weekdaysShort.${d}`) })),
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          {toggles(
            'slots',
            t('fields.slots'),
            SLOTS.map((s) => ({ value: s, label: t(`slots.${s}`) })),
          )}
          {toggles(
            'channels',
            t('fields.channels'),
            CHANNELS.map((ch) => ({ value: ch, label: t(`channels.${ch}`) })),
          )}
        </div>
        <Row control={c} name="holidays" label={t('fields.holidays')}>
          {(f) => (
            <SingleToggle
              value={f.value as string}
              onChange={f.onChange}
              label={t('fields.holidays')}
              options={(['any', 'only', 'exclude'] as const).map((h) => ({
                value: h,
                label: t(`holidays.${h}`),
              }))}
            />
          )}
        </Row>

        <Separator />
        <div className="grid gap-4 sm:grid-cols-3">
          <TextRow
            control={c}
            name="price_categories"
            label={t('fields.price_categories')}
            description={t('hints.priceCategories')}
          />
          <TextRow
            control={c}
            name="spot_attributes"
            label={t('fields.spot_attributes')}
            description={t('hints.spotAttributes')}
          />
          <TextRow
            control={c}
            name="customer_tags"
            label={t('fields.customer_tags')}
            description={t('hints.customerTags')}
          />
        </div>
        <div className="grid gap-4 sm:grid-cols-4">
          <TextRow control={c} name="min_days" label={t('fields.min_days')} inputProps={numeric} />
          <TextRow control={c} name="max_days" label={t('fields.max_days')} inputProps={numeric} />
          <TextRow
            control={c}
            name="lead_min"
            label={t('fields.lead_min')}
            description={t('hints.leadTime')}
            inputProps={numeric}
          />
          <TextRow control={c} name="lead_max" label={t('fields.lead_max')} inputProps={numeric} />
        </div>

        <Accordion
          type="single"
          collapsible
          defaultValue={occupancy.some((v) => v !== '' && v !== undefined) ? 'advanced' : undefined}
          className="rounded-lg border bg-background px-3"
        >
          <AccordionItem value="advanced" className="border-b-0">
            <AccordionTrigger>{t('advanced')}</AccordionTrigger>
            <AccordionContent className="space-y-3">
              <p className="text-sm text-muted-foreground">{t('hints.occupancy')}</p>
              <div className="grid gap-4 sm:grid-cols-3">
                <TextRow
                  control={c}
                  name="occupancy_min"
                  label={t('fields.occupancy_min')}
                  inputProps={numeric}
                />
                <TextRow
                  control={c}
                  name="occupancy_max"
                  label={t('fields.occupancy_max')}
                  inputProps={numeric}
                />
              </div>
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      </div>
    </EditorSection>
  );
}
