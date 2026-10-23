import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { json, problem } from '../../configuration/testing';
import type { BookingDetail } from '../api';
import { bodyOf, bookingDetail, estPath, renderWithApi } from '../testing';
import { CancelBookingDialog } from './CancelBookingDialog';

const booking = bookingDetail() as unknown as BookingDetail;
const CANCEL = estPath(`/bookings/${booking.id}/cancel`);

function refund(amount: number, as = 'refund') {
  return {
    paid_amount: 5000,
    refund_percent: Math.round((amount / 5000) * 100),
    amount,
    as,
    tier: { until_days_before: 7, refund_percent: 100 },
  };
}

function setup(handler: (req: Request) => Promise<Response> | Response, canRefund = true) {
  const onOpenChange = vi.fn();
  const utils = renderWithApi(
    <CancelBookingDialog
      establishmentId="11111111-1111-4111-8111-111111111111"
      booking={booking}
      open
      onOpenChange={onOpenChange}
      canRefund={canRefund}
      onReload={vi.fn()}
    />,
    { [`POST ${CANCEL}`]: handler },
  );
  return { ...utils, onOpenChange, user: userEvent.setup() };
}

const requests = (fetchMock: ReturnType<typeof vi.fn>) =>
  fetchMock.mock.calls
    .map((c) => c[0] as Request)
    .filter((r) => new URL(r.url).pathname === CANCEL);

describe('CancelBookingDialog', () => {
  it('previews the policy refund with a dry run, then cancels with If-Match', async () => {
    const { fetchMock, user, onOpenChange } = setup(async (req) => {
      const body = (await bodyOf(req)) as { dry_run?: boolean };
      return json(200, { applied: !body.dry_run, booking, refund: refund(5000) });
    });
    expect(await screen.findByTestId('refund-amount')).toHaveTextContent('50,00 € · rimborso');
    expect(screen.getByText('Fino a 7 giorni prima: 100% come rimborso')).toBeInTheDocument();
    const [preview] = requests(fetchMock);
    expect(await bodyOf(preview!)).toEqual({
      settlement: 'policy',
      refund_method: 'cash',
      dry_run: true,
    });
    expect(preview!.headers.get('If-Match')).toBe('"4"');

    await user.type(screen.getByLabelText(/Motivo/), 'Maltempo previsto');
    await user.click(screen.getByRole('button', { name: 'Cancella prenotazione' }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const last = requests(fetchMock).at(-1)!;
    expect(await bodyOf(last)).toEqual({
      settlement: 'policy',
      refund_method: 'cash',
      reason: 'Maltempo previsto',
    });
  });

  it('forces a credit of another amount', async () => {
    const { fetchMock, user } = setup(async (req) => {
      const body = (await bodyOf(req)) as { dry_run?: boolean };
      return json(200, { applied: !body.dry_run, booking, refund: refund(2500) });
    });
    await screen.findByTestId('refund-amount');
    await user.click(screen.getByRole('combobox', { name: 'Rimborso' }));
    await user.click(await screen.findByRole('option', { name: 'Credito al cliente' }));
    const amount = await screen.findByLabelText('Importo');
    // Prefilled with the policy amount.
    expect(amount).toHaveValue('25,00');
    await user.clear(amount);
    await user.type(amount, '30');
    await user.click(screen.getByRole('button', { name: 'Cancella prenotazione' }));
    await waitFor(() =>
      expect(requests(fetchMock).some((r) => r.headers.get('content-length') !== '0')).toBe(true),
    );
    const bodies = await Promise.all(requests(fetchMock).map(bodyOf));
    expect(bodies.at(-1)).toEqual({ settlement: 'credit', refund_amount: 3000 });
  });

  it('refuses a refund above the paid amount without calling the server', async () => {
    const { fetchMock, user } = setup(async () =>
      json(200, { applied: false, booking, refund: refund(5000) }),
    );
    await screen.findByTestId('refund-amount');
    await user.click(screen.getByRole('combobox', { name: 'Rimborso' }));
    await user.click(await screen.findByRole('option', { name: 'Rimborso di un importo diverso' }));
    const amount = await screen.findByLabelText('Importo');
    await user.clear(amount);
    await user.type(amount, '80');
    const before = requests(fetchMock).length;
    await user.click(screen.getByRole('button', { name: 'Cancella prenotazione' }));
    expect(await screen.findByText("L'importo supera quello consentito.")).toBeInTheDocument();
    // Only the dry-run previews (re-run on the settlement change), no real cancel.
    const bodies = await Promise.all(requests(fetchMock).slice(before).map(bodyOf));
    expect(bodies.every((b) => (b as { dry_run?: boolean }).dry_run)).toBe(true);
  });

  it('offers only credit or nothing without refunds.issue', async () => {
    const { user } = setup(
      async () => json(200, { applied: false, booking, refund: refund(5000, 'credit') }),
      false,
    );
    await screen.findByTestId('refund-amount');
    await user.click(screen.getByRole('combobox', { name: 'Rimborso' }));
    const options = (await screen.findAllByRole('option')).map((o) => o.textContent);
    expect(options).toEqual(['Credito al cliente', 'Nessun rimborso']);
  });

  it('explains a state change made by someone else', async () => {
    setup(() => problem(409, 'invalid_transition', { current_status: 'checked_in' }));
    expect(
      await screen.findByText('Operazione non ammessa: ora la prenotazione è «Presente».'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ricarica' })).toBeInTheDocument();
  });
});
