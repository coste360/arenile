#!/usr/bin/env node
// Derives the outlined SVGs and the raster exports of the brand from the kit
// sources in assets/brand (ADR 0018). The outputs are committed, so neither
// the build nor the CI needs this script or its tools; run it only when the
// kit changes:
//
//   mkdir -p /tmp/brandtools && cd /tmp/brandtools && npm init -y >/dev/null &&
//     npm i @fontsource/titillium-web@5.3.0 opentype.js@2.0.0
//   ARENILE_BRAND_TOOLS=/tmp/brandtools/node_modules node scripts/brand/build-brand.mjs
//
// Text is converted to outlines with Titillium Web (OFL-1.1, the kit font) so
// the logos render the same where the font is not available (<img>, emails,
// desktop icons, favicons, social images). PNGs are rendered by the Chromium
// of the e2e Playwright install (e2e/node_modules), which handles the kit
// gradients faithfully.
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
const C = join(root, 'assets', 'brand', 'coste360');
const OUT_SVG = join(A, 'outlined');
const OUT_PNG = join(A, 'png');
mkdirSync(OUT_SVG, { recursive: true });
mkdirSync(OUT_PNG, { recursive: true });

const fontFile = (w) =>
  readFileSync(join(tools, '@fontsource', 'titillium-web', 'files', `titillium-web-latin-${w}-normal.woff`));
const fonts = Object.fromEntries(
  [600, 700, 900].map((w) => {
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

const read = (dir, f) => readFileSync(join(dir, f), 'utf8');
const write = (f, s) => writeFileSync(join(OUT_SVG, f), s.trim() + '\n');

// 1. The kit logos with outlined text.
for (const f of [
  'arenile-logo-horizontal.svg',
  'arenile-logo-horizontal-dark.svg',
  'arenile-logo-header.svg',
  'arenile-logo-stacked.svg',
  'arenile-wordmark.svg',
]) {
  write(f, outline(read(A, f)));
}
// Header logo for dark surfaces (colours of the kit's horizontal-dark logo).
write(
  'arenile-logo-header-dark.svg',
  outline(
    read(A, 'arenile-logo-header.svg')
      .replace('fill="#0B305F"', 'fill="#FFFFFF"')
      .replace('fill="#277D9F"', 'fill="#9EE2EE"')
      .replace('— Header', '— Header per sfondo scuro'),
  ),
);

// 2. The name alone ("Arenile", Titillium Web 700), for the app chrome next to the symbol.
const name = textPath({ text: 'Arenile', x: 0, y: 0, size: 100, weight: 700, spacing: -1.7 });
const nameBox = fonts[700].getPath('Arenile', 0, 0, 100).getBoundingBox();
for (const [variant, fill] of [
  ['navy', '#0B305F'],
  ['white', '#FFFFFF'],
]) {
  const w = Math.ceil(name.width + 2);
  const h = Math.ceil(-nameBox.y1 + 2);
  write(
    `arenile-name-${variant}.svg`,
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" role="img" aria-label="Arenile"><title>Arenile</title><path fill="${fill}" transform="translate(1 ${(h - 1).toFixed(2)})" d="${name.d}"/></svg>`,
  );
}

// 3. Coste360 lockup: the company's original raster logo (assets/brand/coste360/logo-*.png, 781x312, with transparent margins)
// is used as is; it is never redrawn here.

// 4. Raster exports.
const symbolInner = read(A, 'arenile-symbol-color.svg')
  .replace(/^[\s\S]*?<defs>/, '<defs>')
  .replace(/<\/svg>\s*$/, '');
const tile = (rx, scale = 1) =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024"><rect width="1024" height="1024" rx="${rx}" fill="#092D62"/><g transform="translate(${512 - 512 * scale} ${512 - 512 * scale}) scale(${scale})">${symbolInner}</g></svg>`;
const navySymbol = read(A, 'arenile-symbol-navy.svg');
// macOS menu bar template image: the mono symbol in black, cropped to its
// bounds and with a heavier wave so it stays legible at 18 px.
const templateSymbol = navySymbol
  .replace('color:#082F63', 'color:#000000')
  .replace('viewBox="0 0 1024 1024"', 'viewBox="80 90 880 880"')
  .replace('stroke-width="27"', 'stroke-width="58"');
const appIcon = read(A, 'arenile-app-icon.svg');
const macIcon = appIcon.replace(
  /<svg([^>]*)viewBox="0 0 1024 1024"([^>]*)>/,
  '<svg$1viewBox="-100 -100 1224 1224"$2>',
);
const ogImage = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#FFF0D1"/><rect y="560" width="1200" height="70" fill="#092D62"/>${read(OUT_SVG, 'arenile-logo-horizontal.svg').replace('<svg ', '<svg x="240" y="168" width="720" height="224" ')}</svg>`;

const jobs = [
  // name, svg, width, height
  ['arenile-app-icon-macos-1024.png', macIcon, 1024],
  ['apple-touch-icon-180.png', tile(0, 0.86), 180],
  ['maskable-icon-512.png', tile(0, 0.7), 512],
  ['pwa-icon-512.png', read(A, 'arenile-app-icon-flat.svg'), 512],
  ['pwa-icon-192.png', read(A, 'arenile-app-icon-flat.svg'), 192],
  ['favicon-32.png', read(A, 'arenile-favicon.svg'), 32],
  ['favicon-16.png', read(A, 'arenile-favicon.svg'), 16],
  ['favicon-48.png', read(A, 'arenile-favicon.svg'), 48],
  ['email-mark-64.png', read(A, 'arenile-app-icon-flat.svg'), 64],
  ['tray-template-18.png', templateSymbol, 18],
  ['tray-template-36.png', templateSymbol, 36],
  ['tray-color-32.png', read(A, 'arenile-app-icon-flat.svg'), 32],
  ['tray-color-64.png', read(A, 'arenile-app-icon-flat.svg'), 64],
  ['og-image-1200x630.png', ogImage, 1200, 630],
  ['arenile-logo-horizontal-640.png', read(OUT_SVG, 'arenile-logo-horizontal.svg'), 640, 199],
];

const browser = await chromium.launch();
const page = await browser.newPage();
for (const [file, svg, w, h = w] of jobs) {
  await page.setViewportSize({ width: w, height: h });
  const src = `data:image/svg+xml;base64,${Buffer.from(svg).toString('base64')}`;
  await page.setContent(
    `<html><body style="margin:0;background:transparent"><img src="${src}" width="${w}" height="${h}" style="display:block"></body></html>`,
  );
  await page.waitForFunction(() => document.images[0]?.complete);
  writeFileSync(join(OUT_PNG, file), await page.screenshot({ omitBackground: true, type: 'png' }));
}
await browser.close();
console.log(`build-brand: ${jobs.length} PNGs in assets/brand/arenile/png, outlined SVGs in assets/brand/arenile/outlined`);
