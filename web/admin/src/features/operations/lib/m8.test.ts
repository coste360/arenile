import { describe, expect, it } from 'vitest';
import type { MapSpot } from '@arenile/shared/map';
import { opBooking, opSpot } from '../testing/fixtures';
import { groupMoveTargets } from './groupMove';
import {
  DEFAULT_LABEL_PARTS,
  labelCaptions,
  labelModeOf,
  labelPartsKey,
  mapIndicators,
  normalizeParts,
  readLabelParts,
  shortDate,
} from './mapLabels';
import { commonWindow, defaultMoveFrom, moveFromChoices, moveFromDate } from './moveFrom';
import { agreementOptions, combineHighlights, filterSpots, groupOptions } from './quickFilters';
import { dateRuns, rangeSpots, rangeState } from './rangeView';
import { calendarMonths, cellStatus, proposedStay, type SpotCalendarDay } from './spotCalendar';
import { indicatorsBySpot } from './status';

const t = (key: string, o?: Record<string, unknown>) => (o ? `${key}:${JSON.stringify(o)}` : key);

describe('mapLabels', () => {
  it('maps the parts onto the renderer modes', () => {
    expect(labelModeOf(['code'])).toBe('code');
    expect(labelModeOf(['customer'])).toBe('customer');
    expect(labelModeOf(['end', 'setup'])).toBe('customer');
    expect(labelModeOf(['code', 'end'])).toBe('both');
  });

  it('normalizes the order and falls back to the default when empty', () => {
    expect(normalizeParts(['setup', 'code', 'setup', 'x'])).toEqual(['code', 'setup']);
    expect(normalizeParts([])).toEqual(DEFAULT_LABEL_PARTS);
  });

  it('reads the stored parts per user and migrates the old single choice', () => {
    localStorage.clear();
    expect(readLabelParts('u1')).toEqual(['code', 'customer']);
    localStorage.setItem('arenile.map.labelMode.u1', 'customer');
    expect(readLabelParts('u1')).toEqual(['customer']);
    localStorage.setItem(labelPartsKey('u1'), JSON.stringify(['end', 'code']));
    expect(readLabelParts('u1')).toEqual(['code', 'end']);
    localStorage.setItem(labelPartsKey('u1'), '{bad');
    expect(readLabelParts('u1')).toEqual(['code', 'customer']);
    expect(readLabelParts(undefined)).toEqual(['code', 'customer']);
    localStorage.clear();
  });

  it('formats the end date as day/month', () => {
    expect(shortDate('2026-08-16')).toBe('16/8');
    expect(shortDate('2026-07-05')).toBe('5/7');
  });

  it('builds captions from the chosen parts, the customer only with bookings.read', () => {
    const spots = [
      opSpot('s1', 'A1', 'free'),
      opSpot('s2', 'A2', 'booked', {
        booking: opBooking({ ends_on: '2026-08-16', setup_label: '2L 1S' }),
      }),
      opSpot('s3', 'A3', 'booked', {
        booking: opBooking({ ends_on: '2026-08-20', segment_ends_on: '2026-08-15' }),
      }),
    ];
    expect(labelCaptions(spots, 'FULL', ['customer', 'end', 'setup'], t, true)).toEqual({
      s2: 'Rossi · 16/8 · 2L 1S',
      s3: 'Rossi · 15/8',
    });
    expect(labelCaptions(spots, 'FULL', ['customer', 'setup'], t, false)).toEqual({
      s2: '2L 1S',
    });
    expect(labelCaptions(spots, 'FULL', ['code'], t, true)).toEqual({});
  });

  it('caps the drawn indicators by priority and keeps short lists as they are', () => {
    expect(mapIndicators({ a: ['extra', 'vip'] })).toEqual({ a: ['extra', 'vip'] });
    expect(mapIndicators({ a: ['extra', 'note', 'vip', 'arrival', 'departure', 'group'] })).toEqual(
      { a: ['arrival', 'departure', 'vip', 'note'] },
    );
  });

  it('adds the arrival, departure, agreement and group indicators (spec 30.4)', () => {
    const spots = [
      opSpot('s1', 'A1', 'booked', {
        booking: opBooking({
          is_first_day: true,
          is_last_day: true,
          agreement: { id: 'ag1', name: 'Residenti' },
          group_name: 'Gita CRAL',
        }),
      }),
    ];
    expect(indicatorsBySpot(spots)).toEqual({
      s1: ['arrival', 'departure', 'agreement', 'group'],
    });
  });
});

