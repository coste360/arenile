#!/usr/bin/env node
// i18n completeness check (run by `npm run lint`, hence by `make lint` / `make check`).
// Italian is the source language (spec 20); every other locale must mirror it.
// For every locale set (a directory holding it.json plus en/de/fr.json under
// admin/, booking/ and shared/, and the email catalogs and templates in
// internal/domain/notify) it fails on:
//   1. missing or extra keys, or a value whose type (string/object) differs from Italian;
//   2. plural forms (i18next JSON v4 suffixes `_one`, `_other`, ...) that the target
//      language needs but lacks, or that its CLDR plural rules do not have;
//   3. empty translations of non-empty Italian strings;
//   4. interpolation errors: a `{{variable}}` of the Italian text missing from the
//      translation (unused variable) or a variable the Italian text does not have
//      (`count` may be omitted in `_one`/`_zero` forms, as in "un giorno");
//   5. syntax errors: unbalanced `{{`/`}}`, ICU message syntax (`{n, plural, ...}`,
//      which i18next does not parse), literal tokens such as `{number}` or `$t(...)`
//      nesting and `<0>`/`<strong>` Trans tags that differ from Italian;
//   6. informal register in German and French (Sie / vous everywhere, spec 20).
// Translations identical to Italian are reported as warnings only (brand names,
// "Extra", "Gazebo"... are legitimately the same); pass --warnings to list them.
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

export const WEB = join(dirname(fileURLToPath(import.meta.url)), '..');
export const REPO = join(WEB, '..');
export const SOURCE = 'it';
export const TARGETS = ['en', 'de', 'fr'];

const SCAN = ['admin/src', 'booking/src', 'shared/src'];
const SKIP_DIRS = new Set(['node_modules', 'dist', 'dev-dist', 'coverage', 'generated']);
const NOTIFY = join(REPO, 'internal', 'domain', 'notify');

const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;
const VAR = /\{\{\s*(-?\s*[^{}\s,]+)\s*(?:,[^{}]*)?\}\}/g;
const ICU = /\{\s*[\w.]+\s*,\s*(plural|select|selectordinal|number|date|time)\b/;
const LITERAL_TOKEN = /(?<!\{)\{([A-Za-z_][\w.]*)\}(?!\})/g;
const NESTING = /\$t\(([^)]*)\)/g;
const TRANS_TAG = /<\/?(\d+|strong|b|i|em|br|code|a|link|kbd)\s*\/?>/g;

/** Informal second-person forms that must not appear (formal register: Sie / vous). */
export const INFORMAL = {
  de: /(?<![\p{L}\w{])([Dd]u|[Dd]ich|[Dd]ir|[Dd]ein(?:e|em|en|er|es)?|[Ee]uch|[Ee]uer|[Ee]ure)(?![\p{L}\w}])/u,
  fr: /(?<![\p{L}\w{])([Tt]u|[Tt]oi|[Tt]on|[Tt]a|[Tt]es|[Tt]ien|[Tt]ienne)(?![\p{L}\w}])/u,
};

/** Flattens nested messages to dotted keys. */
export function flatten(obj, prefix = '', out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) flatten(v, key, out);
    else out[key] = v;
  }
  return out;
}

function collect(re, s, group = 1) {
  const out = [];
  for (const m of String(s).matchAll(re)) out.push(m[group].replace(/\s+/g, ''));
  return out.sort();
}

export const variables = (s) => [...new Set(collect(VAR, s))];
const literalTokens = (s) => collect(LITERAL_TOKEN, s);
const nesting = (s) => collect(NESTING, s);
const transTags = (s) => collect(TRANS_TAG, s, 0).map((t) => t.replace(/\s+/g, ''));

const sameList = (a, b) => a.length === b.length && a.every((x, i) => x === b[i]);

/** CLDR plural categories a locale can select for whole numbers in everyday ranges. */
export function pluralCategories(locale) {
  const rules = new Intl.PluralRules(locale);
  const required = new Set();
  for (let n = 0; n <= 1000; n++) required.add(rules.select(n));
  const allowed = new Set([...rules.resolvedOptions().pluralCategories, 'zero']);
  return { required, allowed };
}

