import { describe, expect, it } from 'vitest';
import { proposalBooking, segmentsText } from './lib';
import type { FindSpotProposal, FindSpotRequest } from './api';

const t = (key: string, o?: Record<string, unknown>) =>
  key === 'segment.day' ? `${o?.spot} il ${o?.date}` : `${o?.spot} dal ${o?.from} al ${o?.to}`;

const proposal: FindSpotProposal = {
  id: 'p1',
  rank: 1,
  spot_changes: 1,
  spot_ids: ['s5', 's7'],
  lines: [
    {
      segments: [
        {
          spot_id: 's5',
          spot_label: 'A5',
          zone_id: 'z',
          starts_on: '2026-08-10',
          ends_on: '2026-08-14',
        },
        {
          spot_id: 's7',
          spot_label: 'A7',
          zone_id: 'z',
          starts_on: '2026-08-15',
          ends_on: '2026-08-15',
        },
      ],
    },
    {
      segments: [
        {
          spot_id: 's6',
          spot_label: 'A6',
          zone_id: 'z',
          starts_on: '2026-08-10',
          ends_on: '2026-08-15',
        },
      ],
    },
  ],
  total: 12000,
};

describe('findspot lib', () => {
  it('describes the segments of a line (spec 30.6)', () => {
    expect(segmentsText(proposal.lines[0]!.segments, t)).toBe(
      'A5 dal 10/08/2026 al 14/08/2026, A7 il 15/08/2026',
    );
  });

  it('turns a proposal into a booking with a line per proposal line', () => {
    const request: FindSpotRequest = {
      starts_on: '2026-08-10',
      ends_on: '2026-08-15',
      slots: 'FULL',
      pack_id: 'pk1',
      agreement_id: 'ag1',
    };
    expect(proposalBooking(request, proposal, 'c1')).toEqual({
      customer_id: 'c1',
      starts_on: '2026-08-10',
      ends_on: '2026-08-15',
      slots: 'FULL',
      formula: 'multi_day',
      lines: [
        {
          pack_id: 'pk1',
          segments: [
            { spot_id: 's5', starts_on: '2026-08-10', ends_on: '2026-08-14' },
            { spot_id: 's7', starts_on: '2026-08-15', ends_on: '2026-08-15' },
          ],
        },
        { pack_id: 'pk1', spot_id: 's6' },
      ],
      agreement_id: 'ag1',
      status: 'confirmed',
    });
  });
});
