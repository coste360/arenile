import { render, screen, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { printData, ticket } from './testing';
import { TicketPrint } from './TicketPrint';
import type { TicketPrintData } from './api';

function renderTicket(data: TicketPrintData) {
  const i18n = createAdminTestI18n();
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
  );
  return render(<TicketPrint data={data} timeZone="Europe/Rome" locale="it" />, {
    wrapper: Wrapper,
  });
}

describe('TicketPrint', () => {
  it('lays out the 80 mm receipt (snapshot)', () => {
    const { container } = renderTicket(printData());
    const paper = screen.getByRole('article', { name: 'Biglietto #037' });
    expect(paper).toHaveAttribute('data-format', 'thermal_80');
    expect(paper.className).toContain('w-[72mm]');
    expect(paper.className).toContain('font-mono');
    expect(container.firstChild).toMatchSnapshot();
  });

  it('lays out the A4 sheet (snapshot)', () => {
    const { container } = renderTicket(printData({ format: 'a4' }));
    const paper = screen.getByRole('article', { name: 'Biglietto #037' });
    expect(paper.className).toContain('max-w-[180mm]');
    expect(paper.className).not.toContain('font-mono');
    expect(container.firstChild).toMatchSnapshot();
  });

  it('prints everything the desk and the gate need', () => {
    renderTicket(printData({ format: 'thermal_58' }));
    const paper = screen.getByRole('article', { name: 'Biglietto #037' });
    const t = within(paper);
    expect(paper.className).toContain('w-[48mm]');
    expect(t.getByText('Lido Demo')).toBeInTheDocument();
    expect(t.getByText('Lido Demo SRL · P. IVA IT01234567890')).toBeInTheDocument();
    expect(t.getByText('#037')).toBeInTheDocument();
    expect(t.getByText('Postazione A1')).toBeInTheDocument();
    expect(t.getByText('Giornata intera')).toBeInTheDocument();
    expect(t.getByText('Ombrellone + 2 lettini')).toBeInTheDocument();
    expect(t.getByText('1 × Lettino aggiuntivo')).toBeInTheDocument();
    expect(t.getByText('IVA 22%')).toBeInTheDocument();
    expect(t.getByText('Contanti')).toBeInTheDocument();
    expect(t.getByRole('img', { name: 'Codice QR del biglietto AR-7K3Q9' })).toHaveAttribute(
      'data-qr',
      'AR-7K3Q9',
    );
    expect(t.getByText('Grazie e buona giornata!')).toBeInTheDocument();
    expect(t.queryByText('Ristampa')).toBeNull();
  });

  it('marks reprints, voided tickets and zone-first tickets still to assign', () => {
    renderTicket(
      printData({
        reprint: true,
        ticket: ticket({
          status: 'voided',
          state: 'voided',
          spot_id: undefined,
          spot_label: undefined,
          needs_assignment: true,
        }),
      }),
    );
    expect(screen.getByText('Ristampa')).toBeInTheDocument();
    expect(screen.getByText('Annullato')).toBeInTheDocument();
    expect(screen.getByText('Prima fila (da assegnare)')).toBeInTheDocument();
  });
});
