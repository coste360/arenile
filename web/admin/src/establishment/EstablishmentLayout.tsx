import { useEstablishmentEvents } from '@arenile/shared/api';
import { MapKitProvider } from '@arenile/shared/map';
import { Navigate, Outlet } from '@tanstack/react-router';
import { useCurrentEstablishment, useEstablishmentId } from './context';

/**
 * Layout of every /e/$establishmentId route: keeps the real-time stream of
 * the establishment open (spec 21.1) so that maps, lists and the cash desk
 * refresh their TanStack Query data when bookings, allocations, holds or
 * payments change. It also provides the establishment's map kit (theme.map_kit,
 * ADR 0024) to every map, editor palette and thumbnail below it.
 */
export function EstablishmentLayout() {
  useEstablishmentEvents(useEstablishmentId());
  const { establishment } = useCurrentEstablishment();
  return (
    <MapKitProvider kit={establishment?.theme?.map_kit}>
      <Outlet />
    </MapKitProvider>
  );
}

/**
 * Legacy top-level path of an operational section (/oggi, /cassa, ...):
 * redirects to the same section of the current establishment, or to the
 * establishments list when the user has none.
 */
export function EstablishmentSectionRedirect({ section }: { section: string }) {
  const current = useCurrentEstablishment();
  if (current.isLoading) return null;
  if (!current.id) return <Navigate to="/configurazione" replace />;
  return (
    <Navigate
      to={`/e/$establishmentId/${section}` as never}
      params={{ establishmentId: current.id } as never}
      replace
    />
  );
}
