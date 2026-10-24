import { useProblemText } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import {
  DataTable,
  EmptyState,
  FormSheet,
  QueryState,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { FileTextIcon, InboxIcon, RotateCwIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../../establishment';
import {
  useRedeliverWebhook,
  useWebhookDeliveries,
  useWebhookDelivery,
  useWebhookDeliveryAttempts,
  type WebhookDelivery,
  type WebhookDeliveryAttempt,
  type WebhookDeliveryStatus,
  type WebhookEndpoint,
} from '../api';
import { canRedeliver } from '../lib/deliveries';
import { useEventLabel } from './useEventLabel';

const DELIVERY_TONE: Record<WebhookDeliveryStatus, StatusTone> = {
  pending: 'info',
  delivered: 'success',
  failed: 'danger',
};

const STATUSES = ['', 'pending', 'delivered', 'failed'] as const;

export interface DeliveriesSheetProps {
  establishmentId: string;
  /** Endpoint whose log is open; null closes the sheet. */
  endpoint: WebhookEndpoint | null;
  onOpenChange: (open: boolean) => void;
}

/**
 * Delivery log of an endpoint (newest first, cursor pagination, status
 * filter), the detail of one delivery (envelope sent, response excerpt) and
 * "Reinvia".
 */
export function DeliveriesSheet({ establishmentId, endpoint, onOpenChange }: DeliveriesSheetProps) {
  const { t } = useTranslation('integrations');
  return (
    <FormSheet
      open={endpoint !== null}
      onOpenChange={onOpenChange}
      size="lg"
      title={t('deliveries.title')}
      description={endpoint?.url}
    >
      {endpoint && <DeliveriesLog establishmentId={establishmentId} endpoint={endpoint} />}
    </FormSheet>
  );
}

function DeliveriesLog({
  establishmentId,
  endpoint,
}: {
  establishmentId: string;
  endpoint: WebhookEndpoint;
}) {
  const { t } = useTranslation('integrations');
  const fmt = useInstantFormat();
  const eventLabel = useEventLabel();
  const problemText = useProblemText();
  const [status, setStatus] = useState<WebhookDeliveryStatus | ''>('');
  const [detailId, setDetailId] = useState<string | undefined>();
  const log = useWebhookDeliveries(establishmentId, endpoint.id, status || undefined);
  const redeliver = useRedeliverWebhook(establishmentId, endpoint.id);
  const rows = log.data?.pages.flatMap((p) => p.items) ?? [];

  const doRedeliver = (d: WebhookDelivery) =>
    redeliver.mutate(d.id, {
      onSuccess: () => toast.success(t('deliveries.redelivered')),
      onError: (err) => toast.error(problemText(err)),
    });

  const when = (d: WebhookDelivery) =>
    d.delivered_at
      ? t('deliveries.deliveredAt', { at: fmt.dateTime(d.delivered_at) })
      : d.failed_at
        ? t('deliveries.failedAt', { at: fmt.dateTime(d.failed_at) })
        : d.next_attempt_at
          ? t('deliveries.nextAt', { at: fmt.dateTime(d.next_attempt_at) })
          : fmt.dateTime(d.created_at);

  const columns: ColumnDef<WebhookDelivery>[] = [
    {
      id: 'event',
      accessorFn: (d) => d.event_type,
      meta: { label: t('deliveries.event') },
      header: t('deliveries.event'),
      cell: ({ row }) => (
        <span className="flex flex-col">
          <span className="font-medium">{eventLabel(row.original.event_type)}</span>
          <span className="text-xs text-muted-foreground">
            {fmt.dateTime(row.original.created_at)}
          </span>
        </span>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('deliveries.status') },
      header: t('deliveries.status'),
      cell: ({ row }) => (
        <StatusBadge tone={DELIVERY_TONE[row.original.status]}>
          {t(`deliveries.statusOption.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'attempts',
      meta: { label: t('deliveries.attempts') },
      header: t('deliveries.attempts'),
      cell: ({ row }) => <span className="tabular-nums">{row.original.attempts}</span>,
    },
    {
      id: 'code',
      accessorFn: (d) => d.status_code ?? 0,
      meta: { label: t('deliveries.code') },
      header: t('deliveries.code'),
      cell: ({ row }) => <span className="tabular-nums">{row.original.status_code ?? '—'}</span>,
    },
    {
      id: 'when',
      accessorFn: (d) => d.updated_at,
      meta: { label: t('deliveries.when') },
      header: t('deliveries.when'),
      cell: ({ row }) => (
        <span className="text-sm whitespace-nowrap text-muted-foreground">
          {when(row.original)}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const d = row.original;
        return (
          <div className="flex items-center justify-end gap-0.5">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('deliveries.detailAria', { event: d.event_type })}
              title={t('deliveries.detail')}
              onClick={() => setDetailId(d.id)}
            >
              <FileTextIcon aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('deliveries.redeliverAria', { event: d.event_type })}
              title={t('deliveries.redeliver')}
              disabled={!canRedeliver(endpoint, d) || redeliver.isPending}
              onClick={() => doRedeliver(d)}
            >
              <RotateCwIcon aria-hidden="true" />
            </Button>
          </div>
        );
      },
    },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-4">
        <div className="space-y-2">
          <Label htmlFor="deliveries-status">{t('deliveries.filter')}</Label>
          <NativeSelect
            id="deliveries-status"
            value={status}
            onChange={(e) => setStatus(e.target.value as WebhookDeliveryStatus | '')}
          >
            {STATUSES.map((s) => (
              <NativeSelectOption key={s || 'all'} value={s}>
                {s ? t(`deliveries.statusOption.${s}`) : t('deliveries.allStatuses')}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
      </div>
      {log.error ? (
        <QueryState isLoading={false} error={log.error} onRetry={() => void log.refetch()} />
      ) : (
        <DataTable
          caption={t('deliveries.title')}
          columns={columns}
          data={rows}
          loading={log.isPending}
          getRowId={(d) => d.id}
          pageSize={0}
          columnToggle={false}
          empty={<EmptyState icon={InboxIcon} title={t('deliveries.empty')} bordered={false} />}
        />
      )}
      {log.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            disabled={log.isFetchingNextPage}
            onClick={() => void log.fetchNextPage()}
          >
            {log.isFetchingNextPage && <Spinner />}
            {t('deliveries.more')}
          </Button>
        </div>
      )}
      <DeliveryDetailDialog
        establishmentId={establishmentId}
        endpoint={endpoint}
        deliveryId={detailId}
        onClose={() => setDetailId(undefined)}
        onRedeliver={doRedeliver}
        redelivering={redeliver.isPending}
      />
    </div>
  );
}

function DeliveryDetailDialog({
  establishmentId,
  endpoint,
  deliveryId,
  onClose,
  onRedeliver,
  redelivering,
}: {
  establishmentId: string;
  endpoint: WebhookEndpoint;
  deliveryId: string | undefined;
  onClose: () => void;
  onRedeliver: (d: WebhookDelivery) => void;
  redelivering: boolean;
}) {
  const { t } = useTranslation('integrations');
  const eventLabel = useEventLabel();
  const fmt = useInstantFormat();
  const detail = useWebhookDelivery(establishmentId, endpoint.id, deliveryId);
  const d = detail.data;
  return (
    <Dialog open={deliveryId !== undefined} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('deliveries.detail')}</DialogTitle>
          <DialogDescription>
            {d ? `${eventLabel(d.event_type)} · ${d.event_id}` : t('deliveries.loading')}
          </DialogDescription>
        </DialogHeader>
        {detail.error ? (
          <QueryState
            isLoading={false}
            error={detail.error}
            onRetry={() => void detail.refetch()}
          />
        ) : !d ? (
          <QueryState isLoading error={null} rows={3} />
        ) : (
          <div className="min-w-0 space-y-4 text-sm">
            <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
              <div>
                <dt className="text-muted-foreground">{t('deliveries.status')}</dt>
                <dd>
                  <StatusBadge tone={DELIVERY_TONE[d.status]}>
                    {t(`deliveries.statusOption.${d.status}`)}
                  </StatusBadge>
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t('deliveries.attempts')}</dt>
                <dd className="tabular-nums">{d.attempts}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t('deliveries.code')}</dt>
                <dd className="tabular-nums">{d.status_code ?? '—'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">{t('deliveries.created')}</dt>
                <dd>{fmt.dateTime(d.created_at)}</dd>
              </div>
            </dl>
            {d.last_error && (
              <p>
                <span className="text-muted-foreground">{t('deliveries.lastError')}: </span>
                {d.last_error}
              </p>
            )}
            <AttemptsHistory
              establishmentId={establishmentId}
              endpointId={endpoint.id}
              deliveryId={d.id}
            />
            <div className="space-y-1">
              <p className="font-medium">{t('deliveries.body')}</p>
              <pre className="max-h-64 overflow-auto rounded-md border bg-muted/50 p-3 font-mono text-xs">
                {JSON.stringify(d.request_body, null, 2)}
              </pre>
            </div>
            <div className="space-y-1">
              <p className="font-medium">{t('deliveries.response')}</p>
              <pre className="max-h-40 overflow-auto rounded-md border bg-muted/50 p-3 font-mono text-xs whitespace-pre-wrap">
                {d.response_excerpt || t('deliveries.noResponse')}
              </pre>
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.close', { ns: 'translation' })}
          </Button>
          {d && (
            <Button
              disabled={!canRedeliver(endpoint, d) || redelivering}
              onClick={() => onRedeliver(d)}
            >
              <RotateCwIcon aria-hidden="true" />
              {t('deliveries.redeliver')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Every attempt of a delivery (ADR 0019 F), not only the last one. */
function AttemptsHistory({
  establishmentId,
  endpointId,
  deliveryId,
}: {
  establishmentId: string;
  endpointId: string;
  deliveryId: string;
}) {
  const { t } = useTranslation('integrations');
  const fmt = useInstantFormat();
  const q = useWebhookDeliveryAttempts(establishmentId, endpointId, deliveryId);
  const rows = q.data?.pages.flatMap((p) => p.items) ?? [];
  const columns: ColumnDef<WebhookDeliveryAttempt>[] = [
    {
      accessorKey: 'attempt',
      meta: { label: t('deliveries.attempt'), align: 'right' },
      header: t('deliveries.attempt'),
    },
    {
      id: 'at',
      accessorFn: (a) => a.attempted_at,
      meta: { label: t('deliveries.attemptAt') },
      header: t('deliveries.attemptAt'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap">{fmt.dateTime(row.original.attempted_at)}</span>
      ),
    },
    {
      id: 'code',
      accessorFn: (a) => a.status_code ?? 0,
      meta: { label: t('deliveries.code'), align: 'right' },
      header: t('deliveries.code'),
      cell: ({ row }) => row.original.status_code ?? '—',
    },
    {
      accessorKey: 'duration_ms',
      meta: { label: t('deliveries.duration'), align: 'right' },
      header: t('deliveries.duration'),
      cell: ({ row }) => t('deliveries.durationValue', { ms: row.original.duration_ms }),
    },
    {
      id: 'error',
      accessorFn: (a) => a.error ?? '',
      meta: { label: t('deliveries.error') },
      header: t('deliveries.error'),
      cell: ({ row }) => (
        <span className="break-words text-muted-foreground">{row.original.error || '—'}</span>
      ),
    },
  ];
  return (
    <div className="min-w-0 space-y-2">
      <p className="font-medium">{t('deliveries.attemptsTitle')}</p>
      {q.error ? (
        <QueryState isLoading={false} error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <DataTable
          caption={t('deliveries.attemptsTitle')}
          columns={columns}
          data={rows}
          loading={q.isPending}
          getRowId={(a) => a.id}
          pageSize={0}
          columnToggle={false}
          empty={<EmptyState title={t('deliveries.noAttempts')} bordered={false} />}
        />
      )}
      {q.hasNextPage && (
        <Button
          variant="outline"
          disabled={q.isFetchingNextPage}
          onClick={() => void q.fetchNextPage()}
        >
          {q.isFetchingNextPage && <Spinner />}
          {t('deliveries.moreAttempts')}
        </Button>
      )}
    </div>
  );
}
