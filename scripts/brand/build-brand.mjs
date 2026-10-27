#!/usr/bin/env node
// Derives the outlined SVGs and the raster exports of the brand from the kit
// sources in assets/brand/arenile (brand kit v2.0, ADR 0018). The outputs are
// committed, so neither the build nor the CI needs this script or its tools;
// run it only when the kit changes:
//
//   mkdir -p /tmp/brandtools && cd /tmp/brandtools && npm init -y >/dev/null &&
//     npm i @fontsource/titillium-web@5.3.0 opentype.js@2.0.0
//   ARENILE_BRAND_TOOLS=/tmp/brandtools/node_modules node scripts/brand/build-brand.mjs
//
// Text is converted to outlines with Titillium Web (OFL-1.1, the kit font) so
// the logos render the same where the font is not available (<img>, emails,
// desktop icons, favicons, social images). PNGs are rendered by the Chromium
// of the e2e Playwright install (e2e/node_modules). After this script, run
// scripts/desktop/gen-icons.sh for the desktop package icons.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const tools = process.env.ARENILE_BRAND_TOOLS;
if (!tools) {
  console.error('build-brand: set ARENILE_BRAND_TOOLS to a node_modules with opentype.js and @fontsource/titillium-web');
  process.exit(1);
}
const req = createRequire(join(tools, 'noop.js'));
const opentype = req('opentype.js');
const { chromium } = createRequire(join(root, 'e2e', 'noop.js'))('@playwright/test');

const A = join(root, 'assets', 'brand', 'arenile');
const OUT_SVG = join(A, 'outlined');
const OUT_PNG = join(A, 'png');
mkdirSync(OUT_SVG, { recursive: true });
mkdirSync(OUT_PNG, { recursive: true });

const NAVY = '#0B2542';
const WHITE = '#FFFFFF';

const fontFile = (w) =>
  readFileSync(join(tools, '@fontsource', 'titillium-web', 'files', `titillium-web-latin-${w}-normal.woff`));
const fonts = Object.fromEntries(
  [600, 700].map((w) => {
    const b = fontFile(w);
    return [w, opentype.parse(b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength))];
  }),
);

/** Outlines one line of text; returns the path data and the advance width. */
function textPath({ text, x, y, size, weight, spacing = 0, anchor = 'start' }) {
  const font = fonts[weight];
  const glyphs = font.stringToGlyphs(text);
  const scale = size / font.unitsPerEm;
  let width = 0;
  glyphs.forEach((g, i) => {
    width += g.advanceWidth * scale + (i < glyphs.length - 1 ? spacing : 0);
    if (i < glyphs.length - 1) width += font.getKerningValue(g, glyphs[i + 1]) * scale;
  });
  let cx = anchor === 'middle' ? x - width / 2 : x;
  let d = '';
  glyphs.forEach((g, i) => {
    d += g.getPath(cx, y, size).toPathData(2);
    cx += g.advanceWidth * scale + spacing;
    if (i < glyphs.length - 1) cx += font.getKerningValue(g, glyphs[i + 1]) * scale;
  });
  return { d, width };
}

const attr = (s, n) => new RegExp(`\\s${n}="([^"]*)"`).exec(s)?.[1];

/** Replaces every <text> of a kit SVG with an outlined <path>. */
function outline(svg) {
  return svg.replace(/<text([^>]*)>([^<]*)<\/text>/g, (_, a, text) => {
    const { d } = textPath({
      text,
      x: Number(attr(a, 'x')),
      y: Number(attr(a, 'y')),
      size: Number(attr(a, 'font-size')),
      weight: Number(attr(a, 'font-weight')),
      spacing: Number(attr(a, 'letter-spacing') ?? 0),
      anchor: attr(a, 'text-anchor') ?? 'start',
    });
    return `<path fill="${attr(a, 'fill')}" d="${d}"/>`;
  });
}

