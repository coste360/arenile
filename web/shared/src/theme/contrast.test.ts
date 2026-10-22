import { describe, expect, it } from 'vitest';
import {
  WCAG_AA_NORMAL,
  contrastRatio,
  ensureContrast,
  parseHexColor,
  readableTextColor,
  relativeLuminance,
} from './contrast';

const rgb = (hex: string) => {
  const c = parseHexColor(hex);
  if (!c) throw new Error(`bad colour ${hex}`);
  return c;
};

describe('parseHexColor', () => {
  it.each([
    ['#fff', { r: 255, g: 255, b: 255 }],
    ['000000', { r: 0, g: 0, b: 0 }],
    ['#0E6BA8', { r: 14, g: 107, b: 168 }],
  ])('parses %s', (input, expected) => {
    expect(parseHexColor(input)).toEqual(expected);
  });

  it.each(['', 'red', '#12', '#1234567', 'url(x)'])('rejects %j', (input) => {
    expect(parseHexColor(input)).toBeNull();
  });
});

describe('contrastRatio', () => {
  it('matches the WCAG reference values', () => {
    expect(contrastRatio(rgb('#000'), rgb('#fff'))).toBeCloseTo(21, 5);
    expect(contrastRatio(rgb('#fff'), rgb('#fff'))).toBeCloseTo(1, 5);
    expect(contrastRatio(rgb('#767676'), rgb('#fff'))).toBeCloseTo(4.54, 2);
  });

  it('is symmetric', () => {
    expect(contrastRatio(rgb('#0e6ba8'), rgb('#f2b134'))).toBeCloseTo(
      contrastRatio(rgb('#f2b134'), rgb('#0e6ba8')),
      10,
    );
  });

  it('luminance is bounded', () => {
    expect(relativeLuminance(rgb('#000'))).toBe(0);
    expect(relativeLuminance(rgb('#fff'))).toBeCloseTo(1, 10);
  });
});

describe('readableTextColor', () => {
  it.each([
    ['#0e6ba8', '#ffffff'],
    ['#f2b134', '#111827'],
    ['#ffffff', '#111827'],
    ['#000000', '#ffffff'],
    ['#ffff00', '#111827'],
  ])('picks a readable colour on %s', (bg, expected) => {
    expect(readableTextColor(bg)).toBe(expected);
  });

  it('meets WCAG AA for every colour of a dense RGB sweep', () => {
    const steps = [0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187, 204, 221, 238, 255];
    for (const r of steps) {
      for (const g of steps) {
        for (const b of steps) {
          const bg = { r, g, b };
          const fg = rgb(
            readableTextColor(`#${[r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('')}`),
          );
          expect(contrastRatio(bg, fg)).toBeGreaterThanOrEqual(WCAG_AA_NORMAL);
        }
      }
    }
  });

  it('falls back to dark text for invalid input', () => {
    expect(readableTextColor('not-a-colour')).toBe('#111827');
  });
});

describe('ensureContrast', () => {
  it('keeps colours that already pass', () => {
    expect(ensureContrast('#0e6ba8', '#ffffff')).toBe('#0e6ba8');
  });

  it('darkens light colours on a white surface until they pass', () => {
    const out = ensureContrast('#f2b134', '#ffffff');
    expect(contrastRatio(rgb(out), rgb('#ffffff'))).toBeGreaterThanOrEqual(WCAG_AA_NORMAL);
  });
});
