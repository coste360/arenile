#!/usr/bin/env node
// Beach map kit generator (run by `make gen`, hence checked by `make check`).
//
// Reads the Coste360 kit in assets/beach-map-kit (manifest.json + svg/*.svg,
// kept untouched as the source of truth) and writes:
//   - web/shared/src/map/kit/generated/symbols.ts   <symbol> bodies, one string per asset
//   - web/shared/src/map/kit/generated/manifest.ts  typed manifest (ids, categories, kinds)
//   - internal/domain/mapping/kitassets_gen.go      asset ids for backend validation
//
// Only node built-ins: the SVGs are small and regular, so a strict tokenizer
// with an allowlist is enough (no optimiser, no parser dependency). The
// generation FAILS on anything unexpected: elements outside the allowlist
// (script, style, image, use, foreignObject, a, animation...), event handler
// attributes, href/xlink:href, url(...) references, javascript: values, a
// viewBox other than 0 0 120 120, or a manifest that does not match the files.
// See docs/adr/0012-kit-mappa.md.
import { readFileSync, readdirSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
export const REPO = join(HERE, '..', '..');
export const KIT_DIR = join(REPO, 'assets', 'beach-map-kit');
const TS_OUT = join(REPO, 'web', 'shared', 'src', 'map', 'kit', 'generated');
const GO_OUT = join(REPO, 'internal', 'domain', 'mapping', 'kitassets_gen.go');

export const VIEWBOX = '0 0 120 120';

/** Kit categories (Italian, as in manifest.json) to English identifiers. */
export const CATEGORIES = {
  Postazioni: 'spots',
  Percorsi: 'paths',
  Strutture: 'structures',
  Servizi: 'services',
  Salvataggio: 'rescue',
  Nautica: 'nautical',
  'Sport e relax': 'sport',
  Arredi: 'furniture',
  Ambiente: 'nature',
  Segnaletica: 'signage',
};

const TYPES = new Set(['bookable', 'structure', 'decoration']);

/** SpotKind of every bookable asset (API enum SpotKind). */
const SPOT_KIND_RULES = [
  [/^ombrellone_paglia$/, 'palm'],
  [/^ombrellone_/, 'umbrella'],
  [/^gazebo/, 'gazebo'],
  [/^tenda_/, 'tent'],
  [/^cabana$/, 'cabana'],
  [/^amaca$/, 'hammock'],
  [/^(lettino_matrimoniale|daybed)$/, 'daybed'],
  [/^(lettino|lettino_orizzontale|sdraio|sedia_regista|pouf)$/, 'sunbed'],
];

/** DecorationKind suggested for a non-bookable asset; everything else is `custom`. */
const DECORATION_KIND_RULES = [
  [/^passerella_/, 'walkway'],
  [/^doccia/, 'shower'],
  [/^torretta_bagnino$/, 'lifeguard'],
  [/^bar$/, 'bar'],
  [/^ingresso$/, 'entrance'],
  [/^cabin/, 'cabin_block'],
];

/** Modular tiles: same scale, snapped to the tile grid so that pieces join. */
const TILE_RE = /^passerella_/;

/** Colour variants an element may choose (API field `variant`), by asset. */
export const VARIANTS = { bandiera_mare: ['green', 'yellow', 'red'] };

/**
 * Normalisations applied to the art (the raw file stays untouched):
 * ombrellone_quadrato is drawn as a diamond (square rotated 45 degrees around
 * 51,51): rotate it back and re-centre it so that it is axis-aligned like
 * every other asset and follows the element rotation.
 */
export const NORMALISE = { ombrellone_quadrato: 'translate(9 9) rotate(-45 51 51)' };

const ALLOWED_ELEMENTS = new Set([
  'g',
  'path',
  'rect',
  'circle',
  'ellipse',
  'line',
  'polyline',
  'polygon',
  'text',
  'tspan',
]);

const ALLOWED_ATTRS = new Set([
  'fill',
  'fill-opacity',
  'fill-rule',
  'stroke',
  'stroke-width',
  'stroke-linejoin',
  'stroke-linecap',
  'stroke-dasharray',
  'stroke-opacity',
  'opacity',
  'transform',
  'd',
  'x',
  'y',
  'x1',
  'y1',
  'x2',
  'y2',
  'cx',
  'cy',
  'r',
  'rx',
  'ry',
  'width',
  'height',
  'points',
  'font-family',
  'font-size',
  'font-weight',
  'text-anchor',
  'dominant-baseline',
  'letter-spacing',
]);

export class KitError extends Error {}

function fail(file, msg) {
  throw new KitError(`${file}: ${msg}`);
}

function parseAttrs(file, tag, text) {
  const attrs = [];
  const re = /\s*([^\s=/>]+)\s*=\s*"([^"]*)"|\s*([^\s=/>]+)\s*=\s*'([^']*)'|\s*(\S+)/gy;
  let m;
  while ((m = re.exec(text)) !== null) {
    if (m[5] !== undefined) {
      if (m[5] === '/') continue;
      fail(file, `<${tag}>: malformed attribute "${m[5]}"`);
    }
    attrs.push([m[1] ?? m[3], m[2] ?? m[4]]);
  }
  return attrs;
}

