import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf, estPath } from '../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../configuration/testing';

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const SEASONS = [
  {
    id: 's2026',
    establishment_id: EST_ID,
    year: 2026,
    opens_on: '2026-05-15',
    closes_on: '2026-09-20',
    status: 'active',
    ...ts,
  },
  {
    id: 's2027',
    establishment_id: EST_ID,
    year: 2027,
    opens_on: '2027-05-15',
    closes_on: '2027-09-20',
    status: 'draft',
    ...ts,
  },
];

/** Deadline 2 days, 5 hours (and a bit) from now: shown as "2 g 5 h". */
const DEADLINE = new Date(Date.now() + (2 * 24 + 5) * 3_600_000 + 120_000).toISOString();

function campaign(overrides: Record<string, unknown> = {}) {
  return {
    id: 'c1',
    name: 'Rinnovi 2027',
    status: 'open',
    source_season_id: 's2026',
    target_season_id: 's2027',
    deadline_at: DEADLINE,
    counts: { proposed: 1, accepted: 1, declined: 1, expired: 0, converted: 2, sent: 4 },
    total_amount: 750000,
    converted_amount: 300000,
    version: 2,
    created_at: '2026-10-01T08:00:00Z',
    updated_at: '2026-10-01T08:00:00Z',
    ...overrides,
  };
}

function proposal(
  id: string,
  spot: string,
  customer: string,
  overrides: Record<string, unknown> = {},
) {
  return {
    id,
    campaign_id: 'c1',
    status: 'proposed',
    source_pass_id: `pass-${id}`,
    booking_id: `b-${id}`,
    booking_code: `AR-${id}`,
    customer_id: `cust-${id}`,
    customer_display_name: customer,
    customer_has_email: true,
    spot_id: `spot-${id}`,
    spot_label: spot,
    pack_id: 'pk1',
    starts_on: '2027-05-15',
    ends_on: '2027-09-20',
    slots: 'FULL',
    previous_amount: 140000,
    total_amount: 150000,
    deposit_due: 50000,
    paid_amount: 0,
    deadline_at: DEADLINE,
    send_count: 0,
    version: 3,
    created_at: '2026-10-01T08:00:00Z',
    updated_at: '2026-10-01T08:00:00Z',
    ...overrides,
  };
}

const PROPOSALS = [
  proposal('p1', 'A1', 'Rossi Mario'),
  proposal('p2', 'A2', 'Bianchi Luca', {
    status: 'accepted',
    send_count: 2,
    responded_at: '2026-10-05T09:00:00Z',
  }),
  proposal('p3', 'B1', 'Neri Anna', {
    status: 'declined',
    send_count: 1,
    customer_has_email: false,
    responded_at: '2026-10-06T09:00:00Z',
  }),
];

function routes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    ...baseRoutes('manager'),
    [`GET ${estPath('/seasons')}`]: () => list(SEASONS),
    [`GET ${estPath('/renewal-campaigns')}`]: () => list([campaign()]),
    [`GET ${estPath('/renewal-campaigns/c1/proposals')}`]: (req) => {
      const status = new URL(req.url).searchParams.get('status');
      return list(status ? PROPOSALS.filter((p) => p.status === status) : PROPOSALS);
    },
    ...extra,
  };
}

const PAGE = `/e/${EST_ID}/rinnovi`;

