import { useProblemText } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import {
  type ColumnDef,
  DataTable,
  EmptyState,
  QueryState,
  SectionCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { HistoryIcon, RotateCcwIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { usePrintJobs, useRetryPrintJob, type PrintJob } from './api';
import { JOB_STATUS_TONE } from './lib';

/**
 * Ultime stampe: the latest print jobs with their state (queued, printing,
 * printed, failed after the retries, cancelled) and "Riprova" on a failed
 * one. Polls while a job is still in progress.
 */
export function JobsCard({
  establishmentId,
  canRetry,
}: {
  establishmentId: string;
  canRetry: boolean;
}) {
  const { t } = useTranslation('printers');
  const jobs = usePrintJobs(establishmentId);
  const retry = useRetryPrintJob(establishmentId);
  const fmt = useInstantFormat();
  const problemText = useProblemText();

  const columns: ColumnDef<PrintJob>[] = [
    {
      id: 'created',
      meta: { label: t('jobs.when') },
      header: t('jobs.when'),
      cell: ({ row }) => (
        <span className="text-sm whitespace-nowrap tabular-nums">
          {fmt.dateTime(row.original.created_at)}
        </span>
      ),
    },
    {
      id: 'document',
      meta: { label: t('jobs.document') },
      header: t('jobs.document'),
      cell: ({ row }) => (
        <span className="flex min-w-0 flex-col">
          <span className="font-medium">{t(`kind.${row.original.document_kind}`)}</span>
          {row.original.copies > 1 && (
            <span className="text-xs text-muted-foreground">
              {t('jobs.copies', { count: row.original.copies })}
            </span>
          )}
        </span>
      ),
    },
    {
      id: 'printer',
      meta: { label: t('jobs.printer') },
      header: t('jobs.printer'),
      cell: ({ row }) => row.original.printer_name ?? t('jobs.printerDeleted'),
    },
    {
      id: 'status',
      meta: { label: t('jobs.status') },
      header: t('jobs.status'),
      cell: ({ row }) => {
        const j = row.original;
        return (
          <span className="flex min-w-0 flex-col items-start gap-1">
            <StatusBadge tone={JOB_STATUS_TONE[j.status]}>{t(`jobStatus.${j.status}`)}</StatusBadge>
            {(j.status === 'failed' || (j.status === 'queued' && j.attempts > 0)) && (
              <span className="text-xs text-muted-foreground">
                {t('jobs.attempts', { attempts: j.attempts, max: j.max_attempts })}
              </span>
            )}
            {j.last_error && j.status !== 'printed' && (
              <span
                className="max-w-64 truncate text-xs text-muted-foreground"
                title={j.last_error}
              >
                {j.last_error}
              </span>
            )}
          </span>
        );
      },
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) =>
        canRetry && row.original.status === 'failed' ? (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="sm"
              disabled={retry.isPending}
              aria-label={t('jobs.retryAria', {
                document: t(`kind.${row.original.document_kind}`),
              })}
              onClick={() =>
                retry.mutate(row.original.id, {
                  onSuccess: () => toast.success(t('jobs.retried')),
                  onError: (err) => toast.error(problemText(err)),
                })
              }
            >
              <RotateCcwIcon aria-hidden="true" />
              <span className="hidden sm:inline">{t('jobs.retry')}</span>
            </Button>
          </div>
        ) : null,
    },
  ];

  return (
    <SectionCard
      title={t('jobs.title')}
      titleId="printers-jobs-title"
      icon={HistoryIcon}
      description={t('jobs.description')}
      flush
    >
      {jobs.error ? (
        <div className="px-6 pb-6">
          <QueryState isLoading={false} error={jobs.error} onRetry={() => void jobs.refetch()} />
        </div>
      ) : (
        <DataTable
          embedded
          caption={t('jobs.title')}
          columns={columns}
          data={jobs.data ?? []}
          loading={jobs.isPending}
          getRowId={(j) => j.id}
          pageSize={0}
          columnToggle={false}
          empty={<EmptyState icon={HistoryIcon} title={t('jobs.empty')} bordered={false} />}
        />
      )}
    </SectionCard>
  );
}
