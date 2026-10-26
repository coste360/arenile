import { ApiClientProvider, type ApiClient } from '@arenile/shared/api';
import { ThemeProvider } from '@arenile/shared/ui/patterns';
import { Toaster } from '@arenile/shared/ui/sonner';
import { TooltipProvider } from '@arenile/shared/ui/tooltip';
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import { RouterProvider, type RouterHistory } from '@tanstack/react-router';
import type { i18n as I18n } from 'i18next';
import { useEffect, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { createQueryClient } from './lib/queryClient';
import { watchSessionExpiry } from './lib/sessionExpiry';
import { OfflineProvider } from './offline/OfflineProvider';
import type { OfflineRuntime } from './offline/runtime';
import { createAppRouter } from './router';

export interface AppProps {
  api: ApiClient;
  i18n: I18n;
  queryClient?: QueryClient;
  history?: RouterHistory;
  /** Offline runtime restored in main.tsx (spec 21.2); tests get a memory one. */
  offline?: OfflineRuntime;
}

export function App({ api, i18n, queryClient, history, offline }: AppProps) {
  const [qc] = useState(() => queryClient ?? createQueryClient());
  const [router] = useState(() => createAppRouter({ queryClient: qc, api }, history));
  useEffect(() => watchSessionExpiry(qc, router), [qc, router]);
  return (
    <I18nextProvider i18n={i18n}>
      <ThemeProvider>
        <TooltipProvider delayDuration={300}>
          <ApiClientProvider client={api}>
            <QueryClientProvider client={qc}>
              <OfflineProvider runtime={offline}>
                <RouterProvider router={router} />
              </OfflineProvider>
            </QueryClientProvider>
          </ApiClientProvider>
          <Toaster richColors closeButton position="top-right" />
        </TooltipProvider>
      </ThemeProvider>
    </I18nextProvider>
  );
}
