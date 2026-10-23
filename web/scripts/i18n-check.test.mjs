import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  checkMessages,
  checkNotify,
  discoverLocaleDirs,
  flatten,
  parseTemplate,
  pluralCategories,
  run,
  untranslated,
  variables,
} from './i18n-check.mjs';

const messages = (problems) => problems.map((p) => `${p.key}: ${p.message}`);

describe('helpers', () => {
  it('flattens nested messages to dotted keys', () => {
    expect(flatten({ a: { b: 'x', c: { d: 'y' } }, e: 'z' })).toEqual({
      'a.b': 'x',
      'a.c.d': 'y',
      e: 'z',
    });
  });

  it('extracts interpolation variables, ignoring formats and spaces', () => {
    expect(variables('Pay {{amount}} by {{ date, short }} ({{amount}})')).toEqual([
      'amount',
      'date',
    ]);
  });

  it('knows the CLDR plural categories of the shipped languages', () => {
    for (const locale of ['it', 'en', 'de', 'fr']) {
      const { required } = pluralCategories(locale);
      expect([...required].sort()).toEqual(['one', 'other']);
    }
    expect(pluralCategories('de').allowed.has('few')).toBe(false);
  });

  it('splits an email template into front matter and body', () => {
    expect(
      parseTemplate('---\nsubject: Hi {{a}}\npreheader: x\n---\n# T\n\n{{> summary}}\n'),
    ).toEqual({
      fields: { subject: 'Hi {{a}}', preheader: 'x' },
      body: '# T\n\n{{> summary}}\n',
    });
    expect(parseTemplate('no front matter')).toBeNull();
  });
});

describe('checkMessages', () => {
  const it_ = {
    title: 'Prenotazioni',
    pay: 'Paga {{amount}}',
    days_one: 'Un giorno',
    days_other: '{{count}} giorni',
    label: 'Formato ({row}{number})',
    rich: 'Leggi <0>le condizioni</0>',
    group: { a: 'A' },
  };
  const ok = {
    title: 'Bookings',
    pay: 'Pay {{amount}}',
    days_one: 'One day',
    days_other: '{{count}} days',
    label: 'Format ({row}{number})',
    rich: 'Read <0>the conditions</0>',
    group: { a: 'A' },
  };

  it('accepts a complete, consistent translation', () => {
    expect(checkMessages(it_, ok, 'en')).toEqual([]);
  });

  it.each([
    ['missing key', { ...ok, title: undefined }, 'title: missing translation'],
    ['extra key', { ...ok, extra: 'x' }, 'extra: extra key (not in Italian)'],
    ['empty value', { ...ok, title: ' ' }, 'title: empty translation'],
    [
      'type mismatch',
      { ...ok, group: 'A' },
      'group: type mismatch: Italian is object, en is string',
    ],
    ['unused variable', { ...ok, pay: 'Pay now' }, 'pay: unused interpolation variable {{amount}}'],
    [
      'unknown variable',
      { ...ok, pay: 'Pay {{amount}} {{total}}' },
      'pay: unknown interpolation variable {{total}}',
    ],
    ['unbalanced braces', { ...ok, pay: 'Pay {{amount}' }, 'pay: unbalanced {{ }}'],
    [
      'ICU syntax',
      { ...ok, days_other: '{count, plural, other {# days}} {{count}}' },
      'days_other: ICU message syntax (i18next uses _one/_other keys)',
    ],
    [
      'plural other without count',
      { ...ok, days_other: 'Several days' },
      'days_other: unused interpolation variable {{count}}',
    ],
    ['missing plural form', { ...ok, days_one: undefined }, 'days_one: missing plural form "one"'],
    [
      'literal token translated',
      { ...ok, label: 'Format ({riga}{number})' },
      'label: literal tokens differ: number row vs number riga',
    ],
    [
      'Trans tag dropped',
      { ...ok, rich: 'Read the conditions' },
      'rich: Trans tags differ from Italian',
    ],
  ])('reports %s', (_name, target, expected) => {
    const clean = JSON.parse(JSON.stringify(target));
    expect(messages(checkMessages(it_, clean, 'en'))).toContain(expected);
  });

  it('lets the "one" form omit {{count}}', () => {
    expect(
      checkMessages(
        { n_one: '{{count}} giorno', n_other: '{{count}} giorni' },
        { n_one: 'Ein Tag', n_other: '{{count}} Tage' },
        'de',
      ),
    ).toEqual([]);
  });

  it('rejects plural categories the language does not have', () => {
    expect(
      messages(
        checkMessages(
          { n_one: 'x', n_other: '{{count}}' },
          { n_one: 'x', n_few: 'y', n_other: '{{count}}' },
          'de',
        ),
      ),
    ).toContain('n_few: plural category "few" does not exist in de');
  });

  it.each([
    ['de', 'Wähle deinen Platz', 'informal register "deinen" (use Sie)'],
    ['de', 'Du hast gewählt', 'informal register "Du" (use Sie)'],
    ['fr', 'Choisis ta place', 'informal register "ta" (use vous)'],
    ['fr', 'Tu as choisi', 'informal register "Tu" (use vous)'],
  ])('rejects the informal register in %s: %s', (locale, text, expected) => {
    expect(messages(checkMessages({ a: 'Scegli' }, { a: text }, locale))).toContain(
      `a: ${expected}`,
    );
  });

  it('does not mistake variables, env names or similar words for informal pronouns', () => {
    expect(checkMessages({ a: '{{dir}}' }, { a: 'Ordner: {{dir}}' }, 'de')).toEqual([]);
    expect(
      checkMessages({ a: 'ARENILE_MAIL_DIR' }, { a: 'Ordner ARENILE_MAIL_DIR' }, 'de'),
    ).toEqual([]);
    expect(checkMessages({ a: 'Tasse' }, { a: 'Taste Tab, Tablet, Ton-Datei' }, 'de')).toEqual([]);
    expect(checkMessages({ a: 'Tavolo' }, { a: 'Table, tarif, tester' }, 'fr')).toEqual([]);
  });
});

