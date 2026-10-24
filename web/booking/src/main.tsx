import { ApiClientProvider, createApiClient, shouldRetry } from '@arenile/shared/api';
import { createI18n, resolveLocale } from '@arenile/shared/i18n';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode, Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import { BookingPage } from './BookingPage';
import { addPublicMessages } from './i18n';
import { observeServerDate } from './flow/serverClock';
import { parseBookingLocation } from './location';
import { PREVIEW_HEADER, resolveTestPreviewToken } from './testPreview';
import { ManagePage, WaitlistOfferPage } from './views';
import '@arenile/shared/styles/globals.css';

async function bootstrap() {
  const location = parseBookingLocation(window.location.pathname, window.location.search);
  const locale = resolveLocale({ param: location.lang, browser: navigator.languages });
  const i18n = await createI18n(locale);
  await addPublicMessages(i18n, locale);
  document.documentElement.lang = locale;
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: shouldRetry, refetchOnWindowFocus: false } },
  });
  const api = createApiClient();
  // Hold countdowns run on the server's clock (devices are often minutes off).
  api.use({
    onResponse({ response }) {
      observeServerDate(response.headers.get('Date'));
      return undefined;
    },
  });
  // Test-mode preview link (spec 15.4): every public request carries the token.
  const previewToken = resolveTestPreviewToken(location.slug, window.location.search);
  if (previewToken) {
    api.use({
      onRequest({ request }) {
        request.headers.set(PREVIEW_HEADER, previewToken);
        return request;
      },
    });
  }
  const root = document.getElementById('root');
  if (!root) throw new Error('missing #root');
  createRoot(root).render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        <ApiClientProvider client={api}>
          <QueryClientProvider client={queryClient}>
            {location.view === 'manage' ? (
              <Suspense fallback={null}>
                <ManagePage location={location} />
              </Suspense>
            ) : location.view === 'waitlist' ? (
              <Suspense fallback={null}>
                <WaitlistOfferPage location={location} />
              </Suspense>
            ) : (
              <BookingPage location={location} />
            )}
          </QueryClientProvider>
        </ApiClientProvider>
      </I18nextProvider>
    </StrictMode>,
  );
}

void bootstrap();