/** Single-colour version of a kit SVG (every fill, the title and the label adjusted). */
function recolour(svg, fill, label) {
  return svg
    .replace(/fill="#[0-9A-Fa-f]{6}"/g, `fill="${fill}"`)
    .replace(/(aria-label="Arenile — )[^"]*"/, `$1${label}"`)
    .replace(/(<title>Arenile — )[^<]*</, `$1${label}<`);
}

const read = (dir, f) => readFileSync(join(dir, f), 'utf8');
const write = (f, s) => writeFileSync(join(OUT_SVG, f), s.trim() + '\n');

// 1. The kit logos with outlined text. Plain names are for light surfaces
// (navy "A"), -dark for dark surfaces (sand "A", white lettering), -white and
// -navy are single-colour (navy bands and dark mode; mono marks and print).
const IT = { horizontal: 'orizzontale', header: 'header', stacked: 'verticale' };
for (const l of ['horizontal', 'header', 'stacked']) {
  const light = outline(read(A, `arenile-logo-${l}.svg`));
  write(`arenile-logo-${l}.svg`, light);
  write(`arenile-logo-${l}-dark.svg`, outline(read(A, `arenile-logo-${l}-dark.svg`)));
  write(`arenile-logo-${l}-white.svg`, recolour(light, WHITE, `logo ${IT[l]} bianco`));
  write(`arenile-logo-${l}-navy.svg`, recolour(light, NAVY, `logo ${IT[l]} navy`));
}
write('arenile-wordmark.svg', outline(read(A, 'arenile-wordmark.svg')));
write('arenile-wordmark-white.svg', outline(read(A, 'arenile-wordmark-white.svg')));

// 2. The name alone ("Arenile", Titillium Web 700), for the app chrome next to the symbol.
const name = textPath({ text: 'Arenile', x: 0, y: 0, size: 100, weight: 700, spacing: -1.7 });
const nameBox = fonts[700].getPath('Arenile', 0, 0, 100).getBoundingBox();
for (const [variant, fill] of [
  ['navy', NAVY],
  ['white', WHITE],
]) {
  const w = Math.ceil(name.width + 2);
  const h = Math.ceil(-nameBox.y1 + 2);
  write(
    `arenile-name-${variant}.svg`,
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" role="img" aria-label="Arenile"><title>Arenile</title><path fill="${fill}" transform="translate(1 ${(h - 1).toFixed(2)})" d="${name.d}"/></svg>`,
  );
}

// 3. Coste360 lockup: not generated here. The panel uses the supplied assets/brand/coste360/logo.svg
// (recoloured copies logo-{primary,white}.svg), the docs site the original logo-*.png.

// 4. Raster exports.
/** The drawing of a kit SVG without the root element, the title and the tile. */
const inner = (svg) =>
  svg
    .replace(/^[\s\S]*?<\/title>/, '')
    .replace(/<\/svg>\s*$/, '')
    .replace(/<rect[^>]*\/>/, '');
const symbolDark = read(A, 'arenile-symbol-color-dark.svg');
const symbolVB = attr(symbolDark, 'viewBox').split(' ').map(Number);
const [sx, sy, sw, sh] = symbolVB;
/** Navy full-bleed tile with the sand-A symbol centred at `scale` of the tile (apple-touch, maskable). */
const tile = (scale) => {
  const s = (1024 * scale) / Math.max(sw, sh);
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024"><rect width="1024" height="1024" fill="${NAVY}"/><g transform="translate(512 512) scale(${s}) translate(${-(sx + sw / 2)} ${-(sy + sh / 2)})">${inner(symbolDark)}</g></svg>`;
};
// macOS menu bar template image: the mono symbol in black on transparent, cropped to its bounds; a mask
// widens the gaps around the wave so the three shapes stay apart at 18 px (the system tints it).
const mono = read(A, 'arenile-symbol-navy.svg');
const waveD = [...mono.matchAll(/<path d="([^"]+)"/g)].map((m) => m[1])[3];
const side = Math.max(sw, sh) - 60;
const templateSymbol = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${sx + sw / 2 - side / 2} ${sy + sh / 2 - side / 2} ${side} ${side}"><defs><mask id="m" maskUnits="userSpaceOnUse" x="0" y="0" width="1024" height="1024"><rect width="1024" height="1024" fill="#fff"/><path d="${waveD}" fill="#000" stroke="#000" stroke-width="56" stroke-linejoin="round"/></mask></defs><g mask="url(#m)">${inner(mono).replace(/fill="#[0-9A-Fa-f]{6}"/g, 'fill="#000"')}</g><path d="${waveD}" fill="#000"/></svg>`;
const appIcon = read(A, 'arenile-app-icon.svg');
const appFlat = read(A, 'arenile-app-icon-flat.svg');
const favicon = read(A, 'arenile-favicon.svg');
// macOS icon grid: 824 px body with a 100 px margin on the 1024 canvas.
const macIcon = appIcon.replace(/viewBox="0 0 1024 1024"/, 'viewBox="-100 -100 1224 1224"');
const logoLight = read(OUT_SVG, 'arenile-logo-horizontal.svg');
const ogImage = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#FFF0D1"/><rect y="560" width="1200" height="70" fill="${NAVY}"/>${logoLight.replace(/ width="\d+" height="\d+"/, '').replace('<svg ', '<svg x="225" y="166" width="750" height="238" ')}</svg>`;
const symbolH = (w) => Math.round((w * sh) / sw);

