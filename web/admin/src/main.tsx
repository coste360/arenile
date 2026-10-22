import { createApiClient } from '@arenile/shared/api';
import { createI18n, resolveLocale } from '@arenile/shared/i18n';
import { applyTheme } from '@arenile/shared/theme';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import './index.css';

async function bootstrap() {
  const locale = resolveLocale({
    param: new URLSearchParams(window.location.search).get('lang'),
    browser: navigator.languages,
  });
  const i18n = await createI18n(locale);
  document.documentElement.lang = locale;
  applyTheme(null);
  const api = createApiClient();
  const root = document.getElementById('root');
  if (!root) throw new Error('missing #root');
  createRoot(root).render(
    <StrictMode>
      <App api={api} i18n={i18n} />
    </StrictMode>,
  );
}

void bootstrap();
