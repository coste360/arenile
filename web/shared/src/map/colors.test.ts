import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { readableTextColor } from '../theme/contrast';
import { DEFAULT_STATUS_COLORS, STATUS_CLASSES } from './colors';
import type { SpotStatus } from './types';

const css = readFileSync(resolve(import.meta.dirname, '../styles/globals.css'), 'utf8');
const rootBlock = css.slice(css.indexOf(':root {'), css.indexOf('.dark {'));

function cssVar(name: string): string | undefined {
  return new RegExp(`${name}:\\s*([^;]+);`).exec(rootBlock)?.[1]?.trim();
}

describe('status colours', () => {
  it.each(Object.keys(DEFAULT_STATUS_COLORS) as SpotStatus[])(
    '%s: CSS token matches the hex palette and has a readable foreground',
    (status) => {
      const { cssVar: name } = STATUS_CLASSES[status];
      const hex = DEFAULT_STATUS_COLORS[status];
      expect(cssVar(name)).toBe(hex);
      expect(cssVar(`${name}-foreground`)).toBe(readableTextColor(hex));
    },
  );
});
