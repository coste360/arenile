import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import {
  DatePicker,
  FieldRow,
  FormActions,
  FormDialog,
  MoneyInput,
  OptionSelect,
} from '@arenile/shared/ui/patterns';
import { formatCents } from '@arenile/shared/lib/money';
import { zodResolver } from '@hookform/resolvers/zod';
import { useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { newIdempotencyKey } from '../configuration/lib/validation';
import {
  PAYMENT_METHODS,
  useRecordPartnerPayment,
  useVoidPartnerPayment,
  type Partner,
  type PartnerPayment,
} from './api';
import { describeM8Problem } from './lib';
import {
  paymentSchema,
  paymentValues,
  toPaymentInput,
  voidSchema,
  type PaymentValues,
  type VoidValues,
} from './partner.schema';

/** Records a payment received from the partner (bank transfer, cash...): lowers the balance. */
export function RecordPartnerPaymentDialog({
  establishmentId,
  partner,
  today,
  open,
  onOpenChange,
}: {
  establishmentId: string;
  partner: Partner;
  today: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('partners');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('payments.recordTitle', { name: partner.name })}
      description={t('payments.recordDescription')}
    >
      {open && (
        <RecordForm
          establishmentId={establishmentId}
          partner={partner}
          today={today}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function RecordForm({
  establishmentId,
  partner,
  today,
  onDone,
}: {
  establishmentId: string;
  partner: Partner;
  today: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const record = useRecordPartnerPayment(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  // Same key while the body does not change: a double click never records twice.
  const keyRef = useRef<{ body: string; key: string } | null>(null);
  const form = useForm<PaymentValues>({
    resolver: zodResolver(paymentSchema),
    defaultValues: paymentValues(today),
  });
  const submit = async (v: PaymentValues) => {
    setFailure(null);
    const input = toPaymentInput(v);
    const body = JSON.stringify(input);
    if (keyRef.current?.body !== body) keyRef.current = { body, key: newIdempotencyKey() };
    try {
      await record.mutateAsync({
        partnerId: partner.id,
        input,
        idempotencyKey: keyRef.current.key,
      });
      toast.success(t('payments.recorded', { amount: formatCents(input.amount) }));
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  };
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void form.handleSubmit(submit)(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="amount" label={t('payments.amount')}>
            {(field) => (
              <MoneyInput value={field.value} onChange={field.onChange} onBlur={field.onBlur} />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="received_on" label={t('payments.receivedOn')}>
            {(field) => (
              <DatePicker value={field.value} onChange={(v) => field.onChange(v ?? '')} />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="method" label={t('payments.method')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                options={PAYMENT_METHODS.map((m) => ({ value: m, label: t(`methods.${m}`) }))}
              />
            )}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="reference"
            label={t('payments.reference')}
            description={t('payments.referenceHelp')}
            optional
          >
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
        </div>
        <FieldRow control={form.control} name="note" label={t('payments.note')} optional>
          {(field) => <Input autoComplete="off" {...field} />}
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

/** Voids a recorded payment: it stays in the list with the reason, out of the balance. */
export function VoidPartnerPaymentDialog({
  establishmentId,
  partnerId,
  payment,
  onOpenChange,
}: {
  establishmentId: string;
  partnerId: string;
  payment: PartnerPayment | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('partners');
  return (
    <FormDialog
      open={!!payment}
      onOpenChange={onOpenChange}
      title={t('payments.voidTitle')}
      description={
        payment ? t('payments.voidDescription', { amount: formatCents(payment.amount) }) : ''
      }
    >
      {payment && (
        <VoidForm
          establishmentId={establishmentId}
          partnerId={partnerId}
          payment={payment}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function VoidForm({
  establishmentId,
  partnerId,
  payment,
  onDone,
}: {
  establishmentId: string;
  partnerId: string;
  payment: PartnerPayment;
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const voidPayment = useVoidPartnerPayment(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<VoidValues>({
    resolver: zodResolver(voidSchema),
    defaultValues: { reason: '' },
  });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      await voidPayment.mutateAsync({ partnerId, paymentId: payment.id, reason: v.reason });
      toast.success(t('payments.voided'));
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <FieldRow control={form.control} name="reason" label={t('payments.voidReason')}>
          {(field) => <Input autoComplete="off" {...field} />}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={voidPayment.isPending}
          submitLabel={t('payments.void')}
        />
      </form>
    </Form>
  );
}
