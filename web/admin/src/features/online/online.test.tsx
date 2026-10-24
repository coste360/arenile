import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, problem, renderApp } from '../configuration/testing';
import {
  nextTermsVersion,
  toOnlineInput,
  toOnlineValues,
  toTermsInput,
} from './pages/online.schema';
import { paymentSchema, toPaymentInput, toPaymentValues } from './pages/payments.schema';

const API = `/api/admin/v1/establishments/${EST_ID}`;

function onlineSettings(overrides: Record<string, unknown> = {}) {
  return {
    booking_enabled: true,
    allowed_embed_origins: ['https://www.lido.it'],
    theme: {
      primary_color: '#0e6ba8',
      accent_color: '#f2b134',
      font_family: 'Inter',
      radius: 'md',
      show_prices_on_map: true,
      hide_header_in_embed: true,
      custom_texts: { it: { intro: 'Benvenuti', terms: 'legacy' } },
    },
    flow: {
      formulas: ['day', 'multi_day'],
      slots: ['FULL', 'AM', 'PM'],
      show_rate_periods: false,
      promo_codes_enabled: true,
      require_phone: false,
    },
    embed: {
      page_url: 'https://prenota.example/b/lido-demo',
      iframe: '<iframe src="https://prenota.example/b/lido-demo?embed=1"></iframe>',
      script:
        '<div data-arenile="lido-demo"></div>\n<script src="https://prenota.example/embed.js" async></script>',
    },
    version: 7,
    ...overrides,
  };
}

const TERMS = {
  id: 't1',
  version: '2027-04-01',
  texts: { it: { terms: 'Condizioni 2027', privacy: 'Privacy' } },
  published_at: '2027-04-01T08:00:00Z',
  current: true,
};

function paymentSettings(overrides: Record<string, unknown> = {}) {
  return {
    provider: 'stripe',
    available_providers: ['manual', 'stripe'],
    mode: 'test',
    stripe: {
      configured: true,
      publishable_key: 'pk_test_abc',
      secret_key_last4: '4242',
      webhook_secret_configured: false,
      embedded_mode: false,
    },
    webhook_url: `https://prenota.example/webhooks/payments/stripe/${EST_ID}`,
    live_payments_blocked: false,
    version: 2,
    ...overrides,
  };
}

function emailSettings(overrides: Record<string, unknown> = {}) {
  return {
    transport: 'log',
    smtp_configured: false,
    mail_from: 'prenotazioni@arenile.example',
    sender_name: 'Lido Demo',
    manager_email: 'gestore@lido.it',
    manager_notifications: {
      new_online_booking: true,
      approval_requested: true,
      payment_failed: true,
      webhook_error: false,
    },
    customer_notifications: {
      confirmation: true,
      balance_reminder: true,
      day_before_reminder: false,
      cancellation: true,
      refund: true,
    },
    reminders: { day_before_time: '18:00', balance_time: '10:00', balance_lead_days: 2 },
    version: 1,
    ...overrides,
  };
}

