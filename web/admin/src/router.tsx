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
import { RouteError } from './components/RouteError';
import { EstablishmentLayout, EstablishmentSectionRedirect } from './establishment';
import { desktopGated } from './features/desktop/desktopGated';
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

const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'configurazione',
  component: SettingsPage,
});

/**
 * Legacy top-level paths of the operational sections (/oggi, /mappa, ...):
 * they redirect to the same section of the current establishment.
 */
const legacySectionRoutes = NAV_ITEMS.map((item) =>
  createRoute({
    getParentRoute: () => appRoute,
    path: item.section,
    component: () => <EstablishmentSectionRedirect section={item.section} />,
  }),
);

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
  component: EstablishmentLayout,
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

// ---- M4 public page, online payments, email ---------------------------------
// Configuration pages of the online feature (owned by the admin online builder).

const onlineSettingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/online',
  // Desktop edition: no public page, explained in place (spec 29.1).
  component: desktopGated(
    'publicPage',
    lazyRouteComponent(() => import('./features/online'), 'OnlineSettingsPage'),
  ),
});
const paymentSettingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/payments',
  // Desktop edition: manual payments only, explained in place (spec 29.1).
  component: desktopGated(
    'payments',
    lazyRouteComponent(() => import('./features/online'), 'PaymentSettingsPage'),
  ),
});
const emailSettingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/email',
  component: lazyRouteComponent(() => import('./features/online'), 'EmailSettingsPage'),
});

// ---- M6/M7 devices and desktop edition ----------------------------------------
// Devices (pairing QR, revoke) and the desktop app settings (desktop edition
// only); the first-run page is outside the authenticated shell.

const devicesRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/devices',
  component: lazyRouteComponent(() => import('./features/devices'), 'DevicesPage'),
});
// LAN info page (address, certificate fingerprint, pairing): opened by
// the tray item "Mostra QR rete locale" (internal/desktop/lan QRPagePath).
const lanInfoRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/devices/lan',
  component: lazyRouteComponent(() => import('./features/devices/lan'), 'LanInfoPage'),
});
/** /admin/rete-locale: establishment-less entry used by the desktop tray. */
const lanShortcutRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'rete-locale',
  component: () => <EstablishmentSectionRedirect section="config/devices/lan" />,
});
const desktopSettingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/desktop',
  component: lazyRouteComponent(() => import('./features/desktop'), 'DesktopSettingsPage'),
});
/** /admin/desktop: the tray menu link to the desktop settings and backups. */
const desktopShortcutRoute = createRoute({
  getParentRoute: () => appRoute,
  path: 'desktop',
  component: () => <EstablishmentSectionRedirect section="config/desktop" />,
});
const firstRunRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'first-run',
  component: lazyRouteComponent(() => import('./features/first-run'), 'FirstRunPage'),
});

// ---- M3 operations ---------------------------------------------------------
// Establishment-scoped pages of spec 15.1 "Operatività", lazy-loaded from the
// feature folders (owned by the feature builders).

/** `?date=YYYY-MM-DD&slot=AM|PM|FULL` shared by the map and the day view. */
export interface DaySearch {
  date?: string;
  slot?: 'AM' | 'PM' | 'FULL';
}

export function validateDaySearch(search: Record<string, unknown>): DaySearch {
  const date =
    typeof search.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(search.date)
      ? search.date
      : undefined;
  const slot =
    search.slot === 'AM' || search.slot === 'PM' || search.slot === 'FULL'
      ? search.slot
      : undefined;
  return { date, slot };
}

const todayRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'oggi',
  validateSearch: validateDaySearch,
  component: lazyRouteComponent(() => import('./features/operations'), 'TodayPage'),
});
const operationalMapRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'mappa',
  validateSearch: validateDaySearch,
  component: lazyRouteComponent(() => import('./features/operations'), 'OperationalMapPage'),
});
const bookingsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'prenotazioni',
  component: lazyRouteComponent(() => import('./features/bookings'), 'BookingsPage'),
});
const bookingDetailRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'prenotazioni/$bookingId',
  component: lazyRouteComponent(() => import('./features/bookings'), 'BookingDetailPage'),
});
const planningRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'calendario',
  validateSearch: validateDaySearch,
  component: lazyRouteComponent(() => import('./features/planning'), 'PlanningPage'),
});
const customersRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'clienti',
  component: lazyRouteComponent(() => import('./features/customers'), 'CustomersPage'),
});
const customerDetailRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'clienti/$customerId',
  component: lazyRouteComponent(() => import('./features/customers'), 'CustomerDetailPage'),
});
const seasonPassesRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'abbonamenti',
  component: lazyRouteComponent(() => import('./features/passes'), 'SeasonPassesPage'),
});
const cashRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'cassa',
  component: lazyRouteComponent(() => import('./features/cash'), 'CashPage'),
});
const corrispettiviRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'cassa/corrispettivi',
  component: lazyRouteComponent(() => import('./features/cash'), 'CorrispettiviPage'),
});
const reportsRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'report',
  component: lazyRouteComponent(() => import('./features/reports'), 'ReportsPage'),
});

// ---- M5A integrations (API keys, outgoing webhooks) -------------------------
// Configuration pages of the integrations feature (owned by its builder).

const apiKeysRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/api-keys',
  component: lazyRouteComponent(() => import('./features/integrations'), 'ApiKeysPage'),
});
const webhooksRoute = createRoute({
  getParentRoute: () => establishmentRoute,
  path: 'config/webhooks',
  component: lazyRouteComponent(() => import('./features/integrations'), 'WebhooksPage'),
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  firstRunRoute,
  appRoute.addChildren([
    indexRoute,
    settingsRoute,
    desktopShortcutRoute,
    ...legacySectionRoutes,
    lanShortcutRoute,
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
      onlineSettingsRoute,
      paymentSettingsRoute,
      emailSettingsRoute,
      devicesRoute,
      lanInfoRoute,
      desktopSettingsRoute,
      todayRoute,
      operationalMapRoute,
      bookingsRoute,
      bookingDetailRoute,
      planningRoute,
      customersRoute,
      customerDetailRoute,
      seasonPassesRoute,
      cashRoute,
      corrispettiviRoute,
      reportsRoute,
      apiKeysRoute,
      webhooksRoute,
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
