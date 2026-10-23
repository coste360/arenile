import type { Establishment, Me, Schemas } from '@arenile/shared/api';

/**
 * True when the principal holds `permission` on the establishment (spec
 * 16.1: a membership applies when it names the establishment or, without an
 * establishment, its organization; API keys by scope). UI only: the server
 * checks again in the domain services.
 */
export function hasPermission(
  me: Me | undefined,
  establishment: Pick<Establishment, 'id' | 'organization_id'> | undefined,
  permission: Schemas['Permission'],
): boolean {
  if (!me || !establishment) return false;
  if (me.principal_type === 'api_key' && me.api_key) {
    return (
      me.api_key.establishment_id === establishment.id && me.api_key.scopes.includes(permission)
    );
  }
  return me.memberships.some(
    (m) =>
      (m.establishment_id
        ? m.establishment_id === establishment.id
        : m.organization_id === establishment.organization_id) &&
      m.permissions.includes(permission),
  );
}
