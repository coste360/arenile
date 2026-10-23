import { formatIsoDate } from '@arenile/shared/lib/dates';
import { DataTable, QueryState, type ColumnDef } from '@arenile/shared/ui/patterns';
import {
  BanknoteIcon,
  CalendarCheckIcon,
  CalendarXIcon,
  PercentIcon,
  ReceiptTextIcon,
  TicketIcon,
  UserXIcon,
  WalletIcon,
  type LucideIcon,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  reportCsvUrl,
  useReportSummary,
  useYearComparisonReport,
  type ComparisonAlign,
  type ComparisonParams,
  type ComparisonPoint,
  type ReportSummary,
} from '../api';
import { ReportChart } from '../components/ReportChart';
import { ChoiceSelect, ReportSection } from '../components/ReportSection';
import { StatCard, type StatDelta } from '../components/StatCard';
import { numberColumn, textColumn } from '../components/columns';
import {
  deltaLabel,
  deltaTone,
  formatCount,
  formatRatioBP,
  metricsByKey,
  type ComparisonMetric,
  type ComparisonMetricKey,
} from '../lib/delta';
import {
  comparisonGranularity,
  compareYear,
  csvFileName,
  type ResolvedPeriod,
} from '../lib/period';
import type { ReportsContext } from '../useReportsContext';

export interface SectionProps {
  ctx: ReportsContext;
  period: ResolvedPeriod;
  /** Fetch only while the tab is shown. */
  active: boolean;
}

/** Date basis of a card (report.buildFacts): printed under the value. */
type Basis = 'stay' | 'cash';

interface Kpi {
  key: ComparisonMetricKey | 'open_balance';
  icon: LucideIcon;
  value: (s: ReportSummary) => string;
  hint?: (s: ReportSummary) => string;
  basis?: Basis;
}

/**
 * "Riepilogo": KPI cards of the period and, with the comparison switch on,
 * year-over-year deltas (server-computed ComparisonMetric) plus the
 * revenue series of both years as a chart and a table. Every card prints
 * its date basis (stay or cash, see report.buildFacts), so figures sitting
 * side by side are never read as if they shared one.
 */
export function SummarySection({
  ctx,
  period,
  active,
  compare,
}: SectionProps & { compare: boolean }) {
  const { t } = useTranslation('reports');
  const summary = useReportSummary(ctx.establishmentId, period, active);
  const year = compareYear(period);
  const [align, setAlign] = useState<ComparisonAlign>('calendar');
  const comparisonParams = compare
    ? { ...period, compare_year: year, granularity: comparisonGranularity(period), align }
    : undefined;
  const comparison = useYearComparisonReport(ctx.establishmentId, comparisonParams, active);
  const metrics = metricsByKey(compare ? comparison.data?.metrics : undefined);

  const money = ctx.money;
  const kpis: Kpi[] = [
    {
      key: 'occupancy_bp',
      icon: PercentIcon,
      value: (s) => formatRatioBP(s.occupancy_bp),
      hint: (s) =>
        t('summary.unitsHint', {
          occupied: formatCount(s.occupied_units),
          capacity: formatCount(s.capacity_units),
        }),
    },
    {
      key: 'revenue_collected',
      basis: 'cash',
      icon: BanknoteIcon,
      value: (s) => money(s.revenue_collected),
    },
    {
      key: 'revenue_booked',
      basis: 'stay',
      icon: ReceiptTextIcon,
      value: (s) => money(s.revenue_booked),
    },
    {
      key: 'bookings',
      basis: 'stay',
      icon: CalendarCheckIcon,
      value: (s) => formatCount(s.bookings),
    },
    {
      key: 'average_booking_value',
      basis: 'stay',
      icon: WalletIcon,
      value: (s) => money(s.average_booking_value),
    },
    {
      key: 'cancellations',
      basis: 'stay',
      icon: CalendarXIcon,
      value: (s) => formatCount(s.cancellations),
    },
    {
      key: 'no_shows',
      basis: 'stay',
      icon: UserXIcon,
      value: (s) => formatCount(s.no_shows),
    },
    {
      key: 'season_passes_sold',
      basis: 'stay',
      icon: TicketIcon,
      value: (s) => formatCount(s.season_passes_sold),
    },
    {
      key: 'open_balance',
      icon: WalletIcon,
      value: (s) => money(s.open_balance_total),
      hint: (s) => t('summary.openBalanceHint', { n: formatCount(s.open_balance_bookings) }),
    },
  ];

  const formatMetric = (m: ComparisonMetric, v: number) =>
    m.unit === 'cents' ? money(v) : m.unit === 'basis_points' ? formatRatioBP(v) : formatCount(v);

  const deltaOf = (key: Kpi['key']): StatDelta | undefined => {
    if (key === 'open_balance') return undefined;
    const m = metrics[key];
    if (!m) return undefined;
    return {
      text: deltaLabel(m, t('summary.new')),
      tone: deltaTone(m),
      direction: m.delta > 0 ? 'up' : m.delta < 0 ? 'down' : 'flat',
      reference: t('summary.vs', { year, value: formatMetric(m, m.previous) }),
    };
  };

  return (
    <div className="space-y-6">
      {summary.error ? (
        <QueryState
          isLoading={false}
          error={summary.error}
          onRetry={() => void summary.refetch()}
        />
      ) : !summary.data ? (
        <QueryState isLoading error={null} rows={4} />
      ) : (
        <section aria-label={t('summary.kpis')} className="grid grid-cols-2 gap-4 lg:grid-cols-3">
          {kpis.map((k) => (
            <StatCard
              key={k.key}
              label={t(`metric.${k.key}`)}
              value={k.value(summary.data)}
              icon={k.icon}
              hint={[k.hint?.(summary.data), k.basis && t(`summary.basis.${k.basis}`)]
                .filter(Boolean)
                .join(' · ')}
              delta={deltaOf(k.key)}
            />
          ))}
        </section>
      )}
      {compare && (
        <ComparisonCard
          ctx={ctx}
          year={year}
          align={align}
          onAlignChange={setAlign}
          params={comparisonParams}
          query={comparison}
          formatMetric={formatMetric}
        />
      )}
    </div>
  );
}

