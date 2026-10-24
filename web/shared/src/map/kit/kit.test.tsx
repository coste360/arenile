import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithI18n } from '../../testing';
import { BeachMap } from '../BeachMap';
import type { MapGeometry, MapSpot, SpotStatus } from '../types';
import {
  KIT_MANIFEST,
  MapKitProvider,
  decorationAsset,
  geometryKitIds,
  resolveMapKit,
  snapTileCentre,
  snapTileLength,
  spotAsset,
  tileLayout,
} from './index';

const spot = (id: string, x: number, extra: Partial<MapSpot> = {}): MapSpot => ({
  id,
  label: id,
  kind: 'umbrella',
  x,
  y: 60,
  shape: { type: 'circle', w: 40, h: 40 },
  ...extra,
});

const geometry: MapGeometry = {
  width: 400,
  height: 200,
  sea_side: 'bottom',
  spots: [
    spot('A1', 40),
    spot('A2', 100, { asset: 'ombrellone_3_lettini' }),
    spot('G1', 180, { kind: 'gazebo', shape: { type: 'rect', w: 60, h: 60 } }),
    spot('X1', 260, { kind: 'custom' }),
  ],
  decorations: [
    { id: 'w', kind: 'walkway', x: 330, y: 100, shape: { type: 'rect', w: 60, h: 180 } },
    {
      id: 'f',
      kind: 'custom',
      asset: 'bandiera_mare',
      variant: 'red',
      x: 380,
      y: 30,
      shape: { type: 'rect', w: 30, h: 30 },
    },
  ],
};

describe('kit defaults', () => {
  it('maps kinds to default illustrations and ignores non-bookable spot assets', () => {
    expect(spotAsset({ kind: 'umbrella' })).toBe('ombrellone_2_lettini');
    expect(spotAsset({ kind: 'palm' })).toBe('ombrellone_paglia');
    expect(spotAsset({ kind: 'tent', asset: 'bar' })).toBe('tenda_righe');
    expect(spotAsset({ kind: 'custom' })).toBeNull();
    expect(decorationAsset({ kind: 'cabin_block', shape: { w: 1, h: 1 } })).toBe('cabine_multiple');
    expect(decorationAsset({ kind: 'walkway', shape: { w: 60, h: 240 } })).toBe(
      'passerella_verticale',
    );
    expect(decorationAsset({ kind: 'text', shape: { w: 1, h: 1 } })).toBeNull();
  });

  it('every bookable asset has a spot kind', () => {
    for (const a of KIT_MANIFEST.filter((x) => x.type === 'bookable'))
      expect(a.spotKind).toBeTruthy();
  });

  it('lists only the ids a map draws, both straight walkway orientations included', () => {
    expect(geometryKitIds(geometry)).toEqual([
      'bandiera_mare',
      'gazebo',
      'ombrellone_2_lettini',
      'ombrellone_3_lettini',
      'passerella_orizzontale',
      'passerella_verticale',
    ]);
  });
});

describe('tiles', () => {
  it('repeats square tiles along the long side and fixes the orientation', () => {
    const t = tileLayout('passerella_orizzontale', 30, 150, { type: 'rect', w: 60, h: 180 });
    expect(t.asset).toBe('passerella_verticale');
    expect(t.side).toBe(60);
    expect(t.origins).toEqual([
      { x: 0, y: 60 },
      { x: 0, y: 120 },
      { x: 0, y: 180 },
    ]);
  });

  it('snaps lengths and edges to the tile grid; a nudge moves one tile', () => {
    expect(snapTileLength(100)).toBe(120);
    expect(snapTileLength(10)).toBe(60);
    expect(snapTileCentre(47, 60)).toBe(30);
    expect(snapTileCentre(100, 120)).toBe(120);
    expect(snapTileCentre(40, 60, 30)).toBe(90);
    expect(snapTileCentre(20, 60, 30)).toBe(-30);
  });
});

