import { fireEvent, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithI18n } from '../testing';
import { BeachMap } from './BeachMap';
import { spotArtSize } from './kit';
import type { MapGeometry } from './types';

// Public seat picker mode (additive props: pickableStatuses, minTapTargetPx, zoomSlider).

const geometry: MapGeometry = {
  id: 'm1',
  width: 300,
  height: 200,
  sea_side: 'bottom',
  spots: [
    {
      id: 'a',
      label: 'A1',
      row_label: 'A',
      kind: 'umbrella',
      x: 40,
      y: 40,
      shape: { type: 'circle', w: 30, h: 30 },
    },
    {
      id: 'b',
      label: 'A2',
      row_label: 'A',
      kind: 'umbrella',
      x: 90,
      y: 40,
      shape: { type: 'circle', w: 30, h: 30 },
    },
    {
      id: 'c',
      label: 'B1',
      row_label: 'B',
      kind: 'umbrella',
      x: 40,
      y: 100,
      shape: { type: 'circle', w: 30, h: 30 },
    },
  ],
};

const statuses = { a: 'free', b: 'unavailable', c: 'free' } as const;

function viewport() {
  return screen.getByTestId('beach-map-viewport').getAttribute('transform');
}

describe('BeachMap public picker', () => {
  it('only lets pickable statuses be activated, the others stay readable but disabled', async () => {
    const onSpotActivate = vi.fn();
    renderWithI18n(
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        pickableStatuses={['free']}
        onSpotActivate={onSpotActivate}
      />,
    );
    const taken = screen.getByRole('button', { name: 'Postazione A2: Non disponibile' });
    expect(taken).toHaveAttribute('aria-disabled', 'true');
    await userEvent.click(taken);
    fireEvent.keyDown(taken, { key: 'Enter' });
    expect(onSpotActivate).not.toHaveBeenCalled();

    const free = screen.getByRole('button', { name: 'Postazione A1: Libero' });
    expect(free).not.toHaveAttribute('aria-disabled');
    fireEvent.keyDown(free, { key: ' ' });
    expect(onSpotActivate).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }));
  });

  it('keeps unavailable spots reachable with the arrow keys (roving focus)', () => {
    renderWithI18n(
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        pickableStatuses={['free']}
        onSpotActivate={() => {}}
      />,
    );
    const first = screen.getByRole('button', { name: /A1/ });
    first.focus();
    fireEvent.keyDown(first, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(screen.getByRole('button', { name: /A2/ }));
  });

  it('zooms in on a spot smaller than the tap target before activating it', async () => {
    const onSpotActivate = vi.fn();
    renderWithI18n(
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        pickableStatuses={['free']}
        minTapTargetPx={44}
        onSpotActivate={onSpotActivate}
      />,
    );
    // jsdom measures 1 map unit per pixel: the art of a 30-unit spot is under 44 px.
    const art = spotArtSize({ type: 'circle', w: 30, h: 30 });
    expect(art).toBeLessThan(44);
    expect(viewport()).toBe('translate(0 0) scale(1)');
    const spot = screen.getByRole('button', { name: /A1/ });
    await userEvent.click(spot);
    expect(onSpotActivate).not.toHaveBeenCalled();
    const transform = viewport() ?? '';
    const scale = Number(/scale\(([\d.]+)\)/.exec(transform)?.[1]);
    expect(scale * art).toBeGreaterThanOrEqual(44);
    // Now large enough: the next tap activates it.
    await userEvent.click(spot);
    expect(onSpotActivate).toHaveBeenCalledTimes(1);
  });

  it('activates with the keyboard regardless of the on-screen size', () => {
    const onSpotActivate = vi.fn();
    renderWithI18n(
      <BeachMap
        geometry={geometry}
        statuses={statuses}
        minTapTargetPx={44}
        onSpotActivate={onSpotActivate}
      />,
    );
    fireEvent.keyDown(screen.getByRole('button', { name: /A1/ }), { key: 'Enter' });
    expect(onSpotActivate).toHaveBeenCalledTimes(1);
    expect(viewport()).toBe('translate(0 0) scale(1)');
  });

  it('can hide the zoom slider and keep the zoom buttons', () => {
    renderWithI18n(<BeachMap geometry={geometry} zoomSlider={false} />);
    expect(screen.queryByRole('slider')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ingrandisci' })).toBeInTheDocument();
  });
});
