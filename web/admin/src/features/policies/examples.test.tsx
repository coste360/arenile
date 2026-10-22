import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../configuration/testing';
import { augustPeriods, gazeboPacks, policyExample } from './examples';
import { policySchema, policyValues, toPolicyInput } from './policy.schema';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const period = (code: string, from: string, to: string) => ({
  id: `rp-${code}`,
  season_id: 's1',
  code,
  name: { it: code },
  color: '#ff0000',
  ranges: [{ from, to }],
  sort_order: 0,
  version: 1,
  ...ts,
});
const pack = (id: string, code: string, composition: Record<string, number>) => ({
  id,
  code,
  name: { it: code },
  composition,
  allowed_formulas: ['day'],
  channels: ['desk', 'online'],
  max_per_booking: 4,
  requires_approval: false,
  sort_order: 0,
  active: true,
  version: 1,
  ...ts,
});
const PERIODS = [
  period('low', '2027-05-15', '2027-06-30'),
  period('high', '2027-07-01', '2027-07-31'),
  period('agosto2', '2027-08-01', '2027-08-31'),
];
const PACKS = [pack('p1', 'base', { umbrella: 1, sunbed: 2 }), pack('p2', 'tenda', { gazebo: 1 })];

describe('policy examples (spec 9)', () => {
  it('finds August periods by name first, then by dates', () => {
    expect(augustPeriods([{ code: 'peak' }, { code: 'high' }])).toEqual(['peak']);
    expect(augustPeriods(PERIODS)).toEqual(['agosto2']);
    expect(
      augustPeriods([{ code: 'summer', ranges: [{ from: '2027-07-15', to: '2027-09-10' }] }]),
    ).toEqual(['summer']);
    expect(
      augustPeriods([{ code: 'low', ranges: [{ from: '2027-05-01', to: '2027-07-31' }] }]),
    ).toEqual([]);
  });

  it('finds gazebo packs by code or composition', () => {
    expect(gazeboPacks(PACKS)).toEqual(['p2']);
    expect(gazeboPacks([{ id: 'g', code: 'Gazebo-VIP' }])).toEqual(['g']);
  });

  it('produces valid policies for every example', () => {
    const catalog = { packs: PACKS, periods: PERIODS };
    const build = (id: Parameters<typeof policyExample>[0]) => {
      const values = {
        ...policyValues(undefined, 'it'),
        name: id,
        ...policyExample(id, catalog).values,
      };
      expect(policySchema.safeParse(values).success).toBe(true);
      return toPolicyInput('s1', values);
    };
    expect(build('august_weeks')).toMatchObject({
      scope: { channels: ['online'], rate_periods: ['agosto2'] },
      rules: { duration: { allowed_formulas: ['week'], fixed_start_weekday: 6 } },
    });
    expect(build('gazebo_approval')).toMatchObject({
      scope: { packs: ['p2'] },
      rules: { approval_required: true },
    });
    expect(build('online_full')).toMatchObject({
      scope: { channels: ['online'] },
      rules: { payment: { mode: 'full' } },
    });
    expect(build('desk_free')).toMatchObject({
      scope: { channels: ['desk'] },
      rules: { payment: { mode: 'none' } },
    });
  });

  it('warns when the catalog has nothing to match', () => {
    expect(policyExample('gazebo_approval', { packs: [], periods: [] }).warnings).toEqual([
      'examples.warnNoGazebo',
    ]);
    expect(policyExample('august_weeks', { packs: [], periods: [] }).warnings).toEqual([
      'examples.warnNoPeak',
    ]);
  });
});

describe('PoliciesPage example quick-fill', () => {
  it('fills the August weeks example and creates the policy', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/policies`, {
      ...baseRoutes('manager'),
      [`GET ${API}/seasons`]: () =>
        list([
          {
            id: 's1',
            year: 2027,
            opens_on: '2027-05-15',
            closes_on: '2027-09-15',
            status: 'active',
            version: 1,
            ...ts,
          },
        ]),
      [`GET ${API}/rate-periods`]: () => list(PERIODS),
      [`GET ${API}/packs`]: () => list(PACKS),
      [`GET ${API}/extras`]: () => list([]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/policies`]: () => list([]),
      [`POST ${API}/policies`]: async (req) => {
        body = (await req.json()) as Record<string, unknown>;
        return json(201, { ...body, id: 'bp1', version: 1, ...ts }, { ETag: '"1"' });
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Nuova policy' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(
      within(dialog).getByRole('button', { name: 'Agosto online: settimane sabato-sabato' }),
    );
    expect(within(dialog).getByLabelText('Nome')).toHaveValue(
      'Agosto online: settimane sabato-sabato',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/policies`)).toHaveLength(1));
    expect(body).toMatchObject({
      season_id: 's1',
      name: 'Agosto online: settimane sabato-sabato',
      scope: { channels: ['online'], rate_periods: ['agosto2'] },
      rules: { duration: { allowed_formulas: ['week'], fixed_start_weekday: 6 } },
    });
  });
});