function checkAttr(file, tag, name, value) {
  const lower = name.toLowerCase();
  if (lower.startsWith('on')) fail(file, `<${tag}>: event attribute "${name}" not allowed`);
  if (lower === 'href' || lower.endsWith(':href')) fail(file, `<${tag}>: "${name}" not allowed`);
  if (lower === 'style') fail(file, `<${tag}>: "style" attribute not allowed`);
  if (/url\s*\(/i.test(value)) fail(file, `<${tag}>: external reference ${name}="${value}"`);
  if (/javascript:|data:|&#|<|>/i.test(value)) {
    fail(file, `<${tag}>: unsafe value ${name}="${value}"`);
  }
  if (!ALLOWED_ATTRS.has(lower)) fail(file, `<${tag}>: attribute "${name}" not allowed`);
}

const escapeAttr = (v) => v.replace(/&/g, '&amp;').replace(/"/g, '&quot;');

/**
 * Sanitises one kit SVG and returns the body of its <symbol> (the children of
 * the root <svg>, without <title>/<desc>, comments, role, aria-*, width and
 * height). Throws KitError on anything outside the allowlist.
 */
export function symbolBody(file, source, transform) {
  let text = source.replace(/^\uFEFF/, '').trim();
  text = text.replace(/^<\?xml[^>]*\?>\s*/, '');
  if (/<!DOCTYPE|<!ENTITY|<!\[CDATA\[/i.test(text))
    fail(file, 'DOCTYPE, ENTITY or CDATA not allowed');
  text = text.replace(/<!--[\s\S]*?-->/g, '');
  const root = /^<svg\b([^>]*)>([\s\S]*)<\/svg>$/.exec(text);
  if (!root) fail(file, 'not a single <svg> root element');
  const rootAttrs = new Map(parseAttrs(file, 'svg', root[1]));
  if (rootAttrs.get('viewBox') !== VIEWBOX) {
    fail(file, `viewBox must be "${VIEWBOX}", found "${rootAttrs.get('viewBox') ?? ''}"`);
  }
  for (const name of rootAttrs.keys()) {
    if (name.toLowerCase().startsWith('on')) fail(file, `<svg>: event attribute "${name}"`);
  }
  let inner = root[2].replace(/<(title|desc)\b[^>]*>[\s\S]*?<\/\1>/g, '');

  const out = [];
  const stack = [];
  const tagRe = /<(\/?)([a-zA-Z][\w:-]*)([^>]*?)(\/?)>|([^<]+)/g;
  let m;
  while ((m = tagRe.exec(inner)) !== null) {
    if (m[5] !== undefined) {
      const chunk = m[5];
      if (!chunk.trim()) continue;
      const parent = stack[stack.length - 1];
      if (parent !== 'text' && parent !== 'tspan')
        fail(file, `text outside <text>: "${chunk.trim()}"`);
      if (/[<>]|&(?!amp;|lt;|gt;|quot;|apos;)/.test(chunk)) fail(file, 'unsafe text content');
      out.push(chunk.trim());
      continue;
    }
    const [, closing, rawTag, attrText, selfClosing] = m;
    const tag = rawTag.toLowerCase();
    if (!ALLOWED_ELEMENTS.has(tag)) fail(file, `element <${rawTag}> not allowed`);
    if (closing) {
      if (stack.pop() !== tag) fail(file, `unbalanced </${tag}>`);
      out.push(`</${tag}>`);
      continue;
    }
    const attrs = parseAttrs(file, tag, attrText);
    const parts = [];
    for (const [name, value] of attrs) {
      checkAttr(file, tag, name, value);
      parts.push(`${name}="${escapeAttr(value.trim())}"`);
    }
    const open = `<${tag}${parts.length ? ` ${parts.join(' ')}` : ''}`;
    if (selfClosing) {
      out.push(`${open}/>`);
    } else {
      out.push(`${open}>`);
      stack.push(tag);
    }
  }
  if (stack.length) fail(file, `unclosed <${stack.join('>, <')}>`);
  let body = out.join('');
  if (!body) fail(file, 'empty drawing');
  if (transform) body = `<g transform="${escapeAttr(transform)}">${body}</g>`;
  return body;
}

function firstMatch(rules, id) {
  for (const [re, value] of rules) if (re.test(id)) return value;
  return undefined;
}

/**
 * Validates the manifest against the svg directory and returns the normalised
 * asset list, in manifest order. The manifest type `asset` (used by a few
 * v2.1 entries) is folded into `bookable` for Postazioni and `structure`
 * elsewhere.
 */
export function normaliseManifest(manifest, svgFiles) {
  if (manifest.viewBox !== VIEWBOX) fail('manifest.json', `viewBox must be "${VIEWBOX}"`);
  if (!Array.isArray(manifest.assets)) fail('manifest.json', '"assets" must be a list');
  const seen = new Set();
  const assets = manifest.assets.map((a, i) => {
    const where = `manifest.json assets[${i}]`;
    if (!/^[a-z][a-z0-9_]*$/.test(a.id ?? '')) fail(where, `invalid id "${a.id}"`);
    if (seen.has(a.id)) fail(where, `duplicate id "${a.id}"`);
    seen.add(a.id);
    if (a.file !== `svg/${a.id}.svg`) fail(where, `file must be "svg/${a.id}.svg"`);
    const category = CATEGORIES[a.category];
    if (!category) fail(where, `unknown category "${a.category}"`);
    let type = a.type;
    if (type === 'asset') type = category === 'spots' ? 'bookable' : 'structure';
    if (!TYPES.has(type)) fail(where, `unknown type "${a.type}"`);
    if (typeof a.label !== 'string' || !a.label.trim()) fail(where, 'missing label');
    const out = { id: a.id, label: a.label.trim(), category, type };
    if (type === 'bookable') {
      const kind = firstMatch(SPOT_KIND_RULES, a.id);
      if (!kind) fail(where, `no SpotKind rule for bookable "${a.id}" (see SPOT_KIND_RULES)`);
      out.spotKind = kind;
    } else {
      out.decorationKind = firstMatch(DECORATION_KIND_RULES, a.id) ?? 'custom';
    }
    if (TILE_RE.test(a.id)) out.tile = true;
    if (VARIANTS[a.id]) out.variants = VARIANTS[a.id];
    return out;
  });
  if (typeof manifest.total === 'number' && manifest.total !== assets.length) {
    fail('manifest.json', `total ${manifest.total} but ${assets.length} assets`);
  }
  const files = new Set(svgFiles.filter((f) => f.endsWith('.svg')).map((f) => f.slice(0, -4)));
  for (const id of seen) if (!files.has(id)) fail('manifest.json', `missing svg/${id}.svg`);
  for (const f of files) if (!seen.has(f)) fail(`svg/${f}.svg`, 'not listed in manifest.json');
  for (const id of Object.keys(NORMALISE)) {
    if (!seen.has(id)) fail('gen-map-kit.mjs', `NORMALISE names unknown asset "${id}"`);
  }
  for (const id of Object.keys(VARIANTS)) {
    if (!seen.has(id)) fail('gen-map-kit.mjs', `VARIANTS names unknown asset "${id}"`);
  }
  return assets;
}

const HEADER =
  '// Code generated by web/scripts/gen-map-kit.mjs from assets/beach-map-kit; DO NOT EDIT.\n' +
  '// Artwork © Coste360 SRL (see assets/beach-map-kit/NOTICE.md). Run `make gen` to update.\n';

const q = (s) => JSON.stringify(s);
/** Single-quoted TS string literal (Prettier style) when the text allows it. */
const sq = (s) => (/['\\\n\r\u2028\u2029]/.test(s) ? JSON.stringify(s) : `'${s}'`);

export function renderManifestTs(assets, version) {
  const ids = assets.map((a) => `  ${q(a.id)},`).join('\n');
  const cats = Object.values(CATEGORIES);
  const rows = assets
    .map((a) => {
      const extra = [
        a.spotKind ? `spotKind: ${q(a.spotKind)}` : `decorationKind: ${q(a.decorationKind)}`,
        a.tile ? 'tile: true' : '',
        a.variants ? `variants: [${a.variants.map(q).join(', ')}]` : '',
      ]
        .filter(Boolean)
        .join(', ');
      return (
        `  { id: ${q(a.id)}, label: ${q(a.label)}, labelKey: ${q(`kit.asset.${a.id}`)}, ` +
        `category: ${q(a.category)}, type: ${q(a.type)}, ${extra} },`
      );
    })
    .join('\n');
  return `${HEADER}
/** Version of the kit manifest the code was generated from. */
export const KIT_VERSION = ${q(version)};

/** viewBox of every kit symbol. */
export const KIT_VIEWBOX = ${q(VIEWBOX)};

export const KIT_ASSET_IDS = [
${ids}
] as const;

export type KitAssetId = (typeof KIT_ASSET_IDS)[number];

export const KIT_CATEGORIES = [${cats.map(q).join(', ')}] as const;

export type KitCategory = (typeof KIT_CATEGORIES)[number];

export type KitAssetType = 'bookable' | 'structure' | 'decoration';

export interface KitAsset {
  id: KitAssetId;
  /** Italian label from the kit manifest (source language). */
  label: string;
  /** i18n key of the label in the map editor namespace. */
  labelKey: string;
  category: KitCategory;
  type: KitAssetType;
  /** SpotKind of a bookable asset. */
  spotKind?: string;
  /** DecorationKind suggested for a structure or decoration asset. */
  decorationKind?: string;
  /** Modular tile (walkways): fixed scale, snapped to the tile grid. */
  tile?: true;
  /** Colour variants the element may choose (decoration \`variant\`). */
  variants?: readonly string[];
}

export const KIT_MANIFEST: readonly KitAsset[] = [
${rows}
];
`;
}

export function renderSymbolsTs(assets, bodies) {
  const rows = assets.map((a) => `  ${a.id}:\n    ${sq(bodies.get(a.id))},`).join('\n');
  return `${HEADER}
import type { KitAssetId } from './manifest';

/**
 * Sanitised body of the <symbol id="ark-<id>" viewBox="0 0 120 120"> of each
 * asset. Colours are \`var(--beach-*, fallback)\`, so the art follows the theme.
 */
export const KIT_SYMBOLS: Readonly<Record<KitAssetId, string>> = {
${rows}
};
`;
}

export function renderGo(assets) {
  const list = (pred) =>
    assets
      .filter(pred)
      .map((a) => a.id)
      .sort()
      .map((id) => `\t${q(id)},`)
      .join('\n');
  const entries = Object.entries(VARIANTS).sort(([a], [b]) => a.localeCompare(b));
  const width = Math.max(0, ...entries.map(([id]) => q(id).length + 1));
  const variants = entries
    .map(([id, vs]) => `\t${`${q(id)}:`.padEnd(width)} {${vs.map(q).join(', ')}},`)
    .join('\n');
  return `// Code generated by web/scripts/gen-map-kit.mjs from assets/beach-map-kit; DO NOT EDIT.

package mapping

// KitBookableAssets are the kit illustrations a spot may use (manifest type
// bookable), sorted.
var KitBookableAssets = []string{
${list((a) => a.type === 'bookable')}
}

// KitDecorationAssets are the kit illustrations of non-bookable elements
// (manifest types structure and decoration), sorted.
var KitDecorationAssets = []string{
${list((a) => a.type !== 'bookable')}
}

// KitAssetVariants are the colour variants an element with that asset may choose.
var KitAssetVariants = map[string][]string{
${variants}
}
`;
}

/** Reads the kit and returns the rendered files (path relative to the repo -> content). */
export function generate(kitDir = KIT_DIR) {
  const manifest = JSON.parse(readFileSync(join(kitDir, 'manifest.json'), 'utf8'));
  const svgDir = join(kitDir, 'svg');
  const assets = normaliseManifest(manifest, readdirSync(svgDir));
  const bodies = new Map();
  for (const a of assets) {
    const file = `svg/${a.id}.svg`;
    bodies.set(
      a.id,
      symbolBody(file, readFileSync(join(svgDir, `${a.id}.svg`), 'utf8'), NORMALISE[a.id]),
    );
  }
  const rel = (p) => relative(REPO, p).split(sep).join('/');
  return {
    assets,
    files: {
      [rel(join(TS_OUT, 'manifest.ts'))]: renderManifestTs(assets, String(manifest.version ?? '')),
      [rel(join(TS_OUT, 'symbols.ts'))]: renderSymbolsTs(assets, bodies),
      [rel(GO_OUT)]: renderGo(assets),
    },
  };
}

function main() {
  const { assets, files } = generate();
  for (const [path, content] of Object.entries(files)) {
    const full = join(REPO, path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, content);
  }
  console.log(`gen-map-kit: ${assets.length} assets -> ${Object.keys(files).join(', ')}`);
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  try {
    main();
  } catch (err) {
    console.error(`gen-map-kit: ${err instanceof KitError ? err.message : err.stack}`);
    process.exit(1);
  }
}
