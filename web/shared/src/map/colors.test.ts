import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { readableTextColor } from '../theme/contrast';
import { DARK_STATUS_COLORS, DEFAULT_STATUS_COLORS, STATUS_CLASSES } from './colors';
import type { SpotStatus } from './types';

const css = readFileSync(resolve(import.meta.dirname, '../styles/globals.css'), 'utf8');
const rootBlock = css.slice(css.indexOf(':root {'), css.indexOf('\n.dark {'));
const darkStart = css.indexOf('\n.dark {');
const darkBlock = css.slice(darkStart, css.indexOf('\n}', darkStart));

function cssVar(block: string, name: string): string | undefined {
  return new RegExp(`${name}:\\s*([^;]+);`).exec(block)?.[1]?.trim();
}

describe('status colours', () => {
  it.each(Object.keys(DEFAULT_STATUS_COLORS) as SpotStatus[])(
    '%s: CSS tokens match the hex palettes and have a readable foreground',
    (status) => {
      const { cssVar: name } = STATUS_CLASSES[status];
      const hex = DEFAULT_STATUS_COLORS[status];
      expect(cssVar(rootBlock, name)).toBe(hex);
      expect(cssVar(rootBlock, `${name}-foreground`)).toBe(readableTextColor(hex));
      // Dark: the override, or the light value when the dark block has none.
      const dark = DARK_STATUS_COLORS[status];
      expect(cssVar(darkBlock, name) ?? hex).toBe(dark);
      expect(cssVar(darkBlock, `${name}-foreground`) ?? readableTextColor(hex)).toBe(
        readableTextColor(dark),
      );
    },
  );

  it('every status sets --beach-state to its own token (modern kit)', () => {
    for (const [status, c] of Object.entries(STATUS_CLASSES)) {
      // Built from parts: the CSS guard and Tailwind scan for literal class names.
      expect(c.state, status).toBe(['[', '--beach-state:var(', c.cssVar, ')]'].join(''));
    }
  });
});
