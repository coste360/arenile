import { useProblemText } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  EmptyState,
  Page,
  QueryState,
  ResponsiveTabsList,
  SectionCard,
  StatCard,
} from '@arenile/shared/ui/patterns';
import { Progress } from '@arenile/shared/ui/progress';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { Link } from '@tanstack/react-router';
import {
  CalendarDaysIcon,
  CalendarMinusIcon,
  LogInIcon,
  MapIcon,
  PlaneLandingIcon,
  PlusIcon,
  TicketIcon,
  UmbrellaIcon,
  WalletIcon,
} from 'lucide-react';
import { lazy, Suspense, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useRequiredEstablishmentId } from '../../establishment';
import { useCheckIn, useDayTakings, useOperationsDay } from './api/operations';
import { DayToolbar } from './components/DayToolbar';
import { OperationalMapView } from './components/OperationalMapView';
import { StatusChart } from './components/StatusChart';
import { formatDayLabel } from './lib/dates';
import { computeDayKpis } from './lib/kpis';
import { useOperationsPermissions } from './lib/permissions';
import type { OperationalBooking } from './lib/status';
import { bookingRows } from './lib/worklist';
import { useDaySearch } from './lib/useDaySearch';
import { useSelectedMap } from './lib/useSelectedMap';

const NewBookingSheet = lazy(() =>
  import('../bookings').then((m) => ({ default: m.NewBookingSheet })),
);

/**
 * "Oggi" (spec 15.1 point 1): key figures of the day, status distribution,
 * the lists to work through (check-ins, balances, lines to assign), quick
 * actions and the operational map. Everything derives from GET
 * /operations/day (shared cache with the Mappa page) plus the day's takings.
 */
