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

// ---- M1/M2 configuration --------------------------------------------------
// Establishment-scoped routes live under /e/$establishmentId: the URL is the
// source of truth for the current establishment (see ./establishment). Feature
// pages are lazy-loaded from their folder's index.ts and owned by the feature
// builders; this file only wires them.

/** Optional `?seasonId=` shared by the season-dependent pages. */
export interface SeasonSearch {
  seasonId?: string;
}

export function validateSeasonSearch(search: Record<string, unknown>): SeasonSearch {
  return { seasonId: typeof search.seasonId === 'string' ? search.seasonId : undefined };
}

const newEstablishmentRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'establishments/new',
  component: lazyRouteComponent(() => import('./features/configuration'), 'NewEstablishmentPage'),
});

const establishmentRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'e/$establishmentId',
  component: Outlet,
});

const establishmentIndexRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: '/',
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/e/$establishmentId/config/establishment', params });
  },
});

const establishmentSettingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/establishment',
  component: lazyRouteComponent(
    () => import('./features/configuration'),
    'EstablishmentSettingsPage',
  ),
});
const seasonsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/seasons',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/configuration'), 'SeasonsPage'),
});
const closuresRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/closures',
  component: lazyRouteComponent(() => import('./features/configuration'), 'ClosuresPage'),
});
const zonesRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/zones',
  component: lazyRouteComponent(() => import('./features/configuration'), 'ZonesPage'),
});
const poolsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/pools',
  component: lazyRouteComponent(() => import('./features/configuration'), 'PoolsPage'),
});
const catalogRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/catalog',
  component: lazyRouteComponent(() => import('./features/configuration'), 'CatalogPage'),
});
const teamRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/team',
  component: lazyRouteComponent(() => import('./features/configuration'), 'TeamPage'),
});
const mapsListRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'maps',
  component: lazyRouteComponent(() => import('./features/map-editor'), 'MapsListPage'),
});
const mapEditorRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'maps/$mapId',
  component: lazyRouteComponent(() => import('./features/map-editor'), 'MapEditorPage'),
});
const setupWizardRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'setup',
  component: lazyRouteComponent(() => import('./features/setup-wizard'), 'SetupWizardPage'),
});
const pricingRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'pricing',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/pricing'), 'PricingPage'),
});
const priceSimulatorRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'pricing/simulator',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/pricing'), 'PriceSimulatorPage'),
});
const priceMatrixRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'pricing/matrix',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/pricing'), 'PriceMatrixPage'),
});
const coverageRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'pricing/coverage',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/pricing'), 'CoveragePage'),
});
const policiesRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'policies',
  validateSearch: validateSeasonSearch,
  component: lazyRouteComponent(() => import('./features/policies'), 'PoliciesPage'),
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  appRoute.addChildren([
    indexRoute,
    mapRoute,
    settingsRoute,
    ...placeholderRoutes,
    newEstablishmentRoute,
    establishmentRoute.addChildren([
      establishmentIndexRoute,
      establishmentSettingsRoute,
      seasonsRoute,
      closuresRoute,
      zonesRoute,
      poolsRoute,
      catalogRoute,
      teamRoute,
      mapsListRoute,
      mapEditorRoute,
      setupWizardRoute,
      pricingRoute,
      priceSimulatorRoute,
      priceMatrixRoute,
      coverageRoute,
      policiesRoute,
    ]),
  ]),
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
