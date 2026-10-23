import type { Me } from '@arenile/shared/api';
import { describe, expect, it } from 'vitest';
import { cashPermissionsFor } from './permissions';

const est = { id: 'e1', organization_id: 'o1' };
const me = (
  permissions: string[],
  scope: { establishment_id?: string; organization_id?: string } = {},
) =>
  ({
    principal_type: 'user',
    memberships: [{ organization_id: 'o1', role: 'cashier', permissions, ...scope }],
  }) as unknown as Me;

describe('cashPermissionsFor (spec 16.1)', () => {
  it.each([
    [
      'cashier',
      ['bookings.write', 'cash.operate', 'operations.run', 'bookings.read'],
      true,
      true,
      false,
    ],
    ['viewer', ['bookings.read', 'reports.read'], false, true, true],
    ['beach_staff', ['operations.run', 'bookings.read'], false, false, false],
    ['manager', ['cash.operate', 'reports.read', 'refunds.issue'], true, true, true],
  ])('%s', (_role, perms, canOperate, canRead, canReport) => {
    expect(cashPermissionsFor(me(perms), est)).toEqual({ canOperate, canRead, canReport });
  });

  it('ignores memberships of another establishment', () => {
    expect(
      cashPermissionsFor(me(['cash.operate'], { establishment_id: 'e2' }), est).canOperate,
    ).toBe(false);
  });

  it('denies everything while loading', () => {
    expect(cashPermissionsFor(undefined, est)).toEqual({
      canOperate: false,
      canRead: false,
      canReport: false,
    });
  });
});
