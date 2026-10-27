import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import type { Schemas } from '@arenile/shared/api';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../../configuration/testing';
import { bodyOf, bookingDetail, estPath } from '../testing';
import {
  bpToDecimal,
  decimalToBp,
  deskAgreements,
  effectiveAgreementId,
  emptyAgreementValue,
  fromDetail,
  toCreateFields,
  toPatch,
} from './model';

type BookingDetail = Schemas['BookingDetail'];
type Agreement = Schemas['Agreement'];

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

function agreement(o: Record<string, unknown> = {}) {
  return {
    id: 'ag1',
    name: 'Residenti',
    code: 'RES',
    partner_name: '',
    notes: '',
    action: { type: 'percent_off', percent: '20' },
    conditions: {},
    stackable: false,
    active: true,
    requirement: 'Documento di residenza',
    ...ts,
    ...o,
  };
}

const PARTNER = {
  id: 'pa1',
  name: 'Hotel Miramare',
  kind: 'struttura_ricettiva',
  notes: '',
  active: true,
  ...ts,
};

describe('booking agreement model', () => {
  it('uses the customer default until the operator chooses (none = empty string)', () => {
    const v = emptyAgreementValue();
    expect(effectiveAgreementId(v, { agreement_id: 'ag1' })).toBe('ag1');
    expect(effectiveAgreementId({ ...v, agreementId: '' }, { agreement_id: 'ag1' })).toBe('');
    expect(effectiveAgreementId({ ...v, agreementId: 'ag2' }, null)).toBe('ag2');
  });

  it('offers active desk agreements, keeping the current one', () => {
    const list = [
      agreement({ id: 'a', name: 'B', channels: ['online_link'] }),
      agreement({ id: 'b', name: 'A' }),
      agreement({ id: 'c', name: 'C', active: false }),
    ] as unknown as Agreement[];
    expect(deskAgreements(list).map((a) => a.id)).toEqual(['b']);
    expect(deskAgreements(list, 'c').map((a) => a.id)).toEqual(['b', 'c']);
  });

  it('converts the partner share percentage exactly', () => {
    expect(bpToDecimal(5000)).toBe('50');
    expect(bpToDecimal(3350)).toBe('33.5');
    expect(bpToDecimal(5)).toBe('0.05');
    expect(decimalToBp('33.5')).toBe(3350);
    expect(decimalToBp(undefined)).toBeNull();
  });

  it('builds the create fields: verification only with a requirement, names one per line', () => {
    const v = {
      ...emptyAgreementValue(),
      requirementVerified: true,
      partnerId: 'pa1',
      shareType: 'percent' as const,
      sharePercentBp: 5000,
      guestStay: { from: '2026-07-10', to: '2026-07-20' },
      groupName: ' Famiglia Rossi ',
      groupOrigin: 'ignored with a partner',
      adults: 2,
      children: 1,
      guestNames: 'Mario\nLucia',
    };
    expect(toCreateFields(v, { agreement_id: 'ag1' }, 'Documento')).toEqual({
      agreement_id: 'ag1',
      requirement_verified: true,
      partner_id: 'pa1',
      partner_share: { type: 'percent', percent: '50' },
      partner_guest_stay: { from: '2026-07-10', to: '2026-07-20' },
      group_name: 'Famiglia Rossi',
      party_adults: 2,
      party_children: 1,
      guest_names: ['Mario', 'Lucia'],
    });
    expect(toCreateFields(v, { agreement_id: 'ag1' }, undefined)).not.toHaveProperty(
      'requirement_verified',
    );
    expect(toCreateFields(emptyAgreementValue(), null, undefined)).toEqual({});
  });

  it('patches only what changed; an empty string removes agreement and partner', () => {
    const b = bookingDetail({
      agreement_id: 'ag1',
      agreement: { id: 'ag1', name: 'Residenti' },
      partner: { id: 'pa1', name: 'Hotel Miramare', kind: 'struttura_ricettiva' },
      partner_share: { type: 'full' },
      group_name: 'Rossi',
      party_adults: 2,
    }) as unknown as BookingDetail;
    const v = fromDetail(b);
    expect(toPatch(v, b)).toEqual({});
    expect(toPatch({ ...v, agreementId: '', partnerId: '' }, b)).toEqual({
      agreement_id: '',
      partner_id: '',
      partner_share: { type: 'none' },
    });
    expect(toPatch({ ...v, adults: 3, guestNames: 'Ada' }, b)).toEqual({
      party_adults: 3,
      guest_names: ['Ada'],
    });
  });
});

