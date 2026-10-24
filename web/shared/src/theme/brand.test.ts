import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { contrastRatio, parseHexColor, relativeLuminance, type Rgb } from './contrast';
import { DARK_SURFACE, DEFAULT_PRIMARY, resolveTheme, themeToCssVars } from './theme';

/**
 * WCAG AA of the default theme (styles/globals.css, Coste360 design system colours, ADR 0018):
 * every text/background token pair of both colour schemes reaches 4.5:1, and
 * the focus ring 3:1 against the page (WCAG 1.4.11).
 */
const css = readFileSync(resolve(import.meta.dirname, '../styles/globals.css'), 'utf8');

function block(selector: string): Record<string, string> {
  const start = css.indexOf(`\n${selector} {`);
  if (start < 0) throw new Error(`no ${selector} block`);
  const body = css.slice(start, css.indexOf('\n}', start));
  const vars: Record<string, string> = {};
  for (const m of body.matchAll(/^\s*--([\w-]+):\s*([^;]+);/gm))
    vars[m[1] as string] = m[2] as string;
  return vars;
}

/** oklch(L C H) to sRGB (CSS Color 4 reference matrices), clamped to the gamut. */
function oklchToRgb(l: number, c: number, h: number): Rgb {
  const a = c * Math.cos((h * Math.PI) / 180);
  const b = c * Math.sin((h * Math.PI) / 180);
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const lin = [
    4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_,
    -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_,
    -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_,
  ].map((v) => {
    const x = Math.min(1, Math.max(0, v));
    return 255 * (x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055);
  });
  return { r: lin[0] as number, g: lin[1] as number, b: lin[2] as number };
}

/** Resolves a token to an opaque colour; var(--x, fallback) uses the fallback. */
function color(vars: Record<string, string>, name: string): Rgb {
  let value = vars[name];
  if (!value) throw new Error(`no --${name}`);
  const fallback = /^var\(--[\w-]+,\s*(.+)\)$/.exec(value);
  if (fallback) value = fallback[1] as string;
  const hex = parseHexColor(value);
  if (hex) return hex;
  const ok = /^oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)$/.exec(value);
  if (ok) return oklchToRgb(Number(ok[1]), Number(ok[2]), Number(ok[3]));
  throw new Error(`--${name}: unsupported value ${value}`);
}

const light = block(':root');
const dark = { ...light, ...block('.dark') };

const TEXT_PAIRS: [string, string][] = [
  ['foreground', 'background'],
  ['card-foreground', 'card'],
  ['popover-foreground', 'popover'],
  ['primary-foreground', 'primary'],
  ['secondary-foreground', 'secondary'],
  ['muted-foreground', 'muted'],
  ['muted-foreground', 'background'],
  ['muted-foreground', 'card'],
  ['muted-foreground', 'accent'],
  ['muted-foreground', 'secondary'],
  ['muted-foreground', 'sidebar'],
  ['muted-foreground', 'sidebar-accent'],
  ['accent-foreground', 'accent'],
  ['highlight-foreground', 'highlight'],
  ['primary-on-surface', 'background'],
  ['primary-on-surface', 'card'],
  ['primary-on-surface', 'muted'],
  ['primary-on-surface', 'accent'],
  ['primary-on-surface', 'secondary'],
  ['primary-on-surface', 'sidebar'],
  ['sidebar-foreground', 'sidebar'],
  ['sidebar-primary-foreground', 'sidebar-primary'],
  ['sidebar-accent-foreground', 'sidebar-accent'],
  // Error and success messages (text-destructive, text-success) on the surfaces.
  ['destructive', 'background'],
  ['destructive', 'card'],
  ['destructive', 'muted'],
  ['destructive', 'accent'],
  ['success', 'background'],
  ['success', 'card'],
];