describe('untranslated', () => {
  it('lists multi-word strings identical to Italian', () => {
    expect(
      untranslated(
        { a: 'Stripe Checkout', b: 'Extra', c: 'Ciao' },
        { a: 'Stripe Checkout', b: 'Extra', c: 'Hi' },
      ),
    ).toEqual(['a']);
  });
});

describe('repository locales', () => {
  it('discovers every locale set of the web apps', () => {
    const dirs = discoverLocaleDirs().map((d) => d.replace(/\\/g, '/'));
    expect(dirs.some((d) => d.endsWith('shared/src/i18n/locales'))).toBe(true);
    expect(dirs.some((d) => d.endsWith('booking/src/i18n'))).toBe(true);
    expect(dirs.some((d) => d.endsWith('admin/src/features/operations/locales'))).toBe(true);
  });

  it('are complete and consistent with Italian', () => {
    expect(run().problems).toEqual([]);
  });
});

describe('checkNotify', () => {
  function fixture({
    catalogDe = { label: 'Code {code}' },
    templateDe = '---\nsubject: Buchung {{booking.code}}\n---\nHallo {{customer.first_name}}\n\n{{> summary}}\n',
  } = {}) {
    const dir = mkdtempSync(join(tmpdir(), 'notify-'));
    mkdirSync(join(dir, 'i18n'));
    for (const [l, cat] of Object.entries({
      it: { label: 'Codice {code}' },
      en: { label: 'Code {code}' },
      de: catalogDe,
      fr: { label: 'Code {code}' },
    })) {
      writeFileSync(join(dir, 'i18n', `${l}.json`), JSON.stringify(cat));
      mkdirSync(join(dir, 'templates', l), { recursive: true });
    }
    const it =
      '---\nsubject: Prenotazione {{booking.code}}\n---\nCiao {{customer.first_name}}\n\n{{> summary}}\n';
    writeFileSync(join(dir, 'templates', 'it', 'x.md'), it);
    writeFileSync(
      join(dir, 'templates', 'en', 'x.md'),
      it.replace('Prenotazione', 'Booking').replace('Ciao', 'Hi'),
    );
    writeFileSync(
      join(dir, 'templates', 'fr', 'x.md'),
      it.replace('Prenotazione', 'Réservation').replace('Ciao', 'Bonjour'),
    );
    if (templateDe !== null) writeFileSync(join(dir, 'templates', 'de', 'x.md'), templateDe);
    return dir;
  }
  const msgs = (dir) => checkNotify(dir).map((p) => `${p.key}: ${p.message}`);

  it('accepts aligned catalogs and templates', () => {
    expect(checkNotify(fixture())).toEqual([]);
  });

  it('reports catalog placeholders that differ', () => {
    expect(msgs(fixture({ catalogDe: { label: 'Code {kode}' } }))).toContain(
      'label: placeholders differ: code vs kode',
    );
  });

  it('reports a missing template and a dropped partial', () => {
    expect(msgs(fixture({ templateDe: null }))).toContain(': missing template');
    expect(
      msgs(
        fixture({
          templateDe:
            '---\nsubject: Buchung {{booking.code}}\n---\nHallo {{customer.first_name}}\n',
        }),
      ),
    ).toContain(
      'body: variables or partials differ: > summary customer.first_name vs customer.first_name',
    );
  });

  it('reports the informal register in a template', () => {
    expect(
      msgs(
        fixture({
          templateDe:
            '---\nsubject: Deine Buchung {{booking.code}}\n---\nHallo {{customer.first_name}}\n\n{{> summary}}\n',
        }),
      ),
    ).toContain(': informal register "Deine" (use Sie)');
  });
});
