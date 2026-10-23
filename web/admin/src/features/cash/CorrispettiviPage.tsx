import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import {
  BackLink,
  DataTable,
  DatePicker,
  EmptyState,
  Page,
  PageToolbar,
  QueryState,
  SectionCard,
  StatCard,
  type ColumnDef,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
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
  type CorrispettiviRow,
  type MethodRow,
  type Period,
} from './lib/corrispettivi';
import { useCashContext, type CashContext } from './useCashContext';

type VatRow = CorrispettiviReport['totals'][number];

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
    <BackLink>
      <Link to="/e/$establishmentId/cassa" params={{ establishmentId: ctx.establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('session.title')}
      </Link>
    </BackLink>
  );

  if (ctx.loading) return <QueryState isLoading error={null} rows={4} />;
  if (!ctx.canReport) {
    return (
      <Page
        title={t('corrispettivi.title')}
        description={t('corrispettivi.description')}
        eyebrow={back}
      >
        <Alert>
          <LockIcon aria-hidden="true" />
          <AlertTitle>{t('perm.forbiddenTitle')}</AlertTitle>
          <AlertDescription>{t('perm.reportsForbidden')}</AlertDescription>
        </Alert>
      </Page>
    );
  }

  // Secondary (JSON) first, primary (CSV) last.
  const downloads = (['json', 'csv'] as const).map((format) => (
    <Button key={format} variant={format === 'csv' ? 'default' : 'outline'} asChild>
      <a href={exportUrl(ctx.establishmentId, q, format)} download={exportFileName(q, format)}>
        <DownloadIcon aria-hidden="true" />
        {t(format === 'csv' ? 'corrispettivi.downloadCsv' : 'corrispettivi.downloadJson')}
      </a>
    </Button>
  ));

  return (
    <Page
      title={t('corrispettivi.title')}
      description={t('corrispettivi.description')}
      eyebrow={back}
      actions={downloads}
    >
      <PageToolbar>
        <SegmentedControl
          label={t('corrispettivi.period')}
          value={mode}
          onValueChange={(v) => setMode(v as Period['mode'])}
          options={[
            { value: 'day', label: t('corrispettivi.day') },
            { value: 'month', label: t('corrispettivi.month') },
          ]}
        />
        {mode === 'day' ? (
          <DatePicker
            aria-label={t('corrispettivi.date')}
            value={date}
            onChange={(v) => v && setDate(v)}
          />
        ) : (
          <MonthPicker value={month} onChange={setMonth} today={today} />
        )}
      </PageToolbar>

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
    </Page>
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
    <>
      <NativeSelect
        aria-label={t('corrispettivi.monthLabel')}
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
      <NativeSelect
        aria-label={t('corrispettivi.year')}
        value={year}
        onChange={(e) => onChange(`${e.target.value}-${mm}`)}
      >
        {years.map((y) => (
          <NativeSelectOption key={y} value={String(y)}>
            {y}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </>
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
  const money = (label: string, value: (r: VatRow) => number, total: number) =>
    ({
      id: label,
      meta: { label: t(`corrispettivi.${label}`), align: 'right' },
      header: t(`corrispettivi.${label}`),
      cell: ({ row }) => ctx.money(value(row.original)),
      footer: ctx.money(total),
    }) satisfies ColumnDef<VatRow>;

  const vatColumns: ColumnDef<VatRow>[] = [
    {
      id: 'rate',
      meta: { label: t('corrispettivi.rate') },
      header: t('corrispettivi.rate'),
      cell: ({ row }) => formatBasisPoints(row.original.vat_rate_bp),
      footer: t('corrispettivi.total'),
    },
    money('taxable', (r) => r.taxable, vatSum.taxable),
    money('tax', (r) => r.tax, vatSum.tax),
    money('gross', (r) => r.gross, vatSum.gross),
  ];

  const methodColumns: ColumnDef<MethodRow>[] = [
    {
      id: 'method',
      meta: { label: t('fields.method') },
      header: t('fields.method'),
      cell: ({ row }) => t(`method.${row.original.method}`),
      footer: t('corrispettivi.total'),
    },
    {
      id: 'count',
      meta: { label: t('corrispettivi.count'), align: 'right' },
      header: t('corrispettivi.count'),
      cell: ({ row }) => row.original.count,
      footer: String(methods.reduce((n, m) => n + m.count, 0)),
    },
    {
      id: 'gross',
      meta: { label: t('corrispettivi.gross'), align: 'right' },
      header: t('corrispettivi.gross'),
      cell: ({ row }) => ctx.money(row.original.gross),
      footer: ctx.money(methods.reduce((n, m) => n + m.gross, 0)),
    },
  ];

  const detailColumns: ColumnDef<CorrispettiviRow>[] = [
    {
      id: 'period',
      meta: { label: t('corrispettivi.periodColumn') },
      header: t('corrispettivi.periodColumn'),
      cell: ({ row }) => <span className="tabular-nums">{periodLabel(row.original.period)}</span>,
    },
    {
      id: 'rate',
      meta: { label: t('corrispettivi.rate') },
      header: t('corrispettivi.rate'),
      cell: ({ row }) => formatBasisPoints(row.original.vat_rate_bp),
    },
    {
      id: 'method',
      meta: { label: t('fields.method') },
      header: t('fields.method'),
      cell: ({ row }) => t(`method.${row.original.method}`),
    },
    {
      id: 'count',
      meta: { label: t('corrispettivi.count'), align: 'right' },
      header: t('corrispettivi.count'),
      cell: ({ row }) => row.original.count,
    },
    {
      id: 'taxable',
      meta: { label: t('corrispettivi.taxable'), align: 'right' },
      header: t('corrispettivi.taxable'),
      cell: ({ row }) => ctx.money(row.original.taxable),
    },
    {
      id: 'tax',
      meta: { label: t('corrispettivi.tax'), align: 'right' },
      header: t('corrispettivi.tax'),
      cell: ({ row }) => ctx.money(row.original.tax),
    },
    {
      id: 'gross',
      meta: { label: t('corrispettivi.gross'), align: 'right' },
      header: t('corrispettivi.gross'),
      cell: ({ row }) => ctx.money(row.original.gross),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          className="col-span-2"
          label={t('corrispettivi.grandTotal', { period: span })}
          value={<span data-testid="grand-total">{ctx.money(report.grand_total)}</span>}
        />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-2">
        <SectionCard title={t('corrispettivi.byVat')} flush>
          <DataTable
            embedded
            caption={t('corrispettivi.byVat')}
            columns={vatColumns}
            data={vat}
            getRowId={(v) => String(v.vat_rate_bp)}
            pageSize={0}
            columnToggle={false}
          />
        </SectionCard>
        <SectionCard title={t('corrispettivi.byMethod')} flush>
          <DataTable
            embedded
            caption={t('corrispettivi.byMethod')}
            columns={methodColumns}
            data={methods}
            getRowId={(m) => m.method}
            pageSize={0}
            columnToggle={false}
          />
        </SectionCard>
      </div>

      <SectionCard title={t('corrispettivi.detail')} flush>
        <DataTable
          embedded
          caption={t('corrispettivi.detail')}
          columns={detailColumns}
          data={rows}
          getRowId={(r) => `${r.period}-${r.vat_rate_bp}-${r.method}`}
          pageSize={0}
          columnToggle={false}
        />
      </SectionCard>
    </div>
  );
}