describe('OnlineSettingsPage', () => {
  const PATH = `/admin/e/${EST_ID}/config/online`;
  const routes = (extra = {}) => ({
    ...baseRoutes('manager'),
    [`GET ${API}/online-settings`]: () => json(200, onlineSettings(), { ETag: '"7"' }),
    [`GET ${API}/terms-versions`]: () => json(200, { items: [TERMS] }),
    ...extra,
  });

  it('saves flow and theme with If-Match, keeping the other custom texts', async () => {
    const user = userEvent.setup();
    const bodies: Record<string, unknown>[] = [];
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`PUT ${API}/online-settings`]: async (req: Request) => {
          const body = (await req.json()) as Record<string, unknown>;
          bodies.push(body);
          return json(200, { ...onlineSettings(body), version: 8 });
        },
      }),
    );
    const booking = await screen.findByRole('region', { name: 'Prenotazioni online' });
    await user.click(within(booking).getByRole('switch', { name: /Accetta prenotazioni online/ }));
    await user.click(screen.getByRole('switch', { name: 'Campo codice promozionale' }));
    // Phone optional everywhere (ADR 0020): no switch to require it.
    expect(screen.queryByRole('switch', { name: 'Telefono obbligatorio' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Tema e anteprima' }));
    const primary = screen.getByLabelText('Colore principale', {
      selector: 'input:not([type=color])',
    });
    await user.clear(primary);
    await user.type(primary, '#ffe600');
    // The live preview is the real page in preview mode, same origin.
    const frame = screen.getByTitle('Anteprima della pagina pubblica');
    expect(frame.getAttribute('src')).toBe('/b/lido-demo?embed=1&preview=1&lang=it&theme=light');
    expect(screen.getByText(/Contrasto del colore principale/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', `${API}/online-settings`)).toHaveLength(1),
    );
    const [put] = callsTo(fetchMock, 'PUT', `${API}/online-settings`);
    expect(put!.headers.get('If-Match')).toBe('"7"');
    expect(bodies[0]).toMatchObject({
      booking_enabled: false,
      allowed_embed_origins: ['https://www.lido.it'],
      flow: { require_phone: false, formulas: ['day', 'multi_day'], slots: ['FULL', 'AM', 'PM'] },
      theme: {
        primary_color: '#ffe600',
        custom_texts: { it: { intro: 'Benvenuti', terms: 'legacy' } },
      },
    });
  });

  it('rejects embed origins with a path before calling the API', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes());
    const origins = await screen.findByLabelText('Siti autorizzati a incorporare la pagina');
    await user.type(origins, '\nhttps://evil.example/path');
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    expect(await screen.findByText(/Ogni riga deve essere un'origine/)).toBeInTheDocument();
    expect(callsTo(fetchMock, 'PUT', `${API}/online-settings`)).toHaveLength(0);
  });

  it('shows a conflict on a stale version and reloads it', async () => {
    const user = userEvent.setup();
    renderApp(
      PATH,
      routes({ [`PUT ${API}/online-settings`]: () => problem(412, 'version_conflict') }),
    );
    await user.click(await screen.findByRole('switch', { name: 'Campo codice promozionale' }));
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    expect(await screen.findByText(/Un altro utente ha modificato/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Ricarica/ })).toBeInTheDocument();
  });

  it('lists the terms versions and publishes a new one', async () => {
    const user = userEvent.setup();
    let posted: unknown;
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`POST ${API}/terms-versions`]: async (req: Request) => {
          posted = await req.json();
          return json(201, { ...TERMS, id: 't2', version: '2027-05-01' });
        },
      }),
    );
    await user.click(await screen.findByRole('tab', { name: 'Condizioni' }));
    const section = screen.getByRole('region', { name: 'Condizioni e informativa' });
    expect(await within(section).findByText('2027-04-01')).toBeInTheDocument();
    expect(within(section).getByText('Corrente')).toBeInTheDocument();
    await user.click(within(section).getByRole('button', { name: 'Nuova versione' }));
    const dialog = await screen.findByRole('dialog');
    const version = within(dialog).getByLabelText('Versione');
    await user.clear(version);
    await user.type(version, '2027-05-01');
    const terms = within(dialog).getAllByLabelText('Condizioni di prenotazione')[0]!;
    expect(terms).toHaveValue('Condizioni 2027');
    await user.type(terms, ' aggiornate');
    await user.click(within(dialog).getByRole('button', { name: 'Pubblica' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/terms-versions`)).toHaveLength(1),
    );
    expect(
      callsTo(fetchMock, 'POST', `${API}/terms-versions`)[0]!.headers.get('Idempotency-Key'),
    ).toBeTruthy();
    expect(posted).toEqual({
      version: '2027-05-01',
      texts: { it: { terms: 'Condizioni 2027 aggiornate', privacy: 'Privacy' } },
      make_current: true,
    });
  });

  it('shows the embed snippets from the server and the allowed origins', async () => {
    const user = userEvent.setup();
    renderApp(PATH, routes());
    await user.click(await screen.findByRole('tab', { name: 'Incorpora' }));
    expect(screen.getByLabelText(/Codice consigliato/)).toHaveValue(onlineSettings().embed.script);
    expect(screen.getByLabelText('In alternativa: iframe')).toHaveValue(
      onlineSettings().embed.iframe,
    );
    expect(screen.getByText(/Siti autorizzati: https:\/\/www\.lido\.it/)).toBeInTheDocument();
  });
});

describe('OnlineSettingsPage test keys warning (INV-88)', () => {
  const PATH = `/admin/e/${EST_ID}/config/online`;
  const routes = (blocked: boolean) => ({
    ...baseRoutes('owner'),
    [`GET ${API}/online-settings`]: () => json(200, onlineSettings(), { ETag: '"7"' }),
    [`GET ${API}/terms-versions`]: () => json(200, { items: [TERMS] }),
    [`GET ${API}/payment-settings`]: () =>
      json(200, paymentSettings({ live_payments_blocked: blocked })),
  });

  it('shows the banner with the shortcuts when online payments are blocked', async () => {
    renderApp(PATH, routes(true));
    const alert = (
      await screen.findByText('Pagamenti online bloccati per i clienti reali')
    ).closest('[role="alert"]') as HTMLElement;
    expect(within(alert).getByRole('link', { name: 'Pagamenti online' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/config/payments`,
    );
    expect(within(alert).getByRole('link', { name: 'Modalità prova' })).toBeInTheDocument();
  });

  it('shows nothing with live keys', async () => {
    const { fetchMock } = renderApp(PATH, routes(false));
    await screen.findByRole('region', { name: 'Prenotazioni online' });
    await waitFor(() =>
      expect(callsTo(fetchMock, 'GET', `${API}/payment-settings`)).toHaveLength(1),
    );
    expect(
      screen.queryByText('Pagamenti online bloccati per i clienti reali'),
    ).not.toBeInTheDocument();
  });
});

