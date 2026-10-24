import { describe, expect, it } from 'vitest';
import { customerSchema } from './schema';

const base = {
  first_name: 'Mario',
  last_name: 'Rossi',
  email: 'mario@example.it',
  phone: '',
  accept_terms: true,
  marketing_consent: false,
};

function messages(values: Record<string, unknown>, hasTerms = true) {
  const r = customerSchema({ hasTerms }).safeParse(values);
  return r.success ? [] : r.error.issues.map((i) => `${i.path.join('.')}:${i.message}`);
}

describe('customerSchema (public checkout, ADR 0020)', () => {
  it('accepts an empty phone: the phone is always optional', () => {
    expect(messages(base)).toEqual([]);
    expect(messages({ ...base, phone: '   ' })).toEqual([]);
  });

  it('validates the phone only when present', () => {
    expect(messages({ ...base, phone: '+39 333 123 4567' })).toEqual([]);
    expect(messages({ ...base, phone: '12' })).toEqual(['phone:who.validation.phone']);
  });

  it('keeps the email required for the confirmation and magic link', () => {
    expect(messages({ ...base, email: '' })).toContain('email:who.validation.required');
    expect(messages({ ...base, email: 'nope' })).toContain('email:who.validation.email');
  });

  it('requires name, surname and the terms when they exist', () => {
    expect(messages({ ...base, first_name: ' ', last_name: '' })).toEqual([
      'first_name:who.validation.required',
      'last_name:who.validation.required',
    ]);
    expect(messages({ ...base, accept_terms: false })).toEqual([
      'accept_terms:who.validation.terms',
    ]);
    expect(messages({ ...base, accept_terms: false }, false)).toEqual([]);
  });
});
