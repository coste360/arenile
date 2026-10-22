import { ApiProblem, type Me } from '@arenile/shared/api';
import { describe, expect, it, vi } from 'vitest';
import { iconKindFor } from './componentIcons';
import { addDays, daysBetween, monthStarts, rangesOverlap } from './dates';
import { applyFieldErrors, normalizeFieldPath, problemToFormState } from './formErrors';
import { compactI18n, i18nFormValue, localize, orderedLocales } from './i18nText';
import { ownedOrganizations, permissionsFor } from './permissions';
import { embedSnippets, normalizeOrigin, slugify, splitLines, themeWarnings } from './publicPage';
import { ifMatch, isCalendarDate } from './validation';

describe('normalizeFieldPath', () => {
  it.each([
    ['ranges[0].from', 'ranges.0.from'],
    ['/ranges/0/from', 'ranges.0.from'],
    ['ranges.0.from', 'ranges.0.from'],
    ['name', 'name'],
  ])('%s → %s', (input, out) => expect(normalizeFieldPath(input)).toBe(out));
});

describe('applyFieldErrors / problemToFormState', () => {
  const problem = new ApiProblem({
    type: 'validation_failed',
    status: 400,
    title: 'x',
    errors: [
      { field: 'settings.pricing.half_day_factor', message: 'Troppo alto' },
      { field: 'ranges[1].to', message: 'Fuori stagione' },
      { field: 'unknown', message: 'Altro' },
    ],
  });

  it('sets matched fields as server errors and returns the rest', () => {
    const setError = vi.fn();
    const rest = applyFieldErrors(problem, setError, {
      formKeys: ['half_day_factor', 'ranges'],
      aliases: { 'settings.pricing.half_day_factor': 'half_day_factor' },
    });
    expect(setError).toHaveBeenCalledWith(
      'half_day_factor',
      { type: 'server', message: 'Troppo alto' },
      { shouldFocus: true },
    );
    expect(setError).toHaveBeenCalledWith(
      'ranges.1.to',
      { type: 'server', message: 'Fuori stagione' },
      { shouldFocus: false },
    );
    expect(rest).toEqual(['unknown: Altro']);
  });

  it('maps problem types to alerts', () => {
    const setError = vi.fn();
    const opts = { formKeys: [] };
    expect(
      problemToFormState(
        new ApiProblem({ type: 'version_conflict', status: 412, title: '' }),
        setError,
        opts,
      ),
    ).toEqual({ kind: 'conflict' });
    expect(
      problemToFormState(
        new ApiProblem({ type: 'forbidden', status: 403, title: '' }),
        setError,
        opts,
      ),
    ).toEqual({ kind: 'message', key: 'common.errors.forbidden' });
    expect(
      problemToFormState(
        new ApiProblem({ type: 'conflict', status: 409, title: '', detail: 'Periodi sovrapposti' }),
        setError,
        opts,
      ),
    ).toEqual({ kind: 'server', messages: ['Periodi sovrapposti'] });
    expect(problemToFormState(new Error('boom'), setError, opts)).toEqual({
      kind: 'message',
      key: 'common.errors.generic',
    });
  });
});

describe('permissionsFor (spec 16.1)', () => {
  const est = { id: 'e1', organization_id: 'o1' };
  const me = (memberships: Me['memberships']): Me => ({
    principal_type: 'user',
    memberships,
    totp_enrolment_required: false,
  });

  it('owner: configure and users', () => {
    const p = permissionsFor(
      me([
        {
          organization_id: 'o1',
          role: 'owner',
          permissions: ['establishment.configure', 'users.manage'],
        },
      ]),
      est,
    );
    expect(p).toEqual({ canConfigure: true, canManageUsers: true, canView: true });
  });

  it('manager scoped to the establishment: configure only', () => {
    const p = permissionsFor(
      me([
        {
          organization_id: 'o1',
          establishment_id: 'e1',
          role: 'manager',
          permissions: ['establishment.configure', 'bookings.write'],
        },
      ]),
      est,
    );
    expect(p).toEqual({ canConfigure: true, canManageUsers: false, canView: true });
  });

  it('cashier: view only', () => {
    const p = permissionsFor(
      me([
        { organization_id: 'o1', role: 'cashier', permissions: ['bookings.write', 'cash.operate'] },
      ]),
      est,
    );
    expect(p).toEqual({ canConfigure: false, canManageUsers: false, canView: true });
  });

  it('memberships of other establishments or organizations do not apply', () => {
    const p = permissionsFor(
      me([
        { organization_id: 'o2', role: 'owner', permissions: ['establishment.configure'] },
        {
          organization_id: 'o1',
          establishment_id: 'e2',
          role: 'manager',
          permissions: ['establishment.configure'],
        },
      ]),
      est,
    );
    expect(p.canView).toBe(false);
    expect(p.canConfigure).toBe(false);
  });

  it('API keys use their scopes on their establishment only', () => {
    const key: Me = {
      principal_type: 'api_key',
      api_key: {
        id: 'k',
        name: 'k',
        prefix: 'ar_',
        establishment_id: 'e1',
        scopes: ['establishment.configure'],
      },
      memberships: [],
      totp_enrolment_required: false,
    };
    expect(permissionsFor(key, est).canConfigure).toBe(true);
    expect(permissionsFor(key, { id: 'e9', organization_id: 'o1' }).canView).toBe(false);
  });

  it('ownedOrganizations lists distinct owner organizations', () => {
    expect(
      ownedOrganizations(
        me([
          { organization_id: 'o1', role: 'owner', permissions: [] },
          { organization_id: 'o1', establishment_id: 'e1', role: 'owner', permissions: [] },
          { organization_id: 'o2', role: 'manager', permissions: [] },
        ]),
      ),
    ).toEqual(['o1']);
  });
});