describe.each([
  ['light', light],
  ['dark', dark],
])('brand theme, %s scheme', (_, vars) => {
  it.each(TEXT_PAIRS)('%s on %s reaches WCAG AA (4.5:1)', (fg, bg) => {
    expect(contrastRatio(color(vars, fg), color(vars, bg))).toBeGreaterThanOrEqual(4.5);
  });

  it('inactive tab labels (foreground at 60%) reach AA on the tab list', () => {
    const fg = color(vars, 'foreground');
    const bg = color(vars, 'muted');
    const mixed = {
      r: fg.r * 0.6 + bg.r * 0.4,
      g: fg.g * 0.6 + bg.g * 0.4,
      b: fg.b * 0.6 + bg.b * 0.4,
    };
    // Light scheme only: the dark scheme uses muted-foreground (covered above).
    if (vars === light) expect(contrastRatio(mixed, bg)).toBeGreaterThanOrEqual(4.5);
  });

  it('destructive text reaches AA on its 10% tint (danger badges, menu items)', () => {
    for (const bg of ['background', 'card']) {
      const fg = color(vars, 'destructive');
      const base = color(vars, bg);
      const tint = {
        r: fg.r * 0.1 + base.r * 0.9,
        g: fg.g * 0.1 + base.g * 0.9,
        b: fg.b * 0.1 + base.b * 0.9,
      };
      // Light scheme: text-destructive on bg-destructive/10 (the dark scheme uses lighter text).
      if (vars === light) expect(contrastRatio(fg, tint)).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('input borders reach 3:1 against the page, the cards and the panels (WCAG 1.4.11)', () => {
    for (const bg of ['background', 'card', 'muted', 'secondary', 'accent']) {
      expect(contrastRatio(color(vars, 'input'), color(vars, bg))).toBeGreaterThanOrEqual(3);
    }
  });

  it('focus ring reaches 3:1 against the page and the cards', () => {
    for (const bg of ['background', 'card']) {
      expect(contrastRatio(color(vars, 'ring'), color(vars, bg))).toBeGreaterThanOrEqual(3);
    }
  });
});

describe('brand defaults', () => {
  it('uses the Coste360 design system colours', () => {
    expect(light.primary).toBe('#0b2542');
    expect(color(light, 'ring')).toEqual(parseHexColor('#185482'));
    expect(light.foreground).toBe('#0f172a');
    expect(light['muted-foreground']).toBe('#475569');
    expect(light.background).toBe('#f8fafc');
    // Coste360 error one step darker (red-700) for AA on its tint.
    expect(light.destructive).toBe('#b91c1c');
    expect(dark.primary).toBe('#38bdf8');
    expect(dark['primary-foreground']).toBe('#0b2542');
  });

  it('DARK_SURFACE is the lightest dark surface behind links', () => {
    const surface = relativeLuminance(parseHexColor(DARK_SURFACE) as Rgb);
    for (const name of ['background', 'card', 'muted', 'accent', 'secondary']) {
      expect(surface).toBeGreaterThanOrEqual(relativeLuminance(color(dark, name)) - 1e-3);
    }
  });

  it('the public page default primary is Coste360 primary-lighter, AA with its foreground', () => {
    expect(DEFAULT_PRIMARY).toBe('#185482');
    const t = resolveTheme(null);
    // Selected labels on a primary/10 tint (public page steps) stay AA.
    const tint = {
      r: 0.15 * 24 + 0.85 * 255,
      g: 0.15 * 84 + 0.85 * 255,
      b: 0.15 * 130 + 0.85 * 255,
    };
    expect(contrastRatio(parseHexColor(t.primaryOnSurface) as Rgb, tint)).toBeGreaterThanOrEqual(
      4.5,
    );
    expect(
      contrastRatio(parseHexColor(t.primary) as Rgb, parseHexColor(t.primaryForeground) as Rgb),
    ).toBeGreaterThanOrEqual(4.5);
  });

  it('the public page focus ring reaches 3:1 in both schemes with the default theme', () => {
    const vars = themeToCssVars(null);
    expect(vars).not.toHaveProperty('--ring');
    const ring = (name: string) => parseHexColor(vars[name] as string) as Rgb;
    for (const bg of ['background', 'card']) {
      expect(contrastRatio(ring('--ring-light'), color(light, bg))).toBeGreaterThanOrEqual(3);
      expect(contrastRatio(ring('--ring-dark'), color(dark, bg))).toBeGreaterThanOrEqual(3);
    }
  });
});

/**
 * Map status colours on the sand (ADR 0024): the modern kit fills the spot
 * body with --status-*, so the body itself must stand out from the beach
 * (WCAG 1.4.11, 3:1 for graphical objects) in both schemes. Season pass
 * released and unavailable are faint on purpose (absent or unavailable: they
 * carry a badge or the greyed art); every other status reaches 3:1. Free is a
 * neutral grey, apart from blocked, unavailable and present by 2:1 in
 * lightness. "Booked online" and "present" are both blue: they also differ in
 * lightness (2:1), so they stay apart for colour-blind users.
 */
const STRONG_STATUSES = [
  'free',
  'hold',
  'booked-online',
  'booked-desk',
  'season',
  'to-collect',
  'present',
  'blocked',
  'renewal-hold',
];

describe.each([
  ['light', light],
  ['dark', dark],
])('map status colours, %s scheme', (_, vars) => {
  const sand = color(vars, 'beach-sand-soft');

  it.each(STRONG_STATUSES)('%s reaches 3:1 on the sand', (status) => {
    expect(contrastRatio(color(vars, `status-${status}`), sand)).toBeGreaterThanOrEqual(3);
  });

  it('free (Libero) reaches 3:1 on every sand and on the map backdrop', () => {
    for (const bg of ['beach-sand-soft', 'beach-sand', 'map-sand']) {
      expect(contrastRatio(color(vars, 'status-free'), color(vars, bg))).toBeGreaterThanOrEqual(3);
    }
  });

  it('free is a grey apart from blocked, unavailable and present by 2:1', () => {
    const free = color(vars, 'status-free');
    const { r, g, b } = free;
    // Neutral: no channel more than 12% away from the others (a grey, not a hue).
    expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThanOrEqual(0.12 * 255);
    for (const other of ['status-blocked', 'status-unavailable', 'status-present']) {
      expect(contrastRatio(free, color(vars, other))).toBeGreaterThanOrEqual(2);
    }
  });

  it('booked online and present differ in lightness by at least 2:1', () => {
    expect(
      contrastRatio(color(vars, 'status-booked-online'), color(vars, 'status-present')),
    ).toBeGreaterThanOrEqual(2);
  });

  it('the selection outline reaches 3:1 on the sand', () => {
    expect(contrastRatio(color(vars, 'map-spot-selected'), sand)).toBeGreaterThanOrEqual(3);
  });
});
