import type { Schemas } from '@arenile/shared/api';
import { describe, expect, it } from 'vitest';
import { buildMovePatch } from './move';

const detail = {
  id: 'b1',
  starts_on: '2026-07-14',
  ends_on: '2026-07-16',
  version: 4,
  lines: [
    {
      id: 'l1',
      pack_id: 'pk1',
      zone_id: 'z1',
      spot_id: 's-a1',
      assignment_status: 'assigned',
      quantity: 1,
      line_total: 3000,
      extras: [{ id: 'x1', extra_id: 'e1', quantity: 2, total: 400 }],
    },
    {
      id: 'l2',
      pack_id: 'pk2',
      zone_id: 'z9',
      assignment_status: 'pending',
      quantity: 1,
      line_total: 1000,
      extras: [],
    },
  ],
} as unknown as Schemas['BookingDetail'];

describe('buildMovePatch', () => {
  it('shifts the whole booking by the day delta', () => {
    expect(
      buildMovePatch(detail, { dayDelta: 2, sourceSpotId: 's-a1', targetSpotId: 's-a1' }),
    ).toEqual({ starts_on: '2026-07-16', ends_on: '2026-07-18' });
  });

  it('re-sends all lines with the dragged spot replaced', () => {
    expect(
      buildMovePatch(detail, { dayDelta: 0, sourceSpotId: 's-a1', targetSpotId: 's-b1' }),
    ).toEqual({
      lines: [
        { pack_id: 'pk1', spot_id: 's-b1', quantity: 1, extras: [{ extra_id: 'e1', quantity: 2 }] },
        { pack_id: 'pk2', zone_id: 'z9', quantity: 1, extras: [] },
      ],
    });
  });

  it('combines dates and spot', () => {
    const p = buildMovePatch(detail, { dayDelta: -1, sourceSpotId: 's-a1', targetSpotId: 's-b1' });
    expect(p?.starts_on).toBe('2026-07-13');
    expect(p?.lines?.[0]?.spot_id).toBe('s-b1');
  });

  it('is null when nothing changes or the spot is not in the booking', () => {
    expect(
      buildMovePatch(detail, { dayDelta: 0, sourceSpotId: 's-a1', targetSpotId: 's-a1' }),
    ).toBeNull();
    expect(
      buildMovePatch(detail, { dayDelta: 0, sourceSpotId: 's-zz', targetSpotId: 's-b1' }),
    ).toBeNull();
  });
});
