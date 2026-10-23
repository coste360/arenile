import { formatIsoDate } from '@arenile/shared/lib/dates';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@arenile/shared/ui/alert-dialog';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useManageCancel, type ManageBooking } from '../api/public';
import { ChoiceGroup } from '../components/ChoiceGroup';
import { formatMoney } from '../flow/format';
import { problemMessageKey } from '../flow/problems';
import { PUBLIC_NS } from '../i18n';

type Settlement = 'refund' | 'credit';

/**
 * Cancellation within the policy (spec 12.4, 13.4): a dry run shows the
 * refund or credit of the booking's own policy, then the customer confirms
 * in a dialog; the refund goes through the provider when it supports it.
 */
export function CancelSection({
  slug,
  token,
  booking,
}: {
  slug: string;
  token: string;
  booking: ManageBooking;
}) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const id = useId();
  const cancel = useManageCancel(slug, token);
  const [settlement, setSettlement] = useState<Settlement>('refund');
  const [preview, setPreview] = useState(booking.cancellation_preview ?? null);
  const money = (c: number) => formatMoney(c, booking.summary.currency, i18n.language);
  const applied = cancel.data?.applied ? cancel.data : null;

  const runPreview = async (s: Settlement = settlement) => {
    try {
      const r = await cancel.mutateAsync({
        version: booking.version,
        body: { dry_run: true, settlement: s },
      });
      setPreview(r.refund);
    } catch {
      // shown below
    }
  };

  if (applied) {
    return (
      <div role="status" className="space-y-1 text-sm">
        <p className="font-medium">{t('manage.cancelled')}</p>
        {applied.refund_status && <p>{t(`manage.refundStatus.${applied.refund_status}`)}</p>}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {booking.allowed.cancel_until_free && (
        <p className="text-sm text-muted-foreground">
          {t('manage.cancelFreeUntil', { date: formatIsoDate(booking.allowed.cancel_until_free) })}
        </p>
      )}
      {preview && (
        <div className="space-y-3 rounded-lg border bg-muted/30 p-3 text-sm" aria-live="polite">
          <p className="font-medium">
            {preview.as === 'refund'
              ? t('manage.cancelRefund', {
                  amount: money(preview.amount),
                  percent: preview.refund_percent,
                })
              : preview.as === 'credit'
                ? t('manage.cancelCredit', {
                    amount: money(preview.amount),
                    percent: preview.refund_percent,
                  })
                : t('manage.cancelNone')}
          </p>
          {preview.as !== 'none' && preview.amount > 0 && (
            <div className="space-y-2">
              <p id={`${id}-settlement`} className="sr-only">
                {t('manage.cancelTitle')}
              </p>
              <ChoiceGroup<Settlement>
                labelledBy={`${id}-settlement`}
                value={settlement}
                options={[
                  { value: 'refund', label: t('manage.settlementRefund') },
                  { value: 'credit', label: t('manage.settlementCredit') },
                ]}
                onChange={(s) => {
                  setSettlement(s);
                  void runPreview(s);
                }}
              />
            </div>
          )}
        </div>
      )}
      {cancel.error && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemMessageKey(cancel.error))}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {!preview && (
          <Button
            variant="outline"
            className="h-11"
            onClick={() => void runPreview()}
            disabled={cancel.isPending}
          >
            {cancel.isPending && <Spinner />}
            {t('manage.cancelPreview')}
          </Button>
        )}
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive" className="h-11" disabled={cancel.isPending}>
              {t('manage.cancelTitle')}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('manage.cancelTitle')}</AlertDialogTitle>
              <AlertDialogDescription>
                {t('manage.cancelQuestion', { code: booking.code })}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel className="h-11">{tc('common.back')}</AlertDialogCancel>
              <AlertDialogAction
                className="h-11"
                onClick={() =>
                  cancel.mutate({ version: booking.version, body: { dry_run: false, settlement } })
                }
              >
                {t('manage.cancelConfirm')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </div>
  );
}
