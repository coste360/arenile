import { useEstablishmentEvents } from '@arenile/shared/api';
import { Navigate, Outlet } from '@tanstack/react-router';
import { useCurrentEstablishment, useEstablishmentId } from './context';

/**
 * Layout of every /e/$establishmentId route: keeps the real-time stream of
 * the establishment open (spec 21.1) so that maps, lists and the cash desk
 * refresh their TanStack Query data when bookings, allocations, holds or
 * payments change.
 */
export function EstablishmentLayout() {
  useEstablishmentEvents(useEstablishmentId());
  return <Outlet />;
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
