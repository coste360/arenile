import { formatCents } from '@arenile/shared/lib/money';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Textarea } from '@arenile/shared/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  FieldRow,
  FormActions,
  FormDialog,
  MoneyInput,
  OptionSelect,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { newIdempotencyKey } from '../../configuration/lib/validation';
import { CreditAvailableHint } from '../../credits';
import { useRecordPayment, useRecordRefund, type BookingDetail } from '../api';
import {
  paymentSchema,
  refundSchema,
  toPaymentInput,
  toRefundInput,
  type PaymentValues,
  type RefundValues,
} from '../booking.schema';
import { MANUAL_METHODS } from '../model';
import { describeProblem, type ProblemText } from '../problem';
import { ProblemAlert } from '../ProblemAlert';

interface DialogProps {
  establishmentId: string;
  booking: BookingDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Stable Idempotency-Key per dialog session: a retry after a network error
 * never records twice; the form unmounts after a success, so the next
 * payment gets a new key.
 */
function useDialogKey(): string {
  const [key] = useState(newIdempotencyKey);
  return key;
}

/** Manual payment (driver manual, spec 13): default amount = balance due. */
export function RecordPaymentDialog({ establishmentId, booking, open, onOpenChange }: DialogProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('payments.recordTitle')}
      description={t('payments.recordDescription', { balance: formatCents(booking.balance_due) })}
    >
      {open && (
        <PaymentForm
          establishmentId={establishmentId}
          booking={booking}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function PaymentForm({
  establishmentId,
  booking,
  onDone,
}: {
  establishmentId: string;
  booking: BookingDetail;
  onDone: () => void;
}) {
  const { t } = useTranslation('bookings');
  const record = useRecordPayment(establishmentId);
  const idem = useDialogKey();
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<PaymentValues>({
    resolver: zodResolver(paymentSchema()),
    defaultValues: {
      amount: booking.balance_due > 0 ? booking.balance_due : (null as unknown as number),
      method: 'cash',
      note: '',
    },
  });
  const method = useWatch({ control: form.control, name: 'method' });
  const amount = useWatch({ control: form.control, name: 'amount' });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      await record.mutateAsync({
        bookingId: booking.id,
        input: toPaymentInput(v),
        idempotencyKey: idem,
      });
      toast.success(t('payments.recorded', { amount: formatCents(v.amount) }));
      onDone();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="amount" label={t('fields.amount')}>
            {(field) => (
              <MoneyInput
                value={field.value}
                onChange={(c) => field.onChange(c)}
                onBlur={field.onBlur}
              />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="method" label={t('fields.method')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                options={MANUAL_METHODS.map((m) => ({ value: m, label: t(`method.${m}`) }))}
              />
            )}
          </FieldRow>
        </div>
        {method === 'credit' && (
          <CreditAvailableHint
            establishmentId={establishmentId}
            customerId={booking.customer.id}
            amount={amount}
          />
        )}
        <FieldRow control={form.control} name="note" label={t('fields.note')} optional>
          {(field) => <Input {...field} />}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={record.isPending}
          submitLabel={t('payments.record')}
        />
      </form>
    </Form>
  );
}

/** Manual refund or credit (refunds.issue); at most the paid amount (INV-24). */
export function RefundDialog({ establishmentId, booking, open, onOpenChange }: DialogProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('payments.refundTitle')}
      description={t('payments.refundDescription', { paid: formatCents(booking.paid_amount) })}
    >
      {open && (
        <RefundForm
          establishmentId={establishmentId}
          booking={booking}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function RefundForm({
  establishmentId,
  booking,
  onDone,
}: {
  establishmentId: string;
  booking: BookingDetail;
  onDone: () => void;
}) {
  const { t } = useTranslation('bookings');
  const refund = useRecordRefund(establishmentId);
  const idem = useDialogKey();
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<RefundValues>({
    resolver: zodResolver(refundSchema(booking.paid_amount)),
    defaultValues: { amount: null as unknown as number, method: 'cash', as: 'refund', reason: '' },
  });
  const as = useWatch({ control: form.control, name: 'as' });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      await refund.mutateAsync({
        bookingId: booking.id,
        input: toRefundInput(v),
        idempotencyKey: idem,
      });
      toast.success(t('payments.refunded', { amount: formatCents(v.amount) }));
      onDone();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <FieldRow control={form.control} name="as" label={t('payments.refundAs')}>
          {(field) => (
            <ToggleGroup
              type="single"
              variant="outline"
              value={field.value}
              onValueChange={(v) => v && field.onChange(v)}
            >
              <ToggleGroupItem value="refund">{t('cancel.as.refund')}</ToggleGroupItem>
              <ToggleGroupItem value="credit">{t('cancel.as.credit')}</ToggleGroupItem>
            </ToggleGroup>
          )}
        </FieldRow>
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="amount" label={t('fields.amount')}>
            {(field) => (
              <MoneyInput
                value={field.value}
                onChange={(c) => field.onChange(c)}
                onBlur={field.onBlur}
              />
            )}
          </FieldRow>
          {as === 'refund' && (
            <FieldRow control={form.control} name="method" label={t('fields.method')}>
              {(field) => (
                <OptionSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={MANUAL_METHODS.filter((m) => m !== 'credit').map((m) => ({
                    value: m,
                    label: t(`method.${m}`),
                  }))}
                />
              )}
            </FieldRow>
          )}
        </div>
        <FieldRow control={form.control} name="reason" label={t('payments.reason')} optional>
          {(field) => <Textarea rows={2} {...field} />}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={refund.isPending}
          submitLabel={t('payments.refund')}
        />
      </form>
    </Form>
  );
}
