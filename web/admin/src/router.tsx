import { ApiProblem, meQueryOptions, type ApiClient } from '@arenile/shared/api';
import type { QueryClient } from '@tanstack/react-query';
import {
  Outlet,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
  type RouterHistory,
} from '@tanstack/react-router';
import { AppShell } from './components/AppShell';
import { PlaceholderPage } from './components/PlaceholderPage';
import { RouteError } from './components/RouteError';
import { NAV_ITEMS } from './nav';
import { LoginRoute } from './pages/LoginRoute';
import { SettingsPage } from './pages/SettingsPage';

export interface RouterContext {
  queryClient: QueryClient;
  api: ApiClient;
}

/** Keeps only same-app absolute paths, preventing open redirects. */
export function safeRedirect(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return undefined;
  if (value.startsWith('/login')) return undefined;
  return value;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  errorComponent: ({ error, reset }) => <RouteError error={error} reset={reset} />,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'login',
  validateSearch: (search: Record<string, unknown>): { redirect?: string } => ({
    redirect: safeRedirect(search.redirect),
  }),
  component: LoginRoute,
});

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions(context.api));
    } catch (err) {
      if (err instanceof ApiProblem && err.status === 401) {
        throw redirect({ to: '/login', search: { redirect: safeRedirect(location.pathname) } });
      }
      throw err;
    }
  },
  component: AppShell,
});

const indexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/oggi' });
  },
});

const mapRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'mappa',
  component: lazyRouteComponent(() => import('./pages/DemoMapPage'), 'DemoMapPage'),
});

const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'configurazione',
  component: SettingsPage,
});

function placeholderRoute<TPath extends string>(path: TPath) {
  const item = NAV_ITEMS.find((i) => i.to === `/${path}`);
  if (!item) throw new Error(`unknown nav item ${path}`);
  return createRoute({
    getParentRoute: () => appRoute,
    path,
    component: () => (
      <PlaceholderPage
        titleKey={item.labelKey}
        descriptionKey={item.descriptionKey}
        milestone={item.milestone}
      />
    ),
  });
}

const placeholderRoutes = [
  placeholderRoute('oggi'),
  placeholderRoute('prenotazioni'),
  placeholderRoute('calendario'),
  placeholderRoute('clienti'),
  placeholderRoute('abbonamenti'),
  placeholderRoute('cassa'),
  placeholderRoute('report'),
] as const;

const routeTree = rootRoute.addChildren([
  loginRoute,
  appRoute.addChildren([indexRoute, mapRoute, settingsRoute, ...placeholderRoutes]),
]);

export function createAppRouter(context: RouterContext, history?: RouterHistory) {
  return createRouter({
    routeTree,
    context,
    basepath: '/admin',
    defaultPreload: 'intent',
    ...(history ? { history } : {}),
  });
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
