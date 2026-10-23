import { describe, expect, it } from 'vitest';
import { shiftYear } from '../season-rollover/dates';
import { remaining, zonedDate, zonedInstant } from './lib';

describe('zonedInstant', () => {
  it('turns the end of a local day into the UTC instant (summer and winter time)', () => {
    expect(zonedInstant('2027-04-30', 'Europe/Rome')).toBe('2027-04-30T21:59:00Z');
    expect(zonedInstant('2027-01-15', 'Europe/Rome')).toBe('2027-01-15T22:59:00Z');
    expect(zonedInstant('2027-03-28', 'Europe/Rome', '12:00')).toBe('2027-03-28T10:00:00Z');
    expect(zonedInstant('2027-06-01', 'UTC', '08:30')).toBe('2027-06-01T08:30:00Z');
  });

  it('round-trips with zonedDate', () => {
    expect(zonedDate(zonedInstant('2027-10-31', 'Europe/Rome'), 'Europe/Rome')).toBe('2027-10-31');
    expect(zonedDate('2027-04-30T22:30:00Z', 'Europe/Rome')).toBe('2027-05-01');
  });
});

describe('remaining', () => {
  const now = Date.parse('2027-04-01T10:00:00Z');
  it('splits the time left in days, hours and minutes', () => {
    expect(remaining('2027-04-03T14:30:59Z', now)).toEqual({
      expired: false,
      days: 2,
      hours: 4,
      minutes: 30,
    });
    expect(remaining('2027-04-01T10:05:00Z', now)).toMatchObject({ days: 0, hours: 0, minutes: 5 });
  });

  it('is expired at and after the deadline', () => {
    expect(remaining('2027-04-01T10:00:00Z', now).expired).toBe(true);
    expect(remaining('2027-03-01T10:00:00Z', now).expired).toBe(true);
    expect(remaining('not a date', now).expired).toBe(true);
  });
});

describe('shiftYear', () => {
  it('moves a date to another year, 29 February to 28', () => {
    expect(shiftYear('2026-05-15', 1)).toBe('2027-05-15');
    expect(shiftYear('2028-02-29', 1)).toBe('2029-02-28');
    expect(shiftYear('2027-09-30', -2)).toBe('2025-09-30');
  });
});
