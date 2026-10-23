import { formatCents } from '@arenile/shared/lib/money';
import { Form } from '@arenile/shared/ui/form';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Textarea } from '@arenile/shared/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  DatePicker,
  FieldRow,
  FormActions,
  FormDialog,
  OptionSelect,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { InfoIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useAvailability, useBookingQuote } from '../bookings/api';
import { PolicySummary } from '../bookings/QuotePreview';
import { describeProblem, type ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { compareSpots, type EstablishmentSpots } from '../bookings/spots';
import { packs as packsResource, type Season } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { newIdempotencyKey } from '../configuration/lib/validation';
import type { Customer } from '../customers/api';
import { CustomerPicker } from '../customers/CustomerPicker';
import { useDebounced } from '../customers/lib';
import { useCreateSeasonPass } from './api';
import {
  createPassSchema,
  EMPTY_PASS,
  toPassCreate,
  toPassQuote,
  type CreatePassValues,
} from './pass.schema';

export interface CreatePassDialogProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  seasons: Season[];
  defaultSeasonId?: string;
  inventory: EstablishmentSpots;
  onCreated?: (passId: string) => void;
}

/**
 * New season pass (spec 7.7): customer, season, spot (only those free for the
 * whole season are offered), pack and slots. The price comes from the pricing
 * engine (formula season) and is shown before saving.
 */
export function CreatePassDialog({ open, onOpenChange, ...rest }: CreatePassDialogProps) {
  const { t } = useTranslation('passes');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('create.title')}
      description={t('create.description')}
      wide
    >
      {open && <CreatePassForm {...rest} onDone={() => onOpenChange(false)} />}
    </FormDialog>
  );
}

