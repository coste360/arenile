import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/e/${EST_ID}/config/import`;

function job(overrides: Record<string, unknown> = {}) {
  return {
    id: 'j1',
    kind: 'customers',
    status: 'uploaded',
    filename: 'clienti.csv',
    size_bytes: 120,
    sha256: 'abc',
    options: {},
    version: 1,
    created_at: '2027-05-01T08:00:00Z',
    updated_at: '2027-05-01T08:00:00Z',
    ...overrides,
  };
}

const report = (overrides: Record<string, unknown> = {}) => ({
  rows: 3,
  valid: 3,
  invalid: 0,
  to_create: 2,
  to_update: 0,
  to_skip: 1,
  errors: [],
  warnings: [],
  ...overrides,
});

describe('ImportsPage', () => {
  it('uploads a CSV, previews it and applies it with If-Match', async () => {
    const user = userEvent.setup();
    const previews: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/imports`]: () => list([]),
      [`POST ${API}/imports`]: () => json(201, job(), { ETag: '"1"' }),
      [`POST ${API}/imports/j1/preview`]: async (req) => {
        previews.push(await req.json());
        return json(200, job({ status: 'validated', report: report(), version: 2 }), {
          ETag: '"2"',
        });
      },
      [`POST ${API}/imports/j1/apply`]: () =>
        json(
          200,
          job({
            status: 'applied',
            report: report({ created: 2, updated: 0, skipped: 1 }),
            applied_at: '2027-05-01T08:05:00Z',
            version: 3,
          }),
        ),
    });
    const input = await screen.findByLabelText('File CSV');
    await user.upload(input, new File(['cognome;email\nRossi;r@example.org\n'], 'clienti.csv'));
    await user.click(screen.getByRole('button', { name: 'Carica e verifica' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/imports`)).toHaveLength(1));
    const upload = callsTo(fetchMock, 'POST', `${API}/imports`)[0]!;
    expect(new URL(upload.url).searchParams.get('kind')).toBe('customers');
    expect(upload.headers.get('Content-Type')).toBe('text/csv');
    expect(upload.headers.get('Idempotency-Key')).toBeTruthy();

    await user.click(await screen.findByRole('button', { name: 'Anteprima' }));
    expect(await screen.findByText('Da creare')).toBeInTheDocument();
    expect(previews).toEqual([
      { match_by: 'email', update_existing: false, marketing_consent_default: false },
    ]);
    expect(
      callsTo(fetchMock, 'POST', `${API}/imports/j1/preview`)[0]!.headers.get('If-Match'),
    ).toBe('"1"');

    await user.click(screen.getByRole('button', { name: 'Applica' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByText(/Saranno creati 2 elementi, aggiornati 0 e saltati 1/),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole('button', { name: 'Applica' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/imports/j1/apply`)).toHaveLength(1),
    );
    expect(callsTo(fetchMock, 'POST', `${API}/imports/j1/apply`)[0]!.headers.get('If-Match')).toBe(
      '"2"',
    );
    expect(await screen.findByText('Creati 2, aggiornati 0, saltati 1.')).toBeInTheDocument();
  });

  it('shows the validation report and blocks apply while rows have errors', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/imports`]: () =>
        list([
          job({
            status: 'validated',
            report: report({
              valid: 2,
              invalid: 1,
              errors: [{ row: 3, column: 'email', code: 'invalid_email' }],
            }),
          }),
        ]),
    });
    await userEvent.click(
      await screen.findByRole('row', { name: /Apri l'importazione clienti.csv/ }),
    );
    expect(await screen.findByText('Email non valida.')).toBeInTheDocument();
    expect((screen.getByRole('button', { name: 'Applica' }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it('is read-only for a cashier', async () => {
    renderApp(PATH, { ...baseRoutes('cashier'), [`GET ${API}/imports`]: () => list([]) });
    const input = await screen.findByLabelText('File CSV');
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(true));
  });
});
