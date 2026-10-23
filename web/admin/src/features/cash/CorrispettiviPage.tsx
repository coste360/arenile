import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Label } from '@arenile/shared/ui/label';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { DatePicker, EmptyState, PageHeader, QueryState } from '@arenile/shared/ui/patterns';
import { dateToIso, formatIsoDate } from '@arenile/shared/lib/dates';
import { formatBasisPoints } from '@arenile/shared/lib/money';
import { Link } from '@tanstack/react-router';
import { ArrowLeftIcon, DownloadIcon, InfoIcon, LockIcon, ReceiptTextIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCorrispettivi } from './api';
import {
  exportFileName,
  exportUrl,
  periodQuery,
  sumVat,
  totalsByMethod,
  type CorrispettiviReport,
  type Period,
} from './lib/corrispettivi';
import { useCashContext, type CashContext } from './useCashContext';

function currentMonth(today: string): string {
  return today.slice(0, 7);
}

/**
 * Corrispettivi (spec 14.2, driver "export"): takings of a day or a month by
 * VAT rate (imponibile, imposta, totale) and by payment method, with the
 * CSV / JSON export produced by the server for the accountant.
 */
export function CorrispettiviPage() {
  const { t } = useTranslation('cash');
  const ctx = useCashContext();
  const [today] = useState(() => dateToIso(new Date()));
  const [mode, setMode] = useState<Period['mode']>('day');
  const [date, setDate] = useState(today);
  const [month, setMonth] = useState(() => currentMonth(today));
  const period: Period = mode === 'day' ? { mode, date } : { mode, month };
  const q = periodQuery(period);
  const report = useCorrispettivi(ctx.establishmentId, q, ctx.canReport);

  const back = (
    <Button variant="ghost" size="sm" asChild className="-ml-2 w-fit">
      <Link to="/e/$establishmentId/cassa" params={{ establishmentId: ctx.establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('session.title')}
      </Link>
    </Button>
  );

  if (ctx.loading) return <QueryState isLoading error={null} rows={4} />;
  if (!ctx.canReport) {
    return (
      <section aria-labelledby="page-title" className="space-y-6">
        <PageHeader
          title={t('corrispettivi.title')}
          description={t('corrispettivi.description')}
          eyebrow={back}
        />
        <Alert>
          <LockIcon aria-hidden="true" />
          <AlertTitle>{t('perm.forbiddenTitle')}</AlertTitle>
          <AlertDescription>{t('perm.reportsForbidden')}</AlertDescription>
        </Alert>
      </section>
    );
  }

  const downloads = (['csv', 'json'] as const).map((format) => (
    <Button key={format} variant={format === 'csv' ? 'default' : 'outline'} asChild>
      <a href={exportUrl(ctx.establishmentId, q, format)} download={exportFileName(q, format)}>
        <DownloadIcon aria-hidden="true" />
        {t(format === 'csv' ? 'corrispettivi.downloadCsv' : 'corrispettivi.downloadJson')}
      </a>
    </Button>
  ));

  return (
    <section aria-labelledby="page-title" className="space-y-6">
      <PageHeader
        title={t('corrispettivi.title')}
        description={t('corrispettivi.description')}
        eyebrow={back}
        actions={downloads}
      />
      <div className="flex flex-wrap items-end gap-4">
        <div className="space-y-2">
          <span id="corr-mode-label" className="block text-sm leading-none font-medium">
            {t('corrispettivi.period')}
          </span>
          <ToggleGroup
            type="single"
            variant="outline"
            aria-labelledby="corr-mode-label"
            value={mode}
            onValueChange={(v) => v && setMode(v as Period['mode'])}
          >
            <ToggleGroupItem value="day">{t('corrispettivi.day')}</ToggleGroupItem>
            <ToggleGroupItem value="month">{t('corrispettivi.month')}</ToggleGroupItem>
          </ToggleGroup>
        </div>
        {mode === 'day' ? (
          <div className="space-y-2">
            <Label htmlFor="corr-date">{t('corrispettivi.date')}</Label>
            <DatePicker id="corr-date" value={date} onChange={(v) => v && setDate(v)} />
          </div>
        ) : (
          <MonthPicker value={month} onChange={setMonth} today={today} />
        )}
      </div>

      {report.error ? (
        <QueryState isLoading={false} error={report.error} onRetry={() => void report.refetch()} />
      ) : report.isPending ? (
        <QueryState isLoading error={null} rows={5} />
      ) : (
        <ReportView report={report.data} ctx={ctx} />
      )}

      <Alert>
        <InfoIcon aria-hidden="true" />
        <AlertDescription>{t('corrispettivi.disclaimer')}</AlertDescription>
      </Alert>
    </section>
  );
}

function MonthPicker({
  value,
  onChange,
  today,
}: {
  value: string;
  onChange: (month: string) => void;
  today: string;
}) {
  const { t, i18n } = useTranslation('cash');
  const [year, mm] = value.split('-');
  const thisYear = Number(today.slice(0, 4));
  const years = [thisYear - 2, thisYear - 1, thisYear, thisYear + 1];
  const monthName = new Intl.DateTimeFormat(i18n.language, { month: 'long', timeZone: 'UTC' });
  return (
    <div className="flex items-end gap-2">
      <div className="space-y-2">
        <Label htmlFor="corr-month">{t('corrispettivi.monthLabel')}</Label>
        <NativeSelect
          id="corr-month"
          value={mm}
          onChange={(e) => onChange(`${year}-${e.target.value}`)}
        >
          {Array.from({ length: 12 }, (_, i) => {
            const v = String(i + 1).padStart(2, '0');
            return (
              <NativeSelectOption key={v} value={v}>
                {monthName.format(new Date(Date.UTC(2000, i, 1)))}
              </NativeSelectOption>
            );
          })}
        </NativeSelect>
      </div>
      <div className="space-y-2">
        <Label htmlFor="corr-year">{t('corrispettivi.year')}</Label>
        <NativeSelect
          id="corr-year"
          value={year}
          onChange={(e) => onChange(`${e.target.value}-${mm}`)}
        >
          {years.map((y) => (
            <NativeSelectOption key={y} value={String(y)}>
              {y}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
    </div>
  );
}

function ReportView({
  report,
  ctx,
}: {
  report: CorrispettiviReport;
  ctx: Pick<CashContext, 'money'>;
}) {
  const { t } = useTranslation('cash');
  if (report.rows.length === 0 && report.totals.length === 0) {
    return <EmptyState icon={ReceiptTextIcon} title={t('corrispettivi.empty')} />;
  }
  const vat = [...report.totals].sort((a, b) => b.vat_rate_bp - a.vat_rate_bp);
  const vatSum = sumVat(vat);
  const methods = totalsByMethod(report.rows);
  const rows = [...report.rows].sort(
    (a, b) =>
      a.period.localeCompare(b.period) ||
      b.vat_rate_bp - a.vat_rate_bp ||
      a.method.localeCompare(b.method),
  );
  const periodLabel = (p: string) => (p.length === 10 ? formatIsoDate(p) : p);
  const span =
    report.from === report.to
      ? formatIsoDate(report.from)
      : `${formatIsoDate(report.from)} – ${formatIsoDate(report.to)}`;

  return (
    <div className="space-y-6">
      <Card className="gap-1 py-4">
        <CardHeader className="px-4">
          <CardDescription>{t('corrispettivi.grandTotal', { period: span })}</CardDescription>
        </CardHeader>
        <CardContent className="px-4">
          <p
            data-testid="grand-total"
            className="text-3xl font-semibold tracking-tight tabular-nums"
          >
            {ctx.money(report.grand_total)}
          </p>
        </CardContent>
      </Card>

      <div className="grid items-start gap-6 lg:grid-cols-2">
        <Card className="gap-3">
          <CardHeader>
            <CardTitle>{t('corrispettivi.byVat')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <caption className="sr-only">{t('corrispettivi.byVat')}</caption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('corrispettivi.rate')}</TableHead>
                  <TableHead className="text-right">{t('corrispettivi.taxable')}</TableHead>
                  <TableHead className="text-right">{t('corrispettivi.tax')}</TableHead>
                  <TableHead className="text-right">{t('corrispettivi.gross')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {vat.map((v) => (
                  <TableRow key={v.vat_rate_bp}>
                    <TableCell>{formatBasisPoints(v.vat_rate_bp)}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {ctx.money(v.taxable)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{ctx.money(v.tax)}</TableCell>
                    <TableCell className="text-right tabular-nums">{ctx.money(v.gross)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell>{t('corrispettivi.total')}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {ctx.money(vatSum.taxable)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{ctx.money(vatSum.tax)}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {ctx.money(vatSum.gross)}
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </CardContent>
        </Card>

        <Card className="gap-3">
          <CardHeader>
            <CardTitle>{t('corrispettivi.byMethod')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <caption className="sr-only">{t('corrispettivi.byMethod')}</caption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('fields.method')}</TableHead>
                  <TableHead className="text-right">{t('corrispettivi.count')}</TableHead>
                  <TableHead className="text-right">{t('corrispettivi.gross')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {methods.map((m) => (
                  <TableRow key={m.method}>
                    <TableCell>{t(`method.${m.method}`)}</TableCell>
                    <TableCell className="text-right tabular-nums">{m.count}</TableCell>
                    <TableCell className="text-right tabular-nums">{ctx.money(m.gross)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell>{t('corrispettivi.total')}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {methods.reduce((n, m) => n + m.count, 0)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {ctx.money(methods.reduce((n, m) => n + m.gross, 0))}
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </CardContent>
        </Card>
      </div>

      <Card className="gap-3">
        <CardHeader>
          <CardTitle>{t('corrispettivi.detail')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <caption className="sr-only">{t('corrispettivi.detail')}</caption>
            <TableHeader>
              <TableRow>
                <TableHead>{t('corrispettivi.periodColumn')}</TableHead>
                <TableHead>{t('corrispettivi.rate')}</TableHead>
                <TableHead>{t('fields.method')}</TableHead>
                <TableHead className="text-right">{t('corrispettivi.count')}</TableHead>
                <TableHead className="text-right">{t('corrispettivi.taxable')}</TableHead>
                <TableHead className="text-right">{t('corrispettivi.tax')}</TableHead>
                <TableHead className="text-right">{t('corrispettivi.gross')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={`${r.period}-${r.vat_rate_bp}-${r.method}`}>
                  <TableCell className="tabular-nums">{periodLabel(r.period)}</TableCell>
                  <TableCell>{formatBasisPoints(r.vat_rate_bp)}</TableCell>
                  <TableCell>{t(`method.${r.method}`)}</TableCell>
                  <TableCell className="text-right tabular-nums">{r.count}</TableCell>
                  <TableCell className="text-right tabular-nums">{ctx.money(r.taxable)}</TableCell>
                  <TableCell className="text-right tabular-nums">{ctx.money(r.tax)}</TableCell>
                  <TableCell className="text-right tabular-nums">{ctx.money(r.gross)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
