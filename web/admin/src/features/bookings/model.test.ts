import { describe, expect, it } from 'vitest';
import { availableActions } from './model';

const next = ['checked_in', 'cancelled', 'no_show', 'completed'] as const;

describe('availableActions', () => {
  it('offers check-in only on a day of the stay and no-show from its first day', () => {
    const stay = { startsOn: '2027-07-10', endsOn: '2027-07-12' };
    expect(availableActions('confirmed', next, { ...stay, today: '2027-07-01' })).toEqual([
      'cancel',
    ]);
    expect(availableActions('confirmed', next, { ...stay, today: '2027-07-10' })).toEqual([
      'check-in',
      'no-show',
      'cancel',
    ]);
    expect(availableActions('confirmed', next, { ...stay, today: '2027-07-12' })).toEqual([
      'check-in',
      'no-show',
      'cancel',
    ]);
    expect(availableActions('confirmed', next, { ...stay, today: '2027-07-13' })).toEqual([
      'no-show',
      'cancel',
    ]);
  });

  it('keeps the server list when the period is unknown', () => {
    expect(availableActions('confirmed', next)).toEqual(['check-in', 'no-show', 'cancel']);
    expect(availableActions('awaiting_approval', ['confirmed', 'rejected'])).toEqual([
      'approve',
      'reject',
    ]);
  });
});