describe('i18n text', () => {
  it('falls back to the default locale', () => {
    expect(localize({ it: 'Prima fila', en: 'First row' }, 'en', 'it')).toBe('First row');
    expect(localize({ it: 'Prima fila', en: ' ' }, 'en', 'it')).toBe('Prima fila');
    expect(localize({ de: 'Reihe' }, 'en', 'it')).toBe('Reihe');
    expect(localize(undefined, 'en', 'it')).toBe('');
  });
  it('compacts and builds form values', () => {
    expect(compactI18n({ it: ' A ', en: '' })).toEqual({ it: 'A' });
    expect(i18nFormValue({ fr: 'x' }, ['it', 'en'])).toEqual({ it: '', en: '', fr: 'x' });
    expect(orderedLocales('en', ['it', 'en', 'de'])).toEqual(['en', 'it', 'de']);
  });
});

describe('public page helpers', () => {
  it('builds the spec 12.1 snippets', () => {
    const s = embedSnippets('https://prenota.example.it/', 'lido-x', 'it');
    expect(s.url).toBe('https://prenota.example.it/b/lido-x');
    expect(s.iframe).toContain('src="https://prenota.example.it/b/lido-x?embed=1"');
    expect(s.iframe).toContain('allow="payment" loading="lazy"');
    expect(s.script).toBe(
      '<div data-arenile="lido-x" data-lang="it"></div>\n<script src="https://prenota.example.it/embed.js" async></script>',
    );
  });

  it.each([
    ['https://www.lido.it', 'https://www.lido.it'],
    ['https://www.lido.it/', 'https://www.lido.it'],
    ['http://localhost:8080', 'http://localhost:8080'],
    ['https://www.lido.it/prenota', null],
    ['www.lido.it', null],
    ['https://user:pw@lido.it', null],
  ])('normalizeOrigin(%s)', (input, out) => expect(normalizeOrigin(input)).toBe(out));

  it('splits lines and slugifies', () => {
    expect(splitLines(' a \n\n b, c ')).toEqual(['a', 'b', 'c']);
    expect(slugify('Lido Sole & Mare – Sperlonga')).toBe('lido-sole-mare-sperlonga');
    expect(slugify('Bagni Città')).toBe('bagni-citta');
  });

  it('warns about low contrast themes', () => {
    expect(themeWarnings({ primary_color: '#0e6ba8', accent_color: '#b45309' })).toEqual([]);
    const keys = themeWarnings({ primary_color: '#ffff00', accent_color: '#ffff33' }).map(
      (w) => w.key,
    );
    expect(keys).toEqual(['primaryText', 'accentContrast', 'similarColors']);
  });
});

describe('dates and misc', () => {
  it('computes calendar dates', () => {
    expect(daysBetween('2027-05-01', '2027-09-30')).toBe(152);
    expect(addDays('2027-02-28', 1)).toBe('2027-03-01');
    expect(
      rangesOverlap(
        { from: '2027-06-01', to: '2027-06-30' },
        { from: '2027-06-30', to: '2027-07-10' },
      ),
    ).toBe(true);
    expect(
      rangesOverlap(
        { from: '2027-06-01', to: '2027-06-29' },
        { from: '2027-06-30', to: '2027-07-10' },
      ),
    ).toBe(false);
    expect(monthStarts('2027-05-15', '2027-08-01')).toEqual([
      '2027-06-01',
      '2027-07-01',
      '2027-08-01',
    ]);
    expect(isCalendarDate('2027-02-29')).toBe(false);
    expect(isCalendarDate('2028-02-29')).toBe(true);
    expect(ifMatch(5)).toBe('"5"');
  });

  it('picks component icons from codes', () => {
    expect(iconKindFor('umbrella')).toBe('umbrella');
    expect(iconKindFor('lettino')).toBe('sunbed');
    expect(iconKindFor('sdraio')).toBe('deckchair');
    expect(iconKindFor('frigo')).toBe('generic');
  });
});
