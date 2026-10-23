import { useTranslation } from 'react-i18next';
import appIconUrl from '../../brand/arenile-app-icon-flat.svg';
import logoDarkUrl from '../../brand/arenile-logo-horizontal-dark.svg';
import logoUrl from '../../brand/arenile-logo-horizontal.svg';
import nameNavyUrl from '../../brand/arenile-name-navy.svg';
import nameWhiteUrl from '../../brand/arenile-name-white.svg';
import coste360PrimaryUrl from '../../brand/coste360-logo-primary.svg';
import coste360WhiteUrl from '../../brand/coste360-logo-white.svg';
import { cn } from '../../lib/utils';

/**
 * Official brand marks (Arenile brand kit v1.0 and the Coste360 lockup,
 * assets/brand, ADR 0018). The SVGs have their text converted to outlines, so
 * they render as <img> without loading Titillium Web. Light/dark pairs swap
 * with the `dark:` variant; callers size them with a height class (`h-8`).
 * The files under shared/src/brand are copies of assets/brand (a test keeps
 * them identical): never edit them here.
 */

export const ARENILE_URL = 'https://arenile.app';
export const COSTE360_URL = 'https://coste360.it';

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
      <img src={light} alt={alt} className={cn('w-auto dark:hidden', className)} />
      <img src={dark} alt={alt} className={cn('hidden w-auto dark:block', className)} />
    </>
  );
}

/** App icon (navy tile with the A): sidebar header, compact places. Decorative by default. */
export function ArenileAppIcon({ className, alt = '' }: BrandProps & { alt?: string }) {
  return <img src={appIconUrl} alt={alt} className={cn('size-8 shrink-0', className)} />;
}

/** "Arenile" lettering alone (Titillium Web 700, outlined), navy in light, white in dark. */
export function ArenileName({ className }: BrandProps) {
  return <SchemePair light={nameNavyUrl} dark={nameWhiteUrl} alt="Arenile" className={className} />;
}

/** Horizontal logo "Arenile — Coste360.it" (symbol, name, byline). */
export function ArenileLogo({ className }: BrandProps) {
  const { t } = useTranslation();
  return (
    <SchemePair
      light={logoUrl}
      dark={logoDarkUrl}
      alt={t('brand.lockup')}
      className={cn('h-14', className)}
    />
  );
}

/** Coste360.it company logo: primary navy in light, white in dark. */
export function Coste360Logo({ className }: BrandProps) {
  return (
    <SchemePair
      light={coste360PrimaryUrl}
      dark={coste360WhiteUrl}
      alt="Coste360.it"
      className={cn('h-6', className)}
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
      <Coste360Logo className="h-5" />
    </a>
  );
}

export type BrandLogoVariant = 'icon' | 'name' | 'horizontal';

/**
 * The one entry point for the Arenile mark in the UI (never a lucide icon as
 * brand): `icon` = app icon tile, `name` = lettering, `horizontal` = full logo.
 */
export function BrandLogo({
  variant = 'horizontal',
  className,
  decorative = false,
}: BrandProps & {
  variant?: BrandLogoVariant;
  /** Icon only: hidden from assistive tech when the text next to it names the page. */
  decorative?: boolean;
}) {
  if (variant === 'icon')
    return <ArenileAppIcon className={className} alt={decorative ? '' : 'Arenile'} />;
  if (variant === 'name') return <ArenileName className={className} />;
  return <ArenileLogo className={className} />;
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
      <Coste360Logo className="h-4" />
    </a>
  );
}
