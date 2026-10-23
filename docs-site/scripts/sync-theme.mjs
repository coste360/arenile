#!/usr/bin/env node
// Derives the documentation theme from the ONE stylesheet of the frontends,
// web/shared/src/styles/globals.css (ADR 0009), so the docs and the shadcn
// islands use exactly the same tokens (colours, radius, font, map palette,
// beach map kit) without copying them.
//
// The shared file is written for a standalone SPA; inside Starlight four
// things must change, and only those:
//   1. `@import 'tailwindcss'` goes: the docs entry (src/styles/global.css)
//      already imports Tailwind theme + utilities in Starlight's layer order,
//      and Tailwind's full preflight would reset Starlight's own styles;
//   2. bare package imports (tw-animate-css, the Inter font) are rewritten
//      to the copies installed for web/ (resolved like Node would);
//   3. dark mode follows Starlight (`<html data-theme="dark">`) instead of
//      the `.dark` class: the token block is re-targeted and the shared
//      `@custom-variant dark` is dropped (Starlight Tailwind declares the
//      data-theme one);
//   4. the global `@layer base` block (body background, border colour of
//      every element) is dropped: src/styles/global.css applies the same
//      rules scoped to the islands (`.arenile-island`).
// Any other shape of the source fails loudly, so a change to globals.css can
// never silently produce a broken docs theme.
//
// Output: src/styles/generated/shared-theme.css (git-ignored, rebuilt by
// `predev` / `prebuild`).
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const docsRoot = join(here, '..');
const sharedPkg = join(docsRoot, '..', 'web', 'shared', 'package.json');
const sharedSrc = join(docsRoot, '..', 'web', 'shared', 'src');
const source = join(sharedSrc, 'styles', 'globals.css');
const outDir = join(docsRoot, 'src', 'styles', 'generated');
const out = join(outDir, 'shared-theme.css');

/** Relative CSS path from the generated file, always with forward slashes. */
const rel = (target) => {
  const r = relative(outDir, target).split(sep).join('/');
  return r.startsWith('.') ? r : `./${r}`;
};

/** Resolves the stylesheet of a package installed for web/ (exports "style" or "default"). */
function resolveStyle(specifier) {
  // Walk the node_modules folders Node would search from web/shared (the
  // package.json subpath is not always exported, so do not require.resolve it).
  const searchPaths = createRequire(sharedPkg).resolve.paths(specifier) ?? [];
  const pkgJson = searchPaths
    .map((dir) => join(dir, specifier, 'package.json'))
    .find((candidate) => existsSync(candidate));
  if (!pkgJson) throw new Error(`sync-theme: ${specifier} is not installed, run \`make setup\``);
  const pkg = JSON.parse(readFileSync(pkgJson, 'utf8'));
  const entry = pkg.exports?.['.'];
  const file =
    (typeof entry === 'object' && (entry.style ?? entry.default)) ||
    (typeof entry === 'string' && entry) ||
    pkg.style ||
    pkg.main;
  if (!file || !file.endsWith('.css')) {
    throw new Error(`sync-theme: cannot find the stylesheet of ${specifier}`);
  }
  return join(dirname(pkgJson), file);
}

/** Applies `fn` and fails unless it changed the text exactly `times` times. */
function rewrite(text, label, pattern, replacement, times = 1) {
  let count = 0;
  const next = text.replace(pattern, (...args) => {
    count += 1;
    return typeof replacement === 'function' ? replacement(...args) : replacement;
  });
  if (count !== times) {
    throw new Error(
      `sync-theme: expected ${times} match(es) for "${label}" in web/shared/src/styles/globals.css, found ${count}. ` +
        'The shared theme changed shape: update docs-site/scripts/sync-theme.mjs.',
    );
  }
  return next;
}

/** Removes the `@layer base { ... }` block (balanced braces). */
function dropBaseLayer(text) {
  const start = text.indexOf('@layer base {');
  if (start < 0 || text.indexOf('@layer base {', start + 1) >= 0) {
    throw new Error('sync-theme: expected exactly one "@layer base {" block in globals.css');
  }
  let depth = 0;
  for (let i = text.indexOf('{', start); i < text.length; i += 1) {
    if (text[i] === '{') depth += 1;
    if (text[i] === '}') {
      depth -= 1;
      if (depth === 0) return text.slice(0, start) + text.slice(i + 1);
    }
  }
  throw new Error('sync-theme: unbalanced braces in the @layer base block of globals.css');
}

let css = readFileSync(source, 'utf8');
css = rewrite(css, "@import 'tailwindcss'", /^@import ['"]tailwindcss['"];\s*$/m, '');
css = rewrite(
  css,
  'bare package @import',
  /^@import ['"]([^'"./][^'"]*)['"];\s*$/gm,
  (_m, spec) => `@import '${rel(resolveStyle(spec))}';`,
  2,
);
css = rewrite(css, '@source', /^@source ['"]\.\.\/['"];\s*$/m, `@source '${rel(sharedSrc)}';`);
css = rewrite(css, '@custom-variant dark', /^@custom-variant dark [^;]+;\s*$/m, '');
css = rewrite(css, '.dark token block', /^\.dark \{$/m, ":root[data-theme='dark'] {");
css = dropBaseLayer(css);

const banner = `/* GENERATED by docs-site/scripts/sync-theme.mjs from web/shared/src/styles/globals.css.
   Do not edit: change the shared theme and rebuild the docs. */\n`;
mkdirSync(outDir, { recursive: true });
writeFileSync(out, banner + css.trimEnd() + '\n');
console.log(`sync-theme: ${relative(docsRoot, out)} <- web/shared/src/styles/globals.css`);
