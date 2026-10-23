import { useMe, type Me, type Schemas } from '@arenile/shared/api';
import { useCurrentEstablishment } from '../../../establishment';

type Permission = Schemas['Permission'];

/** What the operational pages may offer (spec 16.1); the server checks again. */
export interface OperationsPermissions {
  /** Check-in, no-show, move on the map (operations.run). */
  canRun: boolean;
  /** Create and change bookings, release pass days (bookings.write). */
  canWriteBookings: boolean;
  /** Read takings (cash.operate or reports.read). */
  canSeeTakings: boolean;
  /** Edit maps and configuration (establishment.configure). */
  canConfigure: boolean;
}

/** Permissions granted on `establishment` by memberships or API key scopes. */
export function grantedPermissions(
  me: Me | undefined,
  establishment: { id: string; organization_id: string } | undefined,
): Set<Permission> {
  const granted = new Set<Permission>();
  if (!me || !establishment) return granted;
  if (me.principal_type === 'api_key' && me.api_key) {
    if (me.api_key.establishment_id === establishment.id) {
      for (const p of me.api_key.scopes) granted.add(p);
    }
  }
  for (const m of me.memberships) {
    const applies = m.establishment_id
      ? m.establishment_id === establishment.id
      : m.organization_id === establishment.organization_id;
    if (applies) for (const p of m.permissions) granted.add(p);
  }
  return granted;
}

export function toOperationsPermissions(granted: Set<Permission>): OperationsPermissions {
  return {
    canRun: granted.has('operations.run'),
    canWriteBookings: granted.has('bookings.write'),
    canSeeTakings: granted.has('cash.operate') || granted.has('reports.read'),
    canConfigure: granted.has('establishment.configure'),
  };
}

/** Operations permissions of the current user on the current establishment. */
export function useOperationsPermissions(): OperationsPermissions {
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  return toOperationsPermissions(grantedPermissions(me.data, establishment));
}
