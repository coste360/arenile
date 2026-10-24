import { describe, expect, it } from 'vitest';
import { encodeQr, qrPath } from './qr';

// Reference matrices produced by github.com/skip2/go-qrcode (the server's QR
// library) for the same payload at level M, border disabled.
const AR_7K3Q9_MASK2 = [
  '111111100000101111111',
  '100000100010001000001',
  '101110101010101011101',
  '101110101010101011101',
  '101110101111101011101',
  '100000101000101000001',
  '111111101010101111111',
  '000000001001100000000',
  '101111100100101111100',
  '100001001000101010111',
  '000000110101010100010',
  '011001001000000000110',
  '000111110101000100000',
  '000000001001111011001',
  '111111100110101001110',
  '100000101011111010100',
  '101110101000100101001',
  '101110101110100011100',
  '101110101111010101000',
  '100000100010000011100',
  '111111101101010110010',
];

const rows = (m: boolean[][]) => m.map((r) => r.map((b) => (b ? '1' : '0')).join(''));

describe('encodeQr', () => {
  it('matches the server library bit for bit (version 1, mask 2)', () => {
    expect(rows(encodeQr('ar-7k3q9', 2))).toEqual(AR_7K3Q9_MASK2);
  });

  it('grows the version with the payload and keeps the finder patterns', () => {
    expect(encodeQr('AR-7K3Q9')).toHaveLength(21);
    const long = encodeQr('https://lido.example/t/abcdefghijklmnopq');
    expect(long).toHaveLength(29); // version 3, as the server library
    const finder = ['1111111', '1000001', '1011101', '1011101', '1011101', '1000001', '1111111'];
    expect(
      rows(long)
        .slice(0, 7)
        .map((r) => r.slice(0, 7)),
    ).toEqual(finder);
    expect(
      rows(long)
        .slice(0, 7)
        .map((r) => r.slice(-7)),
    ).toEqual(finder);
  });

  it('refuses payloads beyond version 6', () => {
    expect(() => encodeQr('x'.repeat(200))).toThrow(RangeError);
  });

  it('draws one unit square per dark module, offset by the quiet zone', () => {
    const path = qrPath([
      [true, false],
      [false, true],
    ]);
    expect(path).toBe('M4 4h1v1h-1zM5 5h1v1h-1z');
  });
});
