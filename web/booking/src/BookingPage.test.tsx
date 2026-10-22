import { ApiClientProvider, createApiClient } from '@arenile/shared/api';
import { createTestI18n } from '@arenile/shared/i18n';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import { BookingPage } from './BookingPage';
import { parseBookingLocation } from './location';
import { localBusinessJsonLd } from './seo';

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
    <I18nextProvider i18n={createTestI18n()}>
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
    expect(document.documentElement.style.getPropertyValue('--ar-primary')).toBe('#ffe600');
    expect(document.documentElement.style.getPropertyValue('--ar-primary-foreground')).toBe(
      '#111827',
    );
    expect(document.title).toBe('Lido Demo – Prenota il tuo posto in spiaggia');
    expect(document.getElementById('arenile-jsonld')?.textContent).toContain('LocalBusiness');
    expect(screen.getByRole('link', { name: '+39 0771 000000' })).toHaveAttribute(
      'href',
      'tel:+390771000000',
    );
  });

  it('walks the four steps with honest "in arrivo" content and lazy-loads the map step', async () => {
    const user = userEvent.setup();
    renderPage('/b/lido-demo', ok());
    const steps = await screen.findByRole('region', { name: '1. Quando' });
    expect(steps).toHaveTextContent('In arrivo');
    await user.click(screen.getByRole('button', { name: 'Avanti' }));
    expect(screen.getByRole('region', { name: '2. Dove' })).toBeInTheDocument();
    expect(
      await screen.findAllByText(/La prenotazione online sarà disponibile a breve/),
    ).not.toHaveLength(0);
    await user.click(screen.getByRole('button', { name: /Chi e pagamento/ }));
    expect(screen.getByRole('region', { name: '4. Chi e pagamento' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Avanti' })).toBeDisabled();
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
    expect(await screen.findByRole('alert')).toHaveTextContent('Stabilimento non trovato.');
  });

  it('rejects invalid slugs without calling the API', () => {
    renderPage('/b/', ok());
    expect(screen.getByRole('alert')).toHaveTextContent('Stabilimento non trovato.');
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
