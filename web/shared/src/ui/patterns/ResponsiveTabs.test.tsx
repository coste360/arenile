import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Tabs } from '../tabs';
import { ResponsiveTabsList } from './ResponsiveTabs';

/**
 * Width-based collapse (the sidebar open on a 768/1024 tablet leaves less room
 * than the viewport suggests): ResizeObserver and layout sizes are faked, jsdom
 * has no layout.
 */
describe('ResponsiveTabsList, available width', () => {
  let resize: (() => void) | undefined;
  let containerWidth = 1000;
  let tabBarWidth = 720;

  beforeEach(() => {
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: () => void) {
          resize = cb;
        }
        observe() {}
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.dataset.slot === 'responsive-tabs' ? containerWidth : 0;
    });
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.getAttribute('role') === 'tablist' ? tabBarWidth : 0;
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  const options = Array.from({ length: 8 }, (_, i) => ({ value: `t${i}`, label: `Scheda ${i}` }));
  const ui = (
    <Tabs value="t0" onValueChange={() => {}}>
      <ResponsiveTabsList label="Report" value="t0" onValueChange={() => {}} options={options} />
    </Tabs>
  );

  it('keeps the tab bar when it fits', () => {
    containerWidth = 1000;
    tabBarWidth = 720;
    render(ui);
    expect(screen.getByRole('tablist', { name: 'Report' })).toBeInTheDocument();
  });

  it('becomes a Select when the container is narrower than the bar, and back when it widens', () => {
    containerWidth = 600;
    tabBarWidth = 720;
    render(ui);
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Report' })).toBeInTheDocument();

    containerWidth = 900;
    act(() => resize?.());
    expect(screen.getByRole('tablist', { name: 'Report' })).toBeInTheDocument();
  });

  it('re-measures when the content changes while the Select stands in', () => {
    containerWidth = 600;
    tabBarWidth = 720;
    const { rerender } = render(ui);
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();

    // Fewer, shorter tabs (e.g. another language): the bar now needs 400 px.
    tabBarWidth = 400;
    rerender(
      <Tabs value="t0" onValueChange={() => {}}>
        <ResponsiveTabsList
          label="Report"
          value="t0"
          onValueChange={() => {}}
          options={options.slice(0, 3)}
        />
      </Tabs>,
    );
    expect(screen.getByRole('tablist', { name: 'Report' })).toBeInTheDocument();
  });
});