describe('PaymentSettingsPage', () => {
  const PATH = `/admin/e/${EST_ID}/config/payments`;

  it('shows the status and the webhook URL, sends only the secrets typed', async () => {
    const user = userEvent.setup();
    let body: unknown;
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/payment-settings`]: () => json(200, paymentSettings()),
      [`PUT ${API}/payment-settings`]: async (req) => {
        body = await req.json();
        return json(200, paymentSettings({ version: 3 }));
      },
    });
    expect(await screen.findByText('Chiave configurata ••••4242')).toBeInTheDocument();
    expect(screen.getByText('Segreto del webhook mancante')).toBeInTheDocument();
    expect(screen.getByLabelText("URL dell'endpoint da incollare in Stripe")).toHaveValue(
      `https://prenota.example/webhooks/payments/stripe/${EST_ID}`,
    );
    expect(screen.getByText('checkout.session.completed')).toBeInTheDocument();
    expect(
      screen.queryByText('Pagamenti online bloccati per i clienti reali'),
    ).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('Segreto di firma del webhook'), 'whsec_abc123');
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', `${API}/payment-settings`)).toHaveLength(1),
    );
    expect(callsTo(fetchMock, 'PUT', `${API}/payment-settings`)[0]!.headers.get('If-Match')).toBe(
      '"2"',
    );
    expect(body).toEqual({
      provider: 'stripe',
      mode: 'test',
      stripe: {
        publishable_key: 'pk_test_abc',
        embedded_mode: false,
        webhook_secret: 'whsec_abc123',
      },
    });
  });

  it('warns that test keys block real customers while test mode is off (INV-88)', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/payment-settings`]: () =>
        json(200, paymentSettings({ live_payments_blocked: true })),
    });
    const alert = (
      await screen.findByText('Pagamenti online bloccati per i clienti reali')
    ).closest('[role="alert"]') as HTMLElement;
    expect(alert).toBeInTheDocument();
    expect(within(alert).getByText(/Inserisci le chiavi live/)).toBeInTheDocument();
    expect(within(alert).getByRole('link', { name: 'Modalità prova' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/config/test-mode`,
    );
    // Already on the payment settings: no link to themselves.
    expect(within(alert).queryByRole('link', { name: 'Pagamenti online' })).not.toBeInTheDocument();
  });

  it('refuses a live key in test mode', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/payment-settings`]: () => json(200, paymentSettings()),
    });
    await user.type(await screen.findByLabelText('Chiave segreta'), 'sk_live_xyz');
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    expect(
      await screen.findByText('La chiave non corrisponde alla modalità scelta'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'PUT', `${API}/payment-settings`)).toHaveLength(0);
  });

  it('checks the saved credentials with the provider', async () => {
    const user = userEvent.setup();
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/payment-settings`]: () => json(200, paymentSettings()),
      [`POST ${API}/payment-settings/test`]: () =>
        json(200, { ok: true, provider: 'stripe', mode: 'test', account_label: 'Lido Demo SRL' }),
    });
    await user.click(await screen.findByRole('button', { name: 'Verifica credenziali' }));
    expect(await screen.findByText('Credenziali valide')).toBeInTheDocument();
    expect(screen.getByText('Lido Demo SRL')).toBeInTheDocument();
  });

  it('is read-only without payments.configure (manager)', async () => {
    renderApp(PATH, {
      ...baseRoutes('manager'),
      [`GET ${API}/payment-settings`]: () => json(200, paymentSettings()),
    });
    expect(await screen.findByText(/consente solo la consultazione/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Salva' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Chiave pubblicabile')).toBeDisabled();
  });
});

