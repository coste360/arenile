import { describe, expect, it } from 'vitest';
import { checkHost, printerAddress, routableKinds, toRoutes, toRows } from './lib';

describe('checkHost', () => {
  it.each([
    ['192.168.1.50', 'ok'],
    ['10.0.0.7', 'ok'],
    ['172.16.0.1', 'ok'],
    ['172.31.255.255', 'ok'],
    ['127.0.0.1', 'ok'],
    ['169.254.10.20', 'ok'],
    ['169.254.169.254', 'public'],
    ['100.64.1.2', 'ok'],
    ['100.128.0.1', 'public'],
    ['fe80::1', 'ok'],
    ['fd12:3456::1', 'ok'],
    ['::1', 'ok'],
    ['stampante-bar.local', 'ok'],
    ['8.8.8.8', 'public'],
    ['172.32.0.1', 'public'],
    ['2001:db8::1', 'public'],
    ['300.1.1.1', 'invalid'],
    ['', 'invalid'],
    ['bad host', 'invalid'],
  ])('%s → %s', (host, want) => {
    expect(checkHost(host)).toBe(want);
  });
});

describe('routableKinds', () => {
  it('offers cash documents first and tickets only when the server prints them', () => {
    expect(routableKinds(false)).toEqual(['cash_z', 'payment_receipt']);
    expect(routableKinds(true)).toEqual(['cash_z', 'payment_receipt', 'daily_ticket']);
  });
});

describe('printerAddress', () => {
  it('omits the default port and brackets IPv6', () => {
    expect(printerAddress({ host: '192.168.1.50', port: 9100 })).toBe('192.168.1.50');
    expect(printerAddress({ host: '192.168.1.50', port: 9101 })).toBe('192.168.1.50:9101');
    expect(printerAddress({ host: 'fe80::1', port: 9101 })).toBe('[fe80::1]:9101');
  });
});

describe('routing rows', () => {
  it('round-trips routes, default rule first, and drops the test page', () => {
    const routes = [
      { document_kind: 'cash_z' as const, register_id: 'r2', printer_id: 'p1', copies: 1 },
      { document_kind: 'cash_z' as const, printer_id: 'p2', copies: 2, kick_drawer: true },
      { document_kind: 'test_page' as const, printer_id: 'p1', copies: 1 },
    ];
    const rows = toRows(routes);
    expect(rows.map((r) => r.registerId)).toEqual(['', 'r2']);
    expect(toRoutes(rows)).toEqual([
      { document_kind: 'cash_z', printer_id: 'p2', copies: 2, kick_drawer: true },
      {
        document_kind: 'cash_z',
        register_id: 'r2',
        printer_id: 'p1',
        copies: 1,
        kick_drawer: false,
      },
    ]);
  });
});