function ComparisonCard({
  ctx,
  year,
  align,
  onAlignChange,
  params,
  query,
  formatMetric,
}: {
  ctx: ReportsContext;
  year: number;
  align: ComparisonAlign;
  onAlignChange: (align: ComparisonAlign) => void;
  params: ComparisonParams | undefined;
  query: ReturnType<typeof useYearComparisonReport>;
  formatMetric: (m: ComparisonMetric, v: number) => string;
}) {
  const { t } = useTranslation('reports');
  const data = query.data;
  const currentYear = year + 1;
  const series = useMemo(
    () =>
      (data?.series ?? []).map((p) => ({
        label: p.label,
        current: p.current_revenue,
        previous: p.previous_revenue,
      })),
    [data],
  );
  const pointColumns: ColumnDef<ComparisonPoint>[] = [
    textColumn('label', t('comparison.bucket'), (p) => p.label),
    numberColumn(
      'current_revenue',
      t('comparison.revenueOf', { year: currentYear }),
      (p) => p.current_revenue,
      ctx.money,
    ),
    numberColumn(
      'previous_revenue',
      t('comparison.revenueOf', { year }),
      (p) => p.previous_revenue,
      ctx.money,
    ),
    numberColumn(
      'current_bookings',
      t('comparison.bookingsOf', { year: currentYear }),
      (p) => p.current_bookings,
      formatCount,
    ),
    numberColumn(
      'previous_bookings',
      t('comparison.bookingsOf', { year }),
      (p) => p.previous_bookings,
      formatCount,
    ),
    numberColumn(
      'current_occupancy',
      t('comparison.occupancyOf', { year: currentYear }),
      (p) => p.current_occupancy_bp,
      formatRatioBP,
    ),
    numberColumn(
      'previous_occupancy',
      t('comparison.occupancyOf', { year }),
      (p) => p.previous_occupancy_bp,
      formatRatioBP,
    ),
  ];
  const deltaText = (m: ComparisonMetric) => deltaLabel(m, t('summary.new'));
  const metricColumns: ColumnDef<ComparisonMetric>[] = [
    textColumn('metric', t('comparison.metric'), (m) => t(`metric.${m.metric}`)),
    numberColumn(
      'current',
      String(currentYear),
      (m) => m.current,
      () => '',
    ),
    numberColumn(
      'previous',
      String(year),
      (m) => m.previous,
      () => '',
    ),
    numberColumn(
      'delta',
      t('comparison.delta'),
      (m) => m.delta,
      () => '',
    ),
  ];
  // Values of different units share a column: format each row by its unit.
  const render: Record<string, (m: ComparisonMetric) => string> = {
    current: (m) => formatMetric(m, m.current),
    previous: (m) => formatMetric(m, m.previous),
    delta: deltaText,
  };
  for (const c of metricColumns) {
    const fmt = c.id ? render[c.id] : undefined;
    if (fmt) {
      c.cell = ({ row }) => (
        <span className="block text-right whitespace-nowrap tabular-nums">{fmt(row.original)}</span>
      );
    }
  }

  return (
    <ReportSection
      title={t('comparison.title', { year })}
      description={
        data
          ? t('comparison.description', {
              currentFrom: formatIsoDate(data.current.from),
              currentTo: formatIsoDate(data.current.to),
              previousFrom: formatIsoDate(data.previous.from),
              previousTo: formatIsoDate(data.previous.to),
            })
          : undefined
      }
      csv={
        params
          ? {
              href: reportCsvUrl(ctx.establishmentId, 'comparison', params),
              fileName: csvFileName('comparison', params),
            }
          : undefined
      }
      controls={
        <ChoiceSelect
          label={t('comparison.align')}
          value={align}
          onChange={onAlignChange}
          options={(['calendar', 'weekday'] as const).map((a) => ({
            value: a,
            label: t(`comparison.alignOption.${a}`),
          }))}
        />
      }
      query={query}
    >
      {data && (
        <div className="space-y-6">
          <ReportChart
            kind="line"
            data={series}
            categoryKey="label"
            series={[
              { key: 'current', label: String(currentYear), tone: 1 },
              { key: 'previous', label: String(year), tone: 5, dashed: true },
            ]}
            format={ctx.money}
            caption={t('chart.caption')}
          />
          <DataTable
            caption={t('comparison.metricsCaption')}
            columns={metricColumns}
            data={data.metrics}
            getRowId={(m) => m.metric}
            pageSize={0}
            columnToggle={false}
          />
          <DataTable
            caption={t('comparison.seriesCaption')}
            columns={pointColumns}
            data={data.series}
            getRowId={(p) => p.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}
