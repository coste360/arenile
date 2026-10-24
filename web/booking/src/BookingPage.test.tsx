import { ApiClientProvider, createApiClient } from '@arenile/shared/api';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import { BookingPage } from './BookingPage';
import { parseBookingLocation } from './location';
import { localBusinessJsonLd } from './seo';
import { testI18n } from './test-utils';

const ESTABLISHMENT = {
  slug: 'lido-demo',
  name: 'Lido Demo',
  timezone: 'Europe/Rome',
  currency: 'EUR',
  default_locale: 'it',
  enabled_locales: ['it'],
  booking_enabled: true,
  contact_phone: '+39 0771 000000',
  address: { street: 'Lungomare 1', city: 'Gaeta' },
  theme: {
    primary_color: '#ffe600',
    hide_header_in_embed: true,
    custom_texts: { it: { intro: 'Benvenuti al Lido' } },
  },
};

function renderPage(path: string, response: () => Response) {
  const api = createApiClient({ baseUrl: 'http://localhost', fetch: async () => response() });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const [pathname, search = ''] = path.split('?');
  return render(
    <I18nextProvider i18n={testI18n()}>
      <ApiClientProvider client={api}>
        <QueryClientProvider client={qc}>
          <BookingPage location={parseBookingLocation(pathname!, search ? `?${search}` : '')} />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>,
  );
}

const ok =
  (body: unknown = ESTABLISHMENT) =>
  () =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });

