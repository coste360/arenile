import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { BrandLogo, Coste360Signature, MadeByCoste360 } from './Brand';

const BRAND_DIR = resolve(import.meta.dirname, '../../brand');
const ASSETS = resolve(import.meta.dirname, '../../../../../assets/brand');

/** Where each copy in shared/src/brand comes from (assets/brand is the source of truth). */
function sourceOf(file: string): string {
  // The Coste360 lockup is the company's original raster logo (logo-*.png, 781x312), never redrawn.
  const coste360 = /^coste360-logo-(\w+)\.png$/.exec(file);
  if (coste360) return resolve(ASSETS, 'coste360', `logo-${coste360[1]}.png`);
  if (/^arenile-(logo|name)-/.test(file)) return resolve(ASSETS, 'arenile', 'outlined', file);
  return resolve(ASSETS, 'arenile', file);
}

describe('brand assets', () => {
  it.each(readdirSync(BRAND_DIR))('%s is an unmodified copy of assets/brand', (file) => {
    expect(readFileSync(resolve(BRAND_DIR, file))).toEqual(readFileSync(sourceOf(file)));
  });

  it('the SVG copies have no live text (outlined, no font needed)', () => {
    for (const file of readdirSync(BRAND_DIR).filter((f) => f.endsWith('.svg'))) {
      expect(readFileSync(resolve(BRAND_DIR, file), 'utf8')).not.toContain('<text');
    }
  });
});

describe('BrandLogo', () => {
  it('renders a light and a dark logo, one hidden per scheme', () => {
    render(<BrandLogo />);
    const [light, dark] = screen.getAllByRole('img', { name: 'brand.lockup' });
    expect(light).toHaveClass('dark:hidden');
    expect(dark).toHaveClass('hidden', 'dark:block');
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
});
