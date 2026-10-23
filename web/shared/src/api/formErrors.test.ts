import { describe, expect, it, vi } from 'vitest';
import { ApiProblem } from './client';
import { fieldErrorText, mapProblemToForm, normalizeFieldPath } from './formErrors';

const problem = (errors: { field: string; message: string }[]) =>
  new ApiProblem({ type: 'validation_failed', status: 422, title: 'x', errors });

describe('normalizeFieldPath', () => {
  it.each([
    ['lines[0].extras[1].quantity', 'lines.0.extras.1.quantity'],
    ['/ranges/0/from', 'ranges.0.from'],
    ['customer.email', 'customer.email'],
  ])('%s → %s', (input, out) => expect(normalizeFieldPath(input)).toBe(out));
});

describe('mapProblemToForm', () => {
  it('sets mapped paths on the form fields, focusing the first one', () => {
    const setError = vi.fn();
    const mapped = mapProblemToForm(
      problem([
        { field: 'customer.email', message: 'Email non valida.' },
        { field: 'settings.notes', message: 'Troppo lungo.' },
      ]),
      setError,
      { formKeys: ['customer', 'notes'], aliases: { 'settings.notes': 'notes' } },
    );
    expect(setError).toHaveBeenNthCalledWith(
      1,
      'customer.email',
      { type: 'server', message: 'Email non valida.' },
      { shouldFocus: true },
    );
    expect(setError).toHaveBeenNthCalledWith(
      2,
      'notes',
      { type: 'server', message: 'Troppo lungo.' },
      { shouldFocus: false },
    );
    expect(mapped.unmatched).toEqual([]);
    expect(mapped.errors.map((e) => e.field)).toEqual(['customer.email', 'notes']);
  });

  it('resolves positional paths through a custom resolver with a human label', () => {
    const setError = vi.fn();
    const mapped = mapProblemToForm(
      problem([{ field: 'lines[0].extras[0].quantity', message: 'Massimo 2.' }]),
      setError,
      {
        resolve: (path) =>
          path === 'lines.0.extras.0.quantity'
            ? { name: 'extras.x1', label: 'Lettino aggiuntivo' }
            : undefined,
      },
    );
    expect(setError).toHaveBeenCalledWith(
      'extras.x1',
      { type: 'server', message: 'Massimo 2.' },
      { shouldFocus: true },
    );
    expect(mapped.errors).toEqual([{ field: 'extras.x1', text: 'Lettino aggiuntivo: massimo 2' }]);
  });

  it('turns unknown paths into friendly text, never the raw path', () => {
    const mapped = mapProblemToForm(
      problem([
        { field: 'lines[3].hold_id', message: 'Blocco scaduto.' },
        { field: 'promo_code', message: 'Codice non valido.' },
      ]),
      vi.fn(),
      { formKeys: [], label: (p) => (p === 'promo_code' ? 'Codice promozionale' : undefined) },
    );
    expect(mapped.unmatched).toEqual(['Blocco scaduto.', 'Codice promozionale: codice non valido']);
    expect(mapped.unmatched.join(' ')).not.toMatch(/\[|lines|hold_id/);
  });

  it('lists a repeated error once (per-spot extras repeat it on every line)', () => {
    const setError = vi.fn();
    const mapped = mapProblemToForm(
      problem([
        { field: 'lines[0].extras[0].quantity', message: 'Massimo 2.' },
        { field: 'lines[1].extras[0].quantity', message: 'Massimo 2.' },
      ]),
      setError,
      { resolve: () => ({ name: 'extras.x1', label: 'Lettino aggiuntivo' }) },
    );
    expect(setError).toHaveBeenCalledTimes(1);
    expect(mapped.errors).toHaveLength(1);
  });

  it('returns nothing for errors that are not problems', () => {
    expect(mapProblemToForm(new Error('boom'), vi.fn())).toEqual({ errors: [], unmatched: [] });
  });

  it('formats label and message', () => {
    expect(fieldErrorText('Telo mare', 'Massimo 4.')).toBe('Telo mare: massimo 4');
    expect(fieldErrorText(undefined, 'Massimo 4.')).toBe('Massimo 4.');
    expect(fieldErrorText('Conto', 'IBAN non valido.')).toBe('Conto: IBAN non valido');
  });
});