const jobs = [
  // dir, name, svg, width, height
  // The kit's own PNG exports (same names as supplied).
  ...[16, 32, 64, 128, 180, 192, 256, 512, 1024].map((s) => [A, `arenile-app-icon-${s}.png`, appIcon, s]),
  [A, 'arenile-app-icon-light-1024.png', read(A, 'arenile-app-icon-light.svg'), 1024],
  [A, 'arenile-symbol-color-1024.png', read(A, 'arenile-symbol-color.svg'), 1024, symbolH(1024)],
  [A, 'arenile-symbol-color-dark-1024.png', symbolDark, 1024, symbolH(1024)],
  // Derived exports used by the apps.
  [OUT_PNG, 'arenile-app-icon-macos-1024.png', macIcon, 1024],
  [OUT_PNG, 'apple-touch-icon-180.png', tile(0.74), 180],
  [OUT_PNG, 'maskable-icon-512.png', tile(0.6), 512],
  [OUT_PNG, 'pwa-icon-512.png', appFlat, 512],
  [OUT_PNG, 'pwa-icon-192.png', appFlat, 192],
  [OUT_PNG, 'favicon-16.png', favicon, 16],
  [OUT_PNG, 'favicon-32.png', favicon, 32],
  [OUT_PNG, 'favicon-48.png', favicon, 48],
  [OUT_PNG, 'email-mark-64.png', appFlat, 64],
  [OUT_PNG, 'tray-template-18.png', templateSymbol, 18],
  [OUT_PNG, 'tray-template-36.png', templateSymbol, 36],
  // Inset by 1.5% so the antialiased rounded corners stay fully transparent at 16 px.
  [OUT_PNG, 'tray-color-16.png', favicon.replace('viewBox="0 0 128 128"', 'viewBox="-2 -2 132 132"'), 16],
  [OUT_PNG, 'tray-color-32.png', appFlat, 32],
  [OUT_PNG, 'tray-color-64.png', appFlat, 64],
  [OUT_PNG, 'og-image-1200x630.png', ogImage, 1200, 630],
  [OUT_PNG, 'arenile-logo-horizontal-640.png', logoLight, 640, 203],
];

const browser = await chromium.launch();
const page = await browser.newPage();
for (const [dir, file, svg, w, h = w] of jobs) {
  await page.setViewportSize({ width: w, height: h });
  const src = `data:image/svg+xml;base64,${Buffer.from(svg).toString('base64')}`;
  await page.setContent(
    `<html><body style="margin:0;background:transparent"><img src="${src}" width="${w}" height="${h}" style="display:block"></body></html>`,
  );
  await page.waitForFunction(() => document.images[0]?.complete);
  writeFileSync(join(dir, file), await page.screenshot({ omitBackground: true, type: 'png' }));
}
await browser.close();
console.log(`build-brand: ${jobs.length} PNGs, outlined SVGs in assets/brand/arenile/outlined`);
