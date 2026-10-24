import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, problem, renderApp } from '../configuration/testing';
import {
  insertAt,
  templateSchema,
  timeOptions,
  toRemindersInput,
  unknownPlaceholders,
} from './pages/customerEmails.schema';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/config/customer-emails`;
const PLACEHOLDERS = [
  'nome',
  'cognome',
  'codice',
  'data',
  'postazione',
  'stabilimento',
  'importo',
  'saldo',
  'scadenza',
];

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
    version: 4,
    ...overrides,
  };
}

function template(kind: string, locale: string, overrides: Record<string, unknown> = {}) {
  const defaults = {
    subject: 'Prenotazione {codice} confermata – {stabilimento}',
    opening: 'Ciao {nome}, grazie per aver scelto {stabilimento}.',
    closing: 'A presto in spiaggia!',
    signature: '',
  };
  return {
    kind,
    locale,
    ...defaults,
    defaults,
    customized: false,
    fixed_blocks: ['summary', 'qr', 'manage_button'],
    version: 0,
    ...overrides,
  };
}

function templates() {
  const items = [];
  for (const k of ['booking_confirmed', 'day_before_reminder']) {
    for (const l of ['it', 'en', 'de', 'fr']) items.push(template(k, l));
  }
  return { placeholders: PLACEHOLDERS, items };
}

const routes = (extra = {}) => ({
  ...baseRoutes('manager'),
  [`GET ${API}/email-settings`]: () => json(200, emailSettings()),
  [`GET ${API}/email-templates`]: () => json(200, templates()),
  [`POST ${API}/email-templates/booking_confirmed/it/preview`]: () =>
    json(200, {
      subject: 'Prenotazione PROVA-2027-000123 confermata – Lido Demo',
      html: '<p>Ciao Mario</p>',
      text: 'Ciao Mario',
    }),
  ...extra,
});

describe('CustomerEmailsPage', () => {
  it('saves the reminder toggles and schedule into the email settings with If-Match', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`PUT ${API}/email-settings`]: async (req: Request) => {
          body = (await req.json()) as Record<string, unknown>;
          return json(200, emailSettings({ version: 5 }));
        },
      }),
    );
    const card = await screen.findByRole('region', { name: 'Promemoria automatici' });
    const dayBefore = within(card).getByRole('switch', { name: 'Promemoria il giorno prima' });
    expect(dayBefore).not.toBeChecked();
    expect(
      within(card).getByLabelText('Ora di invio del promemoria del giorno prima'),
    ).toBeDisabled();
    await user.click(dayBefore);
    await user.selectOptions(
      within(card).getByLabelText('Ora di invio del promemoria del giorno prima'),
      '19:30',
    );
    await user.selectOptions(
      within(card).getByLabelText('Quando, rispetto alla scadenza del saldo'),
      '0',
    );
    await user.click(within(card).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PUT', `${API}/email-settings`)).toHaveLength(1));
    expect(callsTo(fetchMock, 'PUT', `${API}/email-settings`)[0]!.headers.get('If-Match')).toBe(
      '"4"',
    );
    expect(body).toMatchObject({
      sender_name: 'Lido Demo',
      customer_notifications: {
        day_before_reminder: true,
        balance_reminder: true,
        confirmation: true,
      },
      reminders: { day_before_time: '19:30', balance_time: '10:00', balance_lead_days: 0 },
    });
  });

  it('edits a template with the placeholder picker, previews it and saves with If-Match', async () => {
    const user = userEvent.setup();
    let body: Record<string, string> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`PUT ${API}/email-templates/booking_confirmed/it`]: async (req: Request) => {
          body = (await req.json()) as Record<string, string>;
          return json(
            200,
            template('booking_confirmed', 'it', { ...body, customized: true, version: 1 }),
            { ETag: '"1"' },
          );
        },
      }),
    );
    const card = await screen.findByRole('region', { name: 'Testi delle email' });
    expect(await within(card).findByText('Testo predefinito')).toBeInTheDocument();
    // Live preview of the defaults (sandboxed iframe).
    expect(await within(card).findByTestId('preview-subject')).toHaveTextContent(
      'Prenotazione PROVA-2027-000123 confermata – Lido Demo',
    );
    expect(within(card).getByTitle("Anteprima dell'email")).toHaveAttribute('sandbox', '');
    const signature = within(card).getByLabelText('Firma');
    await user.click(signature);
    await user.type(signature, 'Lo staff di ');
    await user.click(within(card).getByRole('button', { name: 'Inserisci segnaposto' }));
    await user.click(await screen.findByRole('menuitem', { name: /\{stabilimento\}/ }));
    await waitFor(() => expect(signature).toHaveValue('Lo staff di {stabilimento}'));
    await user.click(within(card).getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', `${API}/email-templates/booking_confirmed/it`)).toHaveLength(
        1,
      ),
    );
    expect(
      callsTo(fetchMock, 'PUT', `${API}/email-templates/booking_confirmed/it`)[0]!.headers.get(
        'If-Match',
      ),
    ).toBe('"0"');
    expect(body).toMatchObject({ signature: 'Lo staff di {stabilimento}' });
    expect(await within(card).findByText('Personalizzata')).toBeInTheDocument();
    expect(
      callsTo(fetchMock, 'POST', `${API}/email-templates/booking_confirmed/it/preview`).length,
    ).toBeGreaterThan(0);
  });

  it('flags an unknown placeholder and shows the server 422 on the field', async () => {
    const user = userEvent.setup();
    renderApp(
      PATH,
      routes({
        [`PUT ${API}/email-templates/booking_confirmed/it`]: () =>
          problem(422, 'email_template_invalid', {
            errors: [{ field: 'closing', message: 'Segnaposto sconosciuto: {x}.' }],
          }),
      }),
    );
    const card = await screen.findByRole('region', { name: 'Testi delle email' });
    const opening = await within(card).findByLabelText('Testo iniziale');
    await user.clear(opening);
    await user.type(opening, 'Ciao {{telefono}');
    expect(await within(card).findByText(/Segnaposto sconosciuto: usa solo/)).toBeInTheDocument();
    await user.clear(opening);
    await user.type(opening, 'Ciao');
    await user.click(within(card).getByRole('button', { name: 'Salva' }));
    expect(await within(card).findByText('Segnaposto sconosciuto: {x}.')).toBeInTheDocument();
  });

  it('switches language, sends a test to the current user and restores the default', async () => {
    const user = userEvent.setup();
    const customized = template('booking_confirmed', 'en', {
      subject: 'Custom {codice}',
      customized: true,
      version: 3,
    });
    const list = templates();
    list.items = list.items.map((i) =>
      i.kind === 'booking_confirmed' && i.locale === 'en' ? customized : i,
    );
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`GET ${API}/email-templates`]: () => json(200, list),
        [`POST ${API}/email-templates/booking_confirmed/en/preview`]: () =>
          json(200, { subject: 'Custom PROVA', html: '<p>x</p>', text: 'x' }),
        [`POST ${API}/email-templates/booking_confirmed/en/test`]: () =>
          json(202, { message_id: 'm1' }),
        [`DELETE ${API}/email-templates/booking_confirmed/en`]: () =>
          json(200, template('booking_confirmed', 'en'), { ETag: '"0"' }),
      }),
    );
    const card = await screen.findByRole('region', { name: 'Testi delle email' });
    await user.selectOptions(await within(card).findByLabelText('Lingua'), 'en');
    expect(await within(card).findByLabelText('Oggetto')).toHaveValue('Custom {codice}');
    await user.click(within(card).getByRole('button', { name: 'Invia una prova a me' }));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'POST', `${API}/email-templates/booking_confirmed/en/test`),
      ).toHaveLength(1),
    );
    await user.click(within(card).getByRole('button', { name: 'Ripristina predefinito' }));
    const dialog = await screen.findByRole('alertdialog');
    await user.click(within(dialog).getByRole('button', { name: 'Ripristina predefinito' }));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'DELETE', `${API}/email-templates/booking_confirmed/en`),
      ).toHaveLength(1),
    );
    expect(
      callsTo(fetchMock, 'DELETE', `${API}/email-templates/booking_confirmed/en`)[0]!.headers.get(
        'If-Match',
      ),
    ).toBe('"3"');
    await waitFor(() =>
      expect(within(card).getByLabelText('Oggetto')).toHaveValue(
        'Prenotazione {codice} confermata – {stabilimento}',
      ),
    );
  });

  it('is read-only for a cashier', async () => {
    renderApp(PATH, { ...routes(), ...baseRoutes('cashier') });
    const card = await screen.findByRole('region', { name: 'Testi delle email' });
    expect(await within(card).findByLabelText('Oggetto')).toBeDisabled();
    expect(within(card).queryByRole('button', { name: 'Salva' })).toBeNull();
  });
});

describe('customer emails schema', () => {
  it('validates placeholders, length and one-line subject like the server', () => {
    const s = templateSchema(PLACEHOLDERS);
    expect(
      s.safeParse({ subject: 'Ciao {nome}', opening: '', closing: '', signature: '' }).success,
    ).toBe(true);
    expect(
      s.safeParse({ subject: 'Ciao {email}', opening: '', closing: '', signature: '' }).success,
    ).toBe(false);
    expect(s.safeParse({ subject: 'a\nb', opening: '', closing: '', signature: '' }).success).toBe(
      false,
    );
    expect(s.safeParse({ subject: ' ', opening: '', closing: '', signature: '' }).success).toBe(
      false,
    );
    expect(
      s.safeParse({ subject: 'x', opening: '', closing: '', signature: 'a'.repeat(501) }).success,
    ).toBe(false);
    expect(unknownPlaceholders('{a} {nome} {a} {b}', PLACEHOLDERS)).toEqual(['a', 'b']);
  });

  it('inserts at the caret and keeps unknown stored times selectable', () => {
    expect(insertAt('Ciao !', '{nome}', 5, 5)).toEqual({ value: 'Ciao {nome}!', caret: 11 });
    expect(insertAt('abc', '{x}', null, null)).toEqual({ value: 'abc{x}', caret: 6 });
    expect(timeOptions('18:00')).toHaveLength(48);
    expect(timeOptions('18:15')).toContain('18:15');
  });

  it('keeps the other email settings when saving the reminders', () => {
    const input = toRemindersInput(emailSettings() as never, {
      day_before_reminder: true,
      balance_reminder: false,
      day_before_time: '17:00',
      balance_time: '09:30',
      balance_lead_days: '3',
    });
    expect(input).toMatchObject({
      sender_name: 'Lido Demo',
      manager_email: 'gestore@lido.it',
      customer_notifications: { refund: true, balance_reminder: false, day_before_reminder: true },
      reminders: { day_before_time: '17:00', balance_time: '09:30', balance_lead_days: 3 },
    });
  });
});
