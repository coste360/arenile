/** Panel navigation (spec 15.1) and the milestone that delivers each section (spec 28). */
export const NAV_ITEMS = [
  {
    to: '/oggi',
    labelKey: 'nav.today',
    descriptionKey: 'admin.placeholder.today',
    milestone: 'M3',
  },
  { to: '/mappa', labelKey: 'nav.map', descriptionKey: 'admin.placeholder.map', milestone: 'M3' },
  {
    to: '/prenotazioni',
    labelKey: 'nav.bookings',
    descriptionKey: 'admin.placeholder.bookings',
    milestone: 'M3',
  },
  {
    to: '/calendario',
    labelKey: 'nav.calendar',
    descriptionKey: 'admin.placeholder.calendar',
    milestone: 'M3',
  },
  {
    to: '/clienti',
    labelKey: 'nav.customers',
    descriptionKey: 'admin.placeholder.customers',
    milestone: 'M3',
  },
  {
    to: '/abbonamenti',
    labelKey: 'nav.seasonPasses',
    descriptionKey: 'admin.placeholder.seasonPasses',
    milestone: 'M3',
  },
  { to: '/cassa', labelKey: 'nav.cash', descriptionKey: 'admin.placeholder.cash', milestone: 'M3' },
  {
    to: '/report',
    labelKey: 'nav.reports',
    descriptionKey: 'admin.placeholder.reports',
    milestone: 'M5',
  },
  {
    to: '/configurazione',
    labelKey: 'nav.settings',
    descriptionKey: 'admin.placeholder.settings',
    milestone: 'M1',
  },
] as const;

export type NavPath = (typeof NAV_ITEMS)[number]['to'];

/**
 * "Configurazione" submenu (spec 15.1 item 9), scoped to the current
 * establishment: every path is under /e/$establishmentId.
 */
export const CONFIG_NAV_ITEMS = [
  { to: '/e/$establishmentId/setup', labelKey: 'nav.config.setup' },
  { to: '/e/$establishmentId/config/establishment', labelKey: 'nav.config.establishment' },
  { to: '/e/$establishmentId/config/seasons', labelKey: 'nav.config.seasons' },
  { to: '/e/$establishmentId/config/closures', labelKey: 'nav.config.closures' },
  { to: '/e/$establishmentId/maps', labelKey: 'nav.config.maps' },
  { to: '/e/$establishmentId/config/zones', labelKey: 'nav.config.zones' },
  { to: '/e/$establishmentId/config/pools', labelKey: 'nav.config.pools' },
  { to: '/e/$establishmentId/config/catalog', labelKey: 'nav.config.catalog' },
  { to: '/e/$establishmentId/pricing', labelKey: 'nav.config.pricing' },
  { to: '/e/$establishmentId/policies', labelKey: 'nav.config.policies' },
  { to: '/e/$establishmentId/config/team', labelKey: 'nav.config.team', ownerOnly: true },
] as const;

export type ConfigNavPath = (typeof CONFIG_NAV_ITEMS)[number]['to'];
