import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../../configuration/testing';
import { bodyOf, bookingDetail, CUSTOMER, estPath } from '../testing';

/** Spec 30.9: duplicate, change customer, WhatsApp summary, printable summary, undo. */

const detail = bookingDetail();

function routes(extra: Record<string, Handler> = {}, booking: unknown = detail) {
  return {
    ...baseRoutes('cashier'),
    [`GET ${estPath('/maps')}`]: () => list([]),
    [`GET ${estPath('/zones')}`]: () => list([]),
    [`GET ${estPath('/packs')}`]: () => list([]),
    [`GET ${estPath('/extras')}`]: () => list([]),
    [`GET ${estPath(`/bookings/${detail.id}`)}`]: () => json(200, booking),
    ...extra,
  };
}

async function openMore(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole('heading', { name: /AR-7K3Q9/, level: 1 });
  await user.click(screen.getByRole('button', { name: 'Altre azioni' }));
}

describe('BookingDeskActions', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-15T10:00:00'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('duplicates the booking on new dates after a dry run', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(
      `/e/${EST_ID}/prenotazioni/${detail.id}`,
      routes({
        [`POST ${estPath(`/bookings/${detail.id}/duplicate`)}`]: async (req) => {
          const b = (await bodyOf(req)) as { dry_run?: boolean };
          bodies.push(b);
          return b.dry_run
            ? json(200, { applied: false, total: 12000, conflicts: [] })
            : json(201, {
                applied: true,
                total: 12000,
                conflicts: [],
                booking: { ...detail, id: 'b-new', code: 'AR-NEW1' },
              });
        },
      }),
    );
    await openMore(user);
    await user.click(await screen.findByRole('menuitem', { name: 'Duplica' }));
    const dialog = await screen.findByRole('dialog', { name: 'Duplica AR-7K3Q9' });
    await user.click(within(dialog).getByRole('button', { name: /^Nuove date/ }));
    await user.click(await screen.findByRole('button', { name: /20 luglio 2026/ }));
    await user.click(screen.getByRole('button', { name: /22 luglio 2026/ }));
    await user.keyboard('{Escape}');
    await user.click(within(dialog).getByRole('button', { name: 'Verifica' }));
    expect(await within(dialog).findByTestId('duplicate-total')).toHaveTextContent('120,00 €');
    await user.click(within(dialog).getByRole('button', { name: 'Crea la prenotazione' }));
    await waitFor(() => expect(bodies).toHaveLength(2));
    expect(bodies[1]).toEqual({ starts_on: '2026-07-20', ends_on: '2026-07-22', dry_run: false });
    const sent = callsTo(fetchMock, 'POST', estPath(`/bookings/${detail.id}/duplicate`));
    expect(sent[1]?.headers.get('Idempotency-Key')).toBeTruthy();
  });

  it('opens the WhatsApp summary only for a customer with a phone', async () => {
    const user = userEvent.setup();
    renderApp(
      `/e/${EST_ID}/prenotazioni/${detail.id}`,
      routes({
        [`POST ${estPath(`/bookings/${detail.id}/share-summary`)}`]: () =>
          json(200, {
            text: 'Prenotazione AR-7K3Q9 dal 14/07 al 16/07',
            locale: 'it',
            whatsapp_url: 'https://wa.me/393331234567?text=Prenotazione',
          }),
      }),
    );
    await openMore(user);
    expect(screen.getByRole('menuitem', { name: 'Stampa riepilogo' })).toHaveAttribute(
      'href',
      `/api/admin/v1/establishments/${EST_ID}/bookings/${detail.id}/summary-print?paper=a4`,
    );
    await user.click(screen.getByRole('menuitem', { name: 'Riepilogo via WhatsApp' }));
    const dialog = await screen.findByRole('dialog', { name: 'Riepilogo di AR-7K3Q9' });
    expect(await within(dialog).findByRole('textbox', { name: 'Testo' })).toHaveValue(
      'Prenotazione AR-7K3Q9 dal 14/07 al 16/07',
    );
    const link = within(dialog).getByRole('link', { name: 'Apri WhatsApp' });
    expect(link).toHaveAttribute('href', 'https://wa.me/393331234567?text=Prenotazione');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('hides the WhatsApp summary without a phone', async () => {
    const user = userEvent.setup();
    renderApp(
      `/e/${EST_ID}/prenotazioni/${detail.id}`,
      routes({}, bookingDetail({ customer: { ...CUSTOMER, phone: '' } })),
    );
    await openMore(user);
    expect(screen.getByRole('menuitem', { name: 'Duplica' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Riepilogo via WhatsApp' })).toBeNull();
  });

  it('changes the customer with a preview of the price difference', async () => {
    const user = userEvent.setup();
    const other = { ...CUSTOMER, id: 'c-other', display_name: 'Verdi Luca', phone: '' };
    const patches: { body: unknown; dry: string | null; ifMatch: string | null }[] = [];
    renderApp(
      `/e/${EST_ID}/prenotazioni/${detail.id}`,
      routes({
        [`GET ${estPath('/customers')}`]: () => list([other]),
        [`PATCH ${estPath(`/bookings/${detail.id}`)}`]: async (req) => {
          const dry = new URL(req.url).searchParams.get('dry_run');
          patches.push({ body: await bodyOf(req), dry, ifMatch: req.headers.get('If-Match') });
          return json(200, {
            applied: dry !== 'true',
            booking: detail,
            previous_total: 9000,
            new_total: 8000,
            price_difference: -1000,
            conflicts: [],
          });
        },
      }),
    );
    await openMore(user);
    await user.click(screen.getByRole('menuitem', { name: 'Cambia cliente' }));
    const dialog = await screen.findByRole('dialog', { name: 'Cambia il cliente di AR-7K3Q9' });
    await user.click(within(dialog).getByRole('combobox', { name: 'Nuovo cliente' }));
    await user.type(await screen.findByPlaceholderText('Cerca cliente'), 'Verdi');
    await user.click(await screen.findByRole('option', { name: /Verdi Luca/ }));
    await user.click(within(dialog).getByRole('button', { name: 'Verifica' }));
    expect(await within(dialog).findByTestId('change-customer-price')).toHaveTextContent(
      'Differenza da rimborsare: 10,00 €',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Cambia cliente' }));
    await waitFor(() => expect(patches).toHaveLength(2));
    expect(patches[1]).toEqual({ body: { customer_id: 'c-other' }, dry: null, ifMatch: '"4"' });
  });

  it('offers the undo of the last move for 5 minutes', async () => {
    const user = userEvent.setup();
    const expires = new Date(Date.now() + 3 * 60_000).toISOString();
    const { fetchMock } = renderApp(
      `/e/${EST_ID}/prenotazioni/${detail.id}`,
      routes(
        {
          [`POST ${estPath('/moves/mv1/undo')}`]: () => json(200, { undone: true, bookings: [] }),
        },
        bookingDetail({
          last_move: {
            move_id: 'mv1',
            kind: 'move',
            expires_at: expires,
            booking_ids: [detail.id],
          },
        }),
      ),
    );
    expect(
      await screen.findByText(
        'Questa prenotazione è stata appena spostata: puoi ancora annullare lo spostamento.',
      ),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Annulla' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/moves/mv1/undo'))).toHaveLength(1),
    );
  });
});