describe('RenewalsPage', () => {
  it('shows the campaign KPIs, the deadline countdown and the proposals', async () => {
    renderApp(PAGE, routes());
    const table = await screen.findByRole('table', { name: 'Proposte di rinnovo' });
    await within(table).findByText('Rossi Mario');
    expect(screen.getByRole('heading', { name: 'Rinnovi 2027' })).toBeInTheDocument();
    expect(screen.getByText('Stagione 2026 → 2027')).toBeInTheDocument();
    // 5 proposals, 4 sent; 1 waiting; 3 renewed (2 confirmed); 1 released.
    expect(screen.getByText('4 inviate')).toBeInTheDocument();
    expect(screen.getByText('2 confermati')).toBeInTheDocument();
    expect(screen.getByText('1 rifiutati · 0 scaduti')).toBeInTheDocument();
    expect(screen.getAllByText('2 g 5 h').length).toBeGreaterThan(0);

    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent(
      /A1.*Rossi Mario.*1\.500,00\s€.*prima 1\.400,00\s€.*In attesa.*Non inviata.*2 g 5 h/,
    );
    expect(rows[1]).toHaveTextContent(/A2.*Bianchi Luca.*Accettato, da pagare.*Inviata 2 volte/);
    expect(rows[2]).toHaveTextContent(/B1.*Neri Anna.*senza email.*Rifiutato/);
  });

  it('filters the proposals by status on the server', async () => {
    const { fetchMock } = renderApp(PAGE, routes());
    const user = userEvent.setup();
    await screen.findByRole('table', { name: 'Proposte di rinnovo' });
    await user.click(screen.getByRole('combobox', { name: 'Stato' }));
    await user.click(await screen.findByRole('option', { name: 'Rifiutato' }));
    await waitFor(() => {
      const calls = callsTo(fetchMock, 'GET', estPath('/renewal-campaigns/c1/proposals'));
      expect(new URL(calls.at(-1)!.url).searchParams.get('status')).toBe('declined');
    });
    const table = screen.getByRole('table', { name: 'Proposte di rinnovo' });
    await waitFor(() => expect(within(table).getAllByRole('row')).toHaveLength(2));
  });

  it('accepts a proposal at the desk with If-Match', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/renewal-proposals/p1/accept')}`]: () =>
          json(200, proposal('p1', 'A1', 'Rossi Mario', { status: 'converted' })),
      }),
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Azioni per A1, Rossi Mario' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Accetta (al banco)' }));
    const dialog = await screen.findByRole('dialog', { name: 'Accetta il rinnovo' });
    expect(within(dialog).getByRole('switch', { name: 'Conferma subito' })).toBeChecked();
    await user.click(within(dialog).getByRole('button', { name: 'Accetta' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/renewal-proposals/p1/accept'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/renewal-proposals/p1/accept'));
    expect(req!.headers.get('If-Match')).toBe('"3"');
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req!)).toEqual({ confirm: true });
  });

  it('sends the proposals not yet sent in bulk and reports failures', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/renewal-campaigns/c1/proposals/bulk')}`]: () =>
          json(200, {
            action: 'send',
            matched: 2,
            succeeded: 1,
            skipped: 0,
            failed: [{ proposal_id: 'p1', type: 'renewal_expired' }],
          }),
      }),
    );
    const user = userEvent.setup();
    await screen.findByRole('table', { name: 'Proposte di rinnovo' });
    await user.click(screen.getByRole('button', { name: /Azioni di gruppo/ }));
    await user.click(
      await screen.findByRole('menuitem', { name: "Invia a chi non l'ha ancora ricevuta" }),
    );
    const confirm = await screen.findByRole('alertdialog');
    await user.click(within(confirm).getByRole('button', { name: /Conferma/ }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('1 proposta non aggiornata');
    expect(alert).toHaveTextContent(/A1 · Rossi Mario: La prelazione è scaduta/);
    const [req] = callsTo(fetchMock, 'POST', estPath('/renewal-campaigns/c1/proposals/bulk'));
    expect(await bodyOf(req!)).toEqual({ action: 'send', status: 'proposed', only_unsent: true });
  });

  it('selects proposals and releases them now (force)', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/renewal-campaigns/c1/proposals/bulk')}`]: () =>
          json(200, { action: 'release', matched: 2, succeeded: 2, skipped: 0, failed: [] }),
      }),
    );
    const user = userEvent.setup();
    await screen.findByText('Rossi Mario');
    await user.click(screen.getByRole('checkbox', { name: 'Seleziona tutte le proposte aperte' }));
    await user.click(screen.getByRole('button', { name: /2 selezionate/ }));
    await user.click(await screen.findByRole('menuitem', { name: 'Libera subito' }));
    const confirm = await screen.findByRole('alertdialog', {
      name: 'Liberare subito le postazioni?',
    });
    await user.click(within(confirm).getByRole('button', { name: /Conferma/ }));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'POST', estPath('/renewal-campaigns/c1/proposals/bulk')),
      ).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/renewal-campaigns/c1/proposals/bulk'));
    expect(await bodyOf(req!)).toEqual({
      action: 'release',
      proposal_ids: ['p1', 'p2'],
      force: true,
    });
  });

  it('previews then creates a campaign', async () => {
    const preview = {
      dry_run: true,
      proposals: [
        {
          source_pass_id: 'pass-1',
          customer_id: 'cu1',
          customer_display_name: 'Rossi Mario',
          spot_id: 'sp1',
          spot_label: 'A1',
          pack_id: 'pk1',
          starts_on: '2027-05-15',
          ends_on: '2027-09-20',
          slots: 'FULL',
          previous_amount: 140000,
          total_amount: 150000,
          deposit_due: 50000,
        },
      ],
      skipped: [
        {
          source_pass_id: 'pass-2',
          customer_display_name: 'Verdi Ugo',
          spot_label: 'C9',
          reason: 'spot_inactive',
        },
      ],
    };
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`GET ${estPath('/renewal-campaigns')}`]: () => list([]),
        [`POST ${estPath('/renewal-campaigns')}`]: async (req) => {
          const body = (await bodyOf(req)) as { dry_run: boolean };
          return body.dry_run
            ? json(200, preview)
            : json(201, { ...preview, dry_run: false, campaign: campaign() });
        },
      }),
    );
    const user = userEvent.setup();
    expect(await screen.findByText('Nessuna campagna di rinnovo')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Passaggio di fine stagione' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/config/seasons/rollover`,
    );
    await user.click(screen.getByRole('button', { name: 'Nuova campagna' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuova campagna' });
    expect(within(sheet).getByRole('combobox', { name: 'Stagione di origine' })).toHaveTextContent(
      '2026',
    );
    expect(within(sheet).getByRole('combobox', { name: 'Nuova stagione' })).toHaveTextContent(
      '2027',
    );
    await user.click(within(sheet).getByRole('button', { name: 'Anteprima' }));
    expect(await within(sheet).findByText('Verrà creata 1 proposta')).toBeInTheDocument();
    expect(within(sheet).getByText('Postazione disattivata')).toBeInTheDocument();
    await user.click(within(sheet).getByRole('button', { name: 'Crea le proposte' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/renewal-campaigns'))).toHaveLength(2),
    );
    const [dry, real] = callsTo(fetchMock, 'POST', estPath('/renewal-campaigns'));
    const dryBody = (await bodyOf(dry!)) as Record<string, unknown>;
    expect(dryBody).toMatchObject({
      source_season_id: 's2026',
      target_season_id: 's2027',
      dry_run: true,
    });
    expect(String(dryBody.deadline_at)).toMatch(/^\d{4}-\d{2}-\d{2}T2[12]:59:00Z$/);
    expect(await bodyOf(real!)).toMatchObject({ dry_run: false });
  });
});
