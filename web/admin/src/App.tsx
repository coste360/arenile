import { ApiClientProvider, type ApiClient } from '@arenile/shared/api';
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import { RouterProvider, type RouterHistory } from '@tanstack/react-router';
import type { i18n as I18n } from 'i18next';
import { useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { createQueryClient } from './lib/queryClient';
import { createAppRouter } from './router';

export interface AppProps {
  api: ApiClient;
  i18n: I18n;
  queryClient?: QueryClient;
  history?: RouterHistory;
}

export function App({ api, i18n, queryClient, history }: AppProps) {
  const [qc] = useState(() => queryClient ?? createQueryClient());
  const [router] = useState(() => createAppRouter({ queryClient: qc, api }, history));
  return (
    <I18nextProvider i18n={i18n}>
      <ApiClientProvider client={api}>
        <QueryClientProvider client={qc}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>
  );
}