/** Syntax problems of a single message, independent of the source. */
export function syntaxProblems(value) {
  const s = String(value);
  const problems = [];
  const opens = s.split('{{').length - 1;
  const closes = s.split('}}').length - 1;
  if (opens !== closes) problems.push('unbalanced {{ }}');
  if (ICU.test(s)) problems.push('ICU message syntax (i18next uses _one/_other keys)');
  return problems;
}

/**
 * Compares a target locale's messages with the Italian ones.
 * Returns a list of `{ key, message }` problems.
 */
export function checkMessages(sourceMessages, targetMessages, locale, { register = true } = {}) {
  const src = flatten(sourceMessages);
  const dst = flatten(targetMessages);
  const problems = [];
  const add = (key, message) => problems.push({ key, message });
  const { required, allowed } = pluralCategories(locale);

  // Plural groups present in the source: base -> { category: key }.
  const groups = new Map();
  for (const key of Object.keys(src)) {
    const m = key.match(PLURAL_SUFFIX);
    if (!m) continue;
    const base = key.slice(0, -m[0].length);
    if (!(`${base}_other` in src)) continue;
    if (!groups.has(base)) groups.set(base, {});
    groups.get(base)[m[1]] = key;
  }
  const groupOf = (key) => {
    const m = key.match(PLURAL_SUFFIX);
    if (!m) return null;
    const base = key.slice(0, -m[0].length);
    return groups.has(base) ? { base, category: m[1] } : null;
  };

  for (const base of groups.keys()) {
    for (const category of required) {
      if (!(`${base}_${category}` in dst))
        add(`${base}_${category}`, `missing plural form "${category}"`);
    }
  }

  // A key that is a string on one side and an object on the other.
  const srcKeys = Object.keys(src);
  const dstKeys = Object.keys(dst);
  const isPrefix = (keys, key) => keys.some((k) => k.startsWith(`${key}.`));
  const shape = new Set();
  for (const key of srcKeys) {
    if (key in dst) continue;
    const parts = key.split('.');
    for (let i = 1; i < parts.length; i++) {
      const prefix = parts.slice(0, i).join('.');
      if (prefix in dst) shape.add(prefix);
    }
    if (isPrefix(dstKeys, key)) shape.add(key);
  }
  for (const key of shape) {
    const srcIsObject = isPrefix(srcKeys, key);
    add(
      key,
      `type mismatch: Italian is ${srcIsObject ? 'object' : 'string'}, ${locale} is ${srcIsObject ? 'string' : 'object'}`,
    );
  }
  const underShape = (key) => [...shape].some((p) => key === p || key.startsWith(`${p}.`));

  for (const key of dstKeys) {
    const g = groupOf(key);
    if (underShape(key)) continue;
    if (!(key in src)) {
      if (g) {
        if (!allowed.has(g.category))
          add(key, `plural category "${g.category}" does not exist in ${locale}`);
        else if (!required.has(g.category) && g.category !== 'zero' && g.category !== 'many')
          add(key, `unexpected plural category "${g.category}"`);
        continue;
      }
      add(key, 'extra key (not in Italian)');
      continue;
    }
  }

  for (const [key, source] of Object.entries(src)) {
    const g = groupOf(key);
    if (underShape(key)) continue;
    if (!(key in dst)) {
      if (g && !required.has(g.category)) continue; // e.g. Italian "_many" absent in target
      if (g) continue; // reported above as a missing plural form
      add(key, 'missing translation');
      continue;
    }
    const target = dst[key];
    if (typeof source !== typeof target) {
      add(key, `type mismatch: Italian is ${typeof source}, ${locale} is ${typeof target}`);
      continue;
    }
    if (typeof source !== 'string') continue;
    if (source.trim() !== '' && target.trim() === '') add(key, 'empty translation');
    for (const p of syntaxProblems(target)) add(key, p);

    // Interpolation variables.
    const want = new Set(variables(source));
    let allowedVars = new Set(want);
    let optional = new Set();
    if (g) {
      allowedVars = new Set(Object.values(groups.get(g.base)).flatMap((k) => variables(src[k])));
      if (g.category === 'one' || g.category === 'zero') optional = new Set(['count']);
    }
    const got = new Set(variables(target));
    for (const v of want) {
      if (!got.has(v) && !optional.has(v)) add(key, `unused interpolation variable {{${v}}}`);
    }
    for (const v of got) {
      if (!allowedVars.has(v) && !(g && v === 'count'))
        add(key, `unknown interpolation variable {{${v}}}`);
    }
    if (g && g.category === 'other' && variables(source).includes('count') && !got.has('count'))
      add(key, 'plural "other" form must show {{count}}');

    if (!sameList(literalTokens(source), literalTokens(target)))
      add(
        key,
        `literal tokens differ: ${literalTokens(source).join(' ')} vs ${literalTokens(target).join(' ')}`,
      );
    if (!sameList(nesting(source), nesting(target)))
      add(key, 'nesting $t(...) differs from Italian');
    if (!sameList(transTags(source), transTags(target))) add(key, 'Trans tags differ from Italian');
    if (register && INFORMAL[locale]) {
      const m = target.replace(VAR, '').match(INFORMAL[locale]);
      if (m) add(key, `informal register "${m[0]}" (use ${locale === 'de' ? 'Sie' : 'vous'})`);
    }
  }
  return problems;
}

