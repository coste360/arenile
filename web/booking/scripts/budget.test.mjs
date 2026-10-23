import { describe, expect, it } from 'vitest';
import { CSS_BUDGET, EMBED_BUDGET, INITIAL_BUDGET, KB, measure, staticClosure } from './budget.mjs';

const manifest = {
  'index.html': {
    file: 'assets/index.js',
    isEntry: true,
    imports: ['_shared.js'],
    css: ['assets/index.css'],
    dynamicImports: ['src/steps/WhereStep.tsx'],
  },
  '_shared.js': { file: 'assets/shared.js', imports: ['_react.js'] },
  '_react.js': { file: 'assets/react.js' },
  'src/steps/WhereStep.tsx': {
    file: 'assets/WhereStep.js',
    src: 'src/steps/WhereStep.tsx',
    isDynamicEntry: true,
    imports: ['_shared.js', '_map.js'],
  },
  '_map.js': { file: 'assets/map.js' },
};

const sizes = {
  'assets/index.js': 40 * KB,
  'assets/shared.js': 50 * KB,
  'assets/react.js': 45 * KB,
  'assets/index.css': 14 * KB,
  'assets/WhereStep.js': 5 * KB,
  'assets/map.js': 30 * KB,
  'embed.js': 1000,
};
const size = (f) => (f in sizes ? sizes[f] : null);

describe('booking budget', () => {
  it('follows the static imports of the entry, not the lazy steps', () => {
    expect([...staticClosure(manifest, 'index.html')]).toEqual([
      'index.html',
      '_shared.js',
      '_react.js',
    ]);
    const r = measure(manifest, size);
    expect(r.js).toBe(135 * KB);
    expect(r.css).toBe(14 * KB);
    expect(r.embed).toBe(1000);
    expect(r.errors).toEqual([]);
    expect(r.lazy.map((l) => l.file)).toContain('assets/map.js');
  });

  it('counts the dynamic imports declared as part of the first screen', () => {
    const r = measure(manifest, size, { startupDynamic: ['src/steps/WhereStep.tsx'] });
    expect(r.js).toBe(170 * KB);
    expect(r.errors).toEqual([
      `first-screen JS 170.0 KB exceeds ${(INITIAL_BUDGET / KB).toFixed(1)} KB`,
    ]);
  });

  it('fails on CSS and embed.js over budget, a missing embed.js or an unknown startup module', () => {
    const big = (f) =>
      f === 'assets/index.css' ? CSS_BUDGET + 1 : f === 'embed.js' ? EMBED_BUDGET + 1 : size(f);
    expect(measure(manifest, big).errors).toHaveLength(2);
    const noEmbed = (f) => (f === 'embed.js' ? null : size(f));
    expect(measure(manifest, noEmbed).errors).toEqual(['dist/embed.js not found']);
    expect(measure(manifest, size, { startupDynamic: ['src/nope.tsx'] }).errors).toEqual([
      'STARTUP_DYNAMIC "src/nope.tsx" is not in the manifest',
    ]);
  });
});
