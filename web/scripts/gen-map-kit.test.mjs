import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  KITS,
  KITS_DIR,
  KitError,
  NORMALISE,
  REPO,
  checkSameCatalogue,
  generate,
  normaliseManifest,
  pathOutlines,
  sanitise,
  symbolBody,
} from './gen-map-kit.mjs';

const svg = (body, attrs = 'viewBox="0 0 120 120"') =>
  `<svg xmlns="http://www.w3.org/2000/svg" ${attrs} width="120" height="120" role="img" aria-label="X"><title>X</title>${body}</svg>`;

describe('symbolBody sanitisation', () => {
  it('keeps the drawing and var() colours, drops title, role, size and comments', () => {
    const body = symbolBody(
      't.svg',
      svg(
        '<!-- c --><g stroke-linecap="round"><circle cx="60" cy="60" r="9" fill="var(--beach-sand, #F5ECDD)"/></g>',
      ),
    );
    expect(body).toBe(
      '<g stroke-linecap="round"><circle cx="60" cy="60" r="9" fill="var(--beach-sand, #F5ECDD)"/></g>',
    );
    expect(body).not.toMatch(/title|role|aria|width="120"/);
  });

  it('wraps the normalisation transform around the drawing', () => {
    expect(
      symbolBody('t.svg', svg('<rect x="1" y="1" width="2" height="2"/>'), 'rotate(-45 51 51)'),
    ).toBe('<g transform="rotate(-45 51 51)"><rect x="1" y="1" width="2" height="2"/></g>');
  });

  it.each([
    ['script', '<script>alert(1)</script>'],
    ['style element', '<style>*{}</style>'],
    ['foreignObject', '<foreignObject><div/></foreignObject>'],
    ['image', '<image href="x.png"/>'],
    ['use', '<use href="#a"/>'],
    ['link', '<a href="https://example.com"><rect/></a>'],
    ['animation', '<rect><set attributeName="x" to="1"/></rect>'],
    ['event attribute', '<rect onclick="alert(1)"/>'],
    ['event attribute (case)', '<rect ONLOAD="x"/>'],
    ['xlink:href', '<path xlink:href="#a"/>'],
    ['external url()', '<rect fill="url(https://evil/x.svg#p)"/>'],
    ['CSS-escaped url()', '<rect fill="u\\72l(#x)"/>'],
    ['backslash in a value', '<path d="M0 0\\"/>'],
    ['stray < between elements', '<rect/> < <rect/>'],
    ['stray < at the end', '<rect/><'],
    ['malformed tag', '<rect/><1bad/>'],
    ['javascript: value', '<path d="javascript:alert(1)"/>'],
    ['style attribute', '<rect style="fill:red"/>'],
    ['unknown attribute', '<rect filter="blur"/>'],
    ['text outside text', 'hello<rect/>'],
    ['unbalanced', '<g><rect/>'],
    ['doctype', '<!DOCTYPE svg><rect/>'],
  ])('rejects %s', (_name, body) => {
    expect(() => symbolBody('bad.svg', svg(body))).toThrow(KitError);
  });

  it('rejects a viewBox other than 0 0 120 120', () => {
    expect(() => symbolBody('v.svg', svg('<rect/>', 'viewBox="0 0 24 24"'))).toThrow(/viewBox/);
  });

  it('rejects event handlers on the root element', () => {
    expect(() => symbolBody('r.svg', svg('<rect/>', 'viewBox="0 0 120 120" onload="x()"'))).toThrow(
      KitError,
    );
  });
});