describe('EmailSettingsPage', () => {
  const PATH = `/admin/e/${EST_ID}/config/email`;
  const routes = (extra = {}) => ({
    ...baseRoutes('manager'),
    [`GET ${API}/email-settings`]: () => json(200, emailSettings()),
    [`GET ${API}/messages`]: () =>
      json(200, {
        items: [
          {
            id: 'm1',
            channel: 'email',
            template: 'booking_confirmed',
            locale: 'it',
            status: 'failed',
            attempts: 3,
            last_error: 'smtp timeout',
            booking_code: 'AR-7Q2K',
            recipient_masked: 'm***a@e***.it',
            created_at: '2027-07-01T08:00:00Z',
          },
        ],
      }),
    ...extra,
  });

  it('explains the transport, lists messages with masked recipients', async () => {
    renderApp(PATH, routes());
    expect(await screen.findByText('Nessun SMTP: email nel log')).toBeInTheDocument();
    expect(screen.getByText(/Imposta ARENILE_SMTP_URL e ARENILE_MAIL_FROM/)).toBeInTheDocument();
    const log = screen.getByRole('region', { name: 'Registro degli invii' });
    expect(await within(log).findByText('m***a@e***.it')).toBeInTheDocument();
    expect(within(log).getByText('Conferma prenotazione')).toBeInTheDocument();
    expect(within(log).getByRole('cell', { name: /Non riuscita/ })).toBeInTheDocument();
    expect(within(log).getByText(/3 tentativi · smtp timeout/)).toBeInTheDocument();
  });

  it('saves sender and notifications with If-Match and sends a test email', async () => {
    const user = userEvent.setup();
    let body: unknown;
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`PUT ${API}/email-settings`]: async (req: Request) => {
          body = await req.json();
          return json(200, emailSettings({ version: 2 }));
        },
        [`POST ${API}/email-settings/test`]: () => json(202, { message_id: 'm2' }),
      }),
    );
    const reply = await screen.findByLabelText('Rispondi a');
    await user.type(reply, 'info@lido.it');
    // Reminders live in "Email ai clienti": no switch here, a link instead.
    expect(screen.queryByRole('switch', { name: 'Promemoria il giorno prima' })).toBeNull();
    expect(screen.getByRole('link', { name: /si configurano/ })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/config/customer-emails`,
    );
    await user.click(screen.getByRole('switch', { name: 'Cancellazione' }));
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PUT', `${API}/email-settings`)).toHaveLength(1));
    expect(callsTo(fetchMock, 'PUT', `${API}/email-settings`)[0]!.headers.get('If-Match')).toBe(
      '"1"',
    );
    expect(body).toMatchObject({
      sender_name: 'Lido Demo',
      reply_to: 'info@lido.it',
      manager_email: 'gestore@lido.it',
      customer_notifications: { cancellation: false },
    });
    await user.type(screen.getByLabelText('Destinatario'), 'prova@lido.it');
    await user.click(screen.getByRole('button', { name: 'Invia prova' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/email-settings/test`)).toHaveLength(1),
    );
    expect(await callsTo(fetchMock, 'POST', `${API}/email-settings/test`)[0]!.json()).toEqual({
      to: 'prova@lido.it',
    });
  });
});

