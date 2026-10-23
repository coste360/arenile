import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { Separator } from '@arenile/shared/ui/separator';
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
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('close.method')}</TableHead>
            <TableHead className="text-right">{t('z.takings')}</TableHead>
            <TableHead className="text-right">{t('close.expected')}</TableHead>
            {closed && <TableHead className="text-right">{t('close.counted')}</TableHead>}
            {closed && <TableHead className="text-right">{t('close.difference')}</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {methods.map((m) => (
            <TableRow key={m}>
              <TableCell>{t(`method.${m}`)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {ctx.money(takings[m] ?? 0)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {ctx.money(expected[m] ?? 0)}
              </TableCell>
              {closed && (
                <TableCell className="text-right tabular-nums">
                  {counted[m] === undefined ? '—' : ctx.money(counted[m])}
                </TableCell>
              )}
              {closed && (
                <TableCell className="text-right">
                  <DifferenceAmount cents={diff.byMethod[m] ?? 0} money={ctx.money} badge={false} />
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell>{t('close.total')}</TableCell>
            <TableCell className="text-right tabular-nums">
              {ctx.money(sumTotals(takings))}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {ctx.money(sumTotals(expected))}
            </TableCell>
            {closed && (
              <TableCell className="text-right tabular-nums">
                {ctx.money(sumTotals(counted))}
              </TableCell>
            )}
            {closed && (
              <TableCell className="text-right">
                <DifferenceAmount cents={session.difference ?? diff.total} money={ctx.money} />
              </TableCell>
            )}
          </TableRow>
        </TableFooter>
      </Table>
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