function CreatePassForm({
  establishmentId,
  seasons,
  defaultSeasonId,
  inventory,
  onCreated,
  onDone,
}: Omit<CreatePassDialogProps, 'open' | 'onOpenChange'> & { onDone: () => void }) {
  const { t, i18n } = useTranslation('passes');
  const locale = i18n.language || 'it';
  const create = useCreateSeasonPass(establishmentId);
  const packList = packsResource.useList(establishmentId);
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const [idemKey] = useState(newIdempotencyKey);
  const form = useForm<CreatePassValues>({
    resolver: zodResolver(createPassSchema),
    defaultValues: { ...EMPTY_PASS, season_id: defaultSeasonId ?? seasons[0]?.id ?? '' },
  });
  const v = useWatch({ control: form.control }) as CreatePassValues;
  const season = seasons.find((s) => s.id === v.season_id);
  const packs = (packList.data ?? [])
    .filter(
      (p) => p.active && (!p.allowed_formulas?.length || p.allowed_formulas.includes('season')),
    )
    .sort((a, b) => a.sort_order - b.sort_order);
  const from = v.starts_on || season?.opens_on;
  const to = v.ends_on || season?.closes_on;
  const availability = useAvailability(
    establishmentId,
    from && to ? { from, to, slots: v.slots, pack_id: v.pack_id || undefined } : null,
  );
  const free = new Set(
    (availability.data?.zones ?? []).flatMap((z) =>
      (z.spots ?? []).filter((s) => s.status === 'available').map((s) => s.spot_id),
    ),
  );
  const spotOptions = inventory.spots
    .filter((s) => free.has(s.id))
    .sort(
      (a, b) =>
        inventory.zoneName(a.zone_id).localeCompare(inventory.zoneName(b.zone_id)) ||
        compareSpots(a, b),
    )
    .map((s) => ({ value: s.id, label: `${s.label} · ${inventory.zoneName(s.zone_id)}` }));

  const quoteReq = toPassQuote(v, season, inventory.spotById.get(v.spot_id)?.zone_id);
  const quoteBody = useDebounced(quoteReq ? JSON.stringify(quoteReq) : null, 300);
  const quote = useBookingQuote(establishmentId, quoteBody ? JSON.parse(quoteBody) : null);

  const onSubmit = form.handleSubmit(async (values) => {
    setFailure(null);
    try {
      const created = await create.mutateAsync({
        body: toPassCreate(values),
        idempotencyKey: idemKey,
      });
      toast.success(t('create.done', { code: created.pass.code }));
      onCreated?.(created.pass.id);
      onDone();
    } catch (err) {
      setFailure(
        describeProblem(err, t, {
          resourceLabel: (g) => inventory.spotById.get(g.resourceId)?.label,
        }),
      );
    }
  });

  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <FieldRow control={form.control} name="customer_id" label={t('fields.customer')}>
          {(field) => (
            <CustomerPicker
              establishmentId={establishmentId}
              value={customer}
              invalid={!!form.formState.errors.customer_id}
              onChange={(c) => {
                setCustomer(c);
                field.onChange(c?.id ?? '');
              }}
            />
          )}
        </FieldRow>
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="season_id" label={t('fields.season')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={(id) => {
                  field.onChange(id);
                  form.setValue('spot_id', '');
                }}
                options={seasons.map((s) => ({ value: s.id, label: String(s.year) }))}
              />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="pack_id" label={t('fields.pack')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                placeholder={t('fields.packPlaceholder')}
                options={packs.map((p) => ({
                  value: p.id,
                  label: localize(p.name, locale, 'it') || p.code,
                }))}
              />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="slots" label={t('fields.slots')}>
            {(field) => (
              <ToggleGroup
                type="single"
                variant="outline"
                value={field.value}
                onValueChange={(s) => s && field.onChange(s)}
              >
                {(['FULL', 'AM', 'PM'] as const).map((s) => (
                  <ToggleGroupItem key={s} value={s}>
                    {t(`bookings:slots.${s}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            )}
          </FieldRow>
          <FieldRow control={form.control} name="spot_id" label={t('fields.spot')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                options={spotOptions}
                disabled={!season || spotOptions.length === 0}
                placeholder={
                  availability.isFetching
                    ? t('fields.spotLoading')
                    : spotOptions.length
                      ? t('fields.spotPick')
                      : t('fields.spotNone')
                }
              />
            )}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="starts_on"
            label={t('fields.startsOn')}
            description={t('fields.datesHelp')}
            optional
          >
            {(field) => (
              <DatePicker
                value={field.value}
                onChange={(d) => field.onChange(d ?? '')}
                min={season?.opens_on}
                max={season?.closes_on}
                clearable
              />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="ends_on" label={t('fields.endsOn')} optional>
            {(field) => (
              <DatePicker
                value={field.value}
                onChange={(d) => field.onChange(d ?? '')}
                min={season?.opens_on}
                max={season?.closes_on}
                clearable
              />
            )}
          </FieldRow>
        </div>
        <section
          aria-label={t('create.price')}
          aria-live="polite"
          className="space-y-3 rounded-lg border p-3"
        >
          {quote.data ? (
            <>
              <div className="flex items-end justify-between gap-2">
                <span className="text-sm text-muted-foreground">{t('create.price')}</span>
                <span className="text-xl font-semibold tabular-nums" data-testid="pass-price">
                  {formatCents(quote.data.breakdown.total)}
                </span>
              </div>
              {quote.data.deposit_due > 0 && (
                <p className="text-sm text-muted-foreground">
                  {t('create.deposit', { amount: formatCents(quote.data.deposit_due) })}
                </p>
              )}
              <PolicySummary rules={quote.data.policy.rules} />
            </>
          ) : quote.error ? (
            <p className="text-sm text-destructive">{describeProblem(quote.error, t).title}</p>
          ) : quote.isFetching ? (
            <Skeleton className="h-8 w-40" />
          ) : (
            <p className="text-sm text-muted-foreground">{t('create.priceHint')}</p>
          )}
          <p className="flex items-start gap-2 text-xs text-muted-foreground">
            <InfoIcon aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
            {t('create.overrideHint')}
          </p>
        </section>
        <FieldRow control={form.control} name="notes_internal" label={t('fields.notes')} optional>
          {(field) => <Textarea rows={2} {...field} />}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={create.isPending}
          submitLabel={t('create.submit')}
        />
      </form>
    </Form>
  );
}
