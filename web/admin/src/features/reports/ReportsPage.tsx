import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Page, QueryState, ResponsiveTabsList } from '@arenile/shared/ui/patterns';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { AlertCircleIcon, LockIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { seasons as seasonsResource } from '../configuration/api/resources';
import { PeriodToolbar } from './components/PeriodToolbar';
import {
  compareYear,
  defaultSeason,
  resolvePeriod,
  type PeriodSelection,
  type ResolvedPeriod,
} from './lib/period';
import { ExportListsMenu } from './components/ExportListsMenu';
import {
  BookingsSection,
  CancellationsSection,
  OccupancySection,
  RevenueSection,
  SeasonPassesSection,
} from './sections/BreakdownSections';
import { OpenBalancesSection, TopCustomersSection } from './sections/ListSections';
import { SummarySection } from './sections/SummarySection';
import { useReportsContext, type ReportsContext } from './useReportsContext';

const TABS = [
  'summary',
  'occupancy',
  'revenue',
  'bookings',
  'cancellations',
  'passes',
  'customers',
  'balances',
] as const;
type Tab = (typeof TABS)[number];

/** Today in the establishment timezone as YYYY-MM-DD. */
function todayIn(timeZone: string): string {
  try {
    return new Intl.DateTimeFormat('en-CA', { timeZone }).format(new Date());
  } catch {
    return new Intl.DateTimeFormat('en-CA').format(new Date());
  }
}

/**
 * "Report" (spec 15.3, M5A): period toolbar (season, month, custom range;
 * compare with previous year) and one tab per report. Every tab shows the
 * server's numbers as KPI cards, an accessible chart (the table below
 * carries the same data) and a DataTable, with the CSV export of exactly
 * what is shown. Permission reports.read.
 */
export function ReportsPage() {
  const { t } = useTranslation('reports');
  const ctx = useReportsContext();

  if (ctx.loading) return <QueryState isLoading error={null} rows={4} />;
  return (
    <Page title={t('title')} description={t('page.description')}>
      {ctx.canRead ? (
        <ReportsBody ctx={ctx} />
      ) : (
        <Alert>
          <LockIcon aria-hidden="true" />
          <AlertTitle>{t('forbidden.title')}</AlertTitle>
          <AlertDescription>{t('forbidden.description')}</AlertDescription>
        </Alert>
      )}
    </Page>
  );
}

function ReportsBody({ ctx }: { ctx: ReportsContext }) {
  const { t } = useTranslation('reports');
  const seasonList = seasonsResource.useList(ctx.establishmentId);
  const [today] = useState(() => todayIn(ctx.establishment?.timezone ?? 'Europe/Rome'));
  const [chosen, setChosen] = useState<PeriodSelection | null>(null);
  const [compare, setCompare] = useState(false);
  const [tab, setTab] = useState<Tab>('summary');

  if (seasonList.isPending) return <QueryState isLoading error={null} rows={4} />;
  const seasons = [...(seasonList.data ?? [])].sort((a, b) => b.opens_on.localeCompare(a.opens_on));
  const fallback = defaultSeason(seasons, today);
  // Until the user picks, show the current season (or the current month without seasons).
  const selection: PeriodSelection =
    chosen ??
    (fallback
      ? { mode: 'season', seasonId: fallback.id }
      : { mode: 'month', month: today.slice(0, 7) });
  const resolved = resolvePeriod(selection, seasons);
  const period: ResolvedPeriod | undefined = 'period' in resolved ? resolved.period : undefined;

  return (
    <div className="space-y-6">
      <PeriodToolbar
        selection={selection}
        onChange={setChosen}
        seasons={seasons}
        today={today}
        fallbackSeasonId={fallback?.id}
        compare={compare}
        onCompareChange={setCompare}
        compareYear={period ? compareYear(period) : undefined}
      />
      {period ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm text-muted-foreground" aria-live="polite">
            {t('period.showing', {
              from: formatIsoDate(period.from),
              to: formatIsoDate(period.to),
            })}
          </p>
          <ExportListsMenu establishmentId={ctx.establishmentId} period={period} />
        </div>
      ) : (
        'error' in resolved && (
          <Alert variant="destructive">
            <AlertCircleIcon aria-hidden="true" />
            <AlertDescription>{t(`period.error.${resolved.error}`)}</AlertDescription>
          </Alert>
        )
      )}
      {period && (
        // Panels stay mounted (hidden) so each keeps its grouping; queries
        // run only for the visible tab.
        <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="gap-6">
          <ResponsiveTabsList
            label={t('tabs.label')}
            value={tab}
            onValueChange={(v) => setTab(v as Tab)}
            options={TABS.map((id) => ({ value: id, label: t(`tabs.${id}`) }))}
          />
          <TabsContent value="summary" forceMount hidden={tab !== 'summary'}>
            <SummarySection
              ctx={ctx}
              period={period}
              active={tab === 'summary'}
              compare={compare}
            />
          </TabsContent>
          <TabsContent value="occupancy" forceMount hidden={tab !== 'occupancy'}>
            <OccupancySection ctx={ctx} period={period} active={tab === 'occupancy'} />
          </TabsContent>
          <TabsContent value="revenue" forceMount hidden={tab !== 'revenue'}>
            <RevenueSection ctx={ctx} period={period} active={tab === 'revenue'} />
          </TabsContent>
          <TabsContent value="bookings" forceMount hidden={tab !== 'bookings'}>
            <BookingsSection ctx={ctx} period={period} active={tab === 'bookings'} />
          </TabsContent>
          <TabsContent value="cancellations" forceMount hidden={tab !== 'cancellations'}>
            <CancellationsSection ctx={ctx} period={period} active={tab === 'cancellations'} />
          </TabsContent>
          <TabsContent value="passes" forceMount hidden={tab !== 'passes'}>
            <SeasonPassesSection ctx={ctx} period={period} active={tab === 'passes'} />
          </TabsContent>
          <TabsContent value="customers" forceMount hidden={tab !== 'customers'}>
            <TopCustomersSection ctx={ctx} period={period} active={tab === 'customers'} />
          </TabsContent>
          <TabsContent value="balances" forceMount hidden={tab !== 'balances'}>
            <OpenBalancesSection ctx={ctx} period={period} active={tab === 'balances'} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
