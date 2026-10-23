import { fireEvent, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { MapCanvas, type MapCanvasProps } from './MapCanvas';
import type { DraftDocument, DraftSpot } from './model/types';

function spot(key: string, x: number, y: number): DraftSpot {
  return {
    key,
    id: key,
    zone_id: 'z1',
    label: key.toUpperCase(),
    row_label: '',
    number: null,
    kind: 'umbrella',
    asset: '',
    x,
    y,
    rotation: 0,
    shape: { type: 'circle', w: 20, h: 20 },
    capacity: {},
    max_extra: {},
    price_category_override: '',
    attributes: [],
    active: true,
  };
}

function doc(spots: DraftSpot[]): DraftDocument {
  return {
    map: {
      name: { it: 'Spiaggia' },
      width: 2000,
      height: 1000,
      grid_size: 10,
      sea_side: 'bottom',
      sort_order: 0,
    },
    spots,
    decorations: [],
  };
}

// jsdom has no SVG geometry: screen coordinates map 1:1 to map units.
const proto = SVGSVGElement.prototype as unknown as Record<string, unknown>;
const saved = { ctm: proto.getScreenCTM, pt: proto.createSVGPoint };
beforeEach(() => {
  proto.getScreenCTM = () => ({ inverse: () => ({}) });
  proto.createSVGPoint = () => {
    const p = { x: 0, y: 0, matrixTransform: () => ({ x: p.x, y: p.y }) };
    return p;
  };
  vi.stubGlobal('requestAnimationFrame', undefined);
});
afterEach(() => {
  proto.getScreenCTM = saved.ctm;
  proto.createSVGPoint = saved.pt;
  vi.unstubAllGlobals();
});

function setup(d: DraftDocument, overrides: Partial<MapCanvasProps> = {}) {
  const props: MapCanvasProps = {
    doc: d,
    selection: new Set(),
    zoneColor: () => '#0ea5e9',
    spotLabel: (s) => `Postazione ${s.label}`,
    decorationLabel: (x) => x.kind,
    onSelect: vi.fn(),
    onSelectKeys: vi.fn(),
    onClearSelection: vi.fn(),
    onMove: vi.fn(),
    onNudge: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
  const utils = render(
    <I18nextProvider i18n={createAdminTestI18n()}>
      <MapCanvas {...props} />
    </I18nextProvider>,
  );
  const svg = screen.getByRole('group', { name: /Mappa con/ });
  return { props, svg, ...utils };
}

describe('MapCanvas', () => {
  it('drags the selection with a snapped transform and commits once on release', () => {
    const d = doc([spot('a', 100, 100), spot('b', 140, 100), spot('c', 300, 300)]);
    const { props, svg } = setup(d, { selection: new Set(['a', 'b']) });
    const a = screen.getByRole('button', { name: 'Postazione A' });
    const nodeB = svg.querySelector('[data-key="b"]') as SVGGElement;
    const nodeC = svg.querySelector('[data-key="c"]') as SVGGElement;

    fireEvent.pointerDown(a, { button: 0, pointerId: 1, clientX: 100, clientY: 100 });
    fireEvent.pointerMove(svg, { pointerId: 1, clientX: 113, clientY: 96 });
    // Live preview only: a translate on the dragged nodes, the document is untouched.
    expect(nodeB.getAttribute('transform')).toBe('translate(10 0)');
    expect(nodeC.getAttribute('transform')).toBeNull();
    expect(props.onMove).not.toHaveBeenCalled();
    fireEvent.pointerMove(svg, { pointerId: 1, clientX: 127, clientY: 108 });
    fireEvent.pointerUp(svg, { pointerId: 1, clientX: 127, clientY: 108 });

    expect(props.onMove).toHaveBeenCalledTimes(1);
    expect(props.onMove).toHaveBeenCalledWith(new Set(['a', 'b']), { x: 30, y: 10 }, 'a');
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it('treats a press without movement as a click', () => {
    const d = doc([spot('a', 100, 100)]);
    const { props, svg } = setup(d);
    const a = screen.getByRole('button', { name: 'Postazione A' });
    fireEvent.pointerDown(a, { button: 0, pointerId: 1, clientX: 100, clientY: 100 });
    fireEvent.pointerUp(svg, { pointerId: 1, clientX: 100, clientY: 100 });
    expect(props.onSelect).toHaveBeenCalledWith('a', false);
    expect(props.onMove).not.toHaveBeenCalled();
  });

  it('selects with a rubber band on the sand, adding with shift', () => {
    const d = doc([spot('a', 100, 100), spot('b', 140, 100), spot('c', 300, 300)]);
    const { props, svg } = setup(d, { selection: new Set(['c']) });
    const sand = svg.querySelector('[data-background="sand"]') as SVGRectElement;
    fireEvent.pointerDown(sand, { button: 0, pointerId: 2, clientX: 80, clientY: 80 });
    fireEvent.pointerMove(svg, { pointerId: 2, clientX: 125, clientY: 120 });
    expect(screen.getByTestId('rubber-band')).toHaveAttribute('visibility', 'visible');
    fireEvent.pointerUp(svg, { pointerId: 2, clientX: 125, clientY: 120 });
    expect(props.onSelectKeys).toHaveBeenLastCalledWith(new Set(['a']));
    expect(screen.getByTestId('rubber-band')).toHaveAttribute('visibility', 'hidden');

    fireEvent.pointerDown(sand, {
      button: 0,
      pointerId: 3,
      clientX: 80,
      clientY: 80,
      shiftKey: true,
    });
    fireEvent.pointerMove(svg, { pointerId: 3, clientX: 200, clientY: 120 });
    fireEvent.pointerUp(svg, { pointerId: 3, clientX: 200, clientY: 120 });
    expect(props.onSelectKeys).toHaveBeenLastCalledWith(new Set(['c', 'a', 'b']));
  });

  it('clears the selection on a simple click on the sand', () => {
    const { props, svg } = setup(doc([spot('a', 100, 100)]));
    const sand = svg.querySelector('[data-background="sand"]') as SVGRectElement;
    fireEvent.pointerDown(sand, { button: 0, pointerId: 1, clientX: 500, clientY: 500 });
    fireEvent.pointerUp(svg, { pointerId: 1, clientX: 500, clientY: 500 });
    expect(props.onClearSelection).toHaveBeenCalled();
    expect(props.onSelectKeys).not.toHaveBeenCalled();
  });

  it('moves by one grid step with the arrows (five with shift, one unit with alt)', () => {
    const { props } = setup(doc([spot('a', 100, 100)]), { selection: new Set(['a']) });
    const a = screen.getByRole('button', { name: 'Postazione A' });
    fireEvent.keyDown(a, { key: 'ArrowLeft' });
    fireEvent.keyDown(a, { key: 'ArrowDown', shiftKey: true });
    fireEvent.keyDown(a, { key: 'ArrowUp', altKey: true });
    expect(props.onNudge).toHaveBeenNthCalledWith(1, { x: -10, y: 0 }, 'a');
    expect(props.onNudge).toHaveBeenNthCalledWith(2, { x: 0, y: 50 }, 'a');
    expect(props.onNudge).toHaveBeenNthCalledWith(3, { x: 0, y: -1 }, 'a');
  });

  it('is one tab stop: a roving tabindex with Home/End and PageUp/PageDown navigation', () => {
    const d = doc([spot('a', 100, 100), spot('b', 140, 100), spot('c', 300, 300)]);
    const { props, svg, rerender } = setup(d, { selection: new Set(['b']) });
    const a = screen.getByRole('button', { name: 'Postazione A' });
    const b = screen.getByRole('button', { name: 'Postazione B' });
    const c = screen.getByRole('button', { name: 'Postazione C' });
    // Only the first selected element is reachable with Tab.
    expect(svg.querySelectorAll('[tabindex="0"]')).toHaveLength(1);
    expect(b).toHaveAttribute('tabindex', '0');
    expect(a).toHaveAttribute('tabindex', '-1');

    b.focus();
    fireEvent.keyDown(b, { key: 'PageDown' });
    expect(props.onSelect).toHaveBeenLastCalledWith('c', false);
    expect(c).toHaveFocus();
    fireEvent.keyDown(c, { key: 'Home' });
    expect(props.onSelect).toHaveBeenLastCalledWith('a', false);
    expect(a).toHaveFocus();
    fireEvent.keyDown(a, { key: 'End' });
    expect(c).toHaveFocus();
    fireEvent.keyDown(c, { key: 'PageUp' });
    expect(b).toHaveFocus();
    // Arrows nudge when editable (never navigate).
    expect(props.onNudge).not.toHaveBeenCalled();
    fireEvent.keyDown(b, { key: 'ArrowRight' });
    expect(props.onNudge).toHaveBeenCalledTimes(1);

    // Without a selection the first spot is the tab stop; read-only arrows navigate.
    rerender(
      <I18nextProvider i18n={createAdminTestI18n()}>
        <MapCanvas {...props} selection={new Set()} readOnly />
      </I18nextProvider>,
    );
    expect(a).toHaveAttribute('tabindex', '0');
    expect(b).toHaveAttribute('tabindex', '-1');
    fireEvent.keyDown(a, { key: 'ArrowRight' });
    expect(b).toHaveFocus();
    expect(props.onNudge).toHaveBeenCalledTimes(1);
  });

  it('renders 1000 spots and keeps unchanged ones when one moves', () => {
    const spots = Array.from({ length: 1000 }, (_, i) =>
      spot(`s${i}`, 20 + (i % 50) * 30, 20 + Math.floor(i / 50) * 30),
    );
    const d = doc(spots);
    const { svg, rerender, props } = setup(d);
    expect(svg.querySelectorAll('g[data-spot-key]')).toHaveLength(1000);
    const before = svg.querySelector('[data-key="s999"]');
    const moved = { ...d, spots: d.spots.map((s, i) => (i === 0 ? { ...s, x: 25 } : s)) };
    rerender(
      <I18nextProvider i18n={createAdminTestI18n()}>
        <MapCanvas {...props} doc={moved} />
      </I18nextProvider>,
    );
    expect(svg.querySelector('[data-key="s0"]')).toHaveAttribute('data-x', '25');
    expect(svg.querySelector('[data-key="s999"]')).toBe(before);
  });
});
