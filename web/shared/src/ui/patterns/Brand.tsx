import { useTranslation } from 'react-i18next';
import appIconUrl from '../../brand/arenile-app-icon-flat.svg';
import logoNavyUrl from '../../brand/arenile-logo-horizontal-navy.svg';
import logoWhiteUrl from '../../brand/arenile-logo-horizontal-white.svg';
import logoUrl from '../../brand/arenile-logo-horizontal.svg';
import nameNavyUrl from '../../brand/arenile-name-navy.svg';
import nameWhiteUrl from '../../brand/arenile-name-white.svg';
import symbolUrl from '../../brand/arenile-symbol-color.svg';
import symbolNavyUrl from '../../brand/arenile-symbol-navy.svg';
import symbolWhiteUrl from '../../brand/arenile-symbol-white.svg';
import coste360PrimaryUrl from '../../brand/coste360-logo-primary.png';
import coste360WhiteUrl from '../../brand/coste360-logo-white.png';
import { cn } from '../../lib/utils';

/**
 * Official brand marks (Arenile brand kit v2.0 and the Coste360 logo,
 * assets/brand, ADR 0018). The SVGs have their text converted to outlines, so
 * they render as <img> without loading Titillium Web. Light/dark pairs swap
 * with the `dark:` variant; callers size them with a height class (`h-8`).
 * The files under shared/src/brand are copies of assets/brand (a test keeps
 * them identical): never edit them here.
 *
 * Usage rules (ADR 0018): the app icon is the full-colour tile; on light
 * surfaces the main mark is the colour logo with the navy "A"; on navy and in
 * dark mode it is white; single-colour marks on light surfaces are navy.
 */

export const ARENILE_URL = 'https://arenile.app';
export const COSTE360_URL = 'https://www.coste360.it/';

interface BrandProps {
  className?: string;
}

/** Light/dark pair of the same mark; only one is displayed (the other is display:none). */
function SchemePair({
  light,
  dark,
  alt,
  className,
}: {
  light: string;
  dark: string;
  alt: string;
  className?: string;
}) {
  return (
    <>
      {/* Paper is always light: the print uses the light-surface mark also in dark mode. */}
      <img src={light} alt={alt} className={cn('w-auto dark:hidden print:block', className)} />
      <img
        src={dark}
        alt={alt}
        className={cn('hidden w-auto dark:block print:hidden', className)}
      />
    </>
  );
}

/** App icon (navy tile with the sand A): app chrome, compact places. Decorative by default. */
export function ArenileAppIcon({ className, alt = '' }: BrandProps & { alt?: string }) {
  return <img src={appIconUrl} alt={alt} className={cn('size-8 shrink-0', className)} />;
}

/**
 * Colour treatment of a mark:
 * - `colour`: colour mark with the navy A on light surfaces, white in dark mode (main brand spots);
 * - `white`: white in both schemes (navy bands, heroes, primary-coloured headers);
 * - `mono`: single colour, navy on light surfaces and white in dark mode (small marks, "powered by", print).
 */
export type BrandTone = 'colour' | 'white' | 'mono';

const SYMBOL: Record<BrandTone, [string, string]> = {
  colour: [symbolUrl, symbolWhiteUrl],
  white: [symbolWhiteUrl, symbolWhiteUrl],
  mono: [symbolNavyUrl, symbolWhiteUrl],
};
const LOGO: Record<BrandTone, [string, string]> = {
  colour: [logoUrl, logoWhiteUrl],
  white: [logoWhiteUrl, logoWhiteUrl],
  mono: [logoNavyUrl, logoWhiteUrl],
};
const NAME: Record<BrandTone, [string, string]> = {
  colour: [nameNavyUrl, nameWhiteUrl],
  white: [nameWhiteUrl, nameWhiteUrl],
  mono: [nameNavyUrl, nameWhiteUrl],
};

/** One mark in the light/dark pair of a tone (a single <img> when both are the same file). */
function Toned({
  pair: [light, dark],
  alt,
  className,
}: {
  pair: [string, string];
  alt: string;
  className?: string;
}) {
  if (light === dark) return <img src={light} alt={alt} className={cn('w-auto', className)} />;
  return <SchemePair light={light} dark={dark} alt={alt} className={className} />;
}

