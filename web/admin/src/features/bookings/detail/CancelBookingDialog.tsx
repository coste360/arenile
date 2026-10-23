import { formatCents } from '@arenile/shared/lib/money';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import { Form } from '@arenile/shared/ui/form';
import { Label } from '@arenile/shared/ui/label';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Textarea } from '@arenile/shared/ui/textarea';
import {
  FieldRow,
  FormActions,
  FormDialog,
  MoneyInput,
  OptionSelect,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCancelBooking, type BookingDetail, type RefundPreview } from '../api';
import { cancelSchema, SETTLEMENTS, toCancelRequest, type CancelValues } from '../booking.schema';
import { MANUAL_METHODS } from '../model';
import { describeProblem, type ProblemText } from '../problem';
import { ProblemAlert } from '../ProblemAlert';

export interface CancelBookingDialogProps {
  establishmentId: string;
  booking: BookingDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Whether the user may issue refunds (refunds.issue): otherwise only credit or nothing. */
  canRefund: boolean;
  onReload: () => void;
}

/**
 * Cancellation with a refund preview (spec 9, 13.4): the server computes the
 * refund from the booking's policy snapshot with `dry_run` (nothing written),
 * the operator may keep it, force a refund or a credit of another amount, or
 * settle nothing; the confirmation applies it in one transaction.
 */
export function CancelBookingDialog({
  establishmentId,
  booking,
  open,
  onOpenChange,
  canRefund,
  onReload,
}: CancelBookingDialogProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('cancel.title', { code: booking.code })}
      description={t('cancel.description')}
    >
      {open && (
        <CancelForm
          establishmentId={establishmentId}
          booking={booking}
          canRefund={canRefund}
          onDone={() => onOpenChange(false)}
          onReload={onReload}
        />
      )}
    </FormDialog>
  );
}

function CancelForm({
  establishmentId,
  booking,
  canRefund,
  onDone,
  onReload,
}: {
  establishmentId: string;
  booking: BookingDetail;
  canRefund: boolean;
  onDone: () => void;
  onReload: () => void;
}) {
  const { t } = useTranslation('bookings');
  const preview = useCancelBooking(establishmentId);
  const apply = useCancelBooking(establishmentId);
  const [refund, setRefund] = useState<RefundPreview | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<CancelValues>({
    resolver: zodResolver(cancelSchema(booking.paid_amount)),
    defaultValues: {
      settlement: canRefund ? 'policy' : 'credit',
      refund_amount: null,
      refund_method: 'cash',
      weather: false,
      reason: '',
    },
  });
  const settlement = useWatch({ control: form.control, name: 'settlement' });
  const weather = useWatch({ control: form.control, name: 'weather' });
  const { mutateAsync: runPreview } = preview;

  // Preview the policy outcome whenever the settlement form or the weather flag changes.
  useEffect(() => {
    let alive = true;
    const values = form.getValues();
    runPreview({
      bookingId: booking.id,
      version: booking.version,
      request: toCancelRequest(
        {
          ...values,
          refund_amount: null,
          settlement: values.settlement === 'none' ? 'none' : 'policy',
        },
        true,
      ),
    })
      .then((r) => {
        if (!alive) return;
        setRefund(r.refund);
        if (form.getValues('refund_amount') === null)
          form.setValue('refund_amount', r.refund.amount);
      })
      .catch((err: unknown) => alive && setFailure(describeProblem(err, t)));
    return () => {
      alive = false;
    };
  }, [settlement, weather, booking.id, booking.version, runPreview, form, t]);

  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      const result = await apply.mutateAsync({
        bookingId: booking.id,
        version: booking.version,
        request: toCancelRequest(v, false),
      });
      const r = result.refund;
      toast.success(
        r.amount > 0
          ? t('cancel.doneWith', { amount: formatCents(r.amount), as: t(`cancel.as.${r.as}`) })
          : t('cancel.done'),
      );
      onDone();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });

  const forced = settlement === 'refund' || settlement === 'credit';
  const settlementOptions = SETTLEMENTS.filter(
    (s) => canRefund || s === 'credit' || s === 'none',
  ).map((s) => ({ value: s, label: t(`cancel.settlement.${s}`) }));

  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} onReload={onReload} />
        <section
          aria-label={t('cancel.preview')}
          aria-live="polite"
          className="rounded-lg border p-3 text-sm"
        >
          {refund ? (
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1">
              <dt className="text-muted-foreground">{t('cancel.paid')}</dt>
              <dd className="text-right tabular-nums">{formatCents(refund.paid_amount)}</dd>
              <dt className="text-muted-foreground">{t('cancel.percent')}</dt>
              <dd className="text-right tabular-nums">{refund.refund_percent}%</dd>
              <dt className="font-medium">{t('cancel.policyAmount')}</dt>
              <dd className="text-right font-medium tabular-nums" data-testid="refund-amount">
                {formatCents(refund.amount)} · {t(`cancel.as.${refund.as}`)}
              </dd>
              {refund.tier && (
                <dd className="col-span-2 text-xs text-muted-foreground">
                  {t('policy.tier', {
                    count: refund.tier.until_days_before,
                    percent: refund.tier.refund_percent,
                    as: t(`policy.as.${refund.tier.as ?? 'refund'}`),
                  })}
                </dd>
              )}
            </dl>
          ) : preview.isPending ? (
            <div role="status" className="space-y-2">
              <span className="sr-only">{t('cancel.computing')}</span>
              <Skeleton className="h-4 w-1/2" />
              <Skeleton className="h-4 w-2/3" />
            </div>
          ) : null}
        </section>

        <FieldRow control={form.control} name="settlement" label={t('cancel.settlementLabel')}>
          {(field) => (
            <OptionSelect
              value={field.value}
              onChange={field.onChange}
              options={settlementOptions}
            />
          )}
        </FieldRow>
        {forced && (
          <FieldRow control={form.control} name="refund_amount" label={t('cancel.amount')}>
            {(field) => (
              <MoneyInput
                value={field.value}
                onChange={(c) => field.onChange(c)}
                onBlur={field.onBlur}
              />
            )}
          </FieldRow>
        )}
        {(settlement === 'policy' || settlement === 'refund') && (
          <FieldRow control={form.control} name="refund_method" label={t('fields.method')}>
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
        <div className="flex items-center gap-2">
          <Checkbox
            id="cancel-weather"
            checked={weather}
            onCheckedChange={(c) => form.setValue('weather', c === true)}
          />
          <Label htmlFor="cancel-weather" className="font-normal">
            {t('cancel.weather')}
          </Label>
        </div>
        <FieldRow control={form.control} name="reason" label={t('cancel.reason')} optional>
          {(field) => <Textarea rows={2} {...field} />}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={apply.isPending}
          submitLabel={t('cancel.confirm')}
        />
      </form>
    </Form>
  );
}
