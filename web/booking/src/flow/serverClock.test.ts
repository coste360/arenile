import { afterEach, describe, expect, it } from 'vitest';
import { observeServerDate, resetServerClock, serverNow } from './serverClock';

describe('server clock', () => {
  afterEach(() => resetServerClock());

  it('follows the server when the device clock is off', () => {
    const local = Date.parse('2027-07-10T08:00:00Z');
    observeServerDate('Sat, 10 Jul 2027 08:15:00 GMT', local);
    expect(serverNow() - Date.now()).toBeGreaterThan(14 * 60_000);
  });

  it('ignores sub-second jitter and malformed headers', () => {
    const local = Date.parse('2027-07-10T08:00:00.800Z');
    observeServerDate('Sat, 10 Jul 2027 08:00:00 GMT', local);
    expect(Math.abs(serverNow() - Date.now())).toBeLessThan(50);
    observeServerDate('not a date', local);
    observeServerDate(null, local);
    expect(Math.abs(serverNow() - Date.now())).toBeLessThan(50);
  });
});