describe('BeachMap with the kit', () => {
  it('draws each spot with a <use> of its symbol, never inline art', () => {
    const { container } = renderWithI18n(<BeachMap geometry={geometry} />);
    const href = (id: string) =>
      screen.getByRole('button', { name: new RegExp(id) }).querySelector('use[data-asset]');
    expect(href('A1')?.getAttribute('href')).toBe('#ark-modern-ombrellone_2_lettini');
    expect(href('A2')?.getAttribute('href')).toBe('#ark-modern-ombrellone_3_lettini');
    expect(href('G1')?.getAttribute('href')).toBe('#ark-modern-gazebo');
    expect(href('X1')).toBeNull(); // custom without asset: plain footprint
    // Walkway of 180 units = 3 tiles at the same scale; flag variant sets --beach-flag.
    const walkway = container.querySelector('[data-decoration="walkway"]') as Element;
    expect(walkway.querySelectorAll('use[href="#ark-modern-passerella_verticale"]')).toHaveLength(3);
    const flag = container.querySelector('[data-decoration="custom"] use') as Element;
    expect(flag.parentElement).toHaveClass('[--beach-flag:var(--beach-flag-red)]');
    // The flag is named with its colour and captioned, so the sea state is not colour-only.
    expect(screen.getByRole('img', { name: 'Bandiera stato del mare: rossa' })).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Passerella' })).toBeInTheDocument();
    expect(container.querySelector('[data-decoration="custom"]')?.textContent).toContain('rossa');
  });

  it('classic: draws the halo as a ring around the art, larger than the art box', () => {
    const { container } = renderWithI18n(<BeachMap geometry={geometry} kit="classic" />);
    const a1 = screen.getByRole('button', { name: /A1/ });
    const halo = a1.querySelector('[data-halo]') as SVGCircleElement;
    const art = a1.querySelector('use[data-asset]') as SVGUseElement;
    const r = Number(halo.getAttribute('r'));
    const artSize = Number(art.getAttribute('width'));
    expect(artSize).toBe(40 * 1.3);
    expect(r * 2).toBeGreaterThan(artSize);
    expect(container.querySelectorAll('[data-halo]')).toHaveLength(4);
  });

  it('classic: renders the status as halo + badge outside the art', () => {
    const statuses: Record<string, SpotStatus> = {
      A1: 'present',
      A2: 'to_collect',
      G1: 'booked_online',
    };
    renderWithI18n(<BeachMap geometry={geometry} statuses={statuses} kit="classic" />);
    const a1 = screen.getByRole('button', { name: /A1: Presente/ });
    expect(a1.querySelector('[data-halo]')).toHaveClass('fill-status-present');
    expect(a1.querySelector('[data-badge="present"] use')).toHaveAttribute(
      'href',
      '#ark-badge-check',
    );
    const a2 = screen.getByRole('button', { name: /A2: Da incassare/ });
    expect(a2.querySelector('[data-badge] use')).toHaveAttribute('href', '#ark-badge-euro');
    // Online and desk bookings differ by glyph, not only by hue.
    expect(
      screen.getByRole('button', { name: /G1/ }).querySelector('[data-badge="booked_online"] use'),
    ).toHaveAttribute('href', '#ark-badge-globe');
    // Free: no badge.
    expect(screen.getByRole('button', { name: /X1/ }).querySelector('[data-badge]')).toBeNull();
  });

  it('hands the sprite over when the leader unmounts while other maps stay', async () => {
    const other: MapGeometry = {
      ...geometry,
      spots: [spot('Z1', 40, { kind: 'hammock' })],
      decorations: [],
    };
    const view = renderWithI18n(
      <>
        <BeachMap geometry={geometry} />
        <BeachMap geometry={other} />
      </>,
    );
    await waitFor(() => expect(document.querySelector('#ark-modern-gazebo')).not.toBeNull());
    view.rerender(<BeachMap geometry={other} />);
    expect(screen.getAllByTestId('map-kit-defs')).toHaveLength(1);
    await waitFor(() => expect(document.querySelector('#ark-modern-amaca')).not.toBeNull());
    expect(document.querySelector('#ark-modern-gazebo')).toBeNull();
    // The sprite lives on <body>, outside the subtree of the map that leads.
    expect(screen.getByTestId('map-kit-defs').parentElement).toBe(document.body);
  });

  it('keeps ONE sprite with only the used symbols, even with two maps on the page', async () => {
    const other: MapGeometry = {
      ...geometry,
      spots: [spot('Z1', 40, { kind: 'hammock' })],
      decorations: [],
    };
    renderWithI18n(
      <>
        <BeachMap geometry={geometry} />
        <BeachMap geometry={other} />
      </>,
    );
    const sprites = screen.getAllByTestId('map-kit-defs');
    expect(sprites).toHaveLength(1);
    await waitFor(() => expect(sprites[0]?.querySelector('#ark-modern-amaca')).not.toBeNull());
    const symbols = [...(sprites[0] as Element).querySelectorAll('symbol[id^="ark-"]')]
      .map((s) => s.id)
      .filter((id) => !id.startsWith('ark-badge-'));
    expect(symbols).toEqual([
      'ark-modern-amaca',
      'ark-modern-bandiera_mare',
      'ark-modern-gazebo',
      'ark-modern-ombrellone_2_lettini',
      'ark-modern-ombrellone_3_lettini',
      'ark-modern-passerella_orizzontale',
      'ark-modern-passerella_verticale',
    ]);
    // The symbol keeps the theme variables of the kit.
    expect(sprites[0]?.querySelector('#ark-modern-gazebo')?.innerHTML).toContain('var(--beach-');
  });

  it('1,000 spots: one <use> each and one symbol in total (performance budget)', async () => {
    const spots = Array.from({ length: 1000 }, (_, i) =>
      spot(`S${i}`, 30 + (i % 40) * 50, { y: 30 + Math.floor(i / 40) * 60 }),
    );
    const big: MapGeometry = { width: 2100, height: 1600, sea_side: 'bottom', spots };
    const t0 = performance.now();
    const { container } = renderWithI18n(<BeachMap geometry={big} showLegend={false} />);
    const elapsed = performance.now() - t0;
    expect(container.querySelectorAll('use[data-asset="ombrellone_2_lettini"]')).toHaveLength(1000);
    await waitFor(() =>
      expect(document.querySelectorAll('symbol[id="ark-modern-ombrellone_2_lettini"]')).toHaveLength(1),
    );
    // jsdom is ~10x slower than a browser and neither lays out nor paints: the
    // bound only catches accidental O(n^2) work. Real pan/zoom timings are
    // measured in a browser (docs/adr/0012-kit-mappa.md, Verifiche).
    expect(elapsed).toBeLessThan(4000);
  });
});

