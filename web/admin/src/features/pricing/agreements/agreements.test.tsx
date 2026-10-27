import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf } from '../../bookings/testing';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../../configuration/testing';
import type { Agreement } from '../api';
import { agreementSchema, agreementValues, toAgreementInput } from '../rules.schema';
import { DEFAULT_RELEASE_DAYS } from './agreement.schema';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

const AGREEMENT = {
  id: 'ag1',
  name: 'Residenti',
  code: 'RES',
  partner_name: '',
  notes: '',
  action: { type: 'percent_off', percent: '20' },
  conditions: {},
  stackable: false,
  active: true,
  target: 'customer_category',
  customer_category_id: 'cc1',
  pack_prices: [{ pack_id: 'p1', amount: 1200, per: 'day' }],
  scope: { pack_ids: ['p1'] },
  season_ids: ['s1'],
  quota: { max_per_day: 10, reserved_per_day: 5, release_days_before: 7 },
  requirement: 'Documento di residenza',
  channels: ['desk', 'online_code'],
  register_category: 'residents',
  ...ts,
} as unknown as Agreement;

describe('agreement M8 fields', () => {
  it('round-trips target, pack prices, scope, seasons, quota, requirement and channels', () => {
    const v = agreementSchema.parse(agreementValues(AGREEMENT));
    expect(toAgreementInput(v)).toMatchObject({
      name: 'Residenti',
      code: 'RES',
      target: 'customer_category',
      customer_category_id: 'cc1',
      pack_prices: [{ pack_id: 'p1', amount: 1200, per: 'day' }],
      scope: { pack_ids: ['p1'] },
      season_ids: ['s1'],
      quota: { max_per_day: 10, reserved_per_day: 5, release_days_before: 7 },
      requirement: 'Documento di residenza',
      channels: ['desk', 'online_code'],
      register_category: 'residents',
    });
  });

  it('reads a pre-M8 agreement as individual customers at the desk', () => {
    const legacy = { ...AGREEMENT } as Record<string, unknown>;
    for (const k of [
      'target',
      'customer_category_id',
      'pack_prices',
      'scope',
      'season_ids',
      'quota',
      'requirement',
      'channels',
      'register_category',
    ]) {
      delete legacy[k];
    }
    const input = toAgreementInput(agreementSchema.parse(agreementValues(legacy as Agreement)));
    expect(input).toMatchObject({
      target: 'individual_customers',
      channels: ['desk'],
      register_category: 'other',
    });
    expect(input).not.toHaveProperty('quota');
    expect(input).not.toHaveProperty('pack_prices');
  });

  it('defaults the release of a reserved quota to 3 days (ADR 0028)', () => {
    const v = agreementValues(undefined);
    const input = toAgreementInput(
      agreementSchema.parse({ ...v, name: 'X', percent: '10', quota_reserved: '4' }),
    );
    expect(input.quota).toEqual({ reserved_per_day: 4, release_days_before: DEFAULT_RELEASE_DAYS });
  });

  it.each([
    ['a partner target without partner', { target: 'partner' }, 'partner_id'],
    ['a category target without category', { target: 'customer_category' }, 'customer_category_id'],
    ['an online channel without code', { agreement_channels: ['online_link'] }, 'code'],
    ['no channel at all', { agreement_channels: [] }, 'agreement_channels'],
    ['more reserved than the maximum', { quota_max: '2', quota_reserved: '3' }, 'quota_reserved'],
    [
      'a pack price without amount',
      { pack_prices: [{ pack_id: 'p1', amount: null }] },
      'pack_prices',
    ],
  ])('rejects %s', (_name, patch, path) => {
    const v = { ...agreementValues(undefined), name: 'X', percent: '10', ...patch };
    const result = agreementSchema.safeParse(v);
    expect(result.success).toBe(false);
    expect(result.error?.issues.map((i) => i.path[0])).toContain(path);
  });
});

describe('agreement editor (Prezzi → Codici e convenzioni)', () => {
  it('creates an agreement for a partner with the agreement link channel', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/pricing`, {
      ...baseRoutes('owner'),
      [`GET ${API}/seasons`]: () =>
        list([
          {
            id: 's1',
            year: 2027,
            opens_on: '2027-06-01',
            closes_on: '2027-09-10',
            status: 'active',
            ...ts,
          },
        ]),
      [`GET ${API}/rate-periods`]: () => list([]),
      [`GET ${API}/packs`]: () => list([]),
      [`GET ${API}/extras`]: () => list([]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/agreements`]: () => list([]),
      [`GET ${API}/promo-codes`]: () => list([]),
      [`GET ${API}/price-rules`]: () => list([]),
      [`GET ${API}/fixed-prices`]: () => list([]),
      [`GET ${API}/customer-categories`]: () => list([]),
      [`GET ${API}/partners`]: () =>
        list([
          {
            id: 'pa1',
            name: 'Hotel Miramare',
            kind: 'struttura_ricettiva',
            notes: '',
            active: true,
            ...ts,
          },
        ]),
      [`POST ${API}/agreements`]: () => json(201, { ...AGREEMENT, id: 'new' }),
    });
    await user.click(await screen.findByRole('tab', { name: 'Codici e convenzioni' }));
    await user.click(await screen.findByRole('button', { name: 'Nuova convenzione' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuova convenzione' });
    await user.type(within(sheet).getByLabelText('Nome'), 'Ospiti Miramare');
    await user.type(within(sheet).getByLabelText('Percentuale (%)'), '10');
    await user.click(within(sheet).getByRole('radio', { name: 'Partner e i suoi ospiti' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Partner destinatario' }));
    await user.click(await screen.findByRole('option', { name: 'Hotel Miramare' }));
    await user.click(within(sheet).getByRole('button', { name: 'Link della convenzione' }));
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    expect(await within(sheet).findByText('Per i canali online serve un codice.')).toBeVisible();

    await user.type(within(sheet).getByLabelText('Codice'), 'miramare');
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/agreements`)).toHaveLength(1));
    const [req] = callsTo(fetchMock, 'POST', `${API}/agreements`);
    expect(await bodyOf(req!)).toMatchObject({
      name: 'Ospiti Miramare',
      code: 'miramare',
      action: { type: 'percent_off', percent: '10' },
      target: 'partner',
      partner_id: 'pa1',
      channels: ['desk', 'online_link'],
      register_category: 'other',
    });
  });
});
