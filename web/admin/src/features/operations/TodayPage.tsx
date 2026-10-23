import { useProblemText } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { EmptyState, PageHeader, QueryState } from '@arenile/shared/ui/patterns';
import { Progress } from '@arenile/shared/ui/progress';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
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
import { useQueueableCheckIn } from '../../offline/useQueueableCheckIn';
import { useDayTakings, useOperationsDay } from './api/operations';
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

function Kpi({
  title,
  value,
  hint,
  icon,
  children,
}: {
  title: string;
  value: ReactNode;
  hint?: ReactNode;
  icon: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-2xl tabular-nums">{value}</CardTitle>
        <CardAction className="text-muted-foreground [&_svg]:size-5">{icon}</CardAction>
      </CardHeader>
      {(hint || children) && (
        <CardContent className="space-y-2 px-4 text-sm text-muted-foreground">
          {children}
          {hint && <p>{hint}</p>}
        </CardContent>
      )}
    </Card>
  );
}

/**
 * "Oggi" (spec 15.1 point 1): key figures of the day, status distribution,
 * the lists to work through (check-ins, balances, lines to assign), quick
 * actions and the operational map. Everything derives from GET
 * /operations/day (shared cache with the Mappa page) plus the day's takings.
 */
export function TodayPage() {
  const { t, i18n } = useTranslation('operations');
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
  const checkIn = useQueueableCheckIn(establishmentId);
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
      { bookingId: b.booking_id, version: b.version, label: b.code },
      {
        onSuccess: (res) =>
          toast.success(
            res.queued
              ? t('offline:actions.queuedCheckIn', { code: b.code })
              : t('sheet.checkedIn', { code: b.code }),
          ),
        onError: (e) => toast.error(problemText(e)),
      },
    );

  const isToday = date === today;

  return (
    <div className="space-y-6">
      <PageHeader
        title={isToday ? t('today.title') : formatDayLabel(date, i18n.language)}
        description={
          isToday
            ? t('today.subtitle', { date: formatDayLabel(date, i18n.language) })
            : t('today.description')
        }
        actions={
          <>
            {permissions.canWriteBookings && (
              <Button onClick={() => setNewBooking(true)}>
                <PlusIcon aria-hidden="true" />
                {t('actions.newBooking')}
              </Button>
            )}
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
          </>
        }
      />
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
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
        }
      />

      {kpis && (
        <section aria-label={t('today.kpis')} className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <Kpi
            title={t('kpi.occupancy')}
            value={`${kpis.occupancyPercent}%`}
            icon={<UmbrellaIcon aria-hidden="true" />}
            hint={t('kpi.occupancyHint', { occupied: kpis.occupied, spots: kpis.spots })}
          >
            <Progress value={kpis.occupancyPercent} aria-label={t('kpi.occupancy')} />
          </Kpi>
          <Kpi
            title={t('kpi.arrivals')}
            value={kpis.arrivals}
            icon={<PlaneLandingIcon aria-hidden="true" />}
            hint={t('kpi.departures', { count: kpis.departures })}
          />
          <Kpi
            title={t('kpi.pendingCheckIns')}
            value={kpis.pendingCheckIns}
            icon={<LogInIcon aria-hidden="true" />}
            hint={t('kpi.checkedIn', { count: kpis.checkedIn })}
          />
          <Kpi
            title={t('kpi.passHolders')}
            value={kpis.passHolders}
            icon={<TicketIcon aria-hidden="true" />}
          />
          <Kpi
            title={t('kpi.released')}
            value={kpis.released}
            icon={<CalendarMinusIcon aria-hidden="true" />}
            hint={t('kpi.releasedHint')}
          />
          {permissions.canSeeTakings ? (
            <Kpi
              title={t('kpi.takings')}
              value={
                takings.isLoading ? (
                  <Skeleton className="h-8 w-28" />
                ) : takings.error ? (
                  '—'
                ) : (
                  formatCents(takings.data ?? 0)
                )
              }
              icon={<WalletIcon aria-hidden="true" />}
              hint={t('kpi.balanceDue', { amount: formatCents(kpis.balanceDue) })}
            />
          ) : (
            <Kpi
              title={t('kpi.balanceDueTitle')}
              value={formatCents(kpis.balanceDue)}
              icon={<WalletIcon aria-hidden="true" />}
            />
          )}
        </section>
      )}

      {day.data && kpis && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t('today.chartTitle')}</CardTitle>
              <CardDescription>{t('today.chartDescription')}</CardDescription>
            </CardHeader>
            <CardContent>
              <StatusChart byStatus={kpis.byStatus} />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('today.worklist')}</CardTitle>
            </CardHeader>
            <CardContent>
              <Tabs defaultValue="checkin">
                <TabsList className="flex-wrap group-data-[orientation=horizontal]/tabs:h-auto">
                  <TabsTrigger value="checkin">
                    {t('today.toCheckIn')} <Badge variant="secondary">{toCheckIn.length}</Badge>
                  </TabsTrigger>
                  <TabsTrigger value="balance">
                    {t('today.toCollect')} <Badge variant="secondary">{toCollect.length}</Badge>
                  </TabsTrigger>
                  <TabsTrigger value="assign">
                    {t('today.toAssign')}{' '}
                    <Badge variant="secondary">{day.data.pending_assignments.length}</Badge>
                  </TabsTrigger>
                </TabsList>
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
            </CardContent>
          </Card>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>{t('today.mapTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
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
        </CardContent>
      </Card>

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
    </div>
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
