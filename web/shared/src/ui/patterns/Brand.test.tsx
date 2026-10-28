import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ArenileMonoSymbol, BrandLogo, Coste360Signature, MadeByCoste360 } from './Brand';

const BRAND_DIR = resolve(import.meta.dirname, '../../brand');
const ASSETS = resolve(import.meta.dirname, '../../../../../assets/brand');

/** Where each copy in shared/src/brand comes from (assets/brand is the source of truth). */
function sourceOf(file: string): string {
  // The Coste360 lockup is the company's own SVG (logo.svg), only recoloured per tone, never redrawn.
  const coste360 = /^coste360-logo-(\w+)\.svg$/.exec(file);
  if (coste360) return resolve(ASSETS, 'coste360', `logo-${coste360[1]}.svg`);
  if (/^arenile-(logo|name)-/.test(file)) return resolve(ASSETS, 'arenile', 'outlined', file);
  return resolve(ASSETS, 'arenile', file);
}

describe('brand assets', () => {
  it.each([
    ['primary', '#0c2442'],
    ['white', '#fff'],
  ])(
    'the Coste360 %s lockup is the supplied logo.svg with only its fill set to %s',
    (tone, fill) => {
      const original = readFileSync(resolve(ASSETS, 'coste360', 'logo.svg'), 'utf8');
      const variant = readFileSync(resolve(ASSETS, 'coste360', `logo-${tone}.svg`), 'utf8');
      const group = `fill="${fill}" id="content_group"`;
      expect(variant.split(group)).toHaveLength(2);
      expect(variant.replace(group, 'fill="#000" id="content_group"')).toBe(original);
    },
  );

  it.each(readdirSync(BRAND_DIR))('%s is an unmodified copy of assets/brand', (file) => {
    expect(readFileSync(resolve(BRAND_DIR, file))).toEqual(readFileSync(sourceOf(file)));
  });

  it('the SVG copies have no live text (outlined, no font needed)', () => {
    for (const file of readdirSync(BRAND_DIR).filter((f) => f.endsWith('.svg'))) {
      expect(readFileSync(resolve(BRAND_DIR, file), 'utf8')).not.toContain('<text');
    }
  });
});

/** The kit label of an inlined SVG (Vite inlines the small marks as data URIs). */
function kitLabel(img: HTMLElement | undefined): string {
  const src = decodeURIComponent(img?.getAttribute('src') ?? '');
  return /aria-label='Arenile — ([^']+)'/.exec(src)?.[1] ?? src;
}

describe('BrandLogo', () => {
  it('renders a light and a dark logo, one hidden per scheme', () => {
    render(<BrandLogo />);
    const [light, dark] = screen.getAllByRole('img', { name: 'Arenile' });
    expect(light).toHaveClass('dark:hidden');
    expect(dark).toHaveClass('hidden', 'dark:block');
    // Colour logo (navy A) on light surfaces, white in dark mode (ADR 0018).
    expect(kitLabel(light)).toBe('logo orizzontale');
    expect(kitLabel(dark)).toBe('logo orizzontale bianco');
  });

  it.each([
    ['colour', 'simbolo colore (sfondi chiari)', 'simbolo bianco'],
    ['mono', 'simbolo navy', 'simbolo bianco'],
  ] as const)('symbol with the %s tone swaps light and dark files', (tone, light, dark) => {
    render(<BrandLogo variant="symbol" tone={tone} />);
    const [l, d] = screen.getAllByRole('img', { name: 'Arenile' });
    expect(kitLabel(l)).toBe(light);
    expect(kitLabel(d)).toBe(dark);
  });

  it.each([
    ['horizontal', 'colour', ['logo orizzontale', 'logo orizzontale bianco']],
    ['horizontal', 'white', ['logo orizzontale bianco']],
    ['horizontal', 'mono', ['logo orizzontale navy', 'logo orizzontale bianco']],
  ] as const)('%s logo with the %s tone uses the right files', (variant, tone, labels) => {
    render(<BrandLogo variant={variant} tone={tone} />);
    expect(screen.getAllByRole('img', { name: 'Arenile' }).map(kitLabel)).toEqual(labels);
  });

  it.each([
    ['colour', 2],
    ['white', 1],
    ['mono', 2],
  ] as const)('name with the %s tone renders %i image(s)', (tone, count) => {
    render(<BrandLogo variant="name" tone={tone} />);
    expect(screen.getAllByRole('img', { name: 'Arenile' })).toHaveLength(count);
  });

  it('decorative symbol is hidden from assistive tech', () => {
    const { container } = render(<BrandLogo variant="symbol" decorative />);
    expect(screen.queryAllByRole('img')).toHaveLength(0);
    for (const img of container.querySelectorAll('img')) expect(img).toHaveAttribute('alt', '');
  });

  it('mono symbol pairs navy (light) and white (dark)', () => {
    render(<ArenileMonoSymbol />);
    expect(screen.getAllByRole('img', { name: 'Arenile' }).map(kitLabel)).toEqual([
      'simbolo navy',
      'simbolo bianco',
    ]);
  });

  it('white tone renders a single white mark in both schemes', () => {
    render(<BrandLogo variant="symbol" tone="white" />);
    const imgs = screen.getAllByRole('img', { name: 'Arenile' });
    expect(imgs).toHaveLength(1);
    expect(kitLabel(imgs[0])).toBe('simbolo bianco');
  });

  it('icon variant can be decorative', () => {
    const { container } = render(<BrandLogo variant="icon" decorative />);
    expect(container.querySelector('img')).toHaveAttribute('alt', '');
  });

  it('Coste360 links go to the company site', () => {
    render(
      <>
        <MadeByCoste360 />
        <Coste360Signature />
      </>,
    );
    for (const link of screen.getAllByRole('link')) {
      expect(link).toHaveAttribute('href', 'https://www.coste360.it/');
    }
  });

  it('sidebar signature puts the "made by" label before the logo', () => {
    render(<Coste360Signature />);
    const link = screen.getByRole('link');
    expect(link.firstElementChild?.tagName).toBe('SPAN');
    expect(link.querySelector('img')).not.toBeNull();
  });
});