describe('normaliseManifest', () => {
  const base = (assets) => ({ viewBox: '0 0 120 120', total: assets.length, assets });
  const entry = (id, extra = {}) => ({
    id,
    label: id,
    category: 'Postazioni',
    type: 'bookable',
    file: `svg/${id}.svg`,
    ...extra,
  });

  it('fails when a file is not in the manifest or an entry has no file', () => {
    expect(() =>
      normaliseManifest(base([entry('gazebo')]), ['gazebo.svg', 'extra.svg']),
    ).toThrow(/not listed/);
    expect(() =>
      normaliseManifest(base([entry('gazebo')]), ['gazebo.svg'], NORMALISE.classic),
    ).toThrow(/NORMALISE/);
    expect(() => normaliseManifest(base([entry('gazebo')]), [])).toThrow(/missing svg\/gazebo.svg/);
  });

  it('folds the manifest type "asset" into bookable or structure', () => {
    const assets = normaliseManifest(
      base([
        entry('ombrellone_quadrato'),
        entry('ombrellone_chiuso', { type: 'asset' }),
        entry('bandiera_mare', { type: 'asset', category: 'Segnaletica' }),
      ]),
      ['ombrellone_quadrato.svg', 'ombrellone_chiuso.svg', 'bandiera_mare.svg'],
    );
    expect(assets.map((a) => [a.id, a.type])).toEqual([
      ['ombrellone_quadrato', 'bookable'],
      ['ombrellone_chiuso', 'bookable'],
      ['bandiera_mare', 'structure'],
    ]);
    expect(assets[2]).toMatchObject({
      decorationKind: 'custom',
      variants: ['green', 'yellow', 'red'],
    });
  });

  it('rejects unknown categories and duplicate ids', () => {
    expect(() =>
      normaliseManifest(base([entry('gazebo', { category: 'Altro' })]), ['gazebo.svg']),
    ).toThrow(/category/);
    expect(() =>
      normaliseManifest(base([entry('gazebo'), entry('gazebo')]), ['gazebo.svg']),
    ).toThrow(/duplicate/);
  });
});

describe('fill profile (v3 kits)', () => {
  const fill = (body) => sanitise('m.svg', svg(body), { profile: 'fill' });
  const STATE = 'var(--beach-state, var(--beach-state-free, #7C8794))';

  it('accepts var(--beach-*, fallback) colours and reports the state variable', () => {
    const r = fill(
      `<g transform="translate(12 12) scale(2)"><circle cx="24" cy="24" r="10" fill="${STATE}"/>` +
        '<path d="M20 24H28" fill="none" stroke="var(--beach-canvas, #FFFFFF)" stroke-width="1"/></g>',
    );
    expect(r.usesState).toBe(true);
    expect(r.corner).toEqual([]);
    expect(fill('<rect x="40" y="40" width="10" height="10" fill="var(--beach-ink, #263746)"/>'))
      .toMatchObject({ usesState: false });
  });

  it.each([
    ['raw hex colour', '<rect x="40" y="40" width="9" height="9" fill="#ff0000"/>'],
    ['currentColor', '<rect x="40" y="40" width="9" height="9" fill="currentColor"/>'],
    ['non-kit variable', '<rect x="40" y="40" width="9" height="9" fill="var(--primary, #000)"/>'],
    ['var without fallback', '<rect x="40" y="40" width="9" height="9" fill="var(--beach-ink)"/>'],
    ['raw stroke', '<path d="M40 40H50" stroke="red"/>'],
    ['text', '<text x="40" y="40">A</text>'],
    ['line', '<line x1="40" y1="40" x2="50" y2="50"/>'],
    ['polygon', '<polygon points="40,40 50,40 45,50"/>'],
  ])('rejects %s', (_name, body) => {
    expect(() => fill(body)).toThrow(KitError);
  });

  it.each([
    ['a rect inside the corner', '<rect x="100" y="4" width="8" height="8"/>'],
    ['a full-bleed background', '<rect x="0" y="0" width="120" height="120"/>'],
    ['a transformed circle', '<g transform="translate(12 12) scale(2)"><circle cx="44" cy="2" r="3"/></g>'],
    ['a stroke reaching in', '<path d="M60 20H91" fill="none" stroke="var(--beach-ink, #000)" stroke-width="6"/>'],
    ['an arc bulging in', '<path d="M80 30A20 20 0 0 1 120 30" fill="none" stroke="var(--beach-ink, #000)" stroke-width="2"/>'],
  ])('finds %s in the indicator corner', (_name, body) => {
    expect(fill(body).corner.length).toBeGreaterThan(0);
  });

  it('does not flag shapes whose bounding box only grazes the corner', () => {
    // A circle whose box covers 26..94 but whose outline stays out of x>92, y<28.
    expect(fill('<circle cx="60" cy="60" r="34"/>').corner).toEqual([]);
    // Rounded rect: the rounded corner keeps the art out of the slot.
    expect(fill('<rect x="26" y="26" width="68" height="68" rx="12"/>').corner).toEqual([]);
  });

  it('samples relative and smooth path commands', () => {
    const [poly] = pathOutlines('p.svg', 'M10 10l10 0q5 5 10 0t10 0s5 5 10 0z');
    expect(poly[0]).toEqual([10, 10]);
    expect(poly.at(-1)).toEqual([10, 10]);
    expect(Math.max(...poly.map((p) => p[0]))).toBeCloseTo(50);
  });
});

