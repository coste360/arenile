import { contrastRatio, parseHexColor, resolveTheme, type Theme } from '@arenile/shared/theme';

export interface ThemeWarning {
  /** i18n key under configuration:theme.warnings. */
  key: 'primaryText' | 'accentContrast' | 'similarColors';
  ratio: number;
  /** Colour actually used for text when the theme auto-corrects it. */
  corrected?: string;
}

const WHITE = { r: 255, g: 255, b: 255 };

/**
 * Contrast warnings of a theme (WCAG 2.1 AA): primary used as text on white
 * needs 4.5:1 (the page darkens it automatically), accent needs 3:1 as a UI
 * colour, and primary/accent too close (in RGB) are hard to tell apart.
 */
export function themeWarnings(theme: Theme): ThemeWarning[] {
  const out: ThemeWarning[] = [];
  const resolved = resolveTheme(theme);
  const primary = parseHexColor(resolved.primary);
  const accent = parseHexColor(resolved.accent);
  if (primary) {
    const ratio = contrastRatio(primary, WHITE);
    if (ratio < 4.5) out.push({ key: 'primaryText', ratio, corrected: resolved.primaryOnSurface });
  }
  if (accent) {
    const ratio = contrastRatio(accent, WHITE);
    if (ratio < 3) out.push({ key: 'accentContrast', ratio });
  }
  if (primary && accent) {
    // Hue matters here, not luminance: compare the colours in RGB space.
    const distance = Math.hypot(primary.r - accent.r, primary.g - accent.g, primary.b - accent.b);
    if (distance < 60) out.push({ key: 'similarColors', ratio: contrastRatio(primary, accent) });
  }
  return out;
}

export interface EmbedSnippets {
  url: string;
  iframe: string;
  script: string;
}

/** Public page URL and embed snippets of spec 12.1. */
export function embedSnippets(origin: string, slug: string, lang: string): EmbedSnippets {
  const host = origin.replace(/\/+$/, '');
  const s = encodeURIComponent(slug);
  return {
    url: `${host}/b/${s}`,
    iframe: [
      `<iframe src="${host}/b/${s}?embed=1"`,
      `        style="width:100%;border:0" allow="payment" loading="lazy"`,
      `        title="Prenota il tuo posto"></iframe>`,
    ].join('\n'),
    script: [
      `<div data-arenile="${slug}" data-lang="${lang}"></div>`,
      `<script src="${host}/embed.js" async></script>`,
    ].join('\n'),
  };
}

/**
 * Validates an embed origin: scheme + host (+ port), no path. Returns the
 * normalised origin or null.
 */
export function normalizeOrigin(value: string): string | null {
  const v = value.trim();
  if (!/^https?:\/\/[^/\s]+\/?$/i.test(v)) return null;
  try {
    const u = new URL(v);
    if (u.username || u.password) return null;
    return u.origin;
  } catch {
    return null;
  }
}

/** Splits a textarea (one origin per line, commas allowed) into trimmed non-empty entries. */
export function splitLines(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/** URL-safe slug from a display name ("Lido Sole & Mare" → "lido-sole-mare"). */
export function slugify(name: string): string {
  return name
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
    .replace(/-+$/g, '');
}
