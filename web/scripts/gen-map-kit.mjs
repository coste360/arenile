#!/usr/bin/env node
// Beach map kits generator (run by `make gen`, hence checked by `make check`).
//
// Reads every Coste360 kit in assets/map-kits/<key>/ (manifest.json +
// svg/*.svg, kept untouched as the source of truth) and writes:
//   - web/shared/src/map/kit/generated/manifest.ts        typed catalogue (shared ids) + kit list
//   - web/shared/src/map/kit/generated/symbols-<key>.ts   <symbol> bodies of one kit (lazy chunk)
//   - internal/domain/mapping/kitassets_gen.go            asset ids and kit keys for the backend
//
// Only node built-ins: the SVGs are small and regular, so a strict tokenizer
// with an allowlist is enough (no optimiser, no parser dependency). The
// generation FAILS on anything unexpected: elements outside the kit's
// allowlist (script, style, image, use, foreignObject, a, animation...),
// event handler attributes, href/xlink:href, url(...) references,
// javascript: values, backslashes (CSS escapes that could spell url() once a
// presentation attribute is parsed as CSS), characters the tokenizer cannot
// place, a viewBox other than 0 0 120 120, a manifest that does not match the
// files, or kits whose catalogues differ (same ids, categories and types in
// every kit, so switching kit never breaks a saved map). Kits with the `fill`
// profile (v3) add: colours only `var(--beach-*, fallback)`, the indicator
// corner (x 92-120, y 0-28) left empty, and the state of every bookable asset
// drawn with `--beach-state`.
// See docs/adr/0012-kit-mappa.md and docs/adr/0024-kit-mappa-v3-e-scelta-del-kit.md.
import { readFileSync, readdirSync, writeFileSync, mkdirSync, rmSync, existsSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
export const REPO = join(HERE, '..', '..');
export const KITS_DIR = join(REPO, 'assets', 'map-kits');
const TS_OUT = join(REPO, 'web', 'shared', 'src', 'map', 'kit', 'generated');
const GO_OUT = join(REPO, 'internal', 'domain', 'mapping', 'kitassets_gen.go');

export const VIEWBOX = '0 0 120 120';

/**
 * The kits shipped in the binary, default first. `profile` selects the
 * validation rules and how the app draws the booking status:
 *   - halo (v2.x): status in a tinted disc under the art, art never recoloured;
 *   - fill (v3.x): the app sets `--beach-state` and the art paints its status part.
 */
export const KITS = [
  { key: 'modern', profile: 'fill' },
  { key: 'classic', profile: 'halo' },
];
export const DEFAULT_KIT = KITS[0].key;

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
 * Normalisations applied to the art of one kit (the raw file stays
 * untouched). Classic: ombrellone_quadrato is drawn as a diamond (square
 * rotated 45 degrees around 51,51): rotate it back and re-centre it so that
 * it is axis-aligned like every other asset and follows the element
 * rotation. Modern draws it axis-aligned already.
 */
export const NORMALISE = {
  classic: { ombrellone_quadrato: 'translate(9 9) rotate(-45 51 51)' },
  modern: {},
};

/** Indicator corner the fill profile keeps empty (manifest `indicatorSlot`). */
export const INDICATOR_SLOT = { x: 92, y: 0, width: 28, height: 28 };
/** Full-bleed backgrounds, the only assets allowed in the indicator corner. */
export const CORNER_EXEMPT = new Set(['mare', 'sabbia']);
/** The variable the app sets to the status colour (manifest `stateVariable`). */
export const STATE_VARIABLE = '--beach-state';

const HALO_ELEMENTS = new Set([
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
const FILL_ELEMENTS = new Set(['g', 'path', 'rect', 'circle', 'ellipse']);

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
const TEXT_ATTRS = new Set([
  'font-family',
  'font-size',
  'font-weight',
  'text-anchor',
  'dominant-baseline',
  'letter-spacing',
  'x1',
  'y1',
  'x2',
  'y2',
  'points',
]);

/** fill profile colours: `none` or var(--beach-*, #hex | var(--beach-*, #hex)). */
const HEX = '#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?';
const FILL_COLOUR_RE = new RegExp(
  `^(?:none|var\\(--beach-[a-z-]+,\\s*(?:${HEX}|var\\(--beach-[a-z-]+,\\s*${HEX}\\))\\))$`,
);
const STATE_COLOUR = new RegExp(`var\\(${STATE_VARIABLE}[,)]`);

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

function checkAttr(file, tag, name, value, profile) {
  const lower = name.toLowerCase();
  if (lower.startsWith('on')) fail(file, `<${tag}>: event attribute "${name}" not allowed`);
  if (lower === 'href' || lower.endsWith(':href')) fail(file, `<${tag}>: "${name}" not allowed`);
  if (lower === 'style') fail(file, `<${tag}>: "style" attribute not allowed`);
  // A backslash is a CSS escape: "u\72l(#x)" is url(#x) once parsed as CSS.
  if (value.includes('\\')) fail(file, `<${tag}>: backslash in ${name}="${value}"`);
  if (/url\s*\(/i.test(value)) fail(file, `<${tag}>: external reference ${name}="${value}"`);
  if (/javascript:|data:|&#|<|>/i.test(value)) {
    fail(file, `<${tag}>: unsafe value ${name}="${value}"`);
  }
  if (!ALLOWED_ATTRS.has(lower)) fail(file, `<${tag}>: attribute "${name}" not allowed`);
  if (profile === 'fill') {
    if (TEXT_ATTRS.has(lower)) fail(file, `<${tag}>: attribute "${name}" not allowed in a v3 kit`);
    if ((lower === 'fill' || lower === 'stroke') && !FILL_COLOUR_RE.test(value.trim())) {
      fail(file, `<${tag}>: ${name}="${value}" must be none or var(--beach-*, fallback)`);
    }
  }
}

const escapeAttr = (v) => v.replace(/&/g, '&amp;').replace(/"/g, '&quot;');

// ---- Geometry (indicator corner check of the fill profile) -------------------

const IDENTITY = [1, 0, 0, 1, 0, 0];

function multiply(a, b) {
  return [
    a[0] * b[0] + a[2] * b[1],
    a[1] * b[0] + a[3] * b[1],
    a[0] * b[2] + a[2] * b[3],
    a[1] * b[2] + a[3] * b[3],
    a[0] * b[4] + a[2] * b[5] + a[4],
    a[1] * b[4] + a[3] * b[5] + a[5],
  ];
}

const nums = (s) => (s.match(/-?(?:\d+\.?\d*|\.\d+)(?:e[-+]?\d+)?/gi) ?? []).map(Number);

/** SVG transform list to a matrix [a b c d e f]. */
export function parseTransform(file, text) {
  let m = IDENTITY;
  const re = /\s*(translate|scale|rotate|matrix)\s*\(([^)]*)\)\s*,?/gy;
  let pos = 0;
  let t;
  while ((t = re.exec(text)) !== null) {
    pos = re.lastIndex;
    const v = nums(t[2]);
    let next;
    switch (t[1]) {
      case 'translate':
        next = [1, 0, 0, 1, v[0] ?? 0, v[1] ?? 0];
        break;
      case 'scale':
        next = [v[0] ?? 1, 0, 0, v[1] ?? v[0] ?? 1, 0, 0];
        break;
      case 'rotate': {
        const a = ((v[0] ?? 0) * Math.PI) / 180;
        const [cx, cy] = [v[1] ?? 0, v[2] ?? 0];
        const r = [Math.cos(a), Math.sin(a), -Math.sin(a), Math.cos(a), 0, 0];
        next = multiply(multiply([1, 0, 0, 1, cx, cy], r), [1, 0, 0, 1, -cx, -cy]);
        break;
      }
      default:
        if (v.length !== 6) fail(file, `matrix() needs 6 numbers: "${text}"`);
        next = v;
    }
    m = multiply(m, next);
  }
  if (text.slice(pos).trim()) fail(file, `unsupported transform "${text}"`);
  return m;
}

const apply = (m, x, y) => [m[0] * x + m[2] * y + m[4], m[1] * x + m[3] * y + m[5]];

/**
 * Outline of a path as polylines, one per subpath (curves and arcs sampled),
 * in the path's own coordinates.
 */
export function pathOutlines(file, d) {
  const tokens = d.match(/[MmLlHhVvCcSsQqTtAaZz]|-?(?:\d+\.?\d*|\.\d+)(?:e[-+]?\d+)?/gi) ?? [];
  const subpaths = [];
  let pts = [];
  let i = 0;
  let cmd = '';
  let x = 0;
  let y = 0;
  let sx = 0;
  let sy = 0;
  let cx = 0;
  let cy = 0; // last control point (for S/T)
  let prev = '';
  const num = () => {
    const v = Number(tokens[i++]);
    if (!Number.isFinite(v)) fail(file, `malformed path data "${d}"`);
    return v;
  };
  const STEPS = 24;
  const bezier = (p0, p1, p2, p3) => {
    for (let k = 1; k <= STEPS; k++) {
      const t = k / STEPS;
      const u = 1 - t;
      if (p3) {
        pts.push([
          u * u * u * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t * t * t * p3[0],
          u * u * u * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t * t * t * p3[1],
        ]);
      } else {
        pts.push([
          u * u * p0[0] + 2 * u * t * p1[0] + t * t * p2[0],
          u * u * p0[1] + 2 * u * t * p1[1] + t * t * p2[1],
        ]);
      }
    }
  };
  // Elliptical arc, endpoint to centre parameterisation (SVG 1.1 F.6.5).
  const arc = (x1, y1, rx, ry, phiDeg, large, sweep, x2, y2) => {
    if (!rx || !ry || (x1 === x2 && y1 === y2)) {
      pts.push([x2, y2]);
      return;
    }
    const phi = (phiDeg * Math.PI) / 180;
    const [c, s] = [Math.cos(phi), Math.sin(phi)];
    const dx = (x1 - x2) / 2;
    const dy = (y1 - y2) / 2;
    const x1p = c * dx + s * dy;
    const y1p = -s * dx + c * dy;
    const lambda = (x1p * x1p) / (rx * rx) + (y1p * y1p) / (ry * ry);
    if (lambda > 1) {
      rx *= Math.sqrt(lambda);
      ry *= Math.sqrt(lambda);
    }
    const num2 = rx * rx * ry * ry - rx * rx * y1p * y1p - ry * ry * x1p * x1p;
    const den = rx * rx * y1p * y1p + ry * ry * x1p * x1p;
    let k = Math.sqrt(Math.max(0, num2 / den));
    if (large === sweep) k = -k;
    const cxp = (k * rx * y1p) / ry;
    const cyp = (-k * ry * x1p) / rx;
    const ccx = c * cxp - s * cyp + (x1 + x2) / 2;
    const ccy = s * cxp + c * cyp + (y1 + y2) / 2;
    const angle = (ux, uy, vx, vy) => Math.atan2(ux * vy - uy * vx, ux * vx + uy * vy);
    const t1 = angle(1, 0, (x1p - cxp) / rx, (y1p - cyp) / ry);
    let dt = angle((x1p - cxp) / rx, (y1p - cyp) / ry, (-x1p - cxp) / rx, (-y1p - cyp) / ry);
    if (!sweep && dt > 0) dt -= 2 * Math.PI;
    if (sweep && dt < 0) dt += 2 * Math.PI;
    for (let n = 1; n <= STEPS; n++) {
      const t = t1 + (dt * n) / STEPS;
      const ex = rx * Math.cos(t);
      const ey = ry * Math.sin(t);
      pts.push([c * ex - s * ey + ccx, s * ex + c * ey + ccy]);
    }
  };
  while (i < tokens.length) {
    if (/[a-z]/i.test(tokens[i])) cmd = tokens[i++];
    else if (!cmd) fail(file, `path data must start with a command: "${d}"`);
    const rel = cmd === cmd.toLowerCase();
    const ox = rel ? x : 0;
    const oy = rel ? y : 0;
    switch (cmd.toUpperCase()) {
      case 'M':
        x = ox + num();
        y = oy + num();
        sx = x;
        sy = y;
        if (pts.length) subpaths.push(pts);
        pts = [[x, y]];
        cmd = rel ? 'l' : 'L';
        break;
      case 'L':
        x = ox + num();
        y = oy + num();
        pts.push([x, y]);
        break;
      case 'H':
        x = ox + num();
        pts.push([x, y]);
        break;
      case 'V':
        y = oy + num();
        pts.push([x, y]);
        break;
      case 'C': {
        const p1 = [ox + num(), oy + num()];
        const p2 = [ox + num(), oy + num()];
        const p3 = [ox + num(), oy + num()];
        bezier([x, y], p1, p2, p3);
        [cx, cy] = p2;
        [x, y] = p3;
        break;
      }
      case 'S': {
        const p1 = /[CS]/i.test(prev) ? [2 * x - cx, 2 * y - cy] : [x, y];
        const p2 = [ox + num(), oy + num()];
        const p3 = [ox + num(), oy + num()];
        bezier([x, y], p1, p2, p3);
        [cx, cy] = p2;
        [x, y] = p3;
        break;
      }
      case 'Q': {
        const p1 = [ox + num(), oy + num()];
        const p2 = [ox + num(), oy + num()];
        bezier([x, y], p1, p2);
        [cx, cy] = p1;
        [x, y] = p2;
        break;
      }
      case 'T': {
        const p1 = /[QT]/i.test(prev) ? [2 * x - cx, 2 * y - cy] : [x, y];
        const p2 = [ox + num(), oy + num()];
        bezier([x, y], p1, p2);
        [cx, cy] = p1;
        [x, y] = p2;
        break;
      }
      case 'A': {
        const rx = Math.abs(num());
        const ry = Math.abs(num());
        const phi = num();
        const large = num() !== 0;
        const sweep = num() !== 0;
        const nx = ox + num();
        const ny = oy + num();
        arc(x, y, rx, ry, phi, large, sweep, nx, ny);
        x = nx;
        y = ny;
        break;
      }
      case 'Z':
        x = sx;
        y = sy;
        pts.push([x, y]);
        break;
      default:
        fail(file, `unsupported path command "${cmd}"`);
    }
    prev = cmd;
  }
  if (pts.length) subpaths.push(pts);
  return subpaths;
}

/** Outline polylines of one shape element, in its own coordinates (null: not a shape). */
function shapeOutlines(file, tag, attrs) {
  const n = (k) => Number(attrs.get(k) ?? 0);
  if (tag === 'rect') {
    const [x, y, w, h] = [n('x'), n('y'), n('width'), n('height')];
    // Rounded corners as SVG computes them (rx defaults to ry and vice versa).
    let rx = attrs.has('rx') ? n('rx') : attrs.has('ry') ? n('ry') : 0;
    let ry = attrs.has('ry') ? n('ry') : rx;
    rx = Math.min(Math.abs(rx), w / 2);
    ry = Math.min(Math.abs(ry), h / 2);
    if (!rx || !ry) {
      return [
        [
          [x, y],
          [x + w, y],
          [x + w, y + h],
          [x, y + h],
          [x, y],
        ],
      ];
    }
    const pts = [];
    const corners = [
      [x + w - rx, y + ry, -Math.PI / 2],
      [x + w - rx, y + h - ry, 0],
      [x + rx, y + h - ry, Math.PI / 2],
      [x + rx, y + ry, Math.PI],
    ];
    for (const [ccx, ccy, start] of corners) {
      for (let k = 0; k <= 12; k++) {
        const t = start + (k / 12) * (Math.PI / 2);
        pts.push([ccx + rx * Math.cos(t), ccy + ry * Math.sin(t)]);
      }
    }
    pts.push(pts[0]);
    return [pts];
  }
  if (tag === 'circle' || tag === 'ellipse') {
    const rx = tag === 'circle' ? n('r') : n('rx');
    const ry = tag === 'circle' ? n('r') : n('ry');
    const [cx, cy] = [n('cx'), n('cy')];
    const pts = [];
    for (let k = 0; k <= 48; k++) {
      const t = (k / 48) * 2 * Math.PI;
      pts.push([cx + rx * Math.cos(t), cy + ry * Math.sin(t)]);
    }
    return [pts];
  }
  if (tag === 'path') return pathOutlines(file, attrs.get('d') ?? '');
  return null;
}

/** Even-odd point in polygon over every subpath (implicitly closed). */
function insideFill(px, py, polys) {
  let inside = false;
  for (const poly of polys) {
    for (let a = 0, b = poly.length - 1; a < poly.length; b = a++) {
      const [xa, ya] = poly[a];
      const [xb, yb] = poly[b];
      if (ya > py !== yb > py && px < ((xb - xa) * (py - ya)) / (yb - ya) + xa) inside = !inside;
    }
  }
  return inside;
}

/** Whether point p lies in the indicator corner grown by `pad` (minus a hair for anti-aliasing). */
function inSlot([px, py], pad, eps = 0.05) {
  const s = INDICATOR_SLOT;
  return (
    px > s.x - pad + eps &&
    px < s.x + s.width + pad &&
    py > s.y - pad &&
    py < s.y + s.height + pad - eps
  );
}

/**
 * Whether a painted element reaches into the indicator corner, in viewBox
 * units: an outline point (lines densified to 0.5 units, stroke half-width
 * as padding) inside the corner, or (filled shapes) the corner covered by
 * the fill. Returns the element's bounding box when it does, else null.
 */
export function cornerHit(file, tag, attrs, matrix, { filled, strokeWidth }) {
  const local = shapeOutlines(file, tag, attrs);
  if (!local) return null;
  const scale = Math.sqrt(Math.abs(matrix[0] * matrix[3] - matrix[1] * matrix[2]));
  const pad = (strokeWidth * scale) / 2;
  const polys = local.map((poly) => poly.map(([x, y]) => apply(matrix, x, y)));
  let box = [Infinity, Infinity, -Infinity, -Infinity];
  let hit = false;
  for (const poly of polys) {
    for (let k = 0; k < poly.length; k++) {
      const [x, y] = poly[k];
      box = [Math.min(box[0], x), Math.min(box[1], y), Math.max(box[2], x), Math.max(box[3], y)];
      const next = poly[k + 1];
      const steps = next ? Math.max(1, Math.ceil(Math.hypot(next[0] - x, next[1] - y) / 0.5)) : 1;
      for (let s = 0; s < steps && !hit; s++) {
        const t = s / steps;
        const p = next ? [x + (next[0] - x) * t, y + (next[1] - y) * t] : [x, y];
        if (inSlot(p, pad)) hit = true;
      }
    }
  }
  if (!hit && filled) {
    const s = INDICATOR_SLOT;
    const probes = [
      [s.x + s.width / 2, s.y + s.height / 2],
      [s.x + 1, s.y + 1],
      [s.x + s.width - 1, s.y + 1],
      [s.x + 1, s.y + s.height - 1],
      [s.x + s.width - 1, s.y + s.height - 1],
    ];
    hit = probes.some(([px, py]) => insideFill(px, py, polys));
  }
  return hit ? box.map((v) => +v.toFixed(2)) : null;
}

// ---- Sanitiser ---------------------------------------------------------------

/**
 * Sanitises one kit SVG. Returns the body of its <symbol> (the children of
 * the root <svg>, without <title>/<desc>, comments, role, aria-*, width and
 * height), whether it paints with `--beach-state`, and (fill profile) the
 * painted elements that reach into the indicator corner. Throws KitError on
 * anything outside the profile's allowlist.
 */
export function sanitise(file, source, { transform, profile = 'halo' } = {}) {
  const elements = profile === 'fill' ? FILL_ELEMENTS : HALO_ELEMENTS;
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
  const inner = root[2].replace(/<(title|desc)\b[^>]*>[\s\S]*?<\/\1>/g, '');

  const out = [];
  const stack = [];
  // Inherited state for the geometry check: matrix, stroke paint and width.
  const ctx = [{ matrix: IDENTITY, fill: 'black', stroke: 'none', strokeWidth: 1 }];
  const corner = [];
  let usesState = false;
  const tagRe = /<(\/?)([a-zA-Z][\w:-]*)([^>]*?)(\/?)>|([^<]+)/g;
  let m;
  let pos = 0;
  while ((m = tagRe.exec(inner)) !== null) {
    // Every character must belong to a token: a stray "<" is an error, not a skip.
    if (m.index !== pos) fail(file, `unexpected "${inner.slice(pos, m.index).trim()}"`);
    pos = tagRe.lastIndex;
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
    if (!elements.has(tag)) fail(file, `element <${rawTag}> not allowed`);
    if (closing) {
      if (stack.pop() !== tag) fail(file, `unbalanced </${tag}>`);
      ctx.pop();
      out.push(`</${tag}>`);
      continue;
    }
    const attrs = parseAttrs(file, tag, attrText);
    const parts = [];
    for (const [name, value] of attrs) {
      checkAttr(file, tag, name, value, profile);
      if ((name === 'fill' || name === 'stroke') && STATE_COLOUR.test(value)) usesState = true;
      parts.push(`${name}="${escapeAttr(value.trim())}"`);
    }
    if (profile === 'fill') {
      const map = new Map(attrs.map(([k, v]) => [k.toLowerCase(), v.trim()]));
      const parent = ctx[ctx.length - 1];
      const here = {
        matrix: map.has('transform')
          ? multiply(parent.matrix, parseTransform(file, map.get('transform')))
          : parent.matrix,
        fill: map.get('fill') ?? parent.fill,
        stroke: map.get('stroke') ?? parent.stroke,
        strokeWidth: map.has('stroke-width') ? Number(map.get('stroke-width')) : parent.strokeWidth,
      };
      const fillNone = here.fill === 'none';
      const painted = !(fillNone && here.stroke === 'none') && map.get('opacity') !== '0';
      if (painted && tag !== 'g') {
        const sw = here.stroke === 'none' ? 0 : here.strokeWidth;
        const box = cornerHit(file, tag, map, here.matrix, { filled: !fillNone, strokeWidth: sw });
        if (box) corner.push({ tag, box });
      }
      if (!selfClosing) ctx.push(here);
    } else if (!selfClosing) {
      ctx.push(ctx[ctx.length - 1]);
    }
    const open = `<${tag}${parts.length ? ` ${parts.join(' ')}` : ''}`;
    if (selfClosing) {
      out.push(`${open}/>`);
    } else {
      out.push(`${open}>`);
      stack.push(tag);
    }
  }
  if (pos !== inner.length) fail(file, `unexpected "${inner.slice(pos).trim()}"`);
  if (stack.length) fail(file, `unclosed <${stack.join('>, <')}>`);
  let body = out.join('');
  if (!body) fail(file, 'empty drawing');
  if (transform) body = `<g transform="${escapeAttr(transform)}">${body}</g>`;
  return { body, usesState, corner };
}

/** Body of the <symbol> of one SVG (halo profile unless told otherwise). */
export function symbolBody(file, source, transform, profile = 'halo') {
  return sanitise(file, source, { transform, profile }).body;
}

function firstMatch(rules, id) {
  for (const [re, value] of rules) if (re.test(id)) return value;
  return undefined;
}

/**
 * Validates the manifest against the svg directory and returns the normalised
 * asset list, in manifest order. The manifest type `asset` (used by a few
 * entries) is folded into `bookable` for Postazioni and `structure`
 * elsewhere. `normalise` lists the asset ids the kit transforms.
 */
export function normaliseManifest(manifest, svgFiles, normalise = {}) {
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
  for (const id of Object.keys(normalise)) {
    if (!seen.has(id)) fail('gen-map-kit.mjs', `NORMALISE names unknown asset "${id}"`);
  }
  for (const id of Object.keys(VARIANTS)) {
    if (!seen.has(id)) fail('gen-map-kit.mjs', `VARIANTS names unknown asset "${id}"`);
  }
  return assets;
}

/** Fill profile rules on the manifest keys (indicator slot, state variable). */
function checkFillManifest(kit, manifest) {
  const where = `${kit}/manifest.json`;
  if (manifest.stateVariable !== STATE_VARIABLE) {
    fail(where, `stateVariable must be "${STATE_VARIABLE}"`);
  }
  const s = manifest.indicatorSlot ?? {};
  for (const k of ['x', 'y', 'width', 'height']) {
    if (s[k] !== INDICATOR_SLOT[k]) fail(where, `indicatorSlot.${k} must be ${INDICATOR_SLOT[k]}`);
  }
}

/** Reads, validates and sanitises one kit directory. */
export function readKit(kitDir, { key, profile }) {
  const manifest = JSON.parse(readFileSync(join(kitDir, 'manifest.json'), 'utf8'));
  const svgDir = join(kitDir, 'svg');
  const normalise = NORMALISE[key] ?? {};
  let assets;
  try {
    assets = normaliseManifest(manifest, readdirSync(svgDir), normalise);
  } catch (err) {
    if (err instanceof KitError) throw new KitError(`${key}/${err.message}`);
    throw err;
  }
  if (profile === 'fill') checkFillManifest(key, manifest);
  const byId = new Map(manifest.assets.map((a) => [a.id, a]));
  const bodies = new Map();
  const stateful = [];
  for (const a of assets) {
    const file = `${key}/svg/${a.id}.svg`;
    const { body, usesState, corner } = sanitise(
      file,
      readFileSync(join(svgDir, `${a.id}.svg`), 'utf8'),
      { transform: normalise[a.id], profile },
    );
    if (profile === 'fill') {
      const declared = byId.get(a.id);
      if (a.type === 'bookable' && !usesState) {
        fail(file, `a bookable asset must paint its status with var(${STATE_VARIABLE}, …)`);
      }
      if (declared.stateful !== undefined && declared.stateful !== usesState) {
        fail(file, `manifest says stateful=${declared.stateful} but the art says ${usesState}`);
      }
      const exempt = CORNER_EXEMPT.has(a.id);
      if (declared.cornerFree === false && !exempt) {
        fail(
          file,
          'cornerFree=false is allowed only for the full-bleed backgrounds (mare, sabbia)',
        );
      }
      if (!exempt && corner.length) {
        const first = corner[0];
        fail(
          file,
          `indicator corner x 92-120, y 0-28 must stay empty: <${first.tag}> covers ` +
            `[${first.box.join(', ')}]${corner.length > 1 ? ` (+${corner.length - 1} more)` : ''}`,
        );
      }
    }
    if (usesState) stateful.push(a.id);
    bodies.set(a.id, body);
  }
  return { key, profile, version: String(manifest.version ?? ''), assets, bodies, stateful };
}

/** Every kit must have the same catalogue: same ids, categories and types, same order. */
export function checkSameCatalogue(kits) {
  const [ref, ...others] = kits;
  const sig = (a) => `${a.id}|${a.category}|${a.type}`;
  const refIds = new Map(ref.assets.map((a) => [a.id, sig(a)]));
  for (const k of others) {
    const ids = new Map(k.assets.map((a) => [a.id, sig(a)]));
    for (const [id, s] of refIds) {
      if (!ids.has(id)) fail(`${k.key}/manifest.json`, `missing asset "${id}" of ${ref.key}`);
      if (ids.get(id) !== s) {
        fail(`${k.key}/manifest.json`, `"${id}" is ${ids.get(id)} but ${s} in ${ref.key}`);
      }
    }
    for (const id of ids.keys()) {
      if (!refIds.has(id)) fail(`${k.key}/manifest.json`, `asset "${id}" not in ${ref.key}`);
    }
  }
}

const HEADER =
  '// Code generated by web/scripts/gen-map-kit.mjs from assets/map-kits; DO NOT EDIT.\n' +
  '// Artwork © Coste360 SRL (see assets/map-kits/*/NOTICE.md). Run `make gen` to update.\n';

const q = (s) => JSON.stringify(s);
/** Single-quoted TS string literal (Prettier style) when the text allows it. */
const sq = (s) => (/['\\\n\r\u2028\u2029]/.test(s) ? JSON.stringify(s) : `'${s}'`);

export function renderManifestTs(kits) {
  const assets = kits[0].assets;
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
  const kitRows = kits
    .map((k) => {
      const stateful = k.stateful.length
        ? `[\n${k.stateful.map((id) => `      ${q(id)},`).join('\n')}\n    ]`
        : '[]';
      return (
        `  ${k.key}: {\n    key: ${q(k.key)},\n    version: ${q(k.version)},\n` +
        `    style: ${q(k.profile)},\n    stateful: ${stateful},\n  },`
      );
    })
    .join('\n');
  return `${HEADER}
/** Kits shipped in the binary (assets/map-kits/<key>), the default first. */
export const MAP_KIT_KEYS = [${kits.map((k) => q(k.key)).join(', ')}] as const;

export type MapKitKey = (typeof MAP_KIT_KEYS)[number];

/** Kit of an establishment that has not chosen one. */
export const DEFAULT_MAP_KIT: MapKitKey = ${q(DEFAULT_KIT)};

export interface MapKitInfo {
  key: MapKitKey;
  /** Version of the kit manifest the code was generated from. */
  version: string;
  /**
   * How the booking status is drawn: \`halo\` a tinted disc under the art,
   * \`fill\` the art paints its status part with \`--beach-state\`.
   */
  style: 'halo' | 'fill';
  /** Assets whose art reads \`--beach-state\`. */
  stateful: readonly KitAssetId[];
}

/** viewBox of every kit symbol. */
export const KIT_VIEWBOX = ${q(VIEWBOX)};

export const KIT_ASSET_IDS = [
${ids}
] as const;

export type KitAssetId = (typeof KIT_ASSET_IDS)[number];

export const MAP_KITS: Readonly<Record<MapKitKey, MapKitInfo>> = {
${kitRows}
};

export const KIT_CATEGORIES = [${cats.map(q).join(', ')}] as const;

export type KitCategory = (typeof KIT_CATEGORIES)[number];

export type KitAssetType = 'bookable' | 'structure' | 'decoration';

export interface KitAsset {
  id: KitAssetId;
  /** Italian label from the default kit manifest (source language). */
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

/** The catalogue, identical in every kit (checked by the generator). */
export const KIT_MANIFEST: readonly KitAsset[] = [
${rows}
];
`;
}

export function renderSymbolsTs(kit) {
  const rows = kit.assets.map((a) => `  ${a.id}:\n    ${sq(kit.bodies.get(a.id))},`).join('\n');
  return `${HEADER}
import type { KitAssetId } from './manifest';

/**
 * Sanitised body of the <symbol id="ark-${kit.key}-<id>" viewBox="0 0 120 120">
 * of each asset of the ${kit.key} kit (v${kit.version}). Colours are
 * \`var(--beach-*, fallback)\`, so the art follows the theme.
 */
export const KIT_SYMBOLS: Readonly<Record<KitAssetId, string>> = {
${rows}
};
`;
}

export function renderGo(kits) {
  const assets = kits[0].assets;
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
  return `// Code generated by web/scripts/gen-map-kit.mjs from assets/map-kits; DO NOT EDIT.

package mapping

// MapKits are the keys of the map kits shipped in the binary, the default first.
var MapKits = []string{${kits.map((k) => q(k.key)).join(', ')}}

// DefaultMapKit is the kit of an establishment that has not chosen one.
const DefaultMapKit = ${q(DEFAULT_KIT)}

// KitBookableAssets are the kit illustrations a spot may use (manifest type
// bookable), sorted. Every kit has the same catalogue.
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

/** Reads every kit and returns the rendered files (path relative to the repo -> content). */
export function generate(kitsDir = KITS_DIR, kitList = KITS) {
  const kits = kitList.map((k) => readKit(join(kitsDir, k.key), k));
  checkSameCatalogue(kits);
  const rel = (p) => relative(REPO, p).split(sep).join('/');
  const files = {
    [rel(join(TS_OUT, 'manifest.ts'))]: renderManifestTs(kits),
    [rel(GO_OUT)]: renderGo(kits),
  };
  for (const k of kits) files[rel(join(TS_OUT, `symbols-${k.key}.ts`))] = renderSymbolsTs(k);
  return { kits, assets: kits[0].assets, files };
}

function main() {
  const { kits, assets, files } = generate();
  // Outputs of kits that no longer exist (and the pre-v3 single symbols.ts).
  if (existsSync(TS_OUT)) {
    const keep = new Set(Object.keys(files).map((p) => p.split('/').pop()));
    for (const f of readdirSync(TS_OUT)) if (!keep.has(f)) rmSync(join(TS_OUT, f));
  }
  for (const [path, content] of Object.entries(files)) {
    const full = join(REPO, path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, content);
  }
  console.log(
    `gen-map-kit: ${kits.map((k) => `${k.key} v${k.version}`).join(', ')}, ` +
      `${assets.length} assets -> ${Object.keys(files).join(', ')}`,
  );
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  try {
    main();
  } catch (err) {
    console.error(`gen-map-kit: ${err instanceof KitError ? err.message : err.stack}`);
    process.exit(1);
  }
}