describe('quickFilters', () => {
  const spots = [
    opSpot('s1', 'A1', 'booked', { booking: opBooking({ is_first_day: true }) }),
    opSpot('s2', 'A2', 'checked_in', {
      booking: opBooking({ is_last_day: true, balance_due: 500, group_name: 'Gita' }),
    }),
    opSpot('s3', 'A3', 'booked', {
      booking: opBooking({
        partner: { id: 'p1', name: 'Hotel Miramare', kind: 'struttura_ricettiva' },
      }),
    }),
    opSpot('s4', 'A4', 'released', { booking: opBooking({ is_first_day: true }) }),
    opSpot('s5', 'A5', 'free'),
  ];

  it('matches arrivals, departures, balances, presents, agreements and groups', () => {
    expect(filterSpots(spots, 'FULL', { kind: 'arrivals' })).toEqual(['s1']);
    expect(filterSpots(spots, 'FULL', { kind: 'departures' })).toEqual(['s2']);
    expect(filterSpots(spots, 'FULL', { kind: 'to_collect' })).toEqual(['s2']);
    expect(filterSpots(spots, 'FULL', { kind: 'present' })).toEqual(['s2']);
    expect(filterSpots(spots, 'FULL', { kind: 'agreement', value: 'p1' })).toEqual(['s3']);
    expect(filterSpots(spots, 'FULL', { kind: 'group', value: 'Gita' })).toEqual(['s2']);
    expect(filterSpots(spots, 'FULL', null)).toEqual([]);
  });

  it('lists the agreements and groups of the day', () => {
    expect(agreementOptions(spots)).toEqual([{ value: 'p1', label: 'Hotel Miramare' }]);
    expect(groupOptions(spots)).toEqual([{ value: 'Gita', label: 'Gita' }]);
  });

  it('intersects search and filter when both are active', () => {
    expect(combineHighlights(['s1', 's2'], ['s2', 's3'])).toEqual(['s2']);
    expect(combineHighlights(null, ['s3'])).toEqual(['s3']);
    expect(combineHighlights(['s1'], null)).toEqual(['s1']);
    expect(combineHighlights(null, null)).toEqual([]);
  });
});

describe('groupMoveTargets', () => {
  const spot = (id: string, x: number, y: number): MapSpot => ({
    id,
    label: id.toUpperCase(),
    x,
    y,
    rotation: 0,
    kind: 'umbrella',
    shape: { type: 'circle', w: 30, h: 30 },
    zone_id: 'z1',
    active: true,
  });
  const spots = [
    spot('a1', 50, 50),
    spot('a2', 100, 50),
    spot('b1', 50, 100),
    spot('b2', 100, 100),
    spot('b3', 150, 100),
  ];

  it('keeps the arrangement: the anchor goes to the target, the others follow', () => {
    expect(groupMoveTargets(spots, ['a1', 'a2'], 'a1', 'b2')).toEqual([
      { fromSpotId: 'a1', toSpotId: 'b2' },
      { fromSpotId: 'a2', toSpotId: 'b3' },
    ]);
  });

  it('leaves a spot unresolved when nothing sits at its new place', () => {
    expect(groupMoveTargets(spots, ['a1', 'a2'], 'a2', 'b1')).toEqual([
      { fromSpotId: 'a1' },
      { fromSpotId: 'a2', toSpotId: 'b1' },
    ]);
  });
});

describe('moveFrom', () => {
  const stay = { starts_on: '2026-07-10', ends_on: '2026-07-20' };

  it('offers today, tomorrow and a date inside a stay already started', () => {
    expect(moveFromChoices(stay, '2026-07-14')).toEqual(['today', 'tomorrow', 'date']);
    expect(moveFromChoices(stay, '2026-07-01')).toEqual(['whole', 'date']);
    expect(
      moveFromChoices({ starts_on: '2026-07-14', ends_on: '2026-07-14' }, '2026-07-14'),
    ).toEqual(['whole']);
  });

  it('defaults to the day shown on the map', () => {
    expect(defaultMoveFrom(stay, '2026-07-14', '2026-07-14')).toEqual({ choice: 'today' });
    expect(defaultMoveFrom(stay, '2026-07-14', '2026-07-15')).toEqual({ choice: 'tomorrow' });
    expect(defaultMoveFrom(stay, '2026-07-14', '2026-07-18')).toEqual({
      choice: 'date',
      date: '2026-07-18',
    });
    expect(defaultMoveFrom(stay, '2026-07-01', '2026-07-01')).toEqual({ choice: 'whole' });
  });

  it('turns the choice into the first day moved (none = the whole line)', () => {
    expect(moveFromDate('whole', stay, '2026-07-01')).toBeUndefined();
    expect(moveFromDate('today', stay, '2026-07-14')).toBe('2026-07-14');
    expect(moveFromDate('tomorrow', stay, '2026-07-14')).toBe('2026-07-15');
    expect(moveFromDate('date', stay, '2026-07-14', '2026-08-01')).toBe('2026-07-20');
    expect(moveFromDate('date', stay, '2026-07-14', '2026-07-10')).toBeUndefined();
  });

  it('computes the common stay of a swap', () => {
    const a = opSpot('s1', 'A1', 'booked', {
      booking: opBooking({ starts_on: '2026-07-10', ends_on: '2026-07-20' }),
    });
    const b = opSpot('s2', 'A2', 'booked', {
      booking: opBooking({ starts_on: '2026-07-14', ends_on: '2026-07-25' }),
    });
    expect(commonWindow(a, b, '2026-07-14')).toEqual({
      starts_on: '2026-07-14',
      ends_on: '2026-07-20',
    });
  });
});