describe('catalogue check across kits', () => {
  const kit = (key, assets) => ({ key, assets });
  const a = (id, type = 'bookable', category = 'spots') => ({ id, type, category });

  it('requires the same ids, categories and types in every kit', () => {
    expect(() => checkSameCatalogue([kit('m', [a('x')]), kit('c', [a('x')])])).not.toThrow();
    expect(() => checkSameCatalogue([kit('m', [a('x'), a('y')]), kit('c', [a('x')])])).toThrow(
      /missing asset "y"/,
    );
    expect(() => checkSameCatalogue([kit('m', [a('x')]), kit('c', [a('x'), a('z')])])).toThrow(
      /"z" not in m/,
    );
    expect(() =>
      checkSameCatalogue([kit('m', [a('x')]), kit('c', [a('x', 'structure', 'structures')])]),
    ).toThrow(/"x" is/);
  });
});

describe('generate (real kits)', () => {
  const { kits, assets, files } = generate();

  it('ships modern (default, fill) and classic (halo)', () => {
    expect(KITS.map((k) => [k.key, k.profile])).toEqual([
      ['modern', 'fill'],
      ['classic', 'halo'],
    ]);
    expect(kits.map((k) => k.version)).toEqual(['3.0.0', '2.1.0']);
  });

  it.each(KITS.map((k) => k.key))(
    '%s: has exactly the ids of its manifest and of its svg directory',
    (key) => {
      const dir = join(KITS_DIR, key);
      const manifest = JSON.parse(readFileSync(join(dir, 'manifest.json'), 'utf8'));
      const ids = assets.map((x) => x.id).sort();
      expect(ids).toEqual(manifest.assets.map((x) => x.id).sort());
      expect(ids).toEqual(
        readdirSync(join(dir, 'svg'))
          .filter((f) => f.endsWith('.svg'))
          .map((f) => f.slice(0, -4))
          .sort(),
      );
      for (const x of manifest.assets) {
        expect(readFileSync(join(dir, x.file), 'utf8'), x.id).toContain('viewBox="0 0 120 120"');
      }
    },
  );

  it('gives a SpotKind to every bookable asset; modern paints all of them with --beach-state', () => {
    const bookable = assets.filter((x) => x.type === 'bookable');
    for (const x of bookable) expect(x.spotKind, x.id).toBeTruthy();
    const modern = kits.find((k) => k.key === 'modern');
    for (const x of bookable) expect(modern.stateful, x.id).toContain(x.id);
    expect(kits.find((k) => k.key === 'classic').stateful).toEqual([]);
  });

  it('normalises ombrellone_quadrato in classic only, and emits safe symbols', () => {
    const classic = files['web/shared/src/map/kit/generated/symbols-classic.ts'];
    const modern = files['web/shared/src/map/kit/generated/symbols-modern.ts'];
    expect(classic).toContain(`<g transform="${NORMALISE.classic.ombrellone_quadrato}">`);
    expect(modern).not.toContain(NORMALISE.classic.ombrellone_quadrato);
    for (const s of [classic, modern]) expect(s).not.toMatch(/<title|<script|onload|href=/i);
  });

  it('is what is committed (run `make gen` otherwise)', () => {
    for (const [path, content] of Object.entries(files)) {
      expect(readFileSync(join(REPO, path), 'utf8'), path).toBe(content);
    }
  });
});