describe('modern kit (fill style, ADR 0024)', () => {
  const statuses: Record<string, SpotStatus> = {
    A1: 'present',
    A2: 'blocked',
    G1: 'booked_online',
  };

  it('is the default and paints the status on the art through --beach-state, no halo', () => {
    const { container } = renderWithI18n(<BeachMap geometry={geometry} statuses={statuses} />);
    expect(screen.getByTestId('beach-map')).toHaveAttribute('data-map-kit', 'modern');
    expect(container.querySelector('[data-halo]')).toBeNull();
    const a1 = screen.getByRole('button', { name: /A1: Presente/ });
    expect(a1.querySelector('[data-state="present"]')).toHaveClass(
      '[--beach-state:var(--status-present)]',
    );
    // Free spots get the free token, the ring stays invisible until hover, focus or selection.
    const x1 = screen.getByRole('button', { name: /X1/ });
    expect(x1.querySelector('[data-state="free"]')).toHaveClass(
      '[--beach-state:var(--status-free)]',
    );
    expect(x1.firstElementChild).toHaveAttribute('data-ring');
    expect(x1.firstElementChild).toHaveClass('stroke-transparent');
    // Badges keep their glyphs and sit in the indicator corner the kit leaves empty.
    const badge = a1.querySelector('[data-badge="present"] circle') as Element;
    const S = 40 * 1.3;
    expect(Number(badge.getAttribute('cx'))).toBeCloseTo(40 + S * 0.383);
    expect(Number(badge.getAttribute('cy'))).toBeCloseTo(60 - S * 0.383);
  });

  it('draws a diagonal bar on blocked spots, a cue that does not rely on colour', () => {
    renderWithI18n(<BeachMap geometry={geometry} statuses={statuses} />);
    expect(
      screen.getByRole('button', { name: /A2: Bloccato/ }).querySelector('[data-blocked-bar]'),
    ).not.toBeNull();
    expect(
      screen.getByRole('button', { name: /A1/ }).querySelector('[data-blocked-bar]'),
    ).toBeNull();
  });

  it('outlines a selected spot with a band and a dark ring visible on any fill', () => {
    renderWithI18n(<BeachMap geometry={geometry} statuses={statuses} selectedIds={['G1']} />);
    const g1 = screen.getByRole('button', { name: /G1/ });
    expect(g1.firstElementChild).toHaveClass('stroke-background');
    expect(g1.querySelector('.stroke-map-spot-selected')).not.toBeNull();
  });

  it('two maps with different kits share one sprite without clashing ids', async () => {
    renderWithI18n(
      <>
        <BeachMap geometry={geometry} kit="modern" />
        <BeachMap geometry={geometry} kit="classic" />
      </>,
    );
    const sprite = screen.getByTestId('map-kit-defs');
    await waitFor(() => {
      expect(sprite.querySelector('#ark-modern-gazebo')).not.toBeNull();
      expect(sprite.querySelector('#ark-classic-gazebo')).not.toBeNull();
    });
    // Same asset id, different art.
    expect(sprite.querySelector('#ark-modern-gazebo')?.innerHTML).toContain('--beach-state');
    expect(sprite.querySelector('#ark-classic-gazebo')?.innerHTML).not.toContain('--beach-state');
  });

  it('follows the MapKitProvider, and an unknown stored value falls back to the default', () => {
    const { unmount } = renderWithI18n(
      <MapKitProvider kit="classic">
        <BeachMap geometry={geometry} />
      </MapKitProvider>,
    );
    expect(screen.getByTestId('beach-map')).toHaveAttribute('data-map-kit', 'classic');
    unmount();
    renderWithI18n(
      <MapKitProvider kit="vintage">
        <BeachMap geometry={geometry} />
      </MapKitProvider>,
    );
    expect(screen.getByTestId('beach-map')).toHaveAttribute('data-map-kit', 'modern');
    expect(resolveMapKit(null)).toBe('modern');
  });
});
