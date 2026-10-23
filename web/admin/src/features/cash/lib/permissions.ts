import type { Establishment, Me, Schemas } from '@arenile/shared/api';

type Permission = Schemas['Permission'];

/** What the signed-in principal may do on the cash pages (spec 16.1). */
export interface CashPermissions {
  /** Open/close sessions, movements, take payments: cash.operate (owner, manager, cashier). */
  canOperate: boolean;
  /** Read history and corrispettivi: cash.operate or reports.read (viewer is read-only). */
  canRead: boolean;
  /** Corrispettivi and exports: reports.read (owner, manager, viewer). */
  canReport: boolean;
}

const NONE: CashPermissions = { canOperate: false, canRead: false, canReport: false };

/**
 * Effective permissions on `establishment`: memberships naming the
 * establishment, or its organization when they name none; API keys scoped to
 * it. The server checks again in the domain services, so this only drives
 * what the UI offers.
 */
export function cashPermissionsFor(
  me: Me | undefined,
  establishment: Pick<Establishment, 'id' | 'organization_id'> | undefined,
): CashPermissions {
  if (!me || !establishment) return NONE;
  const granted = new Set<Permission>();
  if (me.principal_type === 'api_key' && me.api_key) {
    if (me.api_key.establishment_id !== establishment.id) return NONE;
    for (const p of me.api_key.scopes) granted.add(p);
  }
  for (const m of me.memberships) {
    const applies = m.establishment_id
      ? m.establishment_id === establishment.id
      : m.organization_id === establishment.organization_id;
    if (applies) for (const p of m.permissions) granted.add(p);
  }
  const canOperate = granted.has('cash.operate');
  const canReport = granted.has('reports.read');
  return { canOperate, canRead: canOperate || canReport, canReport };
}