describe('online schemas', () => {
  it('normalises and de-duplicates origins and keeps legacy custom texts', () => {
    const s = onlineSettings() as never;
    const values = toOnlineValues(s, ['it', 'en']);
    values.allowed_embed_origins =
      'https://www.lido.it/\nhttps://www.lido.it\nhttps://shop.lido.it:8443';
    values.intro = { it: '', en: 'Welcome' };
    const input = toOnlineInput(values, s);
    expect(input.allowed_embed_origins).toEqual([
      'https://www.lido.it',
      'https://shop.lido.it:8443',
    ]);
    expect(input.theme.custom_texts).toEqual({ it: { terms: 'legacy' }, en: { intro: 'Welcome' } });
  });

  it('keeps the map kit chosen in the establishment settings (ADR 0024)', () => {
    const base = onlineSettings() as unknown as { theme: Record<string, unknown> };
    const s = { ...base, theme: { ...base.theme, map_kit: 'classic' } } as never;
    const input = toOnlineInput(toOnlineValues(s, ['it']), s);
    expect(input.theme.map_kit).toBe('classic');
  });

  it('builds terms versions without empty optional texts', () => {
    expect(
      toTermsInput({
        version: ' v2 ',
        make_current: false,
        texts: {
          it: { terms: 'Testo', privacy: '', marketing: 'News' },
          en: { terms: '', privacy: 'x', marketing: '' },
        },
      }),
    ).toEqual({
      version: 'v2',
      texts: { it: { terms: 'Testo', marketing: 'News' } },
      make_current: false,
    });
    expect(nextTermsVersion(['2027-05-01', '2027-05-01.2'], '2027-05-01')).toBe('2027-05-01.3');
    expect(nextTermsVersion([], '2027-05-01')).toBe('2027-05-01');
  });

  it('payment input: absent secret keeps it, the clear flag removes it, manual has no keys', () => {
    const values = toPaymentValues(paymentSettings() as never);
    expect(toPaymentInput(values).stripe).toEqual({
      publishable_key: 'pk_test_abc',
      embedded_mode: false,
    });
    expect(toPaymentInput({ ...values, clear_secret_key: true }).stripe).toMatchObject({
      secret_key: '',
    });
    expect(toPaymentInput({ ...values, provider: 'manual' })).toEqual({
      provider: 'manual',
      mode: 'test',
    });
    expect(paymentSchema.safeParse({ ...values, publishable_key: 'pk_live_1' }).success).toBe(
      false,
    );
    expect(
      paymentSchema.safeParse({ ...values, mode: 'live', publishable_key: 'pk_live_1' }).success,
    ).toBe(true);
  });
});
