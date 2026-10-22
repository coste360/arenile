import { ApiClientProvider, createApiClient, shouldRetry } from '@arenile/shared/api';
import { createI18n, resolveLocale } from '@arenile/shared/i18n';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import { BookingPage } from './BookingPage';
import { parseBookingLocation } from './location';
import './index.css';

async function bootstrap() {
  const location = parseBookingLocation(window.location.pathname, window.location.search);
  const locale = resolveLocale({ param: location.lang, browser: navigator.languages });
  const i18n = await createI18n(locale);
  document.documentElement.lang = locale;
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: shouldRetry, refetchOnWindowFocus: false } },
  });
  const api = createApiClient();
  const root = document.getElementById('root');
  if (!root) throw new Error('missing #root');
  createRoot(root).render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        <ApiClientProvider client={api}>
          <QueryClientProvider client={queryClient}>
            <BookingPage location={location} />
          </QueryClientProvider>
        </ApiClientProvider>
      </I18nextProvider>
    </StrictMode>,
  );
}

void bootstrap();
