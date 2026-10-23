import { describe, expect, it } from 'vitest';
import { panelMode } from './meta';

describe('panelMode', () => {
  it('maps edition and desktop mode', () => {
    expect(panelMode(undefined, {})).toBe('server');
    expect(panelMode({ edition: 'server' }, {})).toBe('server');
    expect(panelMode({ edition: 'desktop', desktop: { mode: 'local' } }, {})).toBe('local');
    expect(panelMode({ edition: 'desktop', desktop: { mode: 'lan' } }, {})).toBe('lan');
  });

  it('detects the remote mode from the desktop window global', () => {
    expect(panelMode({ edition: 'server' }, { arenileDesktop: { mode: 'remote' } })).toBe('remote');
  });
});
