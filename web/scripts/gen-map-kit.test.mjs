import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  KIT_DIR,
  KitError,
  NORMALISE,
  REPO,
  generate,
  normaliseManifest,
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
      normaliseManifest(base([entry('gazebo')]), [
        'gazebo.svg',
        'extra.svg',
        'ombrellone_quadrato.svg',
      ]),
    ).toThrow(/not listed|NORMALISE|missing/);
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

describe('generate (real kit)', () => {
  const { assets, files } = generate();
  const manifest = JSON.parse(readFileSync(join(KIT_DIR, 'manifest.json'), 'utf8'));

  it('has exactly the ids of the manifest and of the svg directory', () => {
    const ids = assets.map((a) => a.id).sort();
    expect(ids).toEqual(manifest.assets.map((a) => a.id).sort());
    expect(ids).toEqual(
      readdirSync(join(KIT_DIR, 'svg'))
        .filter((f) => f.endsWith('.svg'))
        .map((f) => f.slice(0, -4))
        .sort(),
    );
  });

  it('every source SVG declares the 120x120 viewBox', () => {
    for (const a of manifest.assets) {
      expect(readFileSync(join(KIT_DIR, a.file), 'utf8'), a.id).toContain('viewBox="0 0 120 120"');
    }
  });

  it('gives a SpotKind to every bookable asset and normalises ombrellone_quadrato', () => {
    for (const a of assets.filter((x) => x.type === 'bookable'))
      expect(a.spotKind, a.id).toBeTruthy();
    const symbols = files['web/shared/src/map/kit/generated/symbols.ts'];
    expect(symbols).toContain(`<g transform="${NORMALISE.ombrellone_quadrato}">`);
    expect(symbols).not.toMatch(/<title|<script|onload|href=/i);
  });

  it('is what is committed (run `make gen` otherwise)', () => {
    for (const [path, content] of Object.entries(files)) {
      expect(readFileSync(join(REPO, path), 'utf8'), path).toBe(content);
    }
  });
});