/** The "A" symbol alone (no tile). */
export function ArenileSymbol({
  className,
  tone = 'colour',
  alt = 'Arenile',
}: BrandProps & { tone?: BrandTone; alt?: string }) {
  return <Toned pair={SYMBOL[tone]} alt={alt} className={cn('h-8 shrink-0', className)} />;
}

/**
 * Single-colour symbol (navy on light, white in dark) without the other
 * tones: for budgeted pages such as the public booking page ("powered by").
 */
export function ArenileMonoSymbol({ className, alt = 'Arenile' }: BrandProps & { alt?: string }) {
  return (
    <SchemePair
      light={symbolNavyUrl}
      dark={symbolWhiteUrl}
      alt={alt}
      className={cn('h-8 shrink-0', className)}
    />
  );
}

/** "Arenile" lettering alone (Titillium Web 700, outlined). */
export function ArenileName({ className, tone = 'colour' }: BrandProps & { tone?: BrandTone }) {
  return <Toned pair={NAME[tone]} alt="Arenile" className={className} />;
}

/** Horizontal logo: symbol and "Arenile" (no byline in kit v2.0). */
export function ArenileLogo({ className, tone = 'colour' }: BrandProps & { tone?: BrandTone }) {
  return <Toned pair={LOGO[tone]} alt="Arenile" className={cn('h-14', className)} />;
}

/** Coste360.it company logo: primary navy in light, white in dark. */
export function Coste360Logo({ className }: BrandProps) {
  return (
    <SchemePair
      light={coste360PrimaryUrl}
      dark={coste360WhiteUrl}
      alt="Coste360.it"
      className={cn('h-12', className)}
    />
  );
}

/** "Un prodotto di" + Coste360 logo, linking to the company site (login, first run). */
export function MadeByCoste360({ className }: BrandProps) {
  const { t } = useTranslation();
  return (
    <a
      href={COSTE360_URL}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(
        'inline-flex items-center gap-2 rounded-md text-xs text-muted-foreground outline-ring/50 focus-visible:outline-2',
        className,
      )}
    >
      <span>{t('brand.madeBy')}</span>
      <Coste360Logo className="h-10" />
    </a>
  );
}

export type BrandLogoVariant = 'icon' | 'symbol' | 'name' | 'horizontal';

/**
 * The one entry point for the Arenile mark in the UI (never a lucide icon as
 * brand): `icon` = full-colour app icon tile (tone ignored), `symbol` = the A
 * alone, `name` = lettering, `horizontal` = symbol + lettering. `tone` picks
 * the colour treatment (see BrandTone).
 */
export function BrandLogo({
  variant = 'horizontal',
  tone = 'colour',
  className,
  decorative = false,
}: BrandProps & {
  variant?: BrandLogoVariant;
  tone?: BrandTone;
  /** Icon and symbol: hidden from assistive tech when the text next to it names the page. */
  decorative?: boolean;
}) {
  if (variant === 'icon')
    return <ArenileAppIcon className={className} alt={decorative ? '' : 'Arenile'} />;
  if (variant === 'symbol')
    return <ArenileSymbol className={className} tone={tone} alt={decorative ? '' : 'Arenile'} />;
  if (variant === 'name') return <ArenileName className={className} tone={tone} />;
  return <ArenileLogo className={className} tone={tone} />;
}

/** Discreet Coste360 signature for app chrome (sidebar footer): small, muted, linked. */
export function Coste360Signature({ className }: BrandProps) {
  const { t } = useTranslation();
  return (
    <a
      href={COSTE360_URL}
      target="_blank"
      rel="noopener noreferrer"
      title={t('brand.lockup')}
      className={cn(
        'flex w-fit items-center rounded-md px-2 py-1 opacity-60 outline-ring/50 transition-opacity hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-2',
        className,
      )}
    >
      <Coste360Logo className="h-8" />
    </a>
  );
}
