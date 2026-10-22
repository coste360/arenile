import { describe, expect, it } from 'vitest';
import { parseBookingLocation } from './location';

describe('parseBookingLocation', () => {
  it('reads slug and query parameters', () => {
    expect(
      parseBookingLocation(
        '/b/lido-demo',
        '?lang=en&from=2027-07-10&to=2027-07-12&pack=base&embed=1&origin=https://x.it',
      ),
    ).toEqual({
      slug: 'lido-demo',
      embed: true,
      lang: 'en',
      from: '2027-07-10',
      to: '2027-07-12',
      pack: 'base',
      parentOrigin: 'https://x.it',
    });
  });

  it.each([
    ['/b/', null],
    ['/b/Lido-Demo/', 'lido-demo'],
    ['/b/lido-demo/m/token', 'lido-demo'],
    ['/b/-bad', null],
    ['/b/%E0%A4%A', null],
    ['/admin/x', null],
  ])('slug of %s is %j', (path, slug) => {
    expect(parseBookingLocation(path, '').slug).toBe(slug);
  });

  it('drops malformed dates and defaults embed to false', () => {
    const loc = parseBookingLocation('/b/x', '?from=10/07/2027&embed=0');
    expect(loc.from).toBeNull();
    expect(loc.embed).toBe(false);
  });
});