describe('BookingPage', () => {
  it('shows the establishment, applies the theme and SEO data', async () => {
    renderPage('/b/lido-demo', ok());
    expect(await screen.findByText('Lido Demo')).toBeInTheDocument();
    expect(screen.getByText('Benvenuti al Lido')).toBeInTheDocument();
    expect(document.documentElement.style.getPropertyValue('--primary')).toBe('#ffe600');
    expect(document.documentElement.style.getPropertyValue('--primary-foreground')).toBe('#111827');
    expect(document.title).toBe('Lido Demo – Prenota il tuo posto in spiaggia');
    expect(document.getElementById('arenile-jsonld')?.textContent).toContain('LocalBusiness');
    expect(screen.getByRole('link', { name: '+39 0771 000000' })).toHaveAttribute(
      'href',
      'tel:+390771000000',
    );
  });

  it('keeps the language chosen in the picker (not reset by the automatic choice)', async () => {
    const user = userEvent.setup();
    renderPage('/b/lido-demo', ok({ ...ESTABLISHMENT, enabled_locales: ['it', 'de'] }));
    await screen.findByRole('heading', { name: 'Prenota il tuo posto in spiaggia' });
    await user.selectOptions(screen.getByRole('combobox', { name: 'Lingua' }), 'de');
    expect(
      await screen.findByRole('heading', { name: 'Buchen Sie Ihren Platz am Strand' }),
    ).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 50));
    expect(
      screen.getByRole('heading', { name: 'Buchen Sie Ihren Platz am Strand' }),
    ).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('de');
  });

  it('keeps the later steps unavailable until the previous ones are complete', async () => {
    const user = userEvent.setup();
    renderPage('/b/lido-demo', ok());
    const steps = await screen.findByRole('region', { name: '1. Quando' });
    expect(steps).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Avanti' })).toBeDisabled();
    const where = screen.getByRole('button', { name: /Dove/ });
    expect(where).toHaveAttribute('aria-disabled', 'true');
    expect(where).toHaveAccessibleName(/Completa prima i passi precedenti/);
    await user.click(where);
    expect(screen.getByRole('region', { name: '1. Quando' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: '2. Dove' })).not.toBeInTheDocument();
    expect(screen.getByText(/Scegli date, postazione e pacchetto/)).toBeInTheDocument();
  });

  it('shows the city and the active season under the title and limits the dates to it', async () => {
    const user = userEvent.setup();
    renderPage(
      '/b/lido-demo',
      ok({
        ...ESTABLISHMENT,
        season: { year: 2099, opens_on: '2099-05-01', closes_on: '2099-09-30' },
      }),
    );
    expect(
      await screen.findByText('Gaeta · Stagione 2099: dal 01/05/2099 al 30/09/2099'),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Date del soggiorno/ }));
    const grid = await screen.findByRole('grid');
    // Opens on the first bookable month, days outside the season are disabled.
    expect(grid).toHaveAccessibleName(/maggio 2099/i);
    expect(within(grid).getByRole('button', { name: /\b1 maggio 2099/i })).toBeEnabled();
    // Every day of May is bookable; nothing outside the season is enabled.
    const enabled = within(grid)
      .getAllByRole('button')
      .filter((b) => !b.hasAttribute('disabled') && b.getAttribute('aria-disabled') !== 'true');
    expect(enabled.map((b) => b.getAttribute('aria-label') ?? b.textContent)).toHaveLength(31);
  });

  it('picks the stay dates with the shadcn date range picker (dd/MM/yyyy, lazy calendar)', async () => {
    const user = userEvent.setup();
    renderPage('/b/lido-demo', ok());
    const trigger = await screen.findByRole('button', { name: /Date del soggiorno/ });
    expect(trigger).toHaveTextContent('Seleziona un periodo');
    expect(screen.getByText(/tocca due volte lo stesso giorno/)).toBeInTheDocument();
    expect(document.querySelector('input[type="date"]')).toBeNull();
    await user.click(trigger);
    const grid = await screen.findByRole('grid');
    // The popover is a dialog named after the field, with a named legend.
    expect(screen.getByRole('dialog', { name: 'Date del soggiorno' })).toContainElement(grid);
    expect(screen.getByRole('list', { name: 'Disponibilità' })).toBeInTheDocument();
    const enabled = Array.from(grid.querySelectorAll<HTMLButtonElement>('button:not([disabled])'));
    expect(enabled.length).toBeGreaterThan(0);
    await user.click(enabled[0]!);
    expect(screen.getByRole('button', { name: /Date del soggiorno/ }).textContent).toMatch(
      /\d{2}\/\d{2}\/\d{4} – /,
    );
    expect(screen.getByText('1 giorno selezionato')).toBeInTheDocument();
  });

  it('hides the header in embed mode and marks the page noindex', async () => {
    renderPage('/b/lido-demo?embed=1', ok());
    expect(await screen.findByRole('heading', { level: 1, name: 'Lido Demo' })).toBeInTheDocument();
    expect(screen.queryByRole('banner')).not.toBeInTheDocument();
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex');
    expect(document.getElementById('arenile-jsonld')).toBeNull();
  });

  it('explains when online booking is disabled', async () => {
    renderPage('/b/lido-demo', ok({ ...ESTABLISHMENT, booking_enabled: false }));
    expect(
      await screen.findByText('Le prenotazioni online di questo stabilimento non sono attive.'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: /Quando/ })).not.toBeInTheDocument();
  });

  it('shows not found for unknown slugs', async () => {
    renderPage(
      '/b/nessuno',
      () =>
        new Response(JSON.stringify({ type: 'not_found', title: 'Not found', status: 404 }), {
          status: 404,
          headers: { 'Content-Type': 'application/problem+json' },
        }),
    );
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Stabilimento non trovato.' }),
    ).toBeInTheDocument();
  });

  it('rejects invalid slugs without calling the API', () => {
    renderPage('/b/', ok());
    expect(screen.getByRole('status')).toHaveTextContent('Stabilimento non trovato.');
  });
});

describe('localBusinessJsonLd', () => {
  it('includes only provided fields', () => {
    const ld = localBusinessJsonLd(ESTABLISHMENT as never, 'https://x.it/b/lido-demo');
    expect(ld).toMatchObject({
      '@type': 'LocalBusiness',
      name: 'Lido Demo',
      telephone: '+39 0771 000000',
      address: { '@type': 'PostalAddress', streetAddress: 'Lungomare 1', addressLocality: 'Gaeta' },
    });
    expect(ld).not.toHaveProperty('email');
  });
});
