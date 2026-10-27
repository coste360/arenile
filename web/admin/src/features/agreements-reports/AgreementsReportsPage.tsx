import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import {
  BackLink,
  DataTable,
  EmptyState,
  Page,
  QueryState,
  ResponsiveTabsList,
  SectionCard,
  StatCard,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  BuildingIcon,
  DownloadIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  NotebookTabsIcon,
  PrinterIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import type { Period } from '../partners/api';
import { usePartnerPermissions } from '../partners/lib';
import { usePeriodPicker } from '../partners/PeriodPicker';
import {
  reportFileName,
  reportFileUrl,
  useAgreementsRegister,
  usePartnersReport,
  type AgreementsRegisterCategoryTotal,
  type AgreementsRegisterEntry,
  type PartnersReportRow,
  type ReportFile,
} from './api';

type Tab = 'partners' | 'register';

/**
 * Rendiconti delle convenzioni (spec 30.2.5): report per partner (bookings,
 * people, spot-days, revenue, discount, charged and paid) and the agreements
 * register for the concession tender, with totals per register category,
 * exported to Excel and to a neutral printable page (PDF, ADR 0028).
 */
export function AgreementsReportsPage() {
  const { t } = useTranslation('agreementsReports');
  const { t: tp } = useTranslation('partners');
  const establishmentId = useRequiredEstablishmentId();
  const perms = usePartnerPermissions();
  const [tab, setTab] = useState<Tab>('register');
  const { period, season, picker } = usePeriodPicker(establishmentId, 'agreements-report');

  return (
    <Page
      title={t('title')}
      description={t('description')}
      eyebrow={
        <BackLink>
          <Link to="/e/$establishmentId/convenzioni" params={{ establishmentId }}>
            <ArrowLeftIcon aria-hidden="true" />
            {tp('title')}
          </Link>
        </BackLink>
      }
      actions={
        period &&
        perms.canReport && (
          <ExportMenu
            establishmentId={establishmentId}
            report={tab === 'partners' ? 'partners' : 'agreements-register'}
            period={period}
            seasonId={season?.id}
          />
        )
      }
    >
      {picker}
      {!perms.canReport ? (
        <EmptyState icon={NotebookTabsIcon} title={t('forbidden')} />
      ) : (
        <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="gap-4">
          <ResponsiveTabsList
            label={t('title')}
            value={tab}
            onValueChange={(v) => setTab(v as Tab)}
            options={[
              {
                value: 'register',
                label: t('register.title'),
                icon: NotebookTabsIcon,
              },
              {
                value: 'partners',
                label: t('partners.title'),
                icon: BuildingIcon,
              },
            ]}
          />
          <TabsContent value="register">
            <RegisterSection
              establishmentId={establishmentId}
              period={period}
              seasonId={season?.id}
            />
          </TabsContent>
          <TabsContent value="partners">
            <PartnersSection establishmentId={establishmentId} period={period} />
          </TabsContent>
        </Tabs>
      )}
    </Page>
  );
}

function ExportMenu({
  establishmentId,
  report,
  period,
  seasonId,
}: {
  establishmentId: string;
  report: 'partners' | 'agreements-register';
  period: Period;
  seasonId?: string;
}) {
  const { t } = useTranslation('agreementsReports');
  const formats: ReportFile[] = report === 'partners' ? ['xlsx', 'csv'] : ['xlsx', 'html'];
  const icon = { xlsx: FileSpreadsheetIcon, csv: FileTextIcon, html: PrinterIcon };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <DownloadIcon aria-hidden="true" />
          {t('export.label')}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {formats.map((f) => {
          const Icon = icon[f];
          const href = reportFileUrl(establishmentId, report, period, f, seasonId);
          return (
            <DropdownMenuItem key={f} asChild>
              {f === 'html' ? (
                <a href={href} target="_blank" rel="noopener">
                  <Icon aria-hidden="true" />
                  {t('export.html')}
                </a>
              ) : (
                <a href={href} download={reportFileName(report, period, f)}>
                  <Icon aria-hidden="true" />
                  {t(`export.${f}`)}
                </a>
              )}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const money = (v: number) => formatCents(v);

function RegisterSection({
  establishmentId,
  period,
  seasonId,
}: {
  establishmentId: string;
  period: Period | null;
  seasonId?: string;
}) {
  const { t } = useTranslation('agreementsReports');
  const { t: tp } = useTranslation('partners');
  const query = useAgreementsRegister(establishmentId, period, seasonId);
  const r = query.data;
  const entryColumns: ColumnDef<AgreementsRegisterEntry>[] = [
    {
      id: 'name',
      header: t('columns.agreement'),
      meta: { label: t('columns.agreement') },
      cell: ({ row: { original: e } }) => (
        <span className="space-y-1">
          <span className="flex flex-wrap items-center gap-1.5 font-medium">
            {e.name}
            {e.code && (
              <Badge variant="outline" className="font-mono">
                {e.code}
              </Badge>
            )}
          </span>
          <span className="block text-xs text-muted-foreground">
            {e.partner_name || e.customer_category_name || tp(`targets.${e.target}`)}
          </span>
        </span>
      ),
    },
    {
      id: 'category',
      header: t('columns.category'),
      meta: { label: t('columns.category') },
      accessorFn: (e) => tp(`register.${e.register_category}`),
    },
    {
      id: 'conditions',
      header: t('columns.conditions'),
      meta: { label: t('columns.conditions') },
      cell: ({ row: { original: e } }) => (
        <span className="block max-w-80 text-xs whitespace-pre-line text-muted-foreground">
          {e.conditions_text}
          {(e.valid_from || e.valid_to) && (
            <span className="block">
              {t('validity', {
                from: e.valid_from ? formatIsoDate(e.valid_from) : '…',
                to: e.valid_to ? formatIsoDate(e.valid_to) : '…',
              })}
            </span>
          )}
        </span>
      ),
    },
    ...numberColumns<AgreementsRegisterEntry>(t, r?.totals),
  ];
  const categoryColumns: ColumnDef<AgreementsRegisterCategoryTotal>[] = [
    {
      id: 'category',
      header: t('columns.category'),
      meta: { label: t('columns.category') },
      accessorFn: (c) => (c.register_category ? tp(`register.${c.register_category}`) : ''),
    },
    {
      accessorKey: 'agreements',
      header: t('columns.agreements'),
      meta: { label: t('columns.agreements'), align: 'right' },
      footer: () => r?.totals.agreements ?? '',
    },
    ...numberColumns<AgreementsRegisterCategoryTotal>(t, r?.totals),
  ];

  if (!period) return null;
  if (query.error) {
    return (
      <QueryState isLoading={false} error={query.error} onRetry={() => void query.refetch()} />
    );
  }
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label={t('columns.agreements')} value={r ? r.totals.agreements : '—'} />
        <StatCard label={t('columns.bookings')} value={r ? r.totals.bookings : '—'} />
        <StatCard label={t('columns.spotDays')} value={r ? r.totals.spot_days : '—'} />
        <StatCard label={t('columns.discount')} value={r ? money(r.totals.discount) : '—'} />
      </div>
      <SectionCard
        flush
        icon={NotebookTabsIcon}
        title={t('register.title')}
        description={
          r
            ? t('register.subtitle', {
                name: r.establishment_legal_name || r.establishment_name,
                from: formatIsoDate(r.period.from),
                to: formatIsoDate(r.period.to),
              })
            : undefined
        }
      >
        <DataTable
          embedded
          caption={t('register.title')}
          loading={query.isPending}
          columns={entryColumns}
          data={r?.entries ?? []}
          getRowId={(e) => e.agreement_id}
          empty={
            <EmptyState icon={NotebookTabsIcon} bordered={false} title={t('register.empty')} />
          }
        />
      </SectionCard>
      <SectionCard flush title={t('register.byCategory')}>
        <DataTable
          embedded
          caption={t('register.byCategory')}
          loading={query.isPending}
          columns={categoryColumns}
          data={r?.by_category ?? []}
          getRowId={(c, i) => c.register_category ?? String(i)}
          columnToggle={false}
          pageSize={0}
          empty={<EmptyState bordered={false} title={t('register.empty')} />}
        />
      </SectionCard>
    </div>
  );
}

/** Bookings, spot-days, people, revenue and discount, with the grand total in the footer. */
function numberColumns<T extends AgreementsRegisterCategoryTotal | AgreementsRegisterEntry>(
  t: (k: string) => string,
  totals: AgreementsRegisterCategoryTotal | undefined,
): ColumnDef<T>[] {
  return [
    {
      id: 'bookings',
      header: t('columns.bookings'),
      meta: { label: t('columns.bookings'), align: 'right' },
      accessorFn: (x) => x.bookings,
      footer: () => totals?.bookings ?? '',
    },
    {
      id: 'spot_days',
      header: t('columns.spotDays'),
      meta: { label: t('columns.spotDays'), align: 'right' },
      accessorFn: (x) => x.spot_days,
      footer: () => totals?.spot_days ?? '',
    },
    {
      id: 'people',
      header: t('columns.people'),
      meta: { label: t('columns.people'), align: 'right' },
      accessorFn: (x) => x.people,
      footer: () => totals?.people ?? '',
    },
    {
      id: 'revenue',
      header: t('columns.revenue'),
      meta: { label: t('columns.revenue'), align: 'right' },
      accessorFn: (x) => x.revenue,
      cell: ({ getValue }) => money(getValue() as number),
      footer: () => (totals ? money(totals.revenue) : ''),
    },
    {
      id: 'discount',
      header: t('columns.discount'),
      meta: { label: t('columns.discount'), align: 'right' },
      accessorFn: (x) => x.discount,
      cell: ({ getValue }) => money(getValue() as number),
      footer: () => (totals ? money(totals.discount) : ''),
    },
  ];
}

function PartnersSection({
  establishmentId,
  period,
}: {
  establishmentId: string;
  period: Period | null;
}) {
  const { t } = useTranslation('agreementsReports');
  const { t: tp } = useTranslation('partners');
  const query = usePartnersReport(establishmentId, period);
  const r = query.data;
  const totals = r?.totals;
  const columns: ColumnDef<PartnersReportRow>[] = [
    {
      id: 'partner',
      header: t('columns.partner'),
      meta: { label: t('columns.partner') },
      cell: ({ row: { original: x } }) => (
        <span className="space-y-0.5">
          <Link
            to="/e/$establishmentId/convenzioni/partner/$partnerId"
            params={{ establishmentId, partnerId: x.partner_id }}
            className="block font-medium text-primary-on-surface underline-offset-4 hover:underline"
          >
            {x.partner_name}
          </Link>
          <span className="block text-xs text-muted-foreground">{tp(`kinds.${x.kind}`)}</span>
        </span>
      ),
    },
    {
      accessorKey: 'bookings',
      header: t('columns.bookings'),
      meta: { label: t('columns.bookings'), align: 'right' },
      footer: () => totals?.bookings ?? '',
    },
    {
      accessorKey: 'people',
      header: t('columns.people'),
      meta: { label: t('columns.people'), align: 'right' },
      footer: () => totals?.people ?? '',
    },
    {
      accessorKey: 'spot_days',
      header: t('columns.spotDays'),
      meta: { label: t('columns.spotDays'), align: 'right' },
      footer: () => totals?.spot_days ?? '',
    },
    ...(['revenue', 'discount', 'charged', 'paid'] as const).map(
      (k): ColumnDef<PartnersReportRow> => ({
        id: k,
        header: t(`columns.${k}`),
        meta: { label: t(`columns.${k}`), align: 'right' },
        accessorFn: (x) => x[k],
        cell: ({ getValue }) => money(getValue() as number),
        footer: () => (totals ? money(totals[k]) : ''),
      }),
    ),
  ];
  if (!period) return null;
  if (query.error) {
    return (
      <QueryState isLoading={false} error={query.error} onRetry={() => void query.refetch()} />
    );
  }
  return (
    <SectionCard flush icon={BuildingIcon} title={t('partners.title')}>
      <DataTable
        embedded
        caption={t('partners.title')}
        loading={query.isPending}
        columns={columns}
        data={r?.rows ?? []}
        getRowId={(x) => x.partner_id}
        empty={<EmptyState icon={BuildingIcon} bordered={false} title={t('partners.empty')} />}
      />
    </SectionCard>
  );
}
