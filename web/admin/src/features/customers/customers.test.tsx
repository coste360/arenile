import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { EST_ID, json, problem } from '../configuration/testing';
import { bodyOf, CUSTOMER, estPath, renderWithApi } from '../bookings/testing';
import type { Customer } from './api';
import { AnonymizeDialog, CustomerFormDialog } from './CustomerDialogs';
import {
  anonymizeSchema,
  customerSchema,
  customerValues,
  parseTags,
  toCustomerInput,
} from './customer.schema';
import { duplicateTerm } from './lib';

const customer = CUSTOMER as unknown as Customer;

describe('customer schema', () => {
  it('parses tags (trimmed, lowercase, unique)', () => {
    expect(parseTags(' VIP, famiglia ,vip,, ')).toEqual(['vip', 'famiglia']);
  });

  it('validates email, phone and tag limits', () => {
    const base = customerValues();
    expect(customerSchema.safeParse({ ...base, last_name: 'Rossi' }).success).toBe(true);
    expect(customerSchema.safeParse({ ...base, last_name: '' }).success).toBe(false);
    expect(customerSchema.safeParse({ ...base, last_name: 'Rossi', email: 'nope' }).success).toBe(
      false,
    );
    expect(customerSchema.safeParse({ ...base, last_name: 'Rossi', phone: '12' }).success).toBe(
      false,
    );
    expect(
      customerSchema.safeParse({ ...base, last_name: 'Rossi', tags: 'x'.repeat(31) }).success,
    ).toBe(false);
  });

  it('keeps a recorded consent and stamps a new one with the desk source', () => {
    const now = new Date('2026-07-01T10:00:00Z');
    const given = { given_at: '2026-01-01T00:00:00Z', source: 'online', text_version: 'v3' };
    const prev = { ...customer, marketing_consent: given, agreement_id: 'ag1' };
    const v = { ...customerValues(prev), phone: '333 7654321' };
    expect(toCustomerInput(v, prev, now)).toMatchObject({
      phone: '+393337654321',
      marketing_consent: given,
      agreement_id: 'ag1',
      tags: ['vip'],
    });
    expect(
      toCustomerInput({ ...customerValues(), last_name: 'Neri', marketing: true }, undefined, now),
    ).toEqual({
      last_name: 'Neri',
      locale: 'it',
      marketing_consent: {
        given_at: '2026-07-01T10:00:00.000Z',
        source: 'desk',
        text_version: 'panel-1',
      },
    });
    expect(
      toCustomerInput({ ...v, marketing: false }, prev, now).marketing_consent,
    ).toBeUndefined();
  });

  it('confirms the anonymisation only with the exact surname', () => {
    const schema = anonymizeSchema('Rossi');
    expect(schema.safeParse({ confirmation: 'rossi ' }).success).toBe(true);
    expect(schema.safeParse({ confirmation: 'Ross' }).success).toBe(false);
  });

  it('searches duplicates by phone, then email, then surname', () => {
    expect(duplicateTerm({ phone: '+39 333 123 4567', email: 'a@b.it' })).toBe('3331234567');
    expect(duplicateTerm({ email: 'a@b.it', last_name: 'Rossi' })).toBe('a@b.it');
    expect(duplicateTerm({ last_name: 'Ro' })).toBe('');
  });
});

describe('AnonymizeDialog', () => {
  const ANON = estPath(`/customers/${customer.id}/anonymize`);

  function setup(
    handler = () => json(200, { ...customer, anonymized_at: '2026-07-01T10:00:00Z' }),
  ) {
    const onOpenChange = vi.fn();
    const utils = renderWithApi(
      <AnonymizeDialog
        establishmentId={EST_ID}
        customer={customer}
        open
        onOpenChange={onOpenChange}
      />,
      { [`POST ${ANON}`]: handler },
    );
    return { ...utils, onOpenChange, user: userEvent.setup() };
  }

  it('stays disabled until the surname is typed, then anonymises with If-Match', async () => {
    const { user, fetchMock, onOpenChange } = setup();
    expect(screen.getByText('Operazione irreversibile')).toBeInTheDocument();
    const confirm = screen.getByRole('button', { name: 'Anonimizza definitivamente' });
    expect(confirm).toBeDisabled();
    const input = screen.getByLabelText('Per confermare scrivi il cognome: Rossi');
    await user.type(input, 'Ross');
    expect(confirm).toBeDisabled();
    await user.type(input, 'i');
    expect(confirm).toBeEnabled();
    await user.click(confirm);
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const req = fetchMock.mock.calls.map((c) => c[0] as Request).find((r) => r.method === 'POST')!;
    expect(req.headers.get('If-Match')).toBe('"2"');
  });

  it('shows a stale version conflict', async () => {
    const { user } = setup(() => problem(412, 'version_conflict'));
    await user.type(screen.getByLabelText(/Per confermare/), 'Rossi');
    await user.click(screen.getByRole('button', { name: 'Anonimizza definitivamente' }));
    expect(await screen.findByText(/Un altro utente ha modificato/)).toBeInTheDocument();
  });
});

describe('CustomerFormDialog', () => {
  it('creates a customer with normalised contacts and an Idempotency-Key', async () => {
    const onSaved = vi.fn();
    const { fetchMock } = renderWithApi(
      <CustomerFormDialog establishmentId={EST_ID} open onOpenChange={vi.fn()} onSaved={onSaved} />,
      {
        [`POST ${estPath('/customers')}`]: () => json(201, { ...customer, id: 'new' }),
        [`GET ${estPath('/customers')}`]: () => json(200, { items: [] }),
      },
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Cognome'), 'Bianchi');
    await user.type(screen.getByLabelText(/Telefono/), '347 000 1111');
    await user.type(screen.getByLabelText(/^Tag/), 'Famiglia, vip');
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const req = fetchMock.mock.calls.map((c) => c[0] as Request).find((r) => r.method === 'POST')!;
    expect(req.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req)).toEqual({
      last_name: 'Bianchi',
      phone: '+393470001111',
      locale: 'it',
      tags: ['famiglia', 'vip'],
    });
  });
});