describe('rangeView', () => {
  it('classifies free, partly free and taken spots', () => {
    expect(
      rangeState({ spot_id: 's1', label: 'A1', status: 'available', free_days: 7, total_days: 7 })
        .state,
    ).toBe('free');
    const partial = rangeState({
      spot_id: 's2',
      label: 'A2',
      status: 'unavailable',
      free_days: 3,
      total_days: 7,
      free_dates: ['2026-07-10'],
    });
    expect(partial).toMatchObject({ state: 'partial', freeDays: 3, totalDays: 7 });
    expect(
      rangeState({ spot_id: 's3', label: 'A3', status: 'unavailable', free_days: 0, total_days: 7 })
        .state,
    ).toBe('taken');
  });

  it('marks spots missing from the answer as unavailable', () => {
    const out = rangeSpots(
      {
        from: '2026-07-10',
        to: '2026-07-16',
        slots: 'FULL',
        channel: 'desk',
        zones: [
          {
            zone_id: 'z1',
            code: 'Z',
            assignment_mode: 'seat',
            capacity: 1,
            available: 1,
            spots: [{ spot_id: 's1', label: 'A1', status: 'available' }],
          },
        ],
        pools: [],
      },
      ['s1', 's2'],
    );
    expect(out.get('s1')?.state).toBe('free');
    expect(out.get('s2')?.state).toBe('unavailable');
  });

  it('groups free dates into consecutive runs', () => {
    expect(dateRuns(['2026-07-12', '2026-07-10', '2026-07-11', '2026-07-14'])).toEqual([
      { from: '2026-07-10', to: '2026-07-12' },
      { from: '2026-07-14', to: '2026-07-14' },
    ]);
  });
});

describe('spotCalendar', () => {
  const day = (date: string, am: string, pm = am, booking_id?: string): SpotCalendarDay => ({
    date,
    am: { status: am as SpotCalendarDay['am']['status'], booking_id },
    pm: { status: pm as SpotCalendarDay['pm']['status'], booking_id },
  });

  it('maps cells onto the map palette, open balances as "da saldare"', () => {
    const bookings = new Map([
      [
        'b1',
        {
          booking_id: 'b1',
          code: 'X',
          status: 'confirmed' as const,
          starts_on: '2026-07-01',
          ends_on: '2026-07-02',
          is_season_pass: false,
          balance_due: 100,
        },
      ],
    ]);
    expect(cellStatus({ status: 'booked', booking_id: 'b1' }, bookings)).toBe('to_collect');
    expect(cellStatus({ status: 'booked' }, bookings)).toBe('booked_desk');
    expect(cellStatus({ status: 'season_pass' }, bookings)).toBe('season');
    expect(cellStatus({ status: 'released' }, bookings)).toBe('season_released');
    expect(cellStatus({ status: 'closed' }, bookings)).toBe('unavailable');
  });

  it('groups days by month with 31 columns', () => {
    const months = calendarMonths([day('2026-06-30', 'free'), day('2026-07-01', 'free')]);
    expect(months.map((m) => m.key)).toEqual(['2026-06', '2026-07']);
    expect(months[0]!.days[29]?.date).toBe('2026-06-30');
    expect(months[1]!.days).toHaveLength(31);
  });

  it('proposes the following free days from a free day, or the free half', () => {
    const days = [
      day('2026-07-01', 'free'),
      day('2026-07-02', 'free'),
      day('2026-07-03', 'booked'),
      day('2026-07-04', 'booked', 'free'),
    ];
    expect(proposedStay(days, '2026-07-01')).toEqual({
      startsOn: '2026-07-01',
      endsOn: '2026-07-02',
      slots: 'FULL',
    });
    expect(proposedStay(days, '2026-07-04')).toEqual({
      startsOn: '2026-07-04',
      endsOn: '2026-07-04',
      slots: 'PM',
    });
    expect(proposedStay(days, '2026-07-03')).toBeNull();
  });
});
