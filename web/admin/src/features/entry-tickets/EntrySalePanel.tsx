import { ApiProblem, mapProblemToForm } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  EmptyState,
  ExtraList,
  ExtraRow,
  FieldRow,
  MoneyInput,
  QuantityStepper,
  QueryState,
  SectionCard,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { BanknoteIcon, CircleCheckIcon, CreditCardIcon, TicketIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { describeProblem, type ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { localize } from '../configuration/lib/i18nText';
import { newIdempotencyKey } from '../configuration/lib/validation';
import { useDebounced } from '../customers/lib';
import { todayIn } from '../operations/lib/dates';
import {
  useCreateEntrySale,
  useEntrySaleQuote,
  useEntryTicketTypes,
  type DailySaleResult,
  type EntrySaleQuoteRequest,
} from './api';
import {
  EMPTY_ENTRY_SALE,
  entrySaleSchema,
  SALE_FIELD_ALIASES,
  toEntrySaleCreate,
  toEntrySaleQuote,
  type EntrySaleValues,
} from './entrySale.schema';
import { changeDue, MAX_PER_TYPE, sellableTypes, totalQuantity } from './lib';

export interface EntrySalePanelProps {
  /** Day of the sale (YYYY-MM-DD); default today in the establishment time zone. */
  date?: string;
  /** Called after a sale (e.g. to open the ticket print). */
  onSold?: (result: DailySaleResult) => void;
}

/**
 * "Vendita rapida" → Ingressi (spec 14.3, ADR 0020): quantity per ticket type
 * (Adulto, Ridotto, Bambino, ...), slot, price and remaining daily capacity
 * from the server, cash or card (manual driver; cash needs an open cash
 * session) and an optional name. One numbered ticket with a QR for the gate.
 */
export function EntrySalePanel({ date: dateProp, onSold }: EntrySalePanelProps) {
  const { t } = useTranslation('entryTickets');
  const ctx = useConfigContext();
  const date = dateProp ?? todayIn(ctx.establishment?.timezone);
  const types = useEntryTicketTypes(ctx.establishmentId);
  const sellable = useMemo(() => sellableTypes(types.data?.items ?? []), [types.data]);
  const loc = (text: Record<string, string> | undefined) =>
    localize(text, ctx.uiLocale, ctx.defaultLocale);

  const form = useForm<EntrySaleValues>({
    resolver: zodResolver(entrySaleSchema),
    defaultValues: EMPTY_ENTRY_SALE,
  });
  const [slots, quantities, method, tendered] = useWatch({
    control: form.control,
    name: ['slots', 'quantities', 'method', 'tendered'],
  });

  // Live quote, debounced; compared by value (the watched record is a new object each render).
  const quoteBody = toEntrySaleQuote({ slots, quantities }, sellable, date);
  const quoteKey = quoteBody ? JSON.stringify(quoteBody) : '';
  const debouncedKey = useDebounced(quoteKey, 250);
  const quoteQuery = useEntrySaleQuote(
    ctx.establishmentId,
    debouncedKey ? (JSON.parse(debouncedKey) as EntrySaleQuoteRequest) : null,
  );
  const quote = quoteKey !== '' ? (quoteQuery.data ?? null) : null;
  const quoteError = quoteKey !== '' ? quoteQuery.error : null;
  const quoteFresh =
    quoteKey !== '' && debouncedKey === quoteKey && quoteQuery.isSuccess && !quoteQuery.isFetching;

  // One Idempotency-Key per sale attempt: retries of the same sale reuse it.
  const [saleKey, setSaleKey] = useState(newIdempotencyKey);
  const sale = useCreateEntrySale(ctx.establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const [sold, setSold] = useState<DailySaleResult | null>(null);

  const submit = form.handleSubmit(async (values) => {
    setFailure(null);
    try {
      const result = await sale.mutateAsync({
        body: toEntrySaleCreate(values, sellable, date, quote?.total),
        idempotencyKey: saleKey,
      });
      setSaleKey(newIdempotencyKey());
      setSold(result);
      form.reset(EMPTY_ENTRY_SALE);
      onSold?.(result);
    } catch (err) {
      const mapped = mapProblemToForm(err, form.setError, {
        formKeys: Object.keys(values),
        aliases: SALE_FIELD_ALIASES,
      });
      if (err instanceof ApiProblem && err.type === 'price_changed') {
        // The price moved since the quote: show the new total before retrying.
        setSaleKey(newIdempotencyKey());
        void quoteQuery.refetch();
        setFailure({ title: t('sale.priceChanged'), details: [], reload: false });
        return;
      }
      setFailure(describeProblem(err, t, { fieldErrors: mapped.errors.map((e) => e.text) }));
    }
  });

  if (types.error) {
    return (
      <QueryState isLoading={false} error={types.error} onRetry={() => void types.refetch()} />
    );
  }
  if (types.isPending) {
    return <Skeleton className="h-64 w-full" />;
  }
  if (sellable.length === 0) {
    return (
      <EmptyState icon={TicketIcon} title={t('sale.noTypes')} description={t('sale.noTypesHint')} />
    );
  }

  const count = totalQuantity(quantities);
  const total = quote?.total ?? 0;
  const change = method === 'cash' ? changeDue(total, tendered) : null;
  const unavailable = quote !== null && !quote.available;
  // `quantities` is a record, so RHF types its error as nested: read the root message.
  const qtyMessage: unknown = (
    form.formState.errors.quantities as { message?: unknown } | undefined
  )?.message;
  const qtyError = typeof qtyMessage === 'string' ? qtyMessage : undefined;
  const canSell = count > 0 && quoteFresh && quote !== null && quote.available && !sale.isPending;

  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void submit(e)} className="space-y-6">
        {sold && (
          <Alert variant="success" role="status">
            <CircleCheckIcon aria-hidden="true" />
            <AlertTitle>{t('sale.sold', { number: sold.ticket.number_label })}</AlertTitle>
            <AlertDescription>
              {sold.change_amount != null && sold.change_amount > 0
                ? t('sale.soldChange', { amount: formatCents(sold.change_amount) })
                : t('sale.soldHint')}
            </AlertDescription>
          </Alert>
        )}
        <ProblemAlert problem={failure} />
        <SectionCard
          title={t('sale.title')}
          description={t('sale.forDate', { date: formatIsoDate(date) })}
        >
          <div className="space-y-4">
            <SegmentedControl
              label={t('sale.slots')}
              collapseOnNarrow
              value={slots}
              onValueChange={(v) =>
                form.setValue('slots', v as EntrySaleValues['slots'], { shouldDirty: true })
              }
              options={[
                { value: 'FULL', label: t('slots.FULL') },
                { value: 'AM', label: t('slots.AM') },
                { value: 'PM', label: t('slots.PM') },
              ]}
            />
            <ExtraList title={t('sale.types')}>
              {sellable.map((type) => {
                const name = loc(type.name) || type.code;
                return (
                  <ExtraRow key={type.id} name={name} detail={loc(type.description) || undefined}>
                    <QuantityStepper
                      label={name}
                      value={quantities[type.id] ?? 0}
                      min={0}
                      max={MAX_PER_TYPE}
                      onChange={(n) => {
                        form.setValue(
                          'quantities',
                          { ...form.getValues('quantities'), [type.id]: n },
                          { shouldDirty: true },
                        );
                        if (n > 0) form.clearErrors('quantities');
                        setSold(null);
                      }}
                    />
                  </ExtraRow>
                );
              })}
            </ExtraList>
            {qtyError && (
              <p role="alert" className="text-sm text-destructive">
                {t(qtyError)}
              </p>
            )}
          </div>
        </SectionCard>

        <SectionCard title={t('sale.payment')}>
          <div className="space-y-4">
            <SegmentedControl
              label={t('sale.method')}
              value={method}
              onValueChange={(v) =>
                form.setValue('method', v as EntrySaleValues['method'], { shouldDirty: true })
              }
              options={[
                { value: 'cash', label: t('sale.cash'), icon: BanknoteIcon },
                { value: 'card', label: t('sale.card'), icon: CreditCardIcon },
              ]}
            />
            {method === 'cash' && (
              <div className="grid gap-4 sm:grid-cols-2">
                <FieldRow
                  control={form.control}
                  name="tendered"
                  label={t('sale.tendered')}
                  description={t('sale.cashHint')}
                  optional
                >
                  {(field) => (
                    <MoneyInput
                      value={field.value}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                    />
                  )}
                </FieldRow>
                {change != null && (
                  <p className="self-center text-sm" aria-live="polite">
                    {t('sale.change')}{' '}
                    <span className="font-semibold tabular-nums">{formatCents(change)}</span>
                  </p>
                )}
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <FieldRow
                control={form.control}
                name="first_name"
                label={t('sale.firstName')}
                optional
              >
                {(field) => <Input autoComplete="off" {...field} />}
              </FieldRow>
              <FieldRow control={form.control} name="last_name" label={t('sale.lastName')} optional>
                {(field) => <Input autoComplete="off" {...field} />}
              </FieldRow>
            </div>
          </div>
        </SectionCard>

        {unavailable && (
          <Alert variant="warning" role="status">
            <AlertDescription>{t('sale.full')}</AlertDescription>
          </Alert>
        )}
        {quoteError != null && <ProblemAlert problem={describeProblem(quoteError, t)} />}
        <div className="flex flex-col gap-3 rounded-lg border bg-card p-4 sm:flex-row sm:items-center sm:justify-between">
          <div aria-live="polite">
            <p className="text-sm text-muted-foreground">
              {t('sale.summary', { count })}
              {quote && quote.available && ` · ${t('sale.remaining', { count: quote.remaining })}`}
            </p>
            <p className="text-2xl font-semibold tabular-nums">
              {count > 0 && quote ? formatCents(total) : '—'}
            </p>
          </div>
          <Button type="submit" disabled={!canSell} className="w-full sm:w-auto">
            {(sale.isPending || (count > 0 && !quoteFresh)) && <Spinner />}
            {count > 0 && quote
              ? t('sale.submit', { amount: formatCents(total) })
              : t('sale.submitEmpty')}
          </Button>
        </div>
      </form>
    </Form>
  );
}
