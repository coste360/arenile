import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { NARROW_QUERY } from '@arenile/shared/ui/patterns';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, establishment, json, problem, renderApp } from '../testing';

const PATH = `/admin/e/${EST_ID}/config/establishment`;
const API = `/api/admin/v1/establishments/${EST_ID}`;

describe('EstablishmentSettingsPage', () => {
  it('saves the general section with If-Match and the new version is used next', async () => {
    const user = userEvent.setup();
    let version = 3;
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('manager'),
      [`PATCH ${API}`]: async (req) => {
        const body = (await req.json()) as Record<string, unknown>;
        bodies.push(body);
        version += 1;
        return json(200, establishment({ ...body, version }), { ETag: `"${version}"` });
      },
    });
    const general = await screen.findByRole('region', { name: 'Dati generali' });
    const name = within(general).getByLabelText('Nome');
    await user.clear(name);
    await user.type(name, 'Lido Nuovo');
    await user.click(within(general).getByRole('button', { name: 'Salva' }));
    expect(await screen.findByText('Modifiche salvate')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Prezzi e IVA' }));
    const pricing = screen.getByRole('region', { name: 'Impostazioni dei prezzi' });
    const vat = within(pricing).getByLabelText('IVA predefinita');
    await user.clear(vat);
    await user.type(vat, '10');
    await user.click(within(pricing).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PATCH', API)).toHaveLength(2));

    const [first, second] = callsTo(fetchMock, 'PATCH', API);
    expect(first?.headers.get('If-Match')).toBe('"3"');
    expect(first?.headers.get('X-CSRF-Token')).toBe('csrf');
    expect(bodies[0]).toMatchObject({
      name: 'Lido Nuovo',
      slug: 'lido-demo',
      address: { city: 'Gaeta', country: 'IT' },
    });
    // The "Generale" tab does not send the languages edited in the "Lingue" tab.
    expect(bodies[0]).not.toHaveProperty('enabled_locales');
    // Our own save moved the base version forward: no false conflict.
    expect(second?.headers.get('If-Match')).toBe('"4"');
    expect(bodies[1]).toEqual({
      settings: {
        pricing: { half_day_factor: '0.6', rounding: 'none', default_vat_rate_bp: 1000 },
        default_policy: {},
      },
    });
  });

  it('validates with zod before calling the API', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, baseRoutes('owner'));
    await user.click(await screen.findByRole('tab', { name: 'Prezzi e IVA' }));
    const pricing = await screen.findByRole('region', { name: 'Impostazioni dei prezzi' });
    const factor = within(pricing).getByLabelText('Fattore mezza giornata');
    await user.clear(factor);
    await user.type(factor, '1,5');
    await user.click(within(pricing).getByRole('button', { name: 'Salva' }));
    expect(
      await within(pricing).findByText('Il fattore deve essere maggiore di 0 e al massimo 1'),
    ).toBeInTheDocument();
    expect(factor).toHaveAttribute('aria-invalid', 'true');
    expect(callsTo(fetchMock, 'PATCH', API)).toHaveLength(0);
  });

  it('maps problem+json field errors onto the inputs', async () => {
    const user = userEvent.setup();
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`PATCH ${API}`]: () =>
        problem(400, 'validation_failed', {
          errors: [
            { field: 'slug', message: 'Indirizzo già usato da un altro stabilimento.' },
            { field: 'address.city', message: 'Comune sconosciuto.' },
          ],
        }),
    });
    const general = await screen.findByRole('region', { name: 'Dati generali' });
    await user.click(within(general).getByRole('button', { name: 'Salva' }));
    expect(
      await within(general).findByText('Indirizzo già usato da un altro stabilimento.'),
    ).toBeInTheDocument();
    expect(within(general).getByText('Comune sconosciuto.')).toBeInTheDocument();
    expect(within(general).getByLabelText('Indirizzo pubblico')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });

  it('on 412 offers to reload and retries with the fresh version', async () => {
    const user = userEvent.setup();
    let serverVersion = 3;
    const routes = baseRoutes('owner');
    const { fetchMock } = renderApp(PATH, {
      ...routes,
      [`GET ${API}`]: () =>
        json(200, establishment({ version: serverVersion, name: 'Lido Remoto' })),
      [`PATCH ${API}`]: (req) =>
        req.headers.get('If-Match') === `"${serverVersion}"`
          ? json(200, establishment({ version: serverVersion + 1 }))
          : problem(412, 'version_conflict', { current_version: serverVersion }),
    });
    await user.click(await screen.findByRole('tab', { name: 'Pagina pubblica' }));
    const theme = await screen.findByRole('region', { name: 'Tema della pagina pubblica' });
    // Someone else saves meanwhile.
    serverVersion = 7;
    await user.click(within(theme).getByRole('button', { name: 'Salva' }));
    const alert = await within(theme).findByRole('alert');
    expect(alert).toHaveTextContent('Un altro utente ha modificato questi dati');
    await user.click(within(alert).getByRole('button', { name: 'Ricarica i dati aggiornati' }));
    await waitFor(() => expect(within(theme).queryByRole('alert')).not.toBeInTheDocument());
    await user.click(within(theme).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PATCH', API)).toHaveLength(2));
    expect(callsTo(fetchMock, 'PATCH', API)[1]?.headers.get('If-Match')).toBe('"7"');
  });

  it('shows a live theme preview with contrast warnings', async () => {
    const user = userEvent.setup();
    renderApp(PATH, baseRoutes('owner'));
    await user.click(await screen.findByRole('tab', { name: 'Pagina pubblica' }));
    const theme = await screen.findByRole('region', { name: 'Tema della pagina pubblica' });
    const primary = within(theme).getByLabelText('Colore principale');
    await user.clear(primary);
    await user.type(primary, '#ffff00');
    expect(
      within(theme).getByText(/Contrasto del colore principale sul bianco/),
    ).toBeInTheDocument();
    expect(screen.getByTestId('theme-preview').style.getPropertyValue('--primary')).toBe('#ffff00');
  });

  it('shows the embed snippets of spec 12.1', async () => {
    const user = userEvent.setup();
    renderApp(PATH, baseRoutes('owner'));
    await user.click(await screen.findByRole('tab', { name: 'Pagina pubblica' }));
    const section = await screen.findByRole('region', { name: 'Codice da incorporare' });
    const script = within(section).getByLabelText(/Codice consigliato/) as HTMLTextAreaElement;
    expect(script.value).toContain('<div data-arenile="lido-demo" data-lang="it"></div>');
    expect(script.value).toContain('/embed.js" async></script>');
    const iframe = within(section).getByLabelText(/iframe/) as HTMLTextAreaElement;
    expect(iframe.value).toContain('/b/lido-demo?embed=1');
  });

  it('is read-only for a cashier (spec 16.1: view only)', async () => {
    renderApp(PATH, baseRoutes('cashier'));
    expect(await screen.findByRole('note')).toHaveTextContent('solo la consultazione');
    expect(screen.queryByRole('button', { name: 'Salva' })).not.toBeInTheDocument();
    expect(screen.getAllByLabelText('Nome')[0]).toBeDisabled();
    expect(
      within(screen.getByRole('main')).queryByRole('link', { name: 'Configurazione guidata' }),
    ).toBeNull();
  });

  it('switches sections from a Select on a narrow screen (no tab bar to wrap)', async () => {
    const original = window.matchMedia;
    window.matchMedia = ((query: string) => ({
      matches: query === NARROW_QUERY,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
    try {
      const user = userEvent.setup();
      renderApp(PATH, baseRoutes('owner'));
      const picker = await screen.findByRole('combobox', { name: 'Sezioni delle impostazioni' });
      expect(screen.queryByRole('tablist')).toBeNull();
      expect(picker).toHaveTextContent('Generale');
      await user.click(picker);
      await user.click(await screen.findByRole('option', { name: 'Prezzi e IVA' }));
      expect(
        await screen.findByRole('region', { name: 'Impostazioni dei prezzi' }),
      ).toBeInTheDocument();
    } finally {
      window.matchMedia = original;
    }
  });
});
