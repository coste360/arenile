import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { bodyOf, estPath } from '../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { parseNames, partnerPermissions } from './lib';
import {
  categorySchema,
  paymentSchema,
  partnerSchema,
  partnerValues,
  toCategoryInput,
  toPartnerInput,
  toPaymentInput,
} from './partner.schema';

const STAMP = {
  version: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const HOTEL = {
  id: 'pa000000-0000-4000-8000-000000000001',
  name: 'Hotel Miramare',
  kind: 'struttura_ricettiva',
  accommodation_type: 'hotel',
  municipality: 'Gaeta',
  contact_name: 'Anna Bianchi',
  email: 'info@miramare.it',
  phone: '+390771000000',
  vat_number: 'IT01234567890',
  notes: '',
  active: true,
  agreement_count: 1,
  balance: 12000,
  ...STAMP,
};

const ASSOC = {
  id: 'pa000000-0000-4000-8000-000000000002',
  name: 'Associazione Vela',
  kind: 'associazione',
  notes: '',
  active: false,
  balance: 0,
  ...STAMP,
};

const SEASON = {
  id: 'se1',
  year: 2026,
  opens_on: '2026-05-01',
  closes_on: '2026-09-30',
  status: 'active',
  ...STAMP,
};

function statement(overrides: Record<string, unknown> = {}) {
  return {
    partner: HOTEL,
    period: { from: '2026-05-01', to: '2026-09-30' },
    bookings: [
      {
        booking_id: 'b1',
        code: 'AR-0001',
        status: 'confirmed',
        customer_display_name: 'Rossi Mario',
        group_name: 'Famiglia Rossi',
        starts_on: '2026-07-14',
        ends_on: '2026-07-16',
        spot_days: 3,
        people: 4,
        total_amount: 9000,
        discount_amount: 1000,
        partner_amount: 9000,
      },
    ],
    payments: [
      {
        id: 'pp1',
        partner_id: HOTEL.id,
        amount: 5000,
        received_on: '2026-07-20',
        method: 'bank_transfer',
        reference: 'CRO 123',
        created_at: '2026-07-20T10:00:00Z',
      },
    ],
    totals: {
      bookings: 1,
      spot_days: 3,
      people: 4,
      total_amount: 9000,
      discount_amount: 1000,
      charged: 9000,
      paid: 5000,
      opening_balance: 8000,
      closing_balance: 12000,
    },
    ...overrides,
  };
}

describe('partner schemas', () => {
  it('omits empty optional fields, normalises the phone and keeps the type only for accommodation', () => {
    const v = partnerSchema.parse({
      ...partnerValues(),
      name: ' Hotel Miramare ',
      phone: '0771 000000',
      email: 'INFO@Miramare.it',
      vat_number: 'it01234567890',
      accommodation_type: 'hotel',
    });
    expect(toPartnerInput(v)).toEqual({
      name: 'Hotel Miramare',
      kind: 'struttura_ricettiva',
      accommodation_type: 'hotel',
      active: true,
      email: 'info@miramare.it',
      phone: '+390771000000',
      vat_number: 'IT01234567890',
    });
    expect(toPartnerInput({ ...v, kind: 'associazione' }).accommodation_type).toBeUndefined();
    expect(partnerSchema.safeParse({ ...partnerValues(), name: 'X', phone: '12' }).success).toBe(
      false,
    );
  });

  it('validates categories and partner payments (integer cents, positive)', () => {
    const c = categorySchema.parse({
      code: 'residenti',
      name: 'Residenti',
      description: '',
      requirement: 'Documento di residenza',
      register_category: 'residents',
      active: true,
    });
    expect(toCategoryInput(c)).toMatchObject({ code: 'residenti', register_category: 'residents' });
    expect(categorySchema.safeParse({ ...c, code: 'Res Identi' }).success).toBe(false);
    const p = { amount: 5000, received_on: '2026-07-20', method: 'cash', reference: '', note: '' };
    expect(toPaymentInput(paymentSchema.parse(p))).toEqual({
      amount: 5000,
      received_on: '2026-07-20',
      method: 'cash',
    });
    expect(paymentSchema.safeParse({ ...p, amount: 0 }).success).toBe(false);
    expect(paymentSchema.safeParse({ ...p, amount: null }).success).toBe(false);
  });

  it('parses guest names and derives the permissions', () => {
    expect(parseNames('Mario, Lucia\n\n Paolo ')).toEqual(['Mario', 'Lucia', 'Paolo']);
    const est = { id: EST_ID, organization_id: 'o1' };
    const me = {
      principal_type: 'user',
      memberships: [{ organization_id: 'o1', role: 'cashier', permissions: ['cash.operate'] }],
    } as unknown as Parameters<typeof partnerPermissions>[0];
    expect(partnerPermissions(me, est)).toEqual({
      canConfigure: false,
      canReport: false,
      canTakePayments: true,
      canWrite: false,
    });
  });
});

describe('PartnersPage', () => {
  it('lists active partners with balance, filters inactive ones and creates a partner', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('owner'),
      [`GET ${estPath('/partners')}`]: () => list([HOTEL, ASSOC]),
      [`POST ${estPath('/partners')}`]: () =>
        json(201, { ...HOTEL, id: 'new-partner', name: 'B&B Sole' }),
      [`GET ${estPath('/partners/new-partner')}`]: () =>
        json(200, { ...HOTEL, id: 'new-partner', name: 'B&B Sole' }),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/convenzioni`, routes);
    expect(
      await screen.findByRole('heading', { name: 'Partner e convenzioni', level: 1 }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Partner' });
    expect(await within(table).findByText('Hotel Miramare')).toBeInTheDocument();
    expect(table).toHaveTextContent('120,00 €');
    expect(within(table).queryByText('Associazione Vela')).toBeNull();

    const user = userEvent.setup();
    await user.click(screen.getByRole('switch', { name: 'Mostra non attivi' }));
    expect(await within(table).findByText('Associazione Vela')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Nuovo partner' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuovo partner' });
    await user.type(within(sheet).getByLabelText('Nome'), 'B&B Sole');
    await user.type(within(sheet).getByLabelText(/^Telefono/), '333 1234567');
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', estPath('/partners'))).toHaveLength(1));
    const [req] = callsTo(fetchMock, 'POST', estPath('/partners'));
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req!)).toEqual({
      name: 'B&B Sole',
      kind: 'struttura_ricettiva',
      active: true,
      phone: '+393331234567',
    });
    expect(await screen.findByRole('heading', { name: /B&B Sole/, level: 1 })).toBeInTheDocument();
  });

  it('manages customer categories and explains why one in use cannot be deleted', async () => {
    const category = {
      id: 'cc1',
      code: 'residenti',
      name: 'Residenti',
      description: '',
      requirement: 'Documento di residenza',
      register_category: 'residents',
      active: true,
      member_count: 12,
      ...STAMP,
    };
    const routes: Record<string, Handler> = {
      ...baseRoutes('owner'),
      [`GET ${estPath('/partners')}`]: () => list([]),
      [`GET ${estPath('/customer-categories')}`]: () => list([category]),
      [`POST ${estPath('/customer-categories')}`]: () => json(201, { ...category, id: 'cc2' }),
      [`DELETE ${estPath('/customer-categories/cc1')}`]: () =>
        problem(409, 'customer_category_in_use'),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/convenzioni`, routes);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('tab', { name: 'Categorie di clienti' }));
    const table = await screen.findByRole('table', { name: 'Categorie di clienti' });
    expect(await within(table).findByText('Documento di residenza')).toBeInTheDocument();
    expect(table).toHaveTextContent('Documento di residenza');

    await user.click(screen.getByRole('button', { name: 'Nuova categoria' }));
    const dialog = await screen.findByRole('dialog', { name: 'Nuova categoria' });
    await user.type(within(dialog).getByLabelText('Nome'), 'Over 65');
    await user.type(within(dialog).getByLabelText('Codice'), 'over65');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/customer-categories'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/customer-categories'));
    expect(await bodyOf(req!)).toMatchObject({
      code: 'over65',
      name: 'Over 65',
      register_category: 'residents',
      active: true,
    });

    await user.click(within(table).getByRole('button', { name: 'Elimina la categoria Residenti' }));
    const confirm = await screen.findByRole('alertdialog');
    await user.click(within(confirm).getByRole('button', { name: 'Elimina' }));
    await waitFor(() =>
      expect(screen.getByRole('alertdialog')).toHaveTextContent(
        'La categoria è usata da clienti o convenzioni',
      ),
    );
  });
});

