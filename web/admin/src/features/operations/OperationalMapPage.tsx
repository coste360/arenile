import { EmptyState, Page, QueryState, useMediaQuery } from '@arenile/shared/ui/patterns';
import { Button } from '@arenile/shared/ui/button';
import { Link } from '@tanstack/react-router';
import { MapIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { useOperationsDay } from './api/operations';
import { DayToolbar } from './components/DayToolbar';
import { OperationalMapView } from './components/OperationalMapView';
import { useOperationsPermissions } from './lib/permissions';
import { useDaySearch } from './lib/useDaySearch';
import { useSelectedMap } from './lib/useSelectedMap';

/**
 * "Mappa" (spec 15.1 point 2, 11.3): the operational map for any date and
 * slot, from `?date=&slot=`. Spots are coloured by status; a tap opens the
 * quick sheet with the actions; the stream keeps it live.
 */
/**
 * On wide screens the page uses the whole width (AppShell) and the drawing
 * may grow taller too, so more spots are readable without zooming.
 */
const WIDE_QUERY = '(min-width: 1280px)';
const WIDE_MAP_HEIGHT = '78vh';

export function OperationalMapPage() {
  const wide = useMediaQuery(WIDE_QUERY);
  const { t } = useTranslation('operations');
  const establishmentId = useRequiredEstablishmentId();
  const { date, slot, today, setDay } = useDaySearch();
  const permissions = useOperationsPermissions();
  const { maps, options, mapId, setMapId, geometry, hasNoMaps } = useSelectedMap(establishmentId);
  // Filter by map only when there is a choice: one cache entry with the Oggi page otherwise.
  const day = useOperationsDay(establishmentId, date, slot, options.length > 1 ? mapId : undefined);

  return (
    <Page title={t('map.title')} description={t('map.description')}>
      <DayToolbar
        date={date}
        slot={slot}
        today={today}
        onChange={setDay}
        maps={options}
        mapId={mapId}
        onMapChange={setMapId}
      />
      {hasNoMaps ? (
        <EmptyState
          icon={MapIcon}
          title={t('map.noMapsTitle')}
          description={t('map.noMapsDescription')}
          action={
            permissions.canConfigure ? (
              <Button asChild>
                <Link to="/e/$establishmentId/maps" params={{ establishmentId }}>
                  {t('map.openEditor')}
                </Link>
              </Button>
            ) : undefined
          }
        />
      ) : (
        <>
          <QueryState
            isLoading={maps.isLoading || geometry.isLoading || day.isLoading}
            error={maps.error ?? geometry.error ?? day.error}
            onRetry={() => {
              void maps.refetch();
              void geometry.refetch();
              void day.refetch();
            }}
            skeleton={<div className="h-[65vh] w-full animate-pulse rounded-lg bg-muted" />}
          />
          {geometry.data && day.data && (
            <OperationalMapView
              establishmentId={establishmentId}
              geometry={geometry.data}
              day={day.data}
              date={date}
              slot={slot}
              permissions={permissions}
              height={wide ? WIDE_MAP_HEIGHT : undefined}
            />
          )}
        </>
      )}
    </Page>
  );
}
