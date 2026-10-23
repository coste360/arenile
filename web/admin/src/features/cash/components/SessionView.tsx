import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import { EmptyState, StatusBadge } from '@arenile/shared/ui/patterns';
import { ArrowDownLeftIcon, ArrowUpRightIcon, ReceiptIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { CashSessionDetail } from '../api';
import {
  expectedTotals,
  methodsShown,
  netMovements,
  normalizeTotals,
  sumTotals,
} from '../lib/totals';
import type { CashContext } from '../useCashContext';

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

  return (
    <div className="space-y-6">
      <dl className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {stats.map((s) => (
          <Card key={s.id} className="gap-2 py-4">
            <CardHeader className="px-4">
              <dt>
                <CardDescription>{s.label}</CardDescription>
              </dt>
            </CardHeader>
            <CardContent className="px-4">
              <dd
                data-testid={`stat-${s.id}`}
                className="text-2xl font-semibold tracking-tight tabular-nums"
              >
                {ctx.money(s.value)}
              </dd>
              {s.hint && <p className="mt-1 text-xs text-muted-foreground">{s.hint}</p>}
            </CardContent>
          </Card>
        ))}
      </dl>

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
        <Card className="gap-3">
          <CardHeader>
            <CardTitle>{t('current.takings')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <caption className="sr-only">{t('current.takings')}</caption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('close.method')}</TableHead>
                  <TableHead className="text-right">{t('z.takings')}</TableHead>
                  <TableHead className="text-right">{t('close.expected')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {methodsShown(takings, expected).map((m) => (
                  <TableRow key={m}>
                    <TableCell>{t(`method.${m}`)}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {ctx.money(takings[m] ?? 0)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {ctx.money(expected[m] ?? 0)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {methods.length === 1 && (takings.cash ?? 0) === 0 && (
              <p className="pt-3 text-sm text-muted-foreground">{t('current.noTakings')}</p>
            )}
          </CardContent>
        </Card>

        <Card className="gap-3 lg:col-span-2">
          <CardHeader>
            <CardTitle>{t('current.activity')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="payments">
              <TabsList>
                <TabsTrigger value="payments">
                  {t('tabs.payments')} ({detail.payments.length})
                </TabsTrigger>
                <TabsTrigger value="movements">
                  {t('tabs.movements')} ({detail.movements.length})
                </TabsTrigger>
              </TabsList>
              <TabsContent value="payments" className="pt-2">
                <PaymentsTable detail={detail} ctx={ctx} />
              </TabsContent>
              <TabsContent value="movements" className="pt-2">
                <MovementsTable detail={detail} ctx={ctx} />
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

export function PaymentsTable({
  detail,
  ctx,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'time'>;
}) {
  const { t } = useTranslation('cash');
  if (detail.payments.length === 0) {
    return <EmptyState icon={ReceiptIcon} title={t('payments.empty')} bordered={false} />;
  }
  const rows = [...detail.payments].sort((a, b) => b.created_at.localeCompare(a.created_at));
  return (
    <Table>
      <caption className="sr-only">{t('tabs.payments')}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t('fields.time')}</TableHead>
          <TableHead>{t('payments.what')}</TableHead>
          <TableHead>{t('fields.method')}</TableHead>
          <TableHead className="text-right">{t('fields.amount')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((p) => {
          const refund = p.direction === 'refund';
          return (
            <TableRow key={p.id}>
              <TableCell className="tabular-nums">{ctx.time(p.created_at)}</TableCell>
              <TableCell>
                <span className="flex flex-wrap items-center gap-2">
                  {p.booking_code ? (
                    <span className="font-mono">{p.booking_code}</span>
                  ) : (
                    <span>{p.description || t('payments.counterSale')}</span>
                  )}
                  {refund && <StatusBadge tone="danger">{t('payments.refund')}</StatusBadge>}
                  {p.status !== 'succeeded' && (
                    <StatusBadge tone="neutral">{t(`payments.status.${p.status}`)}</StatusBadge>
                  )}
                </span>
              </TableCell>
              <TableCell>{t(`method.${p.method}`)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {ctx.money(refund ? -p.amount : p.amount)}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

export function MovementsTable({
  detail,
  ctx,
}: {
  detail: CashSessionDetail;
  ctx: Pick<CashContext, 'money' | 'time'>;
}) {
  const { t } = useTranslation('cash');
  if (detail.movements.length === 0) {
    return <EmptyState icon={ArrowDownLeftIcon} title={t('movement.empty')} bordered={false} />;
  }
  const rows = [...detail.movements].sort((a, b) => b.created_at.localeCompare(a.created_at));
  return (
    <Table>
      <caption className="sr-only">{t('tabs.movements')}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t('fields.time')}</TableHead>
          <TableHead>{t('movement.kind')}</TableHead>
          <TableHead>{t('movement.reason')}</TableHead>
          <TableHead className="text-right">{t('fields.amount')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((m) => (
          <TableRow key={m.id}>
            <TableCell className="tabular-nums">{ctx.time(m.created_at)}</TableCell>
            <TableCell>
              <span className="inline-flex items-center gap-1.5">
                {m.kind === 'in' ? (
                  <ArrowDownLeftIcon aria-hidden="true" className="size-4 text-success" />
                ) : (
                  <ArrowUpRightIcon aria-hidden="true" className="size-4 text-destructive" />
                )}
                {t(`movement.${m.kind}`)}
              </span>
            </TableCell>
            <TableCell className="max-w-64 truncate">{m.reason}</TableCell>
            <TableCell className="text-right tabular-nums">
              {ctx.money(m.kind === 'in' ? m.amount : -m.amount)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
