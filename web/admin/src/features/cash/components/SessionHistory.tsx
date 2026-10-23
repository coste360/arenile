import { Button } from '@arenile/shared/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import {
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  QueryState,
  SectionCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { HistoryIcon, PrinterIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useSession, useSessionHistory, type CashSession, type CashSessionDetail } from '../api';
import { normalizeTotals, sumTotals } from '../lib/totals';
import type { CashContext } from '../useCashContext';
import { DifferenceAmount } from './Difference';
import { MovementsTable, PaymentsTable } from './SessionView';
import { ZReport } from './ZReport';
import { registerLabel } from '../lib/register';

/** Past and current sessions; a row opens the detail with its Z summary. */
export function SessionHistory({
  ctx,
  selectedId,
  onSelect,
  onPrint,
}: {
  ctx: CashContext;
  selectedId: string | undefined;
  onSelect: (id: string | undefined) => void;
  onPrint: (detail: CashSessionDetail) => void;
}) {
  const { t } = useTranslation('cash');
  const history = useSessionHistory(ctx.establishmentId, ctx.canRead);

  const columns: ColumnDef<CashSession>[] = [
    {
      accessorKey: 'opened_at',
      meta: { label: t('history.openedAt') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('history.openedAt')} />
      ),
      cell: ({ row }) => (
        <Button
          variant="link"
          className="h-auto p-0 tabular-nums"
          onClick={(e) => {
            e.stopPropagation();
            onSelect(row.original.id);
          }}
        >
          {ctx.dateTime(row.original.opened_at)}
        </Button>
      ),
    },
    {
      accessorKey: 'closed_at',
      meta: { label: t('history.closedAt') },
      header: t('history.closedAt'),
      cell: ({ row }) => (
        <span className="tabular-nums">{ctx.dateTime(row.original.closed_at) || '—'}</span>
      ),
    },
    {
      accessorKey: 'register',
      meta: { label: t('history.register') },
      header: t('history.register'),
      cell: ({ row }) => registerLabel(t, row.original.register),
    },
    {
      accessorKey: 'status',
      meta: { label: t('history.statusLabel') },
      header: t('history.statusLabel'),
      cell: ({ row }) => (
        <StatusBadge tone={row.original.status === 'open' ? 'info' : 'neutral'}>
          {t(`history.status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'expected',
      meta: { label: t('close.expected'), align: 'right' },
      header: t('close.expected'),
      cell: ({ row }) =>
        row.original.status === 'closed'
          ? ctx.money(sumTotals(normalizeTotals(row.original.expected_totals)))
          : '—',
    },
    {
      id: 'counted',
      meta: { label: t('close.counted'), align: 'right' },
      header: t('close.counted'),
      cell: ({ row }) =>
        row.original.counted_totals
          ? ctx.money(sumTotals(normalizeTotals(row.original.counted_totals)))
          : '—',
    },
    {
      id: 'difference',
      meta: { label: t('close.difference'), align: 'right' },
      header: t('close.difference'),
      cell: ({ row }) =>
        row.original.difference === undefined ? (
          <span className="text-muted-foreground">—</span>
        ) : (
          <DifferenceAmount cents={row.original.difference} money={ctx.money} />
        ),
    },
  ];

  return (
    <SectionCard title={t('history.title')} titleId="cash-history-title" flush={!history.error}>
      {history.error ? (
        <div className="px-6">
          <QueryState
            isLoading={false}
            error={history.error}
            onRetry={() => void history.refetch()}
          />
        </div>
      ) : (
        <DataTable
          embedded
          caption={t('history.title')}
          columns={columns}
          data={history.data ?? []}
          loading={history.isPending}
          getRowId={(s) => s.id}
          onRowClick={(s) => onSelect(s.id)}
          pageSize={10}
          columnToggle={false}
          empty={<EmptyState icon={HistoryIcon} title={t('history.empty')} bordered={false} />}
        />
      )}
      <SessionDetailSheet
        ctx={ctx}
        sessionId={selectedId}
        onOpenChange={(o) => !o && onSelect(undefined)}
        onPrint={onPrint}
      />
    </SectionCard>
  );
}

function SessionDetailSheet({
  ctx,
  sessionId,
  onOpenChange,
  onPrint,
}: {
  ctx: CashContext;
  sessionId: string | undefined;
  onOpenChange: (open: boolean) => void;
  onPrint: (detail: CashSessionDetail) => void;
}) {
  const { t } = useTranslation('cash');
  const detail = useSession(ctx.establishmentId, sessionId);
  const d = detail.data;
  return (
    <Sheet open={!!sessionId} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-3xl print:hidden">
        <SheetHeader className="border-b">
          <SheetTitle>
            {d
              ? t('history.detailTitle', { date: ctx.dateTime(d.session.opened_at) })
              : t('history.detail')}
          </SheetTitle>
          <SheetDescription>{t('history.detailDescription')}</SheetDescription>
        </SheetHeader>
        <div className="space-y-6 p-4">
          <QueryState isLoading={detail.isPending && !!sessionId} error={detail.error} />
          {d && (
            <>
              <div className="flex justify-end">
                <Button variant="outline" onClick={() => onPrint(d)}>
                  <PrinterIcon aria-hidden="true" />
                  {t('actions.printZ')}
                </Button>
              </div>
              <ZReport detail={d} ctx={ctx} />
              <div className="space-y-2">
                <h3 className="font-semibold">{t('tabs.payments')}</h3>
                <PaymentsTable detail={d} ctx={ctx} />
              </div>
              <div className="space-y-2">
                <h3 className="font-semibold">{t('tabs.movements')}</h3>
                <MovementsTable detail={d} ctx={ctx} />
              </div>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
