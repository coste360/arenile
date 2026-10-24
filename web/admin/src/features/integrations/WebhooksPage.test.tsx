import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  renderApp,
  type Handler,
  type TestRole,
} from '../configuration/testing';

const PATH = `/admin/e/${EST_ID}/config/webhooks`;
const EPS = `/api/admin/v1/establishments/${EST_ID}/webhook-endpoints`;

const endpoint = (extra: Record<string, unknown> = {}) => ({
  id: 'w1',
  url: 'https://hooks.example.it/arenile',
  description: 'Gestionale',
  events: ['booking.*', 'payment.succeeded'],
  active: true,
  status: 'active',
  consecutive_failures: 0,
  last_delivery_at: '2026-07-14T08:00:00Z',
  created_at: '2026-06-01T08:00:00Z',
  updated_at: '2026-06-01T08:00:00Z',
  ...extra,
});

const delivery = (id: string, extra: Record<string, unknown> = {}) => ({
  id,
  endpoint_id: 'w1',
  event_id: `evt_${id}`,
  event_type: 'booking.confirmed',
  status: 'failed',
  attempts: 3,
  status_code: 500,
  last_error: 'HTTP 500',
  failed_at: '2026-07-14T09:00:00Z',
  created_at: '2026-07-14T08:00:00Z',
  updated_at: '2026-07-14T09:00:00Z',
  ...extra,
});

function routes(role: TestRole, extra: Record<string, Handler> = {}) {
  return {
    ...baseRoutes(role),
    [`GET ${EPS}`]: () =>
      list([
        endpoint(),
        endpoint({
          id: 'w2',
          url: 'https://crm.example.it/in',
          description: '',
          events: ['*'],
          active: true,
          status: 'disabled',
          disabled_reason: 'Troppi errori consecutivi',
          consecutive_failures: 12,
        }),
      ]),
    ...extra,
  };
}

const bodyOf = async (req: Request | undefined) => (await req?.clone().json()) as unknown;

