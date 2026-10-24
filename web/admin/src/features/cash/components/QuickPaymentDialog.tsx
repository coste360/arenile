import { problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { Spinner } from '@arenile/shared/ui/spinner';
import { FormActions, FormDialog, MoneyInput, StatusBadge } from '@arenile/shared/ui/patterns';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { zodResolver } from '@hookform/resolvers/zod';
import { SearchIcon } from 'lucide-react';
import { useDeferredValue, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  newIdempotencyKey,
  useBookingSearch,
  useRecordBookingPayment,
  type BookingSummary,
} from '../api';
import { usePrintDocument, usePrintReady } from '../../printers';
import { DESK_METHODS, type PaymentMethod } from '../lib/totals';
import { quickPaymentSchema, type QuickPaymentValues } from '../schemas';
import type { CashContext } from '../useCashContext';
import { fieldAria } from '../lib/aria';
import { CashField } from './fields';

/**
 * Quick "incassa" (spec 14.1): find a booking by code, propose its balance
 * due, pick the method and record a manual payment on the open session.
 */
export function QuickPaymentDialog({
  ctx,
  sessionId,
  registerId,
  open,
  onOpenChange,
}: {
  ctx: Pick<CashContext, 'establishmentId' | 'money'>;
  sessionId: string;
  /** Register of the session: selects the receipt printer (ADR 0021). */
  registerId?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('cash');
  const [booking, setBooking] = useState<BookingSummary | null>(null);
  const change = (o: boolean) => {
    if (!o) setBooking(null);
    onOpenChange(o);
  };
  return (
    <FormDialog
      open={open}
      onOpenChange={change}
      title={t('quickPay.title')}
      description={t('quickPay.description')}
    >
      {open &&
        (booking ? (
          <PaymentForm
            key={booking.id}
            ctx={ctx}
            sessionId={sessionId}
            registerId={registerId}
            booking={booking}
            onBack={() => setBooking(null)}
            onDone={() => change(false)}
          />
        ) : (
          <BookingSearch ctx={ctx} onSelect={setBooking} onCancel={() => change(false)} />
        ))}
    </FormDialog>
  );
}

function BookingSearch({
  ctx,
  onSelect,
  onCancel,
}: {
  ctx: Pick<CashContext, 'establishmentId' | 'money'>;
  onSelect: (b: BookingSummary) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('cash');
  const [q, setQ] = useState('');
  const term = useDeferredValue(q);
  const search = useBookingSearch(ctx.establishmentId, term);
  const results = term.trim().length >= 2 ? (search.data ?? []) : [];
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <label htmlFor="qp-search" className="text-sm font-medium">
          {t('quickPay.search')}
        </label>
        <InputGroup>
          <InputGroupAddon>
            <SearchIcon aria-hidden="true" />
          </InputGroupAddon>
          <InputGroupInput
            id="qp-search"
            type="search"
            autoFocus
            autoComplete="off"
            placeholder={t('quickPay.searchPlaceholder')}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          {search.isFetching && (
            <InputGroupAddon align="inline-end">
              <Spinner />
            </InputGroupAddon>
          )}
        </InputGroup>
      </div>
      <div aria-live="polite" className="space-y-2">
        {search.error ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t(problemMessageKey(search.error), { ns: 'translation' })}
            </AlertDescription>
          </Alert>
        ) : term.trim().length < 2 ? (
          <p className="text-sm text-muted-foreground">{t('quickPay.minChars')}</p>
        ) : search.isSuccess && results.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('quickPay.noResults')}</p>
        ) : (
          <ul className="divide-y rounded-md border" aria-label={t('quickPay.results')}>
            {results.map((b) => (
              <li key={b.id} className="flex items-center justify-between gap-3 p-3">
                <div className="min-w-0 space-y-0.5">
                  <p className="font-medium">
                    <span className="font-mono">{b.code}</span> · {b.customer_display_name}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {formatIsoDate(b.starts_on)}
                    {b.ends_on !== b.starts_on && ` – ${formatIsoDate(b.ends_on)}`}
                    {b.spot_labels.length > 0 && ` · ${b.spot_labels.join(', ')}`}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  {b.balance_due > 0 ? (
                    <StatusBadge tone="warning">
                      {t('quickPay.balance', { amount: ctx.money(b.balance_due) })}
                    </StatusBadge>
                  ) : (
                    <StatusBadge tone="success">{t('quickPay.paid')}</StatusBadge>
                  )}
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => onSelect(b)}
                    aria-label={t('quickPay.selectAria', { code: b.code })}
                  >
                    {t('quickPay.select')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="flex justify-end border-t pt-4">
        <Button variant="outline" onClick={onCancel}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
      </div>
    </div>
  );
}

function PaymentForm({
  ctx,
  sessionId,
  registerId,
  booking,
  onBack,
  onDone,
}: {
  ctx: Pick<CashContext, 'establishmentId' | 'money'>;
  sessionId: string;
  registerId?: string;
  booking: BookingSummary;
  onBack: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('cash');
  const [idempotencyKey] = useState(newIdempotencyKey);
  const record = useRecordBookingPayment(ctx.establishmentId);
  // Non-fiscal receipt on the thermal printer, offered only when one is ready.
  const receiptReady = usePrintReady('payment_receipt', registerId);
  const printDocument = usePrintDocument();
  const form = useForm<QuickPaymentValues>({
    resolver: zodResolver(quickPaymentSchema),
    defaultValues: {
      amount: booking.balance_due > 0 ? booking.balance_due : null,
      method: 'cash',
      note: '',
    },
  });
  const errors = form.formState.errors;
  const submit = form.handleSubmit(async (v) => {
    try {
      const amount = v.amount as number;
      const payment = await record.mutateAsync({
        bookingId: booking.id,
        idempotencyKey,
        body: {
          amount,
          method: v.method as PaymentMethod,
          cash_session_id: sessionId,
          ...(v.note.trim() ? { note: v.note.trim() } : {}),
        },
      });
      toast.success(
        t('quickPay.recorded', { amount: ctx.money(amount), code: booking.code }),
        receiptReady
          ? {
              action: {
                label: t('receipt.action', { ns: 'printers' }),
                // No browser layout for the receipt: errors are reported, nothing else prints.
                onClick: () =>
                  void printDocument({
                    kind: 'payment_receipt',
                    documentId: payment.id,
                    registerId,
                  }),
              },
            }
          : undefined,
      );
      onDone();
    } catch {
      // Rendered from record.error.
    }
  });
  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      {record.error && (
        <Alert variant="destructive">
          <AlertDescription>
            {t(problemMessageKey(record.error), { ns: 'translation' })}
          </AlertDescription>
        </Alert>
      )}
      <div className="flex items-start justify-between gap-3 rounded-md border bg-muted/40 p-3">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">{t('quickPay.booking')}</dt>
          <dd className="font-mono font-medium">{booking.code}</dd>
          <dt className="text-muted-foreground">{t('quickPay.customer')}</dt>
          <dd>{booking.customer_display_name}</dd>
          <dt className="text-muted-foreground">{t('quickPay.total')}</dt>
          <dd className="tabular-nums">{ctx.money(booking.total_amount)}</dd>
          <dt className="text-muted-foreground">{t('quickPay.paidAmount')}</dt>
          <dd className="tabular-nums">{ctx.money(booking.paid_amount)}</dd>
          <dt className="text-muted-foreground">{t('quickPay.balanceDue')}</dt>
          <dd className="font-semibold tabular-nums">{ctx.money(booking.balance_due)}</dd>
        </dl>
        <Button size="sm" variant="ghost" onClick={onBack}>
          {t('quickPay.change')}
        </Button>
      </div>
      <CashField id="qp-amount" label={t('fields.amount')} error={errors.amount}>
        <Controller
          control={form.control}
          name="amount"
          render={({ field }) => (
            <MoneyInput
              {...fieldAria('qp-amount', errors.amount)}
              value={field.value}
              onChange={(v) => field.onChange(v)}
              onBlur={field.onBlur}
            />
          )}
        />
      </CashField>
      <CashField id="qp-method" label={t('fields.method')} error={errors.method}>
        <NativeSelect {...fieldAria('qp-method', errors.method)} {...form.register('method')}>
          {DESK_METHODS.map((m) => (
            <NativeSelectOption key={m} value={m}>
              {t(`method.${m}`)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </CashField>
      <CashField id="qp-note" label={t('fields.note')} error={errors.note}>
        <Input {...fieldAria('qp-note', errors.note)} {...form.register('note')} />
      </CashField>
      <FormActions
        onCancel={onDone}
        pending={record.isPending}
        submitLabel={t('quickPay.submit')}
      />
    </form>
  );
}
