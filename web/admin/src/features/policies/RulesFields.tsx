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
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
  Switch,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  MultiSelect,
  OptionSelect,
  RowActions,
  type Option,
} from '@arenile/shared/ui/patterns';
import {
  ArrowDownIcon,
  ArrowDownWideNarrowIcon,
  ArrowUpIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react';
import { useId, useMemo, type ComponentProps, type ReactNode } from 'react';
import {
  useFieldArray,
  useWatch,
  type Control,
  type FieldArrayWithId,
  type Path,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { EuroTextInput, TextRow } from '../pricing/kit';
import { nextTier, sortTiers, type RulesValues } from './policy.schema';

const FORMULAS = ['half_day', 'day', 'multi_day', 'week', 'fortnight', 'month', 'season'] as const;
const SECTIONS = ['timing', 'duration', 'payment', 'cancellation', 'other'] as const;

function ChoiceRow({
  control,
  name,
  label,
  options,
}: {
  control: Control<RulesValues>;
  name: Path<RulesValues>;
  label: string;
  options: Option[];
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <OptionSelect
              value={field.value as string}
              onChange={field.onChange}
              options={options}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

/** Integer percent input with a "%" addon. */
function PercentIntRow({
  control,
  name,
  label,
}: {
  control: Control<RulesValues>;
  name: Path<RulesValues>;
  label: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <InputGroup className="max-w-36">
            <FormControl>
              <InputGroupInput
                inputMode="numeric"
                className="text-right tabular-nums"
                {...field}
                value={field.value as string}
              />
            </FormControl>
            <InputGroupAddon align="inline-end">
              <InputGroupText aria-hidden="true">%</InputGroupText>
            </InputGroupAddon>
          </InputGroup>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function MoneyRow({
  control,
  name,
  label,
}: {
  control: Control<RulesValues>;
  name: Path<RulesValues>;
  label: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <EuroTextInput
              value={field.value as string}
              onChange={field.onChange}
              onBlur={field.onBlur}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

/**
 * Editor of the booking rules (spec 9), one Accordion item per section. It is
 * used inside a shadcn <Form> whose values extend RulesValues; "inherit" /
 * empty leaves a rule to the default policy.
 */
export function RulesFields({
  control,
  isDefault,
  before,
}: {
  // The forms extend RulesValues; the editor only touches those fields.
  control: Control<RulesValues>;
  /** Editing the default policy: "inherit" means the built-in default. */
  isDefault?: boolean;
  /** Extra Accordion items rendered before the rule sections (e.g. the scope). */
  before?: ReactNode;
}) {
  const { t } = useTranslation('policies');
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
  const numeric = { inputMode: 'numeric' as const, className: 'max-w-32' };
  const inherit = isDefault ? t('inheritBuiltin') : t('inherit');
  const triOptions = [
    { value: 'inherit', label: inherit },
    { value: 'yes', label: t('yes') },
    { value: 'no', label: t('no') },
  ];
  const section = (id: (typeof SECTIONS)[number], children: ReactNode) => (
    <AccordionItem value={id}>
      <AccordionTrigger className="text-base">{t(`sections.${id}`)}</AccordionTrigger>
      <AccordionContent className="space-y-4 px-1 pt-1">{children}</AccordionContent>
    </AccordionItem>
  );

  return (
    <Accordion
      type="multiple"
      defaultValue={[...(before ? ['scope'] : []), ...SECTIONS]}
      className="rounded-lg border px-4"
    >
      {before}
      {section(
        'timing',
        <div className="grid gap-4 sm:grid-cols-3">
          <TextRow
            control={control}
            name="min_lead_days"
            label={t('fields.min_lead_days')}
            inputProps={numeric}
          />
          <TextRow
            control={control}
            name="cutoff_time"
            label={t('fields.cutoff_time')}
            description={t('hints.cutoff')}
            inputProps={{ type: 'time', className: 'max-w-32' }}
          />
          <TextRow
            control={control}
            name="max_lead_time_days"
            label={t('fields.max_lead_time_days')}
            inputProps={numeric}
          />
          <ChoiceRow
            control={control}
            name="same_day"
            label={t('fields.same_day')}
            options={triOptions}
          />
          {sameDay === 'yes' && (
            <TextRow
              control={control}
              name="same_day_until"
              label={t('fields.same_day_until')}
              inputProps={{ type: 'time', className: 'max-w-32' }}
            />
          )}
        </div>,
      )}
      {section(
        'duration',
        <>
          <FormField
            control={control}
            name="allowed_formulas"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('fields.allowed_formulas')}</FormLabel>
                <FormControl>
                  <MultiSelect
                    value={field.value}
                    onChange={field.onChange}
                    placeholder={inherit}
                    options={FORMULAS.map((f) => ({ value: f, label: t(`pricing:formulas.${f}`) }))}
                  />
                </FormControl>
                <FormDescription>{t('hints.allFormulas')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className="grid gap-4 sm:grid-cols-3">
            <TextRow
              control={control}
              name="min_days"
              label={t('fields.min_days')}
              inputProps={numeric}
            />
            <TextRow
              control={control}
              name="max_days"
              label={t('fields.max_days')}
              inputProps={numeric}
            />
            <ChoiceRow
              control={control}
              name="fixed_start_weekday"
              label={t('fields.fixed_start_weekday')}
              options={[
                { value: '', label: inherit },
                { value: '0', label: t('anyWeekday') },
                ...['1', '2', '3', '4', '5', '6', '7'].map((d) => ({
                  value: d,
                  label: t(`pricing:weekdaysShort.${d}`),
                })),
              ]}
            />
          </div>
        </>,
      )}
      {section(
        'payment',
        <div className="grid gap-4 sm:grid-cols-3">
          <ChoiceRow
            control={control}
            name="payment_mode"
            label={t('fields.payment_mode')}
            options={[
              { value: '', label: inherit },
              { value: 'full', label: t('paymentModes.full') },
              { value: 'deposit', label: t('paymentModes.deposit') },
              { value: 'none', label: t('paymentModes.none') },
            ]}
          />
          {paymentMode === 'deposit' && (
            <>
              <ChoiceRow
                control={control}
                name="deposit_type"
                label={t('fields.deposit_type')}
                options={[
                  { value: 'percent', label: t('depositTypes.percent') },
                  { value: 'amount', label: t('depositTypes.amount') },
                ]}
              />
              {depositType === 'amount' ? (
                <MoneyRow
                  control={control}
                  name="deposit_value"
                  label={t('fields.deposit_amount')}
                />
              ) : (
                <PercentIntRow
                  control={control}
                  name="deposit_value"
                  label={t('fields.deposit_percent')}
                />
              )}
            </>
          )}
          {paymentMode !== '' && paymentMode !== 'full' && (
            <>
              <ChoiceRow
                control={control}
                name="balance_due"
                label={t('fields.balance_due')}
                options={[
                  { value: '', label: inherit },
                  { value: 'on_arrival', label: t('balance.on_arrival') },
                  { value: 'days_before', label: t('balance.days_before') },
                ]}
              />
              {balanceDue === 'days_before' && (
                <TextRow
                  control={control}
                  name="balance_days"
                  label={t('fields.balance_days')}
                  inputProps={numeric}
                />
              )}
            </>
          )}
        </div>,
      )}
      {section(
        'cancellation',
        <>
          <FormField
            control={control}
            name="cancellation_set"
            render={({ field }) => (
              <FormItem className="flex flex-row items-center justify-between gap-4 rounded-lg border p-3">
                <FormLabel>{t('fields.cancellation_set')}</FormLabel>
                <FormControl>
                  <Switch checked={field.value} onCheckedChange={field.onChange} />
                </FormControl>
              </FormItem>
            )}
          />
          {cancellationSet && <TiersTable control={control} />}
          <div className="grid gap-4 sm:grid-cols-3">
            <TextRow
              control={control}
              name="modification_until"
              label={t('fields.modification_until')}
              inputProps={numeric}
            />
            <MoneyRow
              control={control}
              name="modification_fee"
              label={t('fields.modification_fee')}
            />
            <ChoiceRow
              control={control}
              name="weather"
              label={t('fields.weather')}
              options={[
                { value: 'inherit', label: inherit },
                { value: 'off', label: t('weather.off') },
                { value: 'refund', label: t('weather.refund') },
                { value: 'credit', label: t('weather.credit') },
              ]}
            />
          </div>
        </>,
      )}
      {section(
        'other',
        <div className="grid gap-4 sm:grid-cols-3">
          <ChoiceRow
            control={control}
            name="approval_required"
            label={t('fields.approval_required')}
            options={triOptions}
          />
          <TextRow
            control={control}
            name="max_spots_per_booking"
            label={t('fields.max_spots_per_booking')}
            inputProps={numeric}
          />
          <ChoiceRow
            control={control}
            name="allow_spot_change"
            label={t('fields.allow_spot_change')}
            options={triOptions}
          />
          <ChoiceRow
            control={control}
            name="customer_can_release"
            label={t('fields.customer_can_release')}
            options={triOptions}
          />
          <ChoiceRow
            control={control}
            name="release_credit"
            label={t('fields.release_credit')}
            options={[
              { value: 'inherit', label: inherit },
              { value: 'off', label: t('no') },
              { value: 'on', label: t('yes') },
            ]}
          />
          {releaseCredit === 'on' && (
            <PercentIntRow
              control={control}
              name="release_credit_percent"
              label={t('fields.release_credit_percent')}
            />
          )}
        </div>,
      )}
    </Accordion>
  );
}

/** Editable table of the cancellation tiers (days before → refund %, as refund or credit). */
function TiersTable({ control }: { control: Control<RulesValues> }) {
  const { t } = useTranslation('policies');
  const tiers = useFieldArray({ control, name: 'cancellation' });
  const tierValues = useWatch({ control, name: 'cancellation' });
  const captionId = useId();
  const { move, remove } = tiers;
  const count = tiers.fields.length;
  // Memoized: flexRender mounts cell functions as components, so new column
  // objects on every render would remount the inputs and drop their focus.
  const columns = useMemo<ColumnDef<FieldArrayWithId<RulesValues, 'cancellation'>>[]>(
    () => [
      {
        id: 'until_days_before',
        meta: { label: t('fields.until_days_before') },
        header: t('fields.until_days_before'),
        cell: ({ row }) => (
          <FormField
            control={control}
            name={`cancellation.${row.index}.until_days_before`}
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <InputGroupLike
                    ariaLabel={t('fields.until_days_before')}
                    suffix={t('daysShort')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
      },
      {
        id: 'refund_percent',
        meta: { label: t('fields.refund_percent') },
        header: t('fields.refund_percent'),
        cell: ({ row }) => (
          <FormField
            control={control}
            name={`cancellation.${row.index}.refund_percent`}
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <InputGroupLike ariaLabel={t('fields.refund_percent')} suffix="%" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
      },
      {
        id: 'as',
        meta: { label: t('fields.refund_as') },
        header: t('fields.refund_as'),
        cell: ({ row }) => (
          <FormField
            control={control}
            name={`cancellation.${row.index}.as`}
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <OptionSelect
                    aria-label={t('fields.refund_as')}
                    value={field.value}
                    onChange={field.onChange}
                    className="min-w-32"
                    options={[
                      { value: 'refund', label: t('refundAs.refund') },
                      { value: 'credit', label: t('refundAs.credit') },
                    ]}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        ),
      },
      {
        id: 'actions',
        enableHiding: false,
        header: () => <span className="sr-only">{t('actions')}</span>,
        cell: ({ row }) => {
          const i = row.index;
          return (
            <RowActions>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('moveTierUp', { n: i + 1 })}
                disabled={i === 0}
                onClick={() => move(i, i - 1)}
              >
                <ArrowUpIcon aria-hidden="true" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('moveTierDown', { n: i + 1 })}
                disabled={i === count - 1}
                onClick={() => move(i, i + 1)}
              >
                <ArrowDownIcon aria-hidden="true" />
              </Button>
              {/* Same look as the shared DeleteButton (no confirmation: the form is not saved yet). */}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('removeTier', { n: i + 1 })}
                className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                onClick={() => remove(i)}
              >
                <Trash2Icon aria-hidden="true" />
              </Button>
            </RowActions>
          );
        },
      },
    ],
    [control, t, count, move, remove],
  );
  return (
    <div className="space-y-3">
      {tiers.fields.length === 0 ? (
        <p className="rounded-lg border border-dashed px-4 py-3 text-sm text-muted-foreground">
          {t('noRefund')}
        </p>
      ) : (
        <DataTable
          caption={t('tiersCaption')}
          columns={columns}
          data={tiers.fields}
          getRowId={(f) => f.id}
          pageSize={0}
          columnToggle={false}
        />
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => tiers.append(nextTier(tierValues ?? []))}
        >
          <PlusIcon aria-hidden="true" />
          {t('addTier')}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={tiers.fields.length < 2}
          onClick={() => tiers.replace(sortTiers(tierValues ?? []))}
        >
          <ArrowDownWideNarrowIcon aria-hidden="true" />
          {t('sortTiers')}
        </Button>
      </div>
      <p id={captionId} className="text-xs text-muted-foreground">
        {t('hints.tiers')}
      </p>
    </div>
  );
}

/** Numeric cell input with a unit addon; FormControl props land on the input. */
function InputGroupLike({
  ariaLabel,
  suffix,
  value,
  ...props
}: Omit<ComponentProps<'input'>, 'value'> & {
  ariaLabel: string;
  suffix: string;
  value: string;
}) {
  return (
    <InputGroup className="max-w-32">
      <InputGroupInput
        {...props}
        value={value}
        aria-label={ariaLabel}
        inputMode="numeric"
        className="text-right tabular-nums"
      />
      <InputGroupAddon align="inline-end">
        <InputGroupText aria-hidden="true">{suffix}</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  );
}
