import { fireEvent, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithI18n } from '../testing';
import { BeachMap } from './BeachMap';
import { SpotList, groupByRow } from './SpotList';
import type { MapGeometry } from './types';

const geometry: MapGeometry = {
  id: 'm1',
  width: 300,
  height: 200,
  sea_side: 'top',
  spots: [
    {
      id: 's1',
      label: 'A1',
      row_label: 'A',
      number: 1,
      kind: 'umbrella',
      x: 40,
      y: 40,
      shape: { type: 'circle', w: 30, h: 30 },
    },
    {
      id: 's2',
      label: 'A2',
      row_label: 'A',
      number: 2,
      kind: 'umbrella',
      x: 80,
      y: 40,
      shape: { type: 'circle', w: 30, h: 30 },
    },
    {
      id: 's3',
      label: 'B1',
      row_label: 'B',
      number: 1,
      kind: 'gazebo',
      x: 40,
      y: 90,
      rotation: 15,
      shape: { type: 'rect', w: 40, h: 30 },
    },
    {
      id: 's4',
      label: 'B2',
      row_label: 'B',
      number: 2,
      kind: 'umbrella',
      x: 80,
      y: 90,
      shape: { type: 'circle', w: 30, h: 30 },
      active: false,
    },
  ],
  decorations: [
    { id: 'd1', kind: 'shower', x: 250, y: 150, shape: { type: 'circle', w: 30, h: 30 } },
    {
      id: 'd2',
      kind: 'bar',
      x: 200,
      y: 150,
      shape: { type: 'rect', w: 60, h: 30 },
      label: 'Bar Aurora',
    },
  ],
};

describe('BeachMap', () => {
  it('renders a viewBox that includes the sea band', () => {
    const { container } = renderWithI18n(<BeachMap geometry={geometry} />);
    const svg = container.querySelector('svg');
    expect(svg?.getAttribute('viewBox')).toBe('0 -24 300 224');
    expect(container.querySelector('[data-sea-side="top"]')).toHaveTextContent('Mare');
    expect(screen.getByRole('group', { name: 'Mappa dello stabilimento' })).toBeInTheDocument();
  });

  it('exposes each spot as an accessible button with label and status', () => {
    renderWithI18n(<BeachMap geometry={geometry} statuses={{ s2: 'booked_online' }} />);
    const buttons = screen.getAllByRole('button', { name: /^Postazione/ });
    expect(buttons).toHaveLength(4);
    expect(screen.getByRole('button', { name: 'Postazione A1: Libero' })).toHaveAttribute(
      'tabindex',
      '0',
    );
    expect(
      screen.getByRole('button', { name: 'Postazione A2: Prenotato online' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Postazione B2: Non disponibile' })).toHaveAttribute(
      'aria-disabled',
      'true',
    );
  });

  it('activates spots with click, Enter and Space but not inactive ones', async () => {
    const user = userEvent.setup();
    const onActivate = vi.fn();
    renderWithI18n(<BeachMap geometry={geometry} onSpotActivate={onActivate} />);
    await user.click(screen.getByRole('button', { name: 'Postazione A1: Libero' }));
    expect(onActivate).toHaveBeenLastCalledWith(expect.objectContaining({ id: 's1' }));

    const b1 = screen.getByRole('button', { name: 'Postazione B1: Libero' });
    b1.focus();
    await user.keyboard('{Enter}');
    expect(onActivate).toHaveBeenLastCalledWith(expect.objectContaining({ id: 's3' }));
    await user.keyboard(' ');
    expect(onActivate).toHaveBeenCalledTimes(3);

    fireEvent.click(screen.getByRole('button', { name: 'Postazione B2: Non disponibile' }));
    expect(onActivate).toHaveBeenCalledTimes(3);
  });

  it('reflects selection with aria-pressed and appends descriptions', () => {
    renderWithI18n(
      <BeachMap
        geometry={geometry}
        selectedIds={['s1']}
        spotDescription={(s) => (s.id === 's1' ? '25,00 €' : undefined)}
      />,
    );
    expect(screen.getByRole('button', { name: 'Postazione A1: Libero, 25,00 €' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('shows a legend of the statuses on the map', () => {
    const { container } = renderWithI18n(
      <BeachMap geometry={geometry} statuses={{ s1: 'season' }} />,
    );
    const legend = container.querySelector('[data-slot="map-legend"]') as HTMLElement;
    expect(within(legend).getByText('Legenda')).toBeInTheDocument();
    expect(within(legend).getByText('Abbonato')).toBeInTheDocument();
    expect(within(legend).getByText('Libero')).toBeInTheDocument();
    expect(within(legend).getByText('Non disponibile')).toBeInTheDocument();
    expect(within(legend).queryByText('Bloccato')).not.toBeInTheDocument();
  });

  it('renders decorations with translated or custom labels', () => {
    const { container } = renderWithI18n(<BeachMap geometry={geometry} />);
    expect(container.querySelector('[data-decoration="shower"]')).toHaveTextContent('Doccia');
    expect(container.querySelector('[data-decoration="bar"]')).toHaveTextContent('Bar Aurora');
  });

  it('zooms with the controls and resets with "Adatta alla vista"', async () => {
    const user = userEvent.setup();
    renderWithI18n(<BeachMap geometry={geometry} />);
    const viewport = screen.getByTestId('beach-map-viewport');
    expect(viewport.getAttribute('transform')).toBe('translate(0 0) scale(1)');
    await user.click(screen.getByRole('button', { name: 'Ingrandisci' }));
    expect(viewport.getAttribute('transform')).toContain('scale(1.25)');
    await user.click(screen.getByRole('button', { name: 'Adatta alla vista' }));
    expect(viewport.getAttribute('transform')).toBe('translate(0 0) scale(1)');
  });
});

describe('SpotList', () => {
  it('groups spots by row and only enables free spots', async () => {
    const user = userEvent.setup();
    const onActivate = vi.fn();
    renderWithI18n(
      <SpotList
        spots={geometry.spots}
        statuses={{ s2: 'booked_desk' }}
        onSpotActivate={onActivate}
      />,
    );
    expect(screen.getByRole('heading', { name: 'Fila A' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Fila B' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /A2 – Prenotato in cassa/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: /A1 – Libero/ }));
    expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ id: 's1' }));
  });

  it('sorts rows naturally and puts spots without a row last', () => {
    const rows = groupByRow([
      { id: '1', label: 'X', kind: 'custom', x: 0, y: 0, shape: { type: 'circle', w: 1, h: 1 } },
      {
        id: '2',
        label: 'B10',
        row_label: 'B',
        number: 10,
        kind: 'umbrella',
        x: 0,
        y: 0,
        shape: { type: 'circle', w: 1, h: 1 },
      },
      {
        id: '3',
        label: 'B2',
        row_label: 'B',
        number: 2,
        kind: 'umbrella',
        x: 0,
        y: 0,
        shape: { type: 'circle', w: 1, h: 1 },
      },
      {
        id: '4',
        label: 'A1',
        row_label: 'A',
        number: 1,
        kind: 'umbrella',
        x: 0,
        y: 0,
        shape: { type: 'circle', w: 1, h: 1 },
      },
    ]);
    expect(rows.map(([r, list]) => [r, list.map((s) => s.label)])).toEqual([
      ['A', ['A1']],
      ['B', ['B2', 'B10']],
      [null, ['X']],
    ]);
  });
});
