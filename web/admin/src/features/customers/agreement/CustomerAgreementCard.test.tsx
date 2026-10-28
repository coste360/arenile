import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf, CUSTOMER, estPath } from '../../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../../configuration/testing';
import type { Customer } from '../api';
import { toCustomerInput, customerValues } from '../customer.schema';
import { withAgreement } from './model';

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const customer = {
  ...CUSTOMER,
  agreement_id: 'ag1',
  category_ids: ['cc1'],
} as unknown as Customer;

const CATEGORIES = [
  {
    id: 'cc1',
    code: 'residenti',
    name: 'Residenti',
    description: '',
    requirement: 'Documento di residenza',
    register_category: 'residents',
    active: true,
    ...ts,
  },
  {
    id: 'cc2',
    code: 'over65',
    name: 'Over 65',
    description: '',
    requirement: '',
    register_category: 'protected',
    active: true,
    ...ts,
  },
];

describe('customer agreement and categories', () => {
  it('keeps categories on a full replacement and replaces agreement and categories on demand', () => {
    expect(toCustomerInput(customerValues(customer), customer)).toMatchObject({
      agreement_id: 'ag1',
      category_ids: ['cc1'],
    });
    const input = withAgreement(customer, '', ['cc2']);
    expect(input).not.toHaveProperty('agreement_id');
    expect(input).toMatchObject({ last_name: 'Rossi', category_ids: ['cc2'], tags: ['vip'] });
  });

  it('edits the default agreement and categories and records a requirement check', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      [`GET ${estPath(`/customers/${customer.id}`)}`]: () =>
        json(200, {
          customer,
          stats: { bookings_count: 0, total_spent: 0, balance_due: 0 },
          credit_balance: 0,
          credits: [],
          recent_bookings: [],
          season_passes: [],
        }),
      [`GET ${estPath('/agreements')}`]: () =>
        list([
          {
            id: 'ag1',
            name: 'Residenti 2026',
            code: 'RES',
            partner_name: '',
            notes: '',
            action: { type: 'percent_off', percent: '20' },
            conditions: {},
            stackable: false,
            active: true,
            ...ts,
          },
        ]),
      [`GET ${estPath('/customer-categories')}`]: () => list(CATEGORIES),
      [`GET ${estPath(`/customers/${customer.id}/requirement-verifications`)}`]: () => list([]),
      [`POST ${estPath(`/customers/${customer.id}/requirement-verifications`)}`]: () =>
        json(201, {
          id: 'rv1',
          subject: 'customer',
          requirement: 'Documento di residenza',
          verified_at: '2026-07-01T10:00:00Z',
        }),
      [`PUT ${estPath(`/customers/${customer.id}`)}`]: () => json(200, customer),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/clienti/${customer.id}`, routes);
    const card = (await screen.findByText('Convenzione e categorie')).closest(
      '[data-slot="card"]',
    ) as HTMLElement;
    expect(await within(card).findByText('Residenti 2026')).toBeInTheDocument();
    expect(within(card).getByText('Residenti')).toBeInTheDocument();
    expect(within(card).getByText('Nessuna verifica')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(within(card).getByRole('button', { name: 'Registra verifica' }));
    const verify = await screen.findByRole('dialog', { name: 'Verifica del requisito' });
    await user.type(within(verify).getByLabelText('Nota'), 'tessera 2027');
    await user.click(within(verify).getByRole('button', { name: 'Segna verificato' }));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'POST', estPath(`/customers/${customer.id}/requirement-verifications`)),
      ).toHaveLength(1),
    );
    const [post] = callsTo(
      fetchMock,
      'POST',
      estPath(`/customers/${customer.id}/requirement-verifications`),
    );
    expect(await bodyOf(post!)).toEqual({ customer_category_id: 'cc1', note: 'tessera 2027' });

    await user.click(within(card).getByRole('button', { name: 'Modifica' }));
    const edit = await screen.findByRole('dialog', { name: 'Convenzione e categorie' });
    await user.click(within(edit).getByRole('combobox', { name: 'Convenzione predefinita' }));
    await user.click(await screen.findByRole('option', { name: 'Nessuna convenzione' }));
    await user.click(within(edit).getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', estPath(`/customers/${customer.id}`))).toHaveLength(1),
    );
    const [put] = callsTo(fetchMock, 'PUT', estPath(`/customers/${customer.id}`));
    expect(put!.headers.get('If-Match')).toBe('"2"');
    const body = (await bodyOf(put!)) as Record<string, unknown>;
    expect(body).toMatchObject({ last_name: 'Rossi', category_ids: ['cc1'] });
    expect(body).not.toHaveProperty('agreement_id');
  });
});
