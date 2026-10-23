import { describe, expect, it, vi } from 'vitest';
import { listenPreviewTheme } from './preview';

function framedWindow() {
  const listeners: ((e: MessageEvent) => void)[] = [];
  const parent = { postMessage: vi.fn() };
  const win = {
    parent,
    location: { origin: 'https://arenile.example' },
    addEventListener: (_: string, fn: (e: MessageEvent) => void) => listeners.push(fn),
    removeEventListener: vi.fn(),
  } as unknown as Window;
  return { win, parent, listeners };
}

describe('listenPreviewTheme', () => {
  it('announces itself to the same-origin panel', () => {
    const { win, parent } = framedWindow();
    listenPreviewTheme(win, () => {});
    expect(parent.postMessage).toHaveBeenCalledWith(
      { type: 'arenile:preview-ready' },
      'https://arenile.example',
    );
  });

  it('accepts themes only from the same origin and the parent window', () => {
    const { win, parent, listeners } = framedWindow();
    const onTheme = vi.fn();
    listenPreviewTheme(win, onTheme);
    const fire = (data: unknown, origin: string, source: unknown) => {
      const event = new MessageEvent('message', { data, origin });
      Object.defineProperty(event, 'source', { value: source });
      listeners.forEach((fn) => fn(event));
    };
    const theme = { primary_color: '#ff0000' };
    fire({ type: 'arenile:preview-theme', theme }, 'https://evil.example', parent);
    fire({ type: 'arenile:preview-theme', theme }, 'https://arenile.example', {});
    fire({ type: 'arenile:preview-theme', theme: 'red' }, 'https://arenile.example', parent);
    fire({ type: 'other', theme }, 'https://arenile.example', parent);
    expect(onTheme).not.toHaveBeenCalled();
    fire({ type: 'arenile:preview-theme', theme }, 'https://arenile.example', parent);
    expect(onTheme).toHaveBeenCalledWith(theme);
  });

  it('does nothing when the page is not framed', () => {
    const addEventListener = vi.fn();
    const win = { addEventListener } as unknown as Window;
    (win as { parent: Window }).parent = win;
    listenPreviewTheme(win, () => {})();
    expect(addEventListener).not.toHaveBeenCalled();
  });
});