describe('PartnerDetailPage', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-25T10:00:00Z'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('shows the statement of the current season, exports it and records a payment', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('owner'),
      [`GET ${estPath(`/partners/${HOTEL.id}`)}`]: () => json(200, HOTEL),
      [`GET ${estPath('/seasons')}`]: () => list([SEASON]),
      [`GET ${estPath('/agreements')}`]: () =>
        list([
          {
            id: 'ag1',
            name: 'Ospiti Miramare',
            code: 'MIRAMARE',
            partner_name: '',
            partner_id: HOTEL.id,
            notes: '',
            action: { type: 'percent_off', percent: '10' },
            conditions: {},
            stackable: false,
            active: true,
            public_link: '/b/lido-demo?c=MIRAMARE',
            ...STAMP,
          },
        ]),
      [`GET ${estPath(`/partners/${HOTEL.id}/statement`)}`]: () => json(200, statement()),
      [`POST ${estPath(`/partners/${HOTEL.id}/payments`)}`]: () =>
        json(201, statement().payments[0]),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/convenzioni/partner/${HOTEL.id}`, routes);
    expect(
      await screen.findByRole('heading', { name: /Hotel Miramare/, level: 1 }),
    ).toBeInTheDocument();
    await screen.findByRole('link', { name: 'AR-0001' });
    const bookings = screen.getByRole('table', { name: 'Prenotazioni a carico del partner' });
    expect(bookings).toHaveTextContent('Famiglia Rossi');
    expect(screen.getByText('Saldo finale').closest('[data-slot="card"]')).toHaveTextContent(
      '120,00 €',
    );
    expect(screen.getByRole('table', { name: 'Pagamenti ricevuti' })).toHaveTextContent('CRO 123');
    expect(screen.getByText('Ospiti Miramare')).toBeInTheDocument();
    const [get] = callsTo(fetchMock, 'GET', estPath(`/partners/${HOTEL.id}/statement`));
    expect(new URL(get!.url).search).toBe('?from=2026-05-01&to=2026-09-30');

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Esporta' }));
    const excel = await screen.findByRole('menuitem', { name: 'Excel' });
    expect(excel).toHaveAttribute(
      'href',
      `/api/admin/v1/establishments/${EST_ID}/partners/${HOTEL.id}/statement?from=2026-05-01&to=2026-09-30&format=xlsx`,
    );
    expect(screen.getByRole('menuitem', { name: 'Stampa o salva in PDF' })).toHaveAttribute(
      'href',
      expect.stringContaining('format=html'),
    );
    await user.keyboard('{Escape}');

    await user.click(screen.getByRole('button', { name: 'Registra pagamento' }));
    const dialog = await screen.findByRole('dialog', { name: 'Pagamento di Hotel Miramare' });
    await user.type(within(dialog).getByLabelText('Importo'), '50');
    await user.type(within(dialog).getByLabelText(/^Riferimento/), 'CRO 456');
    await user.click(within(dialog).getByRole('button', { name: 'Registra pagamento' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath(`/partners/${HOTEL.id}/payments`))).toHaveLength(1),
    );
    const [post] = callsTo(fetchMock, 'POST', estPath(`/partners/${HOTEL.id}/payments`));
    expect(post!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(post!)).toEqual({
      amount: 5000,
      received_on: '2026-07-25',
      method: 'bank_transfer',
      reference: 'CRO 456',
    });
  });
});
