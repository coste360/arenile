/**
 * List reports without a chart: best customers (name only, never contact
 * details, spec 23) and open balances with a link to each booking.
 */
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Button } from '@arenile/shared/ui/button';
import { DataTable, StatusBadge, type ColumnDef } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  reportCsvUrl,
  useOpenBalancesReport,
  useTopCustomersReport,
  type OpenBalanceRow,
  type TopCustomerRow,
  type TopCustomersSort,
} from '../api';
import { ChoiceSelect, ReportSection } from '../components/ReportSection';
import { numberColumn, textColumn } from '../components/columns';
import { formatCount } from '../lib/delta';
import { csvFileName } from '../lib/period';
import type { SectionProps } from './SummarySection';

const LIMITS = ['10', '20', '50', '100'] as const;
type Limit = (typeof LIMITS)[number];

export function TopCustomersSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const [sort, setSort] = useState<TopCustomersSort>('revenue');
  const [limit, setLimit] = useState<Limit>('20');
  const params = { ...period, sort, limit: Number(limit) };
  const q = useTopCustomersReport(ctx.establishmentId, params, active);
  const columns: ColumnDef<TopCustomerRow>[] = [
    numberColumn('rank', t('customers.rank'), (r) => r.rank, String),
    {
      id: 'name',
      accessorFn: (r) => r.display_name,
      meta: { label: t('customers.name') },
      header: t('customers.name'),
      cell: ({ row }) => (
        <Link
          to="/e/$establishmentId/clienti/$customerId"
          params={{ establishmentId: ctx.establishmentId, customerId: row.original.customer_id }}
          className="font-medium text-primary-on-surface underline-offset-4 hover:underline"
        >
          {row.original.display_name}
        </Link>
      ),
    },
    numberColumn('bookings', t('customers.bookings'), (r) => r.bookings, formatCount),
    numberColumn('spot_days', t('customers.spotDays'), (r) => r.spot_days, formatCount),
    numberColumn('revenue', t('customers.revenue'), (r) => r.revenue, ctx.money),
    textColumn('last', t('customers.last'), (r) =>
      r.last_booking_on ? formatIsoDate(r.last_booking_on) : '—',
    ),
  ];
  return (
    <ReportSection
      title={t('tabs.customers')}
      description={t('customers.help')}
      csv={{
        href: reportCsvUrl(ctx.establishmentId, 'top-customers', params),
        fileName: csvFileName('top-customers', period),
      }}
      controls={
        <>
          <ChoiceSelect
            label={t('customers.sort')}
            value={sort}
            onChange={setSort}
            options={(['revenue', 'bookings'] as const).map((s) => ({
              value: s,
              label: t(`customers.sortOption.${s}`),
            }))}
          />
          <ChoiceSelect
            label={t('customers.limit')}
            value={limit}
            onChange={setLimit}
            options={LIMITS.map((l) => ({ value: l, label: l }))}
          />
        </>
      }
      query={q}
    >
      {q.data && (
        <DataTable
          caption={t('tabs.customers')}
          columns={columns}
          data={q.data.rows}
          getRowId={(r) => r.customer_id}
          pageSize={0}
          searchable
        />
      )}
    </ReportSection>
  );
}

export function OpenBalancesSection({ ctx, period, active }: SectionProps) {
  const { t } = useTranslation('reports');
  const { t: tb } = useTranslation('bookings');
  const q = useOpenBalancesReport(ctx.establishmentId, period, active);
  const columns: ColumnDef<OpenBalanceRow>[] = [
    {
      id: 'code',
      accessorFn: (r) => r.code,
      meta: { label: t('balances.code') },
      header: t('balances.code'),
      cell: ({ row }) => (
        <Button variant="link" className="h-auto p-0 font-mono" asChild>
          <Link
            to="/e/$establishmentId/prenotazioni/$bookingId"
            params={{ establishmentId: ctx.establishmentId, bookingId: row.original.booking_id }}
          >
            {row.original.code}
          </Link>
        </Button>
      ),
    },
    textColumn('customer', t('balances.customer'), (r) => r.display_name),
    {
      id: 'status',
      accessorFn: (r) => r.status,
      meta: { label: t('balances.status') },
      header: t('balances.status'),
      cell: ({ row }) => <StatusBadge>{tb(`status.${row.original.status}`)}</StatusBadge>,
    },
    textColumn('dates', t('balances.dates'), (r) =>
      r.starts_on === r.ends_on
        ? formatIsoDate(r.starts_on)
        : `${formatIsoDate(r.starts_on)} – ${formatIsoDate(r.ends_on)}`,
    ),
    numberColumn('total', t('balances.total'), (r) => r.total_amount, ctx.money),
    numberColumn('paid', t('balances.paid'), (r) => r.paid_amount, ctx.money),
    numberColumn('due', t('balances.due'), (r) => r.balance_due, ctx.money),
  ];
  return (
    <ReportSection
      title={t('tabs.balances')}
      description={
        q.data
          ? t('balances.totalDue', {
              amount: ctx.money(q.data.total_balance),
              n: formatCount(q.data.rows.length),
            })
          : t('balances.help')
      }
      csv={{
        href: reportCsvUrl(ctx.establishmentId, 'open-balances', period),
        fileName: csvFileName('open-balances', period),
      }}
      query={q}
    >
      {q.data && (
        <DataTable
          caption={t('tabs.balances')}
          columns={columns}
          data={q.data.rows}
          getRowId={(r) => r.booking_id}
          pageSize={50}
          searchable
          initialSorting={[{ id: 'due', desc: true }]}
        />
      )}
    </ReportSection>
  );
}
