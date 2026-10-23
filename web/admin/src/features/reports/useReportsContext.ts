import { useMe, type Establishment, type Me } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';

export interface ReportsContext {
  establishmentId: string;
  establishment: Establishment | undefined;
  /** reports.read on this establishment (owner, manager, viewer; API keys with the scope). */
  canRead: boolean;
  /** True until the principal and the establishment are known. */
  loading: boolean;
  /** Integer cents in the establishment currency ("1.234,50 €"). */
  money: (cents: number) => string;
}

/**
 * True when `me` holds reports.read on `establishment`: memberships naming
 * the establishment, or its organization when they name none; API keys
 * scoped to it. The server checks again; this only drives the UI.
 */
export function canReadReports(
  me: Me | undefined,
  establishment: Pick<Establishment, 'id' | 'organization_id'> | undefined,
): boolean {
  if (!me || !establishment) return false;
  if (me.principal_type === 'api_key' && me.api_key) {
    if (me.api_key.establishment_id !== establishment.id) return false;
    if (me.api_key.scopes.includes('reports.read')) return true;
  }
  return me.memberships.some(
    (m) =>
      (m.establishment_id
        ? m.establishment_id === establishment.id
        : m.organization_id === establishment.organization_id) &&
      m.permissions.includes('reports.read'),
  );
}

/** Establishment, permission and money formatter shared by the report sections. */
export function useReportsContext(): ReportsContext {
  const establishmentId = useRequiredEstablishmentId();
  const me = useMe();
  const current = useCurrentEstablishment();
  const est = current.establishment;
  return {
    establishmentId,
    establishment: est,
    canRead: canReadReports(me.data, est),
    loading: me.isPending || current.isLoading,
    money: (cents) => formatCents(cents, est?.currency ?? 'EUR'),
  };
}
