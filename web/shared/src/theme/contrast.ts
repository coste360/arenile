/**
 * WCAG 2.x colour contrast helpers.
 * See https://www.w3.org/TR/WCAG21/#dfn-contrast-ratio
 */

export interface Rgb {
  r: number;
  g: number;
  b: number;
}

const HEX_RE = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i;

/** Parses "#RGB" or "#RRGGBB" (leading # optional). Returns null for invalid input. */
export function parseHexColor(input: string): Rgb | null {
  const match = HEX_RE.exec(input.trim());
  if (!match?.[1]) return null;
  let hex = match[1];
  if (hex.length === 3) {
    hex = hex
      .split('')
      .map((c) => c + c)
      .join('');
  }
  const value = Number.parseInt(hex, 16);
  return { r: (value >> 16) & 0xff, g: (value >> 8) & 0xff, b: value & 0xff };
}

/** Formats an RGB triple as lowercase "#rrggbb". */
export function toHex({ r, g, b }: Rgb): string {
  const part = (n: number) =>
    Math.round(Math.min(255, Math.max(0, n)))
      .toString(16)
      .padStart(2, '0');
  return `#${part(r)}${part(g)}${part(b)}`;
}

function channel(c: number): number {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

/** Relative luminance in [0, 1]. */
export function relativeLuminance(color: Rgb): number {
  return 0.2126 * channel(color.r) + 0.7152 * channel(color.g) + 0.0722 * channel(color.b);
}

/** Contrast ratio in [1, 21] between two colours. */
export function contrastRatio(a: Rgb, b: Rgb): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

/** Minimum contrast for normal-size text at WCAG AA. */
export const WCAG_AA_NORMAL = 4.5;

/** Preferred foreground candidates, in order of preference when they tie. */
const LIGHT_TEXT = '#ffffff';
const DARK_TEXT = '#111827';
const BLACK = '#000000';

/**
 * Returns the text colour to place over `background` so that the pair meets
 * WCAG AA for normal text. White or a near-black slate are preferred; when
 * neither reaches 4.5:1 the function falls back to pure black, which together
 * with white always reaches at least ~4.58:1 for any background.
 */
export function readableTextColor(background: string): string {
  const bg = parseHexColor(background);
  if (!bg) return DARK_TEXT;
  const white = parseHexColor(LIGHT_TEXT) as Rgb;
  const dark = parseHexColor(DARK_TEXT) as Rgb;
  const black = parseHexColor(BLACK) as Rgb;
  const onWhite = contrastRatio(bg, white);
  const onDark = contrastRatio(bg, dark);
  const best = onWhite >= onDark ? LIGHT_TEXT : DARK_TEXT;
  if (Math.max(onWhite, onDark) >= WCAG_AA_NORMAL) return best;
  return onWhite >= contrastRatio(bg, black) ? LIGHT_TEXT : BLACK;
}

/**
 * Darkens or lightens `color` towards black/white until it reaches `minRatio`
 * against `against` (used e.g. for links in the primary colour on a white page).
 */
export function ensureContrast(color: string, against: string, minRatio = WCAG_AA_NORMAL): string {
  const fg = parseHexColor(color);
  const bg = parseHexColor(against);
  if (!fg || !bg) return color;
  if (contrastRatio(fg, bg) >= minRatio) return toHex(fg);
  const target = relativeLuminance(bg) > 0.5 ? { r: 0, g: 0, b: 0 } : { r: 255, g: 255, b: 255 };
  for (let step = 1; step <= 20; step++) {
    const t = step / 20;
    const mixed = {
      r: fg.r + (target.r - fg.r) * t,
      g: fg.g + (target.g - fg.g) * t,
      b: fg.b + (target.b - fg.b) * t,
    };
    if (contrastRatio(mixed, bg) >= minRatio) return toHex(mixed);
  }
  return toHex(target);
}
