import { describe, expect, it } from 'vitest';
import { opBooking, opSpot } from '../testing/fixtures';
import { spotActions } from './actions';
import type { OperationsPermissions } from './permissions';
import { grantedPermissions, toOperationsPermissions } from './permissions';

const all: OperationsPermissions = {
  canRun: true,
  canWriteBookings: true,
  canSeeTakings: true,
  canConfigure: true,
  canSeeCustomers: true,
};
const viewer: OperationsPermissions = {
  canRun: false,
  canWriteBookings: false,
  canSeeTakings: false,
  canConfigure: false,
  canSeeCustomers: true,
};

const on = (a: ReturnType<typeof spotActions>) =>
  Object.entries(a)
    .filter(([, v]) => v)
    .map(([k]) => k)
    .sort();

describe('spotActions', () => {
  it.each([
    ['free spot', opSpot('s', 'A1', 'free'), ['newBooking']],
    [
      'confirmed booking',
      opSpot('s', 'A1', 'booked', { booking: opBooking() }),
      ['checkIn', 'move', 'openDetail'],
    ],
    [
      'booking with balance',
      opSpot('s', 'A1', 'booked', { booking: opBooking({ balance_due: 100 }) }),
      ['checkIn', 'collect', 'move', 'openDetail'],
    ],
    [
      'checked in',
      opSpot('s', 'A1', 'checked_in', { booking: opBooking({ status: 'checked_in' }) }),
      ['move', 'openDetail'],
    ],
    [
      'pass holder',
      opSpot('s', 'A1', 'pass_holder', { booking: opBooking({ is_season_pass: true }) }),
      ['openDetail', 'releaseDay'],
    ],
    [
      'released day',
      opSpot('s', 'A1', 'released', {
        release_id: 'r1',
        booking: opBooking({ is_season_pass: true }),
      }),
      ['newBooking', 'openDetail', 'revokeRelease'],
    ],
    ['blocked', opSpot('s', 'A1', 'blocked'), []],
    [
      'booking without line id',
      opSpot('s', 'A1', 'booked', { booking: opBooking({ line_id: undefined }) }),
      ['checkIn', 'openDetail'],
    ],
  ])('%s', (_name, spot, expected) => {
    expect(on(spotActions(spot, 'FULL', all))).toEqual([...expected].sort());
  });

  it('viewer only opens details', () => {
    const spot = opSpot('s', 'A1', 'booked', { booking: opBooking({ balance_due: 100 }) });
    expect(on(spotActions(spot, 'FULL', viewer))).toEqual(['openDetail']);
    expect(on(spotActions(opSpot('s', 'A1', 'free'), 'FULL', viewer))).toEqual([]);
  });

  it('uses the slot status (free morning of a PM booking)', () => {
    const spot = opSpot('s', 'A1', 'booked', {
      am_status: 'free',
      pm_status: 'booked',
      booking: opBooking({ slots: 'PM' }),
    });
    expect(spotActions(spot, 'AM', all).newBooking).toBe(true);
    expect(spotActions(spot, 'PM', all).newBooking).toBe(false);
  });
});

describe('permissions', () => {
  const est = { id: 'e1', organization_id: 'o1' };
  it('cashier can run and write, viewer cannot', () => {
    const me = (permissions: string[]) =>
      ({
        principal_type: 'user',
        memberships: [{ organization_id: 'o1', role: 'cashier', permissions }],
      }) as unknown as Parameters<typeof grantedPermissions>[0];
    expect(
      toOperationsPermissions(
        grantedPermissions(me(['bookings.write', 'operations.run', 'cash.operate']), est),
      ),
    ).toEqual({
      canRun: true,
      canWriteBookings: true,
      canSeeTakings: true,
      canConfigure: false,
      canSeeCustomers: false,
    });
    expect(toOperationsPermissions(grantedPermissions(me(['bookings.read']), est))).toEqual(viewer);
  });
  it('names on the map need bookings.read (viewer yes, reports-only API key no)', () => {
    const viewerMe = {
      principal_type: 'user',
      memberships: [
        { organization_id: 'o1', role: 'viewer', permissions: ['bookings.read', 'reports.read'] },
      ],
    } as unknown as Parameters<typeof grantedPermissions>[0];
    expect(toOperationsPermissions(grantedPermissions(viewerMe, est)).canSeeCustomers).toBe(true);
    const reportsKey = {
      principal_type: 'api_key',
      api_key: { establishment_id: 'e1', scopes: ['reports.read'] },
      memberships: [],
    } as unknown as Parameters<typeof grantedPermissions>[0];
    expect(toOperationsPermissions(grantedPermissions(reportsKey, est)).canSeeCustomers).toBe(
      false,
    );
  });
  it('ignores memberships of other establishments', () => {
    const me = {
      principal_type: 'user',
      memberships: [
        {
          organization_id: 'o1',
          establishment_id: 'e2',
          role: 'manager',
          permissions: ['operations.run'],
        },
      ],
    } as unknown as Parameters<typeof grantedPermissions>[0];
    expect(grantedPermissions(me, est).size).toBe(0);
  });
});
