/**
 * Grouped reports with a chart and a table: occupancy, revenue, bookings,
 * cancellations and season passes. Each section owns its grouping state
 * (kept while switching tabs, the panels stay mounted) and exports exactly
 * the grouping it shows.
 */
import { DataTable, type ColumnDef } from '@arenile/shared/ui/patterns';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  reportCsvUrl,
  useBookingsReport,
  useCancellationsReport,
  useOccupancyReport,
  useRevenueReport,
  useSeasonPassesReport,
  type BookingsReportGroupBy,
  type BookingsReportRow,
  type CancellationsGroupBy,
  type CancellationsRow,
  type OccupancyGroupBy,
  type OccupancyRow,
  type ReportName,
  type RevenueBasis,
  type RevenueGroupBy,
  type RevenueRow,
  type SeasonPassesGroupBy,
  type SeasonPassesRow,
} from '../api';
import { ReportChart } from '../components/ReportChart';
import { ChoiceSelect, ReportSection } from '../components/ReportSection';
import { StatCard } from '../components/StatCard';
import { numberColumn, textColumn } from '../components/columns';
import { formatCount, formatRatioBP } from '../lib/delta';
import { csvFileName } from '../lib/period';
import { revenueGroupAllowed } from '../lib/revenue';
import type { SectionProps } from './SummarySection';

/** Groupings over time are drawn as lines, categories as bars. */
const TIME_KEYS = new Set(['day', 'week', 'month']);

function csvOf(
  ctx: SectionProps['ctx'],
  name: ReportName,
  params: Record<string, string | number> & { from: string; to: string },
) {
  return {
    href: reportCsvUrl(ctx.establishmentId, name, params),
    fileName: csvFileName(name, params),
  };
}

// ---- Occupancy ---------------------------------------------------------------

const OCCUPANCY_GROUPS: readonly OccupancyGroupBy[] = ['day', 'zone', 'row', 'slot'];