describe('BookingAgreementCard', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-15T10:00:00'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('marks the requirement as verified and changes the agreement after a dry-run preview', async () => {
    const detail = bookingDetail({
      agreement_id: 'ag1',
      agreement: {
        id: 'ag1',
        name: 'Residenti',
        code: 'RES',
        requirement: 'Documento di residenza',
      },
      group_name: 'Famiglia Rossi',
      party_adults: 2,
      party_children: 1,
    });
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      [`GET ${estPath('/maps')}`]: () => list([]),
      [`GET ${estPath('/zones')}`]: () => list([]),
      [`GET ${estPath('/packs')}`]: () => list([]),
      [`GET ${estPath('/extras')}`]: () => list([]),
      [`GET ${estPath('/agreements')}`]: () =>
        list([agreement(), agreement({ id: 'ag2', name: 'Over 65', code: 'OVER65' })]),
      [`GET ${estPath('/partners')}`]: () => list([PARTNER]),
      [`GET ${estPath(`/bookings/${detail.id}`)}`]: () => json(200, detail),
      [`POST ${estPath(`/bookings/${detail.id}/requirement-verification`)}`]: () =>
        json(200, {
          id: 'rv1',
          subject: 'booking',
          requirement: 'Documento di residenza',
          verified_at: '2026-07-15T08:00:00Z',
        }),
      [`PATCH ${estPath(`/bookings/${detail.id}`)}`]: (req) => {
        const dry = new URL(req.url).searchParams.get('dry_run') === 'true';
        return json(200, {
          applied: !dry,
          booking: detail,
          previous_total: 9000,
          new_total: 8000,
          price_difference: -1000,
          conflicts: [],
        });
      },
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni/${detail.id}`, routes);
    await screen.findByRole('heading', { name: /AR-7K3Q9/, level: 1 });
    expect(await screen.findByText('Residenti')).toBeInTheDocument();
    expect(screen.getByText('Famiglia Rossi · 2 adulti, 1 bambini')).toBeInTheDocument();
    expect(screen.getByText('Da verificare')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Segna verificato' }));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'POST', estPath(`/bookings/${detail.id}/requirement-verification`)),
      ).toHaveLength(1),
    );
    const [verify] = callsTo(
      fetchMock,
      'POST',
      estPath(`/bookings/${detail.id}/requirement-verification`),
    );
    expect(verify!.headers.get('If-Match')).toBe('"4"');

    await user.click(screen.getByRole('button', { name: 'Modifica' }));
    const dialog = await screen.findByRole('dialog', { name: 'Convenzione e ospiti di AR-7K3Q9' });
    await user.click(within(dialog).getByRole('combobox', { name: 'Convenzione' }));
    await user.click(await screen.findByRole('option', { name: 'Over 65 · OVER65' }));
    await user.click(within(dialog).getByRole('button', { name: 'Ricalcola' }));
    expect(await within(dialog).findByTestId('change-difference')).toHaveTextContent('10,00 €');
    await user.click(within(dialog).getByRole('button', { name: 'Applica modifica' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PATCH', estPath(`/bookings/${detail.id}`))).toHaveLength(2),
    );
    const [dry, apply] = callsTo(fetchMock, 'PATCH', estPath(`/bookings/${detail.id}`));
    expect(new URL(dry!.url).searchParams.get('dry_run')).toBe('true');
    expect(await bodyOf(apply!)).toEqual({ agreement_id: 'ag2' });
  });
});

describe('NewBookingSheet agreement section', () => {
  it('re-quotes with the chosen agreement and sends verification, partner and group', async () => {
    const zone = {
      id: 'zq',
      map_id: null,
      code: 'lettini',
      name: { it: 'Lettini liberi' },
      assignment_mode: 'quantity',
      online_visible: true,
      price_category: 'std',
      auto_assign: false,
      sort_order: 1,
      spot_count: 0,
      ...ts,
    };
    const pack = {
      id: 'p1',
      code: 'lettino',
      name: { it: 'Lettino' },
      composition: {},
      allowed_formulas: ['day', 'multi_day'],
      channels: ['desk'],
      sort_order: 1,
      active: true,
      ...ts,
    };
    const quote = (total: number) =>
      json(200, {
        breakdown: {
          engine_version: 1,
          currency: 'EUR',
          lines: [],
          discounts: [],
          rounding: 0,
          total,
          vat: [],
        },
        deposit_due: 0,
        policy: { rules: {}, violations: [] },
        available: true,
        conflicts: [],
      });
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      [`GET ${estPath('/maps')}`]: () => list([]),
      [`GET ${estPath('/zones')}`]: () => list([zone]),
      [`GET ${estPath('/packs')}`]: () => list([pack]),
      [`GET ${estPath('/extras')}`]: () => list([]),
      [`GET ${estPath('/bookings')}`]: () => list([]),
      [`GET ${estPath('/customers')}`]: () => list([]),
      [`GET ${estPath('/agreements')}`]: () => list([agreement()]),
      [`GET ${estPath('/partners')}`]: () => list([PARTNER]),
      [`POST ${estPath('/bookings/quote')}`]: async (req) =>
        quote(((await bodyOf(req)) as { agreement_id?: string }).agreement_id ? 1920 : 2400),
      [`POST ${estPath('/bookings')}`]: () =>
        json(201, bookingDetail({ id: 'new-b', code: 'AR-NEW' })),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni`, routes);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Nuova prenotazione' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuova prenotazione' });
    await user.click(within(sheet).getByRole('tab', { name: 'Zona o quantità' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Zona' }));
    await user.click(await screen.findByRole('option', { name: 'Lettini liberi' }));
    await user.click(within(sheet).getByRole('radio', { name: 'Nuovo cliente' }));
    await user.type(within(sheet).getByLabelText('Cognome'), 'Verdi');
    expect(await within(sheet).findByTestId('quote-total')).toHaveTextContent('24,00 €');

    await user.click(within(sheet).getByRole('combobox', { name: 'Convenzione' }));
    await user.click(await screen.findByRole('option', { name: 'Residenti · RES' }));
    await waitFor(() =>
      expect(within(sheet).getByTestId('quote-total')).toHaveTextContent('19,20 €'),
    );
    await user.click(within(sheet).getByRole('checkbox', { name: 'Requisito verificato' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Ospite di' }));
    await user.click(await screen.findByRole('option', { name: 'Hotel Miramare' }));
    await user.click(within(sheet).getByRole('radio', { name: 'Tutto' }));
    await user.type(within(sheet).getByLabelText('Nome del gruppo'), 'Gruppo Verdi');
    await user.click(within(sheet).getByRole('button', { name: 'Aumenta Adulti' }));
    await user.click(within(sheet).getByRole('button', { name: 'Crea prenotazione' }));

    await waitFor(() => expect(callsTo(fetchMock, 'POST', estPath('/bookings'))).toHaveLength(1));
    const [create] = callsTo(fetchMock, 'POST', estPath('/bookings'));
    expect(await bodyOf(create!)).toMatchObject({
      agreement_id: 'ag1',
      requirement_verified: true,
      partner_id: 'pa1',
      partner_share: { type: 'full' },
      group_name: 'Gruppo Verdi',
      party_adults: 1,
      customer: { last_name: 'Verdi' },
    });
  });
});