/** Messages identical to Italian (likely untranslated). */
export function untranslated(sourceMessages, targetMessages) {
  const src = flatten(sourceMessages);
  const dst = flatten(targetMessages);
  return Object.keys(src).filter(
    (k) =>
      typeof src[k] === 'string' &&
      dst[k] === src[k] &&
      /\p{L}{3,}.*\s.*\p{L}{3,}/u.test(src[k].replace(VAR, '')),
  );
}

function walk(dir, found) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path, found);
    else if (name === `${SOURCE}.json`) found.push(dir);
  }
}

/** Directories holding an it.json locale file under the web apps. */
export function discoverLocaleDirs(root = WEB) {
  const found = [];
  for (const s of SCAN) if (existsSync(join(root, s))) walk(join(root, s), found);
  return found.sort();
}

const readJSON = (path) => JSON.parse(readFileSync(path, 'utf8'));

/** Splits an email template into front matter fields and body. */
export function parseTemplate(text) {
  const m = text.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!m) return null;
  const fields = {};
  for (const line of m[1].split('\n')) {
    const i = line.indexOf(':');
    if (i > 0) fields[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return { fields, body: m[2] };
}

const TEMPLATE_TOKEN = /\{\{\s*(>?\s*[\w.]+)\s*\}\}/g;
const templateTokens = (s) => collect(TEMPLATE_TOKEN, s).map((t) => t.replace(/^>/, '> '));
const catalogVars = (s) => collect(/\{(\w+)\}/g, s);

/** Checks the email catalogs (i18n/<lang>.json) and templates (templates/<lang>/*.md). */
export function checkNotify(dir = NOTIFY) {
  const problems = [];
  if (!existsSync(dir)) return problems;
  const add = (file, key, message) => problems.push({ file, key, message });
  const rel = (p) => relative(REPO, p).split(sep).join('/');

  const catPath = (l) => join(dir, 'i18n', `${l}.json`);
  const source = readJSON(catPath(SOURCE));
  for (const l of TARGETS) {
    const file = rel(catPath(l));
    if (!existsSync(catPath(l))) {
      add(file, '', 'missing catalog');
      continue;
    }
    const target = readJSON(catPath(l));
    for (const k of Object.keys(source)) {
      if (!(k in target)) add(file, k, 'missing translation');
      else if (String(source[k]).trim() !== '' && String(target[k]).trim() === '')
        add(file, k, 'empty translation');
      else if (!sameList(catalogVars(source[k]), catalogVars(target[k])))
        add(
          file,
          k,
          `placeholders differ: ${catalogVars(source[k]).join(' ')} vs ${catalogVars(target[k]).join(' ')}`,
        );
      else if (INFORMAL[l] && INFORMAL[l].test(String(target[k]).replace(/\{\w+\}/g, '')))
        add(file, k, `informal register (use ${l === 'de' ? 'Sie' : 'vous'})`);
    }
    for (const k of Object.keys(target))
      if (!(k in source)) add(file, k, 'extra key (not in Italian)');
  }

  const tplDir = (l) => join(dir, 'templates', l);
  const sourceFiles = readdirSync(tplDir(SOURCE))
    .filter((f) => !f.startsWith('.'))
    .sort();
  for (const l of TARGETS) {
    const files = existsSync(tplDir(l))
      ? readdirSync(tplDir(l)).filter((f) => !f.startsWith('.'))
      : [];
    for (const f of files)
      if (!sourceFiles.includes(f))
        add(rel(join(tplDir(l), f)), '', 'extra template (not in Italian)');
    for (const f of sourceFiles) {
      const path = join(tplDir(l), f);
      const file = rel(path);
      if (!existsSync(path)) {
        add(file, '', 'missing template');
        continue;
      }
      const src = parseTemplate(readFileSync(join(tplDir(SOURCE), f), 'utf8'));
      const dst = parseTemplate(readFileSync(path, 'utf8'));
      if (!src || !dst) {
        if (src && !dst) add(file, '', 'invalid front matter');
        continue;
      }
      for (const k of Object.keys(src.fields)) {
        if (!(k in dst.fields)) add(file, k, 'missing front matter field');
        else if (!sameList(templateTokens(src.fields[k]), templateTokens(dst.fields[k])))
          add(file, k, 'variables differ from Italian');
      }
      for (const k of Object.keys(dst.fields))
        if (!(k in src.fields)) add(file, k, 'extra front matter field');
      if (!sameList(templateTokens(src.body), templateTokens(dst.body)))
        add(
          file,
          'body',
          `variables or partials differ: ${templateTokens(src.body).join(' ')} vs ${templateTokens(dst.body).join(' ')}`,
        );
      const text = [...Object.values(dst.fields), dst.body].join('\n').replace(TEMPLATE_TOKEN, '');
      if (INFORMAL[l]) {
        const m = text.match(INFORMAL[l]);
        if (m) add(file, '', `informal register "${m[0]}" (use ${l === 'de' ? 'Sie' : 'vous'})`);
      }
      for (const p of syntaxProblems(text)) add(file, '', p);
    }
  }
  return problems;
}

/** Runs every check; returns `{ problems, warnings }` with repo-relative file names. */
export function run({ web = WEB, notify = NOTIFY } = {}) {
  const problems = [];
  const warnings = [];
  const rel = (p) => relative(REPO, p).split(sep).join('/');
  for (const dir of discoverLocaleDirs(web)) {
    const source = readJSON(join(dir, `${SOURCE}.json`));
    for (const p of syntaxProblems(JSON.stringify(source)).filter((x) => x.startsWith('ICU')))
      problems.push({ file: rel(join(dir, `${SOURCE}.json`)), key: '', message: p });
    for (const l of TARGETS) {
      const path = join(dir, `${l}.json`);
      if (!existsSync(path)) {
        problems.push({ file: rel(path), key: '', message: 'missing locale file' });
        continue;
      }
      const target = readJSON(path);
      for (const p of checkMessages(source, target, l)) problems.push({ file: rel(path), ...p });
      for (const key of untranslated(source, target))
        warnings.push({ file: rel(path), key, message: 'same as Italian' });
    }
  }
  problems.push(...checkNotify(notify));
  return { problems, warnings };
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const { problems, warnings } = run();
  if (process.argv.includes('--warnings')) {
    for (const w of warnings) console.warn(`warning ${w.file}: ${w.key}: ${w.message}`);
  }
  for (const p of problems) console.error(`${p.file}: ${p.key ? `${p.key}: ` : ''}${p.message}`);
  if (problems.length > 0) {
    console.error(
      `\ni18n check: ${problems.length} problem(s). Italian is the source: fix the translations.`,
    );
    process.exit(1);
  }
  console.log(
    `i18n check: OK (${warnings.length} string(s) identical to Italian, --warnings to list).`,
  );
}