export function OccupancySection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [groupBy, setGroupBy] = useState<OccupancyGroupBy>('day');
  const params = { ...period, group_by: groupBy };
  const q = useOccupancyReport(ctx.establishmentId, params, active);
  const columns: ColumnDef<OccupancyRow>[] = [
    textColumn('label', t(`groupBy.${groupBy}`), (r) => r.label),
    numberColumn('occupancy', t('occupancy.rate'), (r) => r.occupancy_bp, formatRatioBP),
    numberColumn('occupied', t('occupancy.occupied'), (r) => r.occupied_units, formatCount),
    numberColumn('capacity', t('occupancy.capacity'), (r) => r.capacity_units, formatCount),
    numberColumn('season', t('occupancy.season'), (r) => r.season_units, formatCount),
    numberColumn('released', t('occupancy.released'), (r) => r.released_units, formatCount),
    numberColumn('blocked', t('occupancy.blocked'), (r) => r.blocked_units, formatCount),
  ];
  const totals = q.data?.totals;
  return (
    <ReportSection
      title={t('tabs.occupancy')}
      description={
        totals
          ? t('occupancy.total', {
              rate: formatRatioBP(totals.occupancy_bp),
              occupied: formatCount(totals.occupied_units),
              capacity: formatCount(totals.capacity_units),
            })
          : t('occupancy.help')
      }
      csv={csvOf(ctx, 'occupancy', params)}
      controls={
        <ChoiceSelect
          label={t('groupBy.label')}
          value={groupBy}
          onChange={setGroupBy}
          options={OCCUPANCY_GROUPS.map((g) => ({ value: g, label: t(`groupBy.${g}`) }))}
        />
      }
      query={q}
    >
      {q.data && (
        <div className="space-y-6">
          {q.data.rows.length > 0 && (
            <ReportChart
              kind={TIME_KEYS.has(groupBy) ? 'line' : 'bar'}
              data={q.data.rows.map((r) => ({ label: r.label, occupancy: r.occupancy_bp }))}
              categoryKey="label"
              series={[{ key: 'occupancy', label: t('occupancy.rate'), tone: 1 }]}
              format={formatRatioBP}
              caption={t('chart.caption')}
            />
          )}
          <DataTable
            caption={t('tabs.occupancy')}
            columns={columns}
            data={q.data.rows}
            getRowId={(r) => r.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}

// ---- Revenue -----------------------------------------------------------------

const REVENUE_GROUPS: readonly RevenueGroupBy[] = [
  'day',
  'week',
  'month',
  'channel',
  'pack',
  'extra',
  'payment_method',
  'vat_rate',
];

export function RevenueSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [basis, setBasis] = useState<RevenueBasis>('collected');
  const [groupBy, setGroupBy] = useState<RevenueGroupBy>('month');
  const params = { ...period, group_by: groupBy, basis };
  const q = useRevenueReport(ctx.establishmentId, params, active);
  const changeBasis = (b: RevenueBasis) => {
    setBasis(b);
    if (!revenueGroupAllowed(groupBy, b)) setGroupBy('month');
  };
  const columns: ColumnDef<RevenueRow>[] = [
    textColumn('label', t(`groupBy.${groupBy}`), (r) => r.label),
    numberColumn(
      'count',
      t(basis === 'collected' ? 'revenue.movements' : 'revenue.lines'),
      (r) => r.count,
      formatCount,
    ),
    numberColumn('gross', t('revenue.gross'), (r) => r.gross, ctx.money),
    numberColumn('taxable', t('revenue.taxable'), (r) => r.taxable, ctx.money),
    numberColumn('tax', t('revenue.tax'), (r) => r.tax, ctx.money),
    ...(basis === 'collected'
      ? [numberColumn<RevenueRow>('refunds', t('revenue.refunds'), (r) => r.refunds, ctx.money)]
      : []),
  ];
  const totals = q.data?.totals;
  return (
    <ReportSection
      title={t('tabs.revenue')}
      description={
        totals
          ? t('revenue.total', { gross: ctx.money(totals.gross), tax: ctx.money(totals.tax) })
          : undefined
      }
      csv={csvOf(ctx, 'revenue', params)}
      controls={
        <>
          <ChoiceSelect
            label={t('revenue.basis')}
            value={basis}
            onChange={changeBasis}
            options={(['collected', 'booked'] as const).map((b) => ({
              value: b,
              label: t(`revenue.basisOption.${b}`),
            }))}
          />
          <ChoiceSelect
            label={t('groupBy.label')}
            value={groupBy}
            onChange={setGroupBy}
            options={REVENUE_GROUPS.map((g) => ({
              value: g,
              label: t(`groupBy.${g}`),
              disabled: !revenueGroupAllowed(g, basis),
            }))}
          />
        </>
      }
      query={q}
    >
      {q.data && (
        <div className="space-y-6">
          <p className="text-sm text-muted-foreground">{t(`revenue.basisHelp.${basis}`)}</p>
          {q.data.rows.length > 0 && (
            <ReportChart
              kind={TIME_KEYS.has(groupBy) ? 'line' : 'bar'}
              data={q.data.rows.map((r) => ({ label: r.label, gross: r.gross }))}
              categoryKey="label"
              series={[{ key: 'gross', label: t('revenue.gross'), tone: 1 }]}
              format={ctx.money}
              caption={t('chart.caption')}
            />
          )}
          <DataTable
            caption={t('tabs.revenue')}
            columns={columns}
            data={q.data.rows}
            getRowId={(r) => r.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}

// ---- Bookings ----------------------------------------------------------------

const BOOKINGS_GROUPS: readonly BookingsReportGroupBy[] = [
  'channel',
  'lead_time',
  'formula',
  'day',
];

export function BookingsSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [groupBy, setGroupBy] = useState<BookingsReportGroupBy>('channel');
  const params = { ...period, group_by: groupBy };
  const q = useBookingsReport(ctx.establishmentId, params, active);
  const columns: ColumnDef<BookingsReportRow>[] = [
    textColumn('label', t(`groupBy.${groupBy}`), (r) => r.label),
    numberColumn('bookings', t('bookings.count'), (r) => r.bookings, formatCount),
    numberColumn('share', t('bookings.share'), (r) => r.share_bp, formatRatioBP),
    numberColumn('value', t('bookings.value'), (r) => r.value, ctx.money),
    numberColumn('average', t('bookings.average'), (r) => r.average_value, ctx.money),
    numberColumn('lead', t('bookings.lead'), (r) => r.average_lead_days, formatCount),
  ];
  const totals = q.data?.totals;
  return (
    <ReportSection
      title={t('tabs.bookings')}
      description={
        totals
          ? t('bookings.total', {
              n: formatCount(totals.bookings),
              value: ctx.money(totals.value),
              average: ctx.money(totals.average_value),
            })
          : undefined
      }
      csv={csvOf(ctx, 'bookings', params)}
      controls={
        <ChoiceSelect
          label={t('groupBy.label')}
          value={groupBy}
          onChange={setGroupBy}
          options={BOOKINGS_GROUPS.map((g) => ({ value: g, label: t(`groupBy.${g}`) }))}
        />
      }
      query={q}
    >
      {q.data && (
        <div className="space-y-6">
          {q.data.rows.length > 0 && (
            <ReportChart
              kind={groupBy === 'day' ? 'line' : 'bar'}
              data={q.data.rows.map((r) => ({ label: r.label, bookings: r.bookings }))}
              categoryKey="label"
              series={[{ key: 'bookings', label: t('bookings.count'), tone: 1 }]}
              format={formatCount}
              caption={t('chart.caption')}
            />
          )}
          <DataTable
            caption={t('tabs.bookings')}
            columns={columns}
            data={q.data.rows}
            getRowId={(r) => r.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}

// ---- Cancellations and no-shows ---------------------------------------------

const CANCELLATION_GROUPS: readonly CancellationsGroupBy[] = ['day', 'month', 'channel'];

export function CancellationsSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [groupBy, setGroupBy] = useState<CancellationsGroupBy>('month');
  const params = { ...period, group_by: groupBy };
  const q = useCancellationsReport(ctx.establishmentId, params, active);
  const columns: ColumnDef<CancellationsRow>[] = [
    textColumn('label', t(`groupBy.${groupBy}`), (r) => r.label),
    numberColumn('bookings', t('cancellations.bookings'), (r) => r.bookings, formatCount),
    numberColumn('cancelled', t('cancellations.cancelled'), (r) => r.cancelled, formatCount),
    numberColumn(
      'cancelled_bp',
      t('cancellations.cancelledRate'),
      (r) => r.cancelled_bp,
      formatRatioBP,
    ),
    numberColumn('no_shows', t('cancellations.noShows'), (r) => r.no_shows, formatCount),
    numberColumn('no_show_bp', t('cancellations.noShowRate'), (r) => r.no_show_bp, formatRatioBP),
    numberColumn('lost', t('cancellations.lost'), (r) => r.lost_value, ctx.money),
    numberColumn('refunded', t('cancellations.refunded'), (r) => r.refunded, ctx.money),
  ];
  const totals = q.data?.totals;
  return (
    <ReportSection
      title={t('tabs.cancellations')}
      description={
        totals
          ? t('cancellations.total', {
              cancelled: formatRatioBP(totals.cancelled_bp),
              noShows: formatRatioBP(totals.no_show_bp),
              lost: ctx.money(totals.lost_value),
            })
          : undefined
      }
      csv={csvOf(ctx, 'cancellations', params)}
      controls={
        <ChoiceSelect
          label={t('groupBy.label')}
          value={groupBy}
          onChange={setGroupBy}
          options={CANCELLATION_GROUPS.map((g) => ({ value: g, label: t(`groupBy.${g}`) }))}
        />
      }
      query={q}
    >
      {q.data && (
        <div className="space-y-6">
          {q.data.rows.length > 0 && (
            <ReportChart
              kind="bar"
              data={q.data.rows.map((r) => ({
                label: r.label,
                cancelled: r.cancelled,
                no_shows: r.no_shows,
              }))}
              categoryKey="label"
              series={[
                { key: 'cancelled', label: t('cancellations.cancelled'), tone: 1 },
                { key: 'no_shows', label: t('cancellations.noShows'), tone: 4 },
              ]}
              format={formatCount}
              caption={t('chart.caption')}
            />
          )}
          <DataTable
            caption={t('tabs.cancellations')}
            columns={columns}
            data={q.data.rows}
            getRowId={(r) => r.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}

// ---- Season passes and releases ---------------------------------------------

const PASS_GROUPS: readonly SeasonPassesGroupBy[] = ['day', 'month', 'zone'];

export function SeasonPassesSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [groupBy, setGroupBy] = useState<SeasonPassesGroupBy>('month');
  const params = { ...period, group_by: groupBy };
  const q = useSeasonPassesReport(ctx.establishmentId, params, active);
  const columns: ColumnDef<SeasonPassesRow>[] = [
    textColumn('label', t(`groupBy.${groupBy}`), (r) => r.label),
    numberColumn('released', t('passes.released'), (r) => r.released_units, formatCount),
    numberColumn('resold', t('passes.resold'), (r) => r.resold_units, formatCount),
    numberColumn('resale_bp', t('passes.resaleRate'), (r) => r.resale_bp, formatRatioBP),
    numberColumn('resale_revenue', t('passes.resaleRevenue'), (r) => r.resale_revenue, ctx.money),
    numberColumn('credits', t('passes.credits'), (r) => r.credits_issued, ctx.money),
  ];
  return (
    <ReportSection
      title={t('tabs.passes')}
      description={t('passes.help')}
      csv={csvOf(ctx, 'season-passes', params)}
      controls={
        <ChoiceSelect
          label={t('groupBy.label')}
          value={groupBy}
          onChange={setGroupBy}
          options={PASS_GROUPS.map((g) => ({ value: g, label: t(`groupBy.${g}`) }))}
        />
      }
      query={q}
    >
      {q.data && (
        <div className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-3">
            <StatCard title={t('passes.sold')} value={formatCount(q.data.passes_sold)} />
            <StatCard title={t('passes.value')} value={ctx.money(q.data.passes_value)} />
            <StatCard title={t('passes.renewals')} value={formatCount(q.data.renewals)} />
          </div>
          {q.data.rows.length > 0 && (
            <ReportChart
              kind="bar"
              data={q.data.rows.map((r) => ({
                label: r.label,
                released: r.released_units,
                resold: r.resold_units,
              }))}
              categoryKey="label"
              series={[
                { key: 'released', label: t('passes.released'), tone: 1 },
                { key: 'resold', label: t('passes.resold'), tone: 4 },
              ]}
              format={formatCount}
              caption={t('chart.caption')}
            />
          )}
          <DataTable
            caption={t('tabs.passes')}
            columns={columns}
            data={q.data.rows}
            getRowId={(r) => r.key}
            pageSize={50}
          />
        </div>
      )}
    </ReportSection>
  );
}
