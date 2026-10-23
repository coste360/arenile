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
      view: 'book',
      manageToken: null,
      checkoutBookingId: null,
      embed: true,
      lang: 'en',
      from: '2027-07-10',
      to: '2027-07-12',
      pack: 'base',
      parentOrigin: 'https://x.it',
      hostPage: null,
      preview: false,
      colorScheme: null,
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

  it('recognises the manage area and validates the token', () => {
    const token = 'a'.repeat(43);
    expect(parseBookingLocation(`/b/lido/m/${token}`, '')).toMatchObject({
      slug: 'lido',
      view: 'manage',
      manageToken: token,
    });
    expect(parseBookingLocation('/b/lido/m/short', '')).toMatchObject({
      view: 'manage',
      manageToken: null,
    });
    // Return from a balance payment: the manage area without the token.
    expect(parseBookingLocation('/b/lido/m', '?paid=1&booking=b1')).toMatchObject({
      slug: 'lido',
      view: 'manage',
      manageToken: null,
    });
    expect(parseBookingLocation('/b/lido/m/', '')).toMatchObject({ view: 'manage', manageToken: null });
  });

  it('reads the host page of the widget only in embed mode', () => {
    const page = encodeURIComponent('https://www.lido.it/prenota?x=1');
    expect(parseBookingLocation('/b/lido', `?embed=1&return=${page}`).hostPage).toBe(
      'https://www.lido.it/prenota?x=1',
    );
    expect(parseBookingLocation('/b/lido', `?return=${page}`).hostPage).toBeNull();
    expect(
      parseBookingLocation('/b/lido', '?embed=1&return=javascript:alert(1)').hostPage,
    ).toBeNull();
    expect(parseBookingLocation('/b/lido', '?embed=1&return=/relative').hostPage).toBeNull();
  });

  it('reads the checkout to resume after the payment return', () => {
    const id = '0192F5A0-1C2B-7D3E-8F40-123456789ABC';
    expect(parseBookingLocation('/b/lido', `?checkout=${id}`).checkoutBookingId).toBe(
      id.toLowerCase(),
    );
    expect(parseBookingLocation('/b/lido', '?checkout=nope').checkoutBookingId).toBeNull();
  });

  it('reads the panel preview flag and the forced colour scheme', () => {
    expect(parseBookingLocation('/b/x', '?preview=1&theme=dark')).toMatchObject({
      preview: true,
      colorScheme: 'dark',
    });
    expect(parseBookingLocation('/b/x', '?theme=blue')).toMatchObject({
      preview: false,
      colorScheme: null,
    });
  });

  it('drops malformed dates and defaults embed to false', () => {
    const loc = parseBookingLocation('/b/x', '?from=10/07/2027&embed=0');
    expect(loc.from).toBeNull();
    expect(loc.embed).toBe(false);
  });
});
