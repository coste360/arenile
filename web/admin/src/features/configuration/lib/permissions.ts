import type { Establishment, Me, Schemas } from '@arenile/shared/api';

type Permission = Schemas['Permission'];

export interface EstablishmentPermissions {
  /** Configuration of establishment, prices, policies (owner, manager). */
  canConfigure: boolean;
  /** Users and roles (owner only). */
  canManageUsers: boolean;
  /** Any access at all (every role can view). */
  canView: boolean;
}

const NONE: EstablishmentPermissions = {
  canConfigure: false,
  canManageUsers: false,
  canView: false,
};

/**
 * Effective permissions of the principal on `establishment` (spec 16.1). A
 * membership applies when it names the establishment or, without an
 * establishment, its organization. The server checks again in the domain
 * services: this only drives what the UI shows.
 */
export function permissionsFor(
  me: Me | undefined,
  establishment: Pick<Establishment, 'id' | 'organization_id'> | undefined,
): EstablishmentPermissions {
  if (!me || !establishment) return NONE;
  const granted = new Set<Permission>();
  let member = false;
  if (me.principal_type === 'api_key' && me.api_key) {
    if (me.api_key.establishment_id !== establishment.id) return NONE;
    member = true;
    for (const p of me.api_key.scopes) granted.add(p);
  }
  for (const m of me.memberships) {
    const applies = m.establishment_id
      ? m.establishment_id === establishment.id
      : m.organization_id === establishment.organization_id;
    if (!applies) continue;
    member = true;
    for (const p of m.permissions) granted.add(p);
  }
  return {
    canConfigure: granted.has('establishment.configure'),
    canManageUsers: granted.has('users.manage'),
    canView: member,
  };
}

/** Organizations where the user is owner (may create establishments there). */
export function ownedOrganizations(me: Me | undefined): string[] {
  const ids = new Set<string>();
  for (const m of me?.memberships ?? []) if (m.role === 'owner') ids.add(m.organization_id);
  return [...ids];
}
