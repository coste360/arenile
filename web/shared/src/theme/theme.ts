import { ensureContrast, parseHexColor, readableTextColor, toHex } from './contrast';

/** Public page theme (spec 12.5). Every field is optional on the wire. */
export interface Theme {
  logo_url?: string | null;
  primary_color?: string | null;
  accent_color?: string | null;
  font_family?: 'Inter' | 'system' | 'serif' | string | null;
  radius?: 'none' | 'sm' | 'md' | 'lg' | string | null;
  show_prices_on_map?: boolean | null;
  hide_header_in_embed?: boolean | null;
  custom_texts?: Record<string, Record<string, string>> | null;
}

/** Theme with defaults applied and values validated. */
export interface ResolvedTheme {
  logoUrl: string | null;
  primary: string;
  primaryForeground: string;
  primaryOnSurface: string;
  accent: string;
  accentForeground: string;
  fontFamily: string;
  radius: string;
  showPricesOnMap: boolean;
  hideHeaderInEmbed: boolean;
  customTexts: Record<string, Record<string, string>>;
}

export const DEFAULT_PRIMARY = '#0e6ba8';
export const DEFAULT_ACCENT = '#f2b134';
const SURFACE = '#ffffff';

const FONT_STACKS: Record<string, string> = {
  Inter: "Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
  system: "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
  serif: "ui-serif, Georgia, Cambria, 'Times New Roman', serif",
};

const RADII: Record<string, string> = {
  none: '0px',
  sm: '0.25rem',
  md: '0.5rem',
  lg: '1rem',
};

function color(value: string | null | undefined, fallback: string): string {
  const parsed = value ? parseHexColor(value) : null;
  return parsed ? toHex(parsed) : fallback;
}

function safeUrl(value: string | null | undefined): string | null {
  if (!value) return null;
  try {
    const url = new URL(value, 'https://placeholder.invalid');
    return url.protocol === 'https:' || url.protocol === 'http:' ? value : null;
  } catch {
    return null;
  }
}

/** Applies defaults and validation to a raw theme. Never throws. */
export function resolveTheme(theme: Theme | null | undefined): ResolvedTheme {
  const t = theme ?? {};
  const primary = color(t.primary_color, DEFAULT_PRIMARY);
  const accent = color(t.accent_color, DEFAULT_ACCENT);
  return {
    logoUrl: safeUrl(t.logo_url),
    primary,
    primaryForeground: readableTextColor(primary),
    primaryOnSurface: ensureContrast(primary, SURFACE),
    accent,
    accentForeground: readableTextColor(accent),
    fontFamily: FONT_STACKS[t.font_family ?? ''] ?? (FONT_STACKS.system as string),
    radius: RADII[t.radius ?? ''] ?? (RADII.md as string),
    showPricesOnMap: t.show_prices_on_map ?? true,
    hideHeaderInEmbed: t.hide_header_in_embed ?? true,
    customTexts: t.custom_texts ?? {},
  };
}

/** CSS custom properties consumed by tokens.css and the Tailwind theme. */
export function themeToCssVars(
  theme: Theme | ResolvedTheme | null | undefined,
): Record<string, string> {
  const r = theme && 'primaryForeground' in theme ? theme : resolveTheme(theme as Theme);
  return {
    '--ar-primary': r.primary,
    '--ar-primary-foreground': r.primaryForeground,
    '--ar-primary-on-surface': r.primaryOnSurface,
    '--ar-accent': r.accent,
    '--ar-accent-foreground': r.accentForeground,
    '--ar-font-family': r.fontFamily,
    '--ar-radius': r.radius,
  };
}

/** Writes the theme variables on `element` (defaults to <html>). */
export function applyTheme(
  theme: Theme | ResolvedTheme | null | undefined,
  element: HTMLElement = document.documentElement,
): void {
  for (const [name, value] of Object.entries(themeToCssVars(theme))) {
    element.style.setProperty(name, value);
  }
}

/** Looks up a manager-provided custom text with fallback to the default locale. */
export function customText(
  theme: ResolvedTheme,
  key: string,
  locale: string,
  fallbackLocale: string,
): string | undefined {
  return theme.customTexts[locale]?.[key] ?? theme.customTexts[fallbackLocale]?.[key];
}
