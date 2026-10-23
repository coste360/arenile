import { Separator } from '@arenile/shared/ui/separator';
import { DataTable, type ColumnDef } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import type { CashSessionDetail } from '../api';
import {
  difference,
  expectedTotals,
  methodsShown,
  netMovements,
  normalizeTotals,
  sumTotals,
} from '../lib/totals';
import type { CashContext } from '../useCashContext';
import { DifferenceAmount } from './Difference';
import { registerLabel } from '../lib/register';

/**
 * Closing summary ("chiusura Z", spec 14.1): float, takings and movements,
 * expected vs counted per method and the difference. Plain table layout that
 * reads the same on screen and on paper (print: utilities only).
 */
export function ZReport({
  detail,
  ctx,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'dateTime'>;
}) {
  const { t } = useTranslation('cash');
  const { session } = detail;
  const takings = normalizeTotals(detail.totals_by_method);
  const expected = expectedTotals(detail);
  const closed = session.status === 'closed';
  const counted = normalizeTotals(session.counted_totals);
  const diff = difference(counted, expected);
  const mv = netMovements(detail.movements);
  const methods = methodsShown(takings, expected, counted);
  type Method = (typeof methods)[number];

  const columns: ColumnDef<Method>[] = [
    {
      id: 'method',
      meta: { label: t('close.method') },
      header: t('close.method'),
      cell: ({ row }) => t(`method.${row.original}`),
      footer: t('close.total'),
    },
    {
      id: 'takings',
      meta: { label: t('z.takings'), align: 'right' },
      header: t('z.takings'),
      cell: ({ row }) => ctx.money(takings[row.original] ?? 0),
      footer: ctx.money(sumTotals(takings)),
    },
    {
      id: 'expected',
      meta: { label: t('close.expected'), align: 'right' },
      header: t('close.expected'),
      cell: ({ row }) => ctx.money(expected[row.original] ?? 0),
      footer: ctx.money(sumTotals(expected)),
    },
    ...(closed
      ? ([
          {
            id: 'counted',
            meta: { label: t('close.counted'), align: 'right' },
            header: t('close.counted'),
            cell: ({ row }) =>
              counted[row.original] === undefined ? '—' : ctx.money(counted[row.original] ?? 0),
            footer: ctx.money(sumTotals(counted)),
          },
          {
            id: 'difference',
            meta: { label: t('close.difference'), align: 'right' },
            header: t('close.difference'),
            cell: ({ row }) => (
              <DifferenceAmount
                cents={diff.byMethod[row.original] ?? 0}
                money={ctx.money}
                badge={false}
              />
            ),
            footer: () => (
              <DifferenceAmount cents={session.difference ?? diff.total} money={ctx.money} />
            ),
          },
        ] satisfies ColumnDef<Method>[])
      : []),
  ];

  return (
    <article aria-label={t('z.title')} className="space-y-4 text-sm print:text-xs print:text-black">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">{t('z.title')}</h2>
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-4">
          <dt className="text-muted-foreground print:text-black">{t('z.register')}</dt>
          <dd className="font-medium">{registerLabel(t, session.register)}</dd>
          <dt className="text-muted-foreground print:text-black">{t('z.status')}</dt>
          <dd className="font-medium">{t(`history.status.${session.status}`)}</dd>
          <dt className="text-muted-foreground print:text-black">{t('z.openedAt')}</dt>
          <dd className="tabular-nums">{ctx.dateTime(session.opened_at)}</dd>
          <dt className="text-muted-foreground print:text-black">{t('z.closedAt')}</dt>
          <dd className="tabular-nums">{closed ? ctx.dateTime(session.closed_at) : '—'}</dd>
        </dl>
      </header>
      <Separator />
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
        <div>
          <dt className="text-muted-foreground print:text-black">{t('z.float')}</dt>
          <dd className="font-medium tabular-nums">{ctx.money(session.opening_float)}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground print:text-black">{t('z.movementsIn')}</dt>
          <dd className="font-medium tabular-nums">{ctx.money(mv.in)}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground print:text-black">{t('z.movementsOut')}</dt>
          <dd className="font-medium tabular-nums">{ctx.money(mv.out)}</dd>
        </div>
      </dl>
      <DataTable
        caption={t('z.title')}
        columns={columns}
        data={methods}
        getRowId={(m) => m}
        pageSize={0}
        columnToggle={false}
      />
      {session.notes && (
        <p>
          <span className="text-muted-foreground print:text-black">{t('z.notes')}: </span>
          {session.notes}
        </p>
      )}
      <p className="hidden text-xs print:block">
        {t('z.payments', { count: detail.payments.length })}
      </p>
    </article>
  );
}
