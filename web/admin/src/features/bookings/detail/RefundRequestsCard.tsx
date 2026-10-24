import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import {
  DataTable,
  SectionCard,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { RotateCcwIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../../establishment';
import { useRetryRefundRequest, type BookingDetail, type RefundRequest } from '../api';
import { describeProblem } from '../problem';

const REFUND_TONE: Record<RefundRequest['status'], StatusTone> = {
  pending: 'warning',
  processing: 'info',
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'neutral',
};

/**
 * Guaranteed provider refunds of a cancellation (ADR 0019): reserved with
 * the cancellation, sent by a job until the provider confirms. A failed
 * one (attempts exhausted) can be sent again by whoever issues refunds.
 */
export function RefundRequestsCard({
  establishmentId,
  booking,
  canRetry,
}: {
  establishmentId: string;
  booking: BookingDetail;
  canRetry: boolean;
}) {
  const { t } = useTranslation('bookings');
  const fmt = useInstantFormat();
  const retry = useRetryRefundRequest(establishmentId);
  const rows = booking.refunds ?? [];
  if (rows.length === 0) return null;

  const onRetry = (id: string) =>
    retry.mutate(
      { refundRequestId: id },
      {
        onSuccess: (rr) =>
          toast.success(
            t(rr.status === 'succeeded' ? 'refunds.retriedDone' : 'refunds.retried', {
              amount: formatCents(rr.amount),
            }),
          ),
        onError: (err) => toast.error(describeProblem(err, t).title),
      },
    );

  const columns: ColumnDef<RefundRequest>[] = [
    {
      id: 'when',
      header: t('refunds.requested'),
      meta: { label: t('refunds.requested') },
      cell: ({ row }) => (
        <span className="tabular-nums">{fmt.dateTime(row.original.created_at)}</span>
      ),
    },
    {
      id: 'status',
      header: t('payments.status'),
      meta: { label: t('payments.status') },
      cell: ({ row: { original: r } }) => (
        <div className="space-y-1">
          <StatusBadge tone={REFUND_TONE[r.status]}>
            {t(`refunds.statuses.${r.status}`)}
          </StatusBadge>
          <p className="text-xs text-muted-foreground">{refundNote(r)}</p>
        </div>
      ),
    },
    {
      id: 'amount',
      header: t('fields.amount'),
      meta: { label: t('fields.amount'), align: 'right' },
      cell: ({ row: { original: r } }) => (
        <span className="flex items-center justify-end gap-2">
          {canRetry && r.status === 'failed' && (
            <Button
              size="sm"
              variant="outline"
              disabled={retry.isPending}
              onClick={() => onRetry(r.id)}
            >
              <RotateCcwIcon aria-hidden="true" />
              {t('refunds.retry')}
            </Button>
          )}
          <span className="tabular-nums">−{formatCents(r.amount)}</span>
        </span>
      ),
    },
  ];

  function refundNote(r: RefundRequest): string {
    if (r.status === 'succeeded' && r.completed_at)
      return t('refunds.completedAt', { at: fmt.dateTime(r.completed_at) });
    const parts: string[] = [t('refunds.attempts', { count: r.attempts })];
    if (r.last_error)
      parts.push(t(`refunds.errors.${r.last_error}`, { defaultValue: t('refunds.errors.other') }));
    if (r.next_attempt_at && (r.status === 'pending' || r.status === 'processing'))
      parts.push(t('refunds.nextAttempt', { at: fmt.dateTime(r.next_attempt_at) }));
    return parts.join(' · ');
  }

  return (
    <SectionCard flush title={t('refunds.title')} description={t('refunds.description')}>
      <DataTable
        embedded
        caption={t('refunds.title')}
        columns={columns}
        data={rows}
        getRowId={(r) => r.id}
        columnToggle={false}
        pageSize={0}
      />
    </SectionCard>
  );
}
