import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import {
  DataTable,
  EmptyState,
  SectionCard,
  StatCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { ArrowDownLeftIcon, ArrowUpRightIcon, ReceiptIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { CashMovement, CashSessionDetail } from '../api';
import {
  expectedTotals,
  methodsShown,
  netMovements,
  normalizeTotals,
  sumTotals,
} from '../lib/totals';
import type { CashContext } from '../useCashContext';

type CashPayment = CashSessionDetail['payments'][number];

/** Running figures of the open session: KPI cards, takings by method, activity. */
export function SessionView({
  detail,
  ctx,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'dateTime' | 'time'>;
}) {
  const { t } = useTranslation('cash');
  const { session } = detail;
  const takings = normalizeTotals(detail.totals_by_method);
  const expected = expectedTotals(detail);
  const mv = netMovements(detail.movements);
  const methods = methodsShown(takings);

  const stats = [
    { id: 'float', label: t('current.float'), value: session.opening_float },
    { id: 'takings', label: t('current.totalTakings'), value: sumTotals(takings) },
    { id: 'movements', label: t('current.netMovements'), value: mv.net },
    {
      id: 'expected-cash',
      label: t('current.expectedCash'),
      value: expected.cash ?? 0,
      hint: t('current.expectedCashHint'),
    },
  ];

  const byMethod: ColumnDef<string>[] = [
    {
      id: 'method',
      meta: { label: t('close.method') },
      header: t('close.method'),
      cell: ({ row }) => t(`method.${row.original}`),
    },
    {
      id: 'takings',
      meta: { label: t('z.takings'), align: 'right' },
      header: t('z.takings'),
      cell: ({ row }) => ctx.money(takings[row.original as keyof typeof takings] ?? 0),
    },
    {
      id: 'expected',
      meta: { label: t('close.expected'), align: 'right' },
      header: t('close.expected'),
      cell: ({ row }) => ctx.money(expected[row.original as keyof typeof expected] ?? 0),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {stats.map((s) => (
          <StatCard
            key={s.id}
            label={s.label}
            value={<span data-testid={`stat-${s.id}`}>{ctx.money(s.value)}</span>}
            hint={s.hint}
          />
        ))}
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-3">
        <SectionCard title={t('current.takings')} flush className="min-w-0">
          <DataTable
            embedded
            caption={t('current.takings')}
            columns={byMethod}
            data={methodsShown(takings, expected)}
            getRowId={(m) => m}
            pageSize={0}
            columnToggle={false}
          />
          {methods.length === 1 && (takings.cash ?? 0) === 0 && (
            <p className="px-6 pt-3 pb-6 text-sm text-muted-foreground">{t('current.noTakings')}</p>
          )}
        </SectionCard>

        <SectionCard title={t('current.activity')} flush className="min-w-0 lg:col-span-2">
          <Tabs defaultValue="payments" className="gap-4">
            <div className="px-6">
              <TabsList>
                <TabsTrigger value="payments">
                  {t('tabs.payments')} ({detail.payments.length})
                </TabsTrigger>
                <TabsTrigger value="movements">
                  {t('tabs.movements')} ({detail.movements.length})
                </TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value="payments">
              <PaymentsTable detail={detail} ctx={ctx} embedded />
            </TabsContent>
            <TabsContent value="movements">
              <MovementsTable detail={detail} ctx={ctx} embedded />
            </TabsContent>
          </Tabs>
        </SectionCard>
      </div>
    </div>
  );
}

export function PaymentsTable({
  detail,
  ctx,
  embedded,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'time'>;
  embedded?: boolean;
}) {
  const { t } = useTranslation('cash');
  const rows = [...detail.payments].sort((a, b) => b.created_at.localeCompare(a.created_at));
  const columns: ColumnDef<CashPayment>[] = [
    {
      id: 'time',
      meta: { label: t('fields.time') },
      header: t('fields.time'),
      cell: ({ row }) => <span className="tabular-nums">{ctx.time(row.original.created_at)}</span>,
    },
    {
      id: 'what',
      meta: { label: t('payments.what') },
      header: t('payments.what'),
      cell: ({ row }) => {
        const p = row.original;
        return (
          <span className="flex flex-wrap items-center gap-2">
            {p.booking_code ? (
              <span className="font-mono">{p.booking_code}</span>
            ) : (
              <span>{p.description || t('payments.counterSale')}</span>
            )}
            {p.direction === 'refund' && (
              <StatusBadge tone="danger">{t('payments.refund')}</StatusBadge>
            )}
            {p.status !== 'succeeded' && (
              <StatusBadge tone="neutral">{t(`payments.status.${p.status}`)}</StatusBadge>
            )}
          </span>
        );
      },
    },
    {
      id: 'method',
      meta: { label: t('fields.method') },
      header: t('fields.method'),
      cell: ({ row }) => t(`method.${row.original.method}`),
    },
    {
      id: 'amount',
      meta: { label: t('fields.amount'), align: 'right' },
      header: t('fields.amount'),
      cell: ({ row }) =>
        ctx.money(row.original.direction === 'refund' ? -row.original.amount : row.original.amount),
    },
  ];
  return (
    <DataTable
      embedded={embedded}
      caption={t('tabs.payments')}
      columns={columns}
      data={rows}
      getRowId={(p) => p.id}
      pageSize={0}
      columnToggle={false}
      empty={<EmptyState icon={ReceiptIcon} title={t('payments.empty')} bordered={false} />}
    />
  );
}

export function MovementsTable({
  detail,
  ctx,
  embedded,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'time'>;
  embedded?: boolean;
}) {
  const { t } = useTranslation('cash');
  const rows = [...detail.movements].sort((a, b) => b.created_at.localeCompare(a.created_at));
  const columns: ColumnDef<CashMovement>[] = [
    {
      id: 'time',
      meta: { label: t('fields.time') },
      header: t('fields.time'),
      cell: ({ row }) => <span className="tabular-nums">{ctx.time(row.original.created_at)}</span>,
    },
    {
      id: 'kind',
      meta: { label: t('movement.kind') },
      header: t('movement.kind'),
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-1.5">
          {row.original.kind === 'in' ? (
            <ArrowDownLeftIcon aria-hidden="true" className="size-4 text-success" />
          ) : (
            <ArrowUpRightIcon aria-hidden="true" className="size-4 text-destructive" />
          )}
          {t(`movement.${row.original.kind}`)}
        </span>
      ),
    },
    {
      id: 'reason',
      meta: { label: t('movement.reason') },
      header: t('movement.reason'),
      cell: ({ row }) => <span className="block max-w-64 truncate">{row.original.reason}</span>,
    },
    {
      id: 'amount',
      meta: { label: t('fields.amount'), align: 'right' },
      header: t('fields.amount'),
      cell: ({ row }) =>
        ctx.money(row.original.kind === 'in' ? row.original.amount : -row.original.amount),
    },
  ];
  return (
    <DataTable
      embedded={embedded}
      caption={t('tabs.movements')}
      columns={columns}
      data={rows}
      getRowId={(m) => m.id}
      pageSize={0}
      columnToggle={false}
      empty={<EmptyState icon={ArrowDownLeftIcon} title={t('movement.empty')} bordered={false} />}
    />
  );
}
