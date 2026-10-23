import { ApiProblem } from '@arenile/shared/api';
import { render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import type { BookingQuote } from './api';
import { BookingProblem } from './problem';
import { QuotePreview } from './QuotePreview';

function quote(overrides: Partial<BookingQuote> = {}): BookingQuote {
  return {
    breakdown: {
      engine_version: 1,
      currency: 'EUR',
      lines: [
        {
          pack: 'standard',
          zone: 'prima-fila',
          spot: 'A12',
          days: [
            { date: '2026-07-14', slots: 'FULL', base: 3000, applied: [], total: 3000 },
            { date: '2026-07-15', slots: 'FULL', base: 3000, applied: [], total: 3000 },
          ],
          extras: [{ code: 'lettino', qty: 1, total: 1000 }],
          total: 7000,
        },
      ],
      discounts: [{ source: 'promo ESTATE', amount: 500 }],
      rounding: 0,
      total: 6500,
      vat: [{ rate: '22', taxable: 5328, tax: 1172 }],
    },
    deposit_due: 1950,
    policy: {
      rules: {
        payment: {
          mode: 'deposit',
          deposit: { type: 'percent', value: 30 },
          balance_due: 'on_arrival',
        },
        cancellation: [
          { until_days_before: 2, refund_percent: 50, as: 'credit' },
          { until_days_before: 7, refund_percent: 100 },
        ],
      },
      violations: [],
    },
    available: true,
    conflicts: [],
    ...overrides,
  };
}

function renderQuote(props: Partial<Parameters<typeof QuotePreview>[0]>) {
  return render(
    <I18nextProvider i18n={createAdminTestI18n()}>
      <QuotePreview quote={undefined} loading={false} error={null} {...props} />
    </I18nextProvider>,
  );
}

describe('QuotePreview', () => {
  it('renders total, deposit, lines, extras, discounts, VAT and the policy terms', () => {
    renderQuote({ quote: quote() });
    expect(screen.getByTestId('quote-total')).toHaveTextContent('65,00 €');
    expect(screen.getByTestId('quote-deposit')).toHaveTextContent('19,50 €');
    const lines = screen.getByRole('list', { name: 'Dettaglio' });
    expect(within(lines).getByText(/A12 · standard/)).toBeInTheDocument();
    expect(within(lines).getByText(/2 giorni/)).toBeInTheDocument();
    expect(within(lines).getByText('lettino × 1')).toBeInTheDocument();
    expect(within(lines).getByText('Sconto promo ESTATE')).toBeInTheDocument();
    expect(screen.getByText('IVA 22%: 11,72 €')).toBeInTheDocument();
    expect(screen.getByText(/Acconto del 30%\. Saldo all'arrivo\./)).toBeInTheDocument();
    const tiers = screen.getByRole('list', { name: 'Cancellazione' });
    // Longest notice first.
    expect(
      within(tiers)
        .getAllByRole('listitem')
        .map((li) => li.textContent),
    ).toEqual([
      'Fino a 7 giorni prima: 100% come rimborso',
      'Fino a 2 giorni prima: 50% come credito',
    ]);
  });

  it('shows unavailability with the conflicting dates and the violations', () => {
    renderQuote({
      quote: quote({
        available: false,
        conflicts: [{ spot_id: 's1', date: '2026-07-15', slot: 'PM', reason: 'spot_unavailable' }],
        policy: { rules: {}, violations: [{ code: 'lead_time_too_short' }] },
      }),
      resourceLabel: () => 'A12',
    });
    expect(screen.getByText('Selezione non disponibile')).toBeInTheDocument();
    expect(screen.getByText('A12: 15/07/2026 (Pomeriggio)')).toBeInTheDocument();
    expect(screen.getByText('Anticipo minimo non rispettato.')).toBeInTheDocument();
    expect(screen.getByText(/Nessun rimborso previsto/)).toBeInTheDocument();
  });

  it('maps a quote error to a friendly message', () => {
    const raw = { type: 'no_price', title: 'x', status: 422 };
    renderQuote({ error: new BookingProblem(new ApiProblem(raw as never), raw) });
    expect(screen.getByText('Manca un prezzo per le date o il pack scelti.')).toBeInTheDocument();
  });

  it('invites to complete the selection, or shows a skeleton while loading', () => {
    const { unmount } = renderQuote({});
    expect(screen.getByText(/Scegli date, pack e postazioni/)).toBeInTheDocument();
    unmount();
    renderQuote({ loading: true });
    expect(screen.getByRole('status')).toHaveTextContent('Calcolo del prezzo…');
  });
});