export function TodayPage() {
  const { t, i18n } = useTranslation('operations');
  const [worklist, setWorklist] = useState('checkin');
  const establishmentId = useRequiredEstablishmentId();
  const { date, slot, today, setDay } = useDaySearch();
  const permissions = useOperationsPermissions();
  const { options, mapId, setMapId, geometry, hasNoMaps } = useSelectedMap(establishmentId);
  // KPIs cover every map of the establishment; the map card shows the selected one.
  const day = useOperationsDay(establishmentId, date, slot);
  // With a single map both queries share one cache entry (same key, one request).
  const mapDay = useOperationsDay(
    establishmentId,
    date,
    slot,
    options.length > 1 ? mapId : undefined,
  );
  const takings = useDayTakings(establishmentId, date, permissions.canSeeTakings);
  const checkIn = useCheckIn(establishmentId);
  const problemText = useProblemText();
  const [newBooking, setNewBooking] = useState(false);

  const kpis = useMemo(() => (day.data ? computeDayKpis(day.data) : null), [day.data]);
  const toCheckIn = useMemo(
    () =>
      day.data
        ? bookingRows(
            day.data,
            (status, b) => status === 'booked' && b.status === 'confirmed' && !b.is_season_pass,
          )
        : [],
    [day.data],
  );
  const toCollect = useMemo(
    () => (day.data ? bookingRows(day.data, (_s, b) => b.balance_due > 0) : []),
    [day.data],
  );

  const doCheckIn = (b: OperationalBooking) =>
    checkIn.mutate(
      { bookingId: b.booking_id, version: b.version },
      {
        onSuccess: () => toast.success(t('sheet.checkedIn', { code: b.code })),
        onError: (e) => toast.error(problemText(e)),
      },
    );

  const isToday = date === today;

  return (
    <Page
      title={isToday ? t('today.title') : formatDayLabel(date, i18n.language)}
      description={
        isToday
          ? t('today.subtitle', { date: formatDayLabel(date, i18n.language) })
          : t('today.description')
      }
      actions={
        <>
          <Button variant="outline" asChild>
            <Link
              to="/e/$establishmentId/mappa"
              params={{ establishmentId }}
              search={{ date, slot }}
            >
              <MapIcon aria-hidden="true" />
              {t('quick.map')}
            </Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="/e/$establishmentId/calendario" params={{ establishmentId }}>
              <CalendarDaysIcon aria-hidden="true" />
              {t('quick.planning')}
            </Link>
          </Button>
          {permissions.canSeeTakings && (
            <Button variant="outline" asChild>
              <Link to="/e/$establishmentId/cassa" params={{ establishmentId }}>
                <WalletIcon aria-hidden="true" />
                {t('quick.cash')}
              </Link>
            </Button>
          )}
          {permissions.canWriteBookings && (
            <Button onClick={() => setNewBooking(true)}>
              <PlusIcon aria-hidden="true" />
              {t('actions.newBooking')}
            </Button>
          )}
        </>
      }
    >
      <DayToolbar
        date={date}
        slot={slot}
        today={today}
        onChange={setDay}
        maps={options}
        mapId={mapId}
        onMapChange={setMapId}
      />

      <QueryState
        isLoading={day.isLoading}
        error={day.error}
        onRetry={() => void day.refetch()}
        skeleton={
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
        }
      />

      {kpis && (
        <section aria-label={t('today.kpis')} className="grid grid-cols-2 gap-4 lg:grid-cols-3">
          <StatCard
            label={t('kpi.occupancy')}
            value={`${kpis.occupancyPercent}%`}
            icon={UmbrellaIcon}
            hint={t('kpi.occupancyHint', { occupied: kpis.occupied, spots: kpis.spots })}
          >
            <Progress value={kpis.occupancyPercent} aria-label={t('kpi.occupancy')} />
          </StatCard>
          <StatCard
            label={t('kpi.arrivals')}
            value={kpis.arrivals}
            icon={PlaneLandingIcon}
            hint={t('kpi.departures', { count: kpis.departures })}
          />
          <StatCard
            label={t('kpi.pendingCheckIns')}
            value={kpis.pendingCheckIns}
            icon={LogInIcon}
            hint={t('kpi.checkedIn', { count: kpis.checkedIn })}
          />
          <StatCard label={t('kpi.passHolders')} value={kpis.passHolders} icon={TicketIcon} />
          <StatCard
            label={t('kpi.released')}
            value={kpis.released}
            icon={CalendarMinusIcon}
            hint={t('kpi.releasedHint')}
          />
          {permissions.canSeeTakings ? (
            <StatCard
              label={t('kpi.takings')}
              value={
                takings.isLoading ? (
                  <>
                    <span aria-hidden="true">…</span>
                    <span className="sr-only">{t('common.loading', { ns: 'translation' })}</span>
                  </>
                ) : takings.error ? (
                  '—'
                ) : (
                  formatCents(takings.data ?? 0)
                )
              }
              icon={WalletIcon}
              hint={t('kpi.balanceDue', { amount: formatCents(kpis.balanceDue) })}
            />
          ) : (
            <StatCard
              label={t('kpi.balanceDueTitle')}
              value={formatCents(kpis.balanceDue)}
              icon={WalletIcon}
              tone={kpis.balanceDue > 0 ? 'danger' : 'default'}
            />
          )}
        </section>
      )}

      {day.data && kpis && (
        <div className="grid gap-6 lg:grid-cols-2">
          <SectionCard title={t('today.chartTitle')} description={t('today.chartDescription')}>
            <StatusChart byStatus={kpis.byStatus} />
          </SectionCard>
          <SectionCard title={t('today.worklist')}>
            <Tabs value={worklist} onValueChange={setWorklist}>
              <ResponsiveTabsList
                label={t('today.worklist')}
                value={worklist}
                onValueChange={setWorklist}
                options={[
                  {
                    value: 'checkin',
                    label: (
                      <>
                        {t('today.toCheckIn')} <Badge variant="secondary">{toCheckIn.length}</Badge>
                      </>
                    ),
                  },
                  {
                    value: 'balance',
                    label: (
                      <>
                        {t('today.toCollect')} <Badge variant="secondary">{toCollect.length}</Badge>
                      </>
                    ),
                  },
                  {
                    value: 'assign',
                    label: (
                      <>
                        {t('today.toAssign')}{' '}
                        <Badge variant="secondary">{day.data.pending_assignments.length}</Badge>
                      </>
                    ),
                  },
                ]}
              />
              <TabsContent value="checkin">
                <WorkList
                  empty={t('today.noCheckIns')}
                  rows={toCheckIn.map((r) => ({
                    id: r.booking.booking_id,
                    title: r.booking.customer_display_name,
                    meta: `${r.booking.code} · ${r.spots.join(', ')}`,
                    action: permissions.canRun ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={checkIn.isPending}
                        onClick={() => doCheckIn(r.booking)}
                        aria-label={t('today.checkInAria', {
                          customer: r.booking.customer_display_name,
                        })}
                      >
                        {checkIn.isPending &&
                        checkIn.variables?.bookingId === r.booking.booking_id ? (
                          <Spinner />
                        ) : (
                          <LogInIcon aria-hidden="true" />
                        )}
                        {t('actions.checkIn')}
                      </Button>
                    ) : null,
                  }))}
                  establishmentId={establishmentId}
                />
              </TabsContent>
              <TabsContent value="balance">
                <WorkList
                  empty={t('today.noBalances')}
                  rows={toCollect.map((r) => ({
                    id: r.booking.booking_id,
                    title: r.booking.customer_display_name,
                    meta: `${r.booking.code} · ${r.spots.join(', ')}`,
                    action: (
                      <span className="text-sm font-medium text-destructive tabular-nums">
                        {formatCents(r.booking.balance_due)}
                      </span>
                    ),
                  }))}
                  establishmentId={establishmentId}
                />
              </TabsContent>
              <TabsContent value="assign">
                <WorkList
                  empty={t('today.noAssignments')}
                  rows={day.data.pending_assignments.map((p) => ({
                    id: p.booking_id,
                    key: p.line_id,
                    title: p.customer_display_name,
                    meta: `${p.code} · ${t(`slot.${p.slots}`)}`,
                    action: null,
                  }))}
                  establishmentId={establishmentId}
                />
              </TabsContent>
            </Tabs>
          </SectionCard>
        </div>
      )}

      <SectionCard title={t('today.mapTitle')}>
        {hasNoMaps ? (
          <EmptyState icon={MapIcon} title={t('map.noMapsTitle')} bordered={false} />
        ) : geometry.data && mapDay.data ? (
          <OperationalMapView
            establishmentId={establishmentId}
            geometry={geometry.data}
            day={mapDay.data}
            date={date}
            slot={slot}
            permissions={permissions}
            height="50vh"
          />
        ) : (
          <QueryState
            isLoading={geometry.isLoading || mapDay.isLoading}
            error={geometry.error ?? mapDay.error}
            skeleton={<Skeleton className="h-[50vh] w-full" />}
          />
        )}
      </SectionCard>

      {newBooking && (
        <Suspense fallback={null}>
          <NewBookingSheet
            establishmentId={establishmentId}
            open
            onOpenChange={(v) => !v && setNewBooking(false)}
            defaults={{ startsOn: date, endsOn: date, slots: slot }}
          />
        </Suspense>
      )}
    </Page>
  );
}

function WorkList({
  rows,
  empty,
  establishmentId,
}: {
  rows: { id: string; key?: string; title: string; meta: string; action: ReactNode }[];
  empty: string;
  establishmentId: string;
}) {
  if (rows.length === 0) return <p className="py-4 text-sm text-muted-foreground">{empty}</p>;
  return (
    <ul className="max-h-72 divide-y overflow-y-auto">
      {rows.map((r) => (
        <li key={r.key ?? r.id} className="flex items-center justify-between gap-3 py-2">
          <div className="min-w-0">
            <Link
              to="/e/$establishmentId/prenotazioni/$bookingId"
              params={{ establishmentId, bookingId: r.id }}
              className="block truncate font-medium underline-offset-4 hover:underline"
            >
              {r.title}
            </Link>
            <p className="truncate text-xs text-muted-foreground">{r.meta}</p>
          </div>
          {r.action}
        </li>
      ))}
    </ul>
  );
}
