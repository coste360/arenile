import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { opDay, opSpot } from '../operations/testing/fixtures';
import { API, SETTINGS, printData, ticket } from './testing';

const PATH = `/admin/e/${EST_ID}/ticket`;

const LIST = {
  items: [
    ticket(),
    ticket({
      id: 't38',
      ticket_number: 38,
      number_label: '#038',
      spot_id: undefined,
      spot_label: undefined,
      needs_assignment: true,
      state: 'unassigned',
      payment_method: 'card',
      created_at: '2026-07-14T08:30:00Z',
    }),
    ticket({
      id: 't36',
      ticket_number: 36,
      number_label: '#036',
      status: 'voided',
      state: 'voided',
      created_at: '2026-07-14T07:50:00Z',
    }),
  ],
  summary: {
    issued: 3,
    voided: 1,
    unassigned: 1,
    entered: 0,
    total_amount: 6000,
    by_method: [
      { method: 'cash', count: 1, amount: 3000 },
      { method: 'card', count: 1, amount: 3000 },
    ],
    vat: [{ rate: '22.00', taxable: 4918, tax: 1082 }],
  },
};

function setup(extra: Record<string, (r: Request) => Response> = {}) {
  return renderApp(PATH, {
    ...baseRoutes('cashier'),
    [`GET ${API}/tickets`]: () => json(200, LIST),
    [`GET ${API}/operations/day`]: () =>
      json(200, opDay([opSpot('s1', 'A1', 'free'), opSpot('s2', 'A2', 'free')])),
    ...extra,
  });
}

describe('TicketsPage (Ticket del giorno)', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
    vi.stubGlobal('print', vi.fn());
  });
  afterAll(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('lists the day with number, time, place, state, amount, method and the summary', async () => {
    const { fetchMock } = setup();
    const table = await screen.findByRole('table', { name: 'Ticket del giorno' });
    await within(table).findByText('#038');
    const rows = within(table).getAllByRole('row').slice(1);
    // Newest number first.
    expect(rows.map((r) => within(r).getAllByRole('cell')[0]?.textContent)).toEqual([
      '#038',
      '#037',
      '#036',
    ]);
    expect(within(rows[0] as HTMLElement).getByText('Da assegnare')).toBeInTheDocument();
    expect(within(rows[1] as HTMLElement).getByText('10:05')).toBeInTheDocument();
    expect(within(rows[1] as HTMLElement).getByText('A1')).toBeInTheDocument();
    expect(within(rows[2] as HTMLElement).getByText('Annullato')).toBeInTheDocument();
    expect(screen.getByText('Contanti 30,00 € · Carta 30,00 €', { exact: false })).toBeVisible();
    expect(screen.getByText('IVA 22%')).toBeInTheDocument();
    const req = callsTo(fetchMock, 'GET', `${API}/tickets`)[0] as Request;
    expect(new URL(req.url).searchParams.get('date')).toBe('2026-07-14');
    expect(
      screen.getByRole('link', { name: 'Esporta CSV dei biglietti del 2026-07-14' }),
    ).toHaveAttribute('href', `${API}/tickets?date=2026-07-14&format=csv`);
    // A voided ticket cannot be voided or moved again.
    expect(screen.getByRole('button', { name: 'Annulla il biglietto #036' })).toBeDisabled();
  });

  it('voids a ticket with a reason and the refund choice', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/tickets/t37/void`]: () =>
        json(200, ticket({ status: 'voided', state: 'voided' })),
    });
    await user.click(await screen.findByRole('button', { name: 'Annulla il biglietto #037' }));
    const dialog = await screen.findByRole('dialog', { name: 'Annullare il biglietto #037?' });
    await user.click(within(dialog).getByRole('button', { name: 'Annulla biglietto' }));
    expect(within(dialog).getByText("Indica il motivo dell'annullamento.")).toBeVisible();
    await user.type(within(dialog).getByLabelText('Motivo'), 'Cliente andato via');
    await user.click(within(dialog).getByRole('radio', { name: 'Nessun rimborso' }));
    await user.click(within(dialog).getByRole('button', { name: 'Annulla biglietto' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/tickets/t37/void`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/tickets/t37/void`)[0] as Request;
    expect(req.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await req.clone().json()).toEqual({
      reason: 'Cliente andato via',
      refund: 'none',
      version: 1,
    });
  });

  it('assigns a zone-first ticket from the list', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/tickets/t38/assign`]: () => json(200, ticket({ id: 't38' })),
    });
    await user.click(
      await screen.findByRole('button', { name: 'Assegna il biglietto #038 a una postazione' }),
    );
    const dialog = await screen.findByRole('dialog', { name: 'Assegna il biglietto #038' });
    await user.click(within(dialog).getByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A2' }));
    await user.click(within(dialog).getByRole('button', { name: 'Assegna' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/tickets/t38/assign`)).toHaveLength(1),
    );
  });

  it('reprints with the chosen format from the preview', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`GET ${API}/tickets/t37/print`]: () => json(200, printData()),
      [`POST ${API}/tickets/t37/reprint`]: () =>
        json(200, printData({ format: 'a4', reprint: true })),
    });
    await user.click(await screen.findByRole('button', { name: 'Stampa il biglietto #037' }));
    const dialog = await screen.findByRole('dialog', { name: 'Stampa del biglietto #037' });
    expect(within(dialog).getByRole('article', { name: 'Biglietto #037' })).toHaveAttribute(
      'data-format',
      'thermal_80',
    );
    await user.click(within(dialog).getByRole('radio', { name: 'A4' }));
    expect(within(dialog).getByRole('article', { name: 'Biglietto #037' })).toHaveAttribute(
      'data-format',
      'a4',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Ristampa' }));
    await waitFor(() => expect(window.print).toHaveBeenCalled());
    const req = callsTo(fetchMock, 'POST', `${API}/tickets/t37/reprint`)[0] as Request;
    expect(await req.clone().json()).toEqual({ format: 'a4' });
    const area = document.querySelector('[data-ticket-print-area]') as HTMLElement;
    expect(within(area).getByText('Ristampa')).toBeInTheDocument();
  });

  it('saves the ticket settings with If-Match (owner only)', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/tickets`]: () => json(200, LIST),
      [`GET ${API}/daily-sale-settings`]: () => json(200, SETTINGS),
      [`PUT ${API}/daily-sale-settings`]: async (req: Request) =>
        json(200, { ...SETTINGS, ...((await req.clone().json()) as object), version: 2 }),
    });
    await user.click(await screen.findByRole('button', { name: 'Impostazioni' }));
    const dialog = await screen.findByRole('dialog', { name: 'Impostazioni dei biglietti' });
    await user.click(within(dialog).getByRole('radio', { name: 'A4' }));
    await user.click(within(dialog).getByRole('switch', { name: 'Stampa subito dopo la vendita' }));
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', `${API}/daily-sale-settings`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'PUT', `${API}/daily-sale-settings`)[0] as Request;
    expect(req.headers.get('If-Match')).toBe('"1"');
    expect(await req.clone().json()).toEqual({
      print_format: 'a4',
      auto_print: false,
      zone_first_enabled: true,
      footer_text: { it: 'Grazie' },
    });
  });

  it('hides the settings from a cashier', async () => {
    setup({ [`GET ${API}/daily-sale-settings`]: () => json(200, SETTINGS) });
    await screen.findByRole('table', { name: 'Ticket del giorno' });
    expect(screen.queryByRole('button', { name: 'Impostazioni' })).not.toBeInTheDocument();
  });
});