describe('WebhooksPage', () => {
  it('lists the endpoints with events, status and failures', async () => {
    renderApp(PATH, routes('owner'));
    const row = (await screen.findByText('https://hooks.example.it/arenile')).closest(
      'tr',
    ) as HTMLElement;
    expect(within(row).getByText('Prenotazioni: tutti')).toBeInTheDocument();
    expect(within(row).getByText('Pagamento riuscito')).toBeInTheDocument();
    expect(within(row).getByText('Attivo')).toBeInTheDocument();
    const disabled = screen.getByText('https://crm.example.it/in').closest('tr') as HTMLElement;
    expect(within(disabled).getByText('Disattivato per errori')).toBeInTheDocument();
    expect(within(disabled).getByText('Troppi errori consecutivi')).toBeInTheDocument();
    expect(within(disabled).getByText('Errori consecutivi: 12')).toBeInTheDocument();
    expect(within(disabled).getByText('Tutti gli eventi')).toBeInTheDocument();
    expect(screen.getByText('Arenile-Signature: t=…,v1=…')).toBeInTheDocument();
  });

  it('creates an endpoint with the event picker and shows the secret once', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`POST ${EPS}`]: () =>
          json(201, { endpoint: endpoint({ id: 'w3' }), secret: 'whsec_T0PS3CR3T' }),
      }),
    );
    await screen.findByText('https://hooks.example.it/arenile');
    await user.click(screen.getByRole('button', { name: 'Nuovo endpoint' }));
    const dialog = await screen.findByRole('dialog', { name: 'Nuovo endpoint' });
    const url = within(dialog).getByLabelText('Indirizzo (URL)');
    await user.clear(url);
    await user.type(url, 'http://hooks.example.it/x');
    await user.click(within(dialog).getByRole('button', { name: 'Crea endpoint' }));
    expect(
      await within(dialog).findByText('Indirizzo https completo (http solo verso localhost).'),
    ).toBeInTheDocument();

    await user.clear(url);
    await user.type(url, 'https://hooks.example.it/new');
    await user.type(within(dialog).getByLabelText('Descrizione'), 'CRM');
    // Default selection is booking.*; picking one payment event adds it,
    // picking "Tutti gli eventi" replaces everything.
    await user.click(within(dialog).getByRole('combobox', { name: 'Eventi' }));
    await user.click(await screen.findByRole('option', { name: /Pagamento fallito/ }));
    await user.click(screen.getByRole('option', { name: /Tutti gli eventi/ }));
    await user.click(screen.getByRole('option', { name: /Prenotazione cancellata/ }));
    await user.keyboard('{Escape}');
    await user.click(within(dialog).getByRole('button', { name: 'Crea endpoint' }));

    const secret = await screen.findByRole('dialog', { name: 'Endpoint creato' });
    expect(within(secret).getByLabelText('Segreto di firma')).toHaveValue('whsec_T0PS3CR3T');
    expect(await bodyOf(callsTo(fetchMock, 'POST', EPS)[0])).toEqual({
      url: 'https://hooks.example.it/new',
      description: 'CRM',
      events: ['booking.cancelled'],
      active: true,
    });
    await user.click(within(secret).getByRole('button', { name: 'Ho salvato, chiudi' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('sends a test event and shows the outcome', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`POST ${EPS}/w1/test`]: () =>
          json(
            200,
            delivery('d9', {
              event_type: 'webhook.test',
              status: 'delivered',
              status_code: 204,
              last_error: undefined,
              delivered_at: '2026-07-15T10:00:00Z',
            }),
          ),
      }),
    );
    await screen.findByText('https://hooks.example.it/arenile');
    await user.click(
      screen.getByRole('button', { name: 'Altre azioni per https://hooks.example.it/arenile' }),
    );
    await user.click(await screen.findByRole('menuitem', { name: 'Invia evento di prova' }));
    expect(
      await screen.findByText('Evento di prova consegnato a https://hooks.example.it/arenile'),
    ).toBeInTheDocument();
    expect(screen.getByText('Risposta HTTP 204')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${EPS}/w1/test`)).toHaveLength(1);
  });

  it('rotates the secret with the chosen grace period', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`POST ${EPS}/w1/rotate-secret`]: () =>
          json(200, {
            endpoint: endpoint({ previous_secret_expires_at: '2026-07-18T10:00:00Z' }),
            secret: 'whsec_N3W',
          }),
      }),
    );
    await screen.findByText('https://hooks.example.it/arenile');
    await user.click(
      screen.getByRole('button', { name: 'Altre azioni per https://hooks.example.it/arenile' }),
    );
    await user.click(await screen.findByRole('menuitem', { name: 'Ruota segreto' }));
    const dialog = await screen.findByRole('dialog', { name: 'Ruota segreto' });
    expect(within(dialog).getByLabelText('Periodo di grazia')).toHaveValue('24');
    await user.selectOptions(within(dialog).getByLabelText('Periodo di grazia'), '72');
    await user.click(within(dialog).getByRole('button', { name: 'Genera nuovo segreto' }));
    const secret = await screen.findByRole('dialog', { name: 'Nuovo segreto di firma' });
    expect(within(secret).getByLabelText('Segreto di firma')).toHaveValue('whsec_N3W');
    expect(await bodyOf(callsTo(fetchMock, 'POST', `${EPS}/w1/rotate-secret`)[0])).toEqual({
      grace_hours: 72,
    });
  });

  it('shows the delivery log with filter, pagination, detail and redelivery', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`GET ${EPS}/w1/deliveries`]: (req: Request) => {
          const p = new URL(req.url).searchParams;
          if (p.get('cursor') === 'c2') return list([delivery('d3', { status: 'delivered' })]);
          if (p.get('status') === 'pending') return list([]);
          return json(200, {
            items: [delivery('d1'), delivery('d2', { status: 'pending', failed_at: undefined })],
            next_cursor: 'c2',
          });
        },
        [`GET ${EPS}/w1/deliveries/d1`]: () =>
          json(200, {
            ...delivery('d1'),
            request_body: { id: 'evt_d1', type: 'booking.confirmed', data: { code: 'AR-1' } },
            response_excerpt: 'Internal Server Error',
          }),
        [`GET ${EPS}/w1/deliveries/d1/attempts`]: () =>
          list([
            {
              id: 'a2',
              delivery_id: 'd1',
              attempt: 2,
              status_code: 500,
              error: 'risposta 500',
              duration_ms: 87,
              attempted_at: '2026-07-01T10:05:00Z',
            },
            {
              id: 'a1',
              delivery_id: 'd1',
              attempt: 1,
              error: 'timeout',
              duration_ms: 10000,
              attempted_at: '2026-07-01T10:00:00Z',
            },
          ]),
        [`POST ${EPS}/w1/deliveries/d1/redeliver`]: () =>
          json(202, delivery('d1', { status: 'pending' })),
      }),
    );
    await screen.findByText('https://hooks.example.it/arenile');
    await user.click(
      screen.getByRole('button', { name: 'Registro consegne di https://hooks.example.it/arenile' }),
    );
    const sheet = await screen.findByRole('dialog', { name: 'Registro consegne' });
    const table = await within(sheet).findByRole('table', { name: 'Registro consegne' });
    await within(table).findAllByText('Prenotazione confermata');
    expect(within(table).getByText('Fallito')).toBeInTheDocument();
    expect(within(table).getByText('In corso')).toBeInTheDocument();
    // A pending delivery cannot be queued again.
    const redeliverButtons = within(table).getAllByRole('button', {
      name: 'Reinvia la consegna booking.confirmed',
    });
    expect(redeliverButtons[0]).toBeEnabled();
    expect(redeliverButtons[1]).toBeDisabled();

    await user.click(within(sheet).getByRole('button', { name: 'Carica altre' }));
    expect(await within(table).findByText('Consegnato')).toBeInTheDocument();
    expect(
      new URL(
        callsTo(fetchMock, 'GET', `${EPS}/w1/deliveries`).at(-1)?.url ?? 'http://x',
      ).searchParams.get('cursor'),
    ).toBe('c2');

    await user.click(
      within(table).getAllByRole('button', {
        name: 'Dettaglio della consegna booking.confirmed',
      })[0] as HTMLElement,
    );
    const detail = await screen.findByRole('dialog', { name: 'Dettaglio consegna' });
    expect(await within(detail).findByText(/"code": "AR-1"/)).toBeInTheDocument();
    expect(within(detail).getByText('Internal Server Error')).toBeInTheDocument();
    // Every attempt, not only the last one (ADR 0019 F).
    const attempts = await within(detail).findByRole('table', { name: 'Storico dei tentativi' });
    expect(within(attempts).getByText('timeout')).toBeInTheDocument();
    expect(within(attempts).getByText('risposta 500')).toBeInTheDocument();
    await user.click(within(detail).getByRole('button', { name: 'Reinvia' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${EPS}/w1/deliveries/d1/redeliver`)).toHaveLength(1),
    );
    await user.click(within(detail).getAllByRole('button', { name: 'Chiudi' })[0] as HTMLElement);

    await user.selectOptions(within(sheet).getByLabelText('Esito'), 'pending');
    expect(await within(sheet).findByText('Nessuna consegna')).toBeInTheDocument();
  });

  it('is reserved to the owner', async () => {
    const { fetchMock } = renderApp(PATH, routes('manager'));
    expect(
      await screen.findByText('Solo il titolare può gestire le integrazioni.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', EPS)).toHaveLength(0);
  });
});
