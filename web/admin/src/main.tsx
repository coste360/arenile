import { createApiClient } from '@arenile/shared/api';
import { resolveLocale } from '@arenile/shared/i18n';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { createAdminI18n } from './i18n';
import { createQueryClient } from './lib/queryClient';
import { startOfflineRuntime } from './offline/bootstrap';
import { registerPwa } from './pwa/register';
import '@arenile/shared/styles/globals.css';

async function bootstrap() {
  const locale = resolveLocale({
    param: new URLSearchParams(window.location.search).get('lang'),
    browser: navigator.languages,
  });
  const api = createApiClient();
  const queryClient = createQueryClient();
  // The offline snapshot is restored before the first render, so that the
  // route guards find the cached user when the panel starts offline.
  const [i18n, offline] = await Promise.all([
    createAdminI18n(locale),
    startOfflineRuntime({ queryClient, api }),
  ]);
  document.documentElement.lang = locale;
  registerPwa();
  const root = document.getElementById('root');
  if (!root) throw new Error('missing #root');
  createRoot(root).render(
    <StrictMode>
      <App api={api} i18n={i18n} queryClient={queryClient} offline={offline} />
    </StrictMode>,
  );
}

void bootstrap();
