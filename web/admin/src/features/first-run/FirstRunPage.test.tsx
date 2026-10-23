import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { baseRoutes, callsTo, json, meFor, problem, renderApp } from '../configuration/testing';
import { tokenFromHash } from './firstRun.schema';

const STATUS = 'GET /api/desktop/v1/first-run';
const CREATE = 'POST /api/desktop/v1/first-run';

async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Azienda o stabilimento'), 'Lido Azzurro srl');
  await user.type(screen.getByLabelText('Nome e cognome'), 'Anna Rossi');
  await user.type(screen.getByLabelText('Email'), 'anna@lido-azzurro.it');
  await user.type(screen.getByLabelText('Password'), 'spiaggia-sicura-2027');
  await user.type(screen.getByLabelText('Ripeti la password'), 'spiaggia-sicura-2027');
}

describe('FirstRunPage', () => {
  it('reads the token from the fragment, creates the owner and continues to the setup', async () => {
    const user = userEvent.setup();
    const { fetchMock, history } = renderApp('/admin/first-run#token=tok-123', {
      ...baseRoutes('owner'),
      [STATUS]: () => json(200, { required: true }),
      [CREATE]: () => json(201, meFor('owner')),
    });
    await fill(user);
    // The token is no longer in the address bar.
    expect(history.location.hash).toBe('');
    await user.click(screen.getByRole('button', { name: "Crea l'utente e continua" }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', '/api/desktop/v1/first-run')).toHaveLength(1),
    );
    const body = (await callsTo(
      fetchMock,
      'POST',
      '/api/desktop/v1/first-run',
    )[0]!.json()) as Record<string, string>;
    expect(body).toEqual({
      setup_token: 'tok-123',
      organization_name: 'Lido Azzurro srl',
      name: 'Anna Rossi',
      email: 'anna@lido-azzurro.it',
      password: 'spiaggia-sicura-2027',
    });
    await waitFor(() => expect(history.location.pathname).toBe('/admin/establishments/new'));
  });

  it('validates on the client before using the one-time token', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp('/admin/first-run#token=tok-123', {
      [STATUS]: () => json(200, { required: true }),
    });
    await user.type(await screen.findByLabelText('Password'), 'corta');
    await user.type(screen.getByLabelText('Ripeti la password'), 'diversa');
    await user.click(screen.getByRole('button', { name: "Crea l'utente e continua" }));
    expect(
      await screen.findByText('La password deve avere almeno 10 caratteri.'),
    ).toBeInTheDocument();
    expect(screen.getByText('Le due password non coincidono.')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', '/api/desktop/v1/first-run')).toHaveLength(0);
  });

  it('explains an expired link and server field errors', async () => {
    const user = userEvent.setup();
    let calls = 0;
    renderApp('/admin/first-run#token=old', {
      [STATUS]: () => json(200, { required: true }),
      [CREATE]: () =>
        ++calls === 1
          ? problem(400, 'validation_failed', {
              errors: [{ field: 'email', message: 'Inserisci un indirizzo email valido.' }],
            })
          : problem(403, 'setup_token_invalid'),
    });
    await fill(user);
    await user.click(screen.getByRole('button', { name: "Crea l'utente e continua" }));
    expect(await screen.findByText('Inserisci un indirizzo email valido.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: "Crea l'utente e continua" }));
    expect(await screen.findByText(/non è più valido/)).toBeInTheDocument();
  });

  it('without a token asks to open the page from the tray menu', async () => {
    renderApp('/admin/first-run', { [STATUS]: () => json(200, { required: true }) });
    expect(await screen.findByText(/Apri questa pagina dal menu di Arenile/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: "Crea l'utente e continua" })).toBeDisabled();
  });

  it('sends to the login once the owner exists', async () => {
    renderApp('/admin/first-run#token=x', { [STATUS]: () => json(200, { required: false }) });
    expect(await screen.findByText('Configurazione iniziale completata')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: "Vai all'accesso" })).toHaveAttribute(
      'href',
      '/admin/login',
    );
  });
});

describe('tokenFromHash', () => {
  it.each([
    ['#token=abc', 'abc'],
    ['token=abc&x=1', 'abc'],
    ['', ''],
    ['#other=1', ''],
  ])('%s -> %s', (hash, want) => expect(tokenFromHash(hash)).toBe(want));
});
