import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { BASE, json, problem, renderWithEnv } from '../../test-utils';
import AgreementBanner from './AgreementBanner';
import { AgreementCode } from './AgreementCode';

const MIRAMARE = {
  code: 'MIRAMARE',
  name: 'Ospiti Miramare',
  partner_name: 'Hotel Miramare',
  summary: '-10% sui giornalieri',
  valid_from: '2099-06-01',
  valid_to: '2099-09-15',
  requirement: 'conferma dell’hotel',
};

describe('AgreementBanner', () => {
  it('shows the agreement of the link with partner, condition, validity and requirement', async () => {
    const { fetchMock } = renderWithEnv(<AgreementBanner code="MIRAMARE" />, {
      [`GET ${BASE}/agreements/MIRAMARE`]: () => json(200, MIRAMARE),
    });
    expect(
      await screen.findByText('Convenzione Ospiti Miramare · Hotel Miramare'),
    ).toBeInTheDocument();
    expect(screen.getByText('-10% sui giornalieri')).toBeInTheDocument();
    expect(screen.getByText('Valida dal 01/06/2099 al 15/09/2099.')).toBeInTheDocument();
    expect(screen.getByText('Da presentare in cassa: conferma dell’hotel.')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('says that an unknown code is not valid without revealing anything else', async () => {
    renderWithEnv(<AgreementBanner code="NOPE" />, {
      [`GET ${BASE}/agreements/NOPE`]: () => problem(404, 'not_found'),
    });
    expect(
      await screen.findByText(/Il codice convenzione NOPE non è valido o non è più attivo/),
    ).toBeInTheDocument();
  });
});

describe('AgreementCode', () => {
  it('applies and removes a code and shows the outcome of the quote', async () => {
    const onApply = vi.fn();
    const user = userEvent.setup();
    const { rerender } = renderWithEnv(<AgreementCode value="" onApply={onApply} />, {});
    await user.type(screen.getByLabelText('Codice convenzione'), 'res');
    await user.click(screen.getByRole('button', { name: 'Applica il codice convenzione' }));
    expect(onApply).toHaveBeenCalledWith('res');

    rerender(
      <AgreementCode
        value="RES"
        onApply={onApply}
        status={{ code: 'res', status: 'applied', name: 'Residenti', requirement: 'documento' }}
      />,
    );
    expect(
      screen.getByText('Convenzione Residenti applicata. Da presentare in cassa: documento.'),
    ).toBeInTheDocument();
    rerender(
      <AgreementCode
        value="RES"
        onApply={onApply}
        status={{ code: 'RES', status: 'quota_exhausted' }}
      />,
    );
    expect(screen.getByText(/Posti in convenzione esauriti/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Rimuovi il codice convenzione' }));
    expect(onApply).toHaveBeenLastCalledWith('');
  });
});
