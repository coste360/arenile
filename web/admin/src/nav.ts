import {
  BanknoteIcon,
  BarChart3Icon,
  BuildingIcon,
  CalendarDaysIcon,
  CalendarRangeIcon,
  ClipboardListIcon,
  CreditCardIcon,
  EuroIcon,
  GlobeIcon,
  LayersIcon,
  LayoutGridIcon,
  MailIcon,
  MapIcon,
  MonitorIcon,
  PackageIcon,
  ShieldCheckIcon,
  SmartphoneIcon,
  SparklesIcon,
  SunIcon,
  TicketIcon,
  UsersIcon,
  UserCogIcon,
  WalletIcon,
  BanIcon,
  type LucideIcon,
} from 'lucide-react';

interface NavItemBase {
  labelKey: string;
  icon: LucideIcon;
}

/**
 * "Operatività" (spec 15.1), scoped to the current establishment: every path
 * is under /e/$establishmentId (`section` is the last segment, also served
 * as a legacy top-level path that redirects to the current establishment).
 * `milestone` is the roadmap milestone that delivers the section (spec 28);
 * items with `available: false` are shown disabled with a "presto" badge.
 */
export const NAV_ITEMS = [
  {
    to: '/e/$establishmentId/oggi',
    section: 'oggi',
    labelKey: 'nav.today',
    descriptionKey: 'admin.placeholder.today',
    milestone: 'M3',
    icon: SunIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/mappa',
    section: 'mappa',
    labelKey: 'nav.map',
    descriptionKey: 'admin.placeholder.map',
    milestone: 'M3',
    icon: MapIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/prenotazioni',
    section: 'prenotazioni',
    labelKey: 'nav.bookings',
    descriptionKey: 'admin.placeholder.bookings',
    milestone: 'M3',
    icon: ClipboardListIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/calendario',
    section: 'calendario',
    labelKey: 'nav.calendar',
    descriptionKey: 'admin.placeholder.calendar',
    milestone: 'M3',
    icon: CalendarDaysIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/clienti',
    section: 'clienti',
    labelKey: 'nav.customers',
    descriptionKey: 'admin.placeholder.customers',
    milestone: 'M3',
    icon: UsersIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/abbonamenti',
    section: 'abbonamenti',
    labelKey: 'nav.seasonPasses',
    descriptionKey: 'admin.placeholder.seasonPasses',
    milestone: 'M3',
    icon: TicketIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/cassa',
    section: 'cassa',
    labelKey: 'nav.cash',
    descriptionKey: 'admin.placeholder.cash',
    milestone: 'M3',
    icon: WalletIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/report',
    section: 'report',
    labelKey: 'nav.reports',
    descriptionKey: 'admin.placeholder.reports',
    milestone: 'M5',
    icon: BarChart3Icon,
    available: false,
  },
] as const satisfies readonly (NavItemBase & {
  to: string;
  section: string;
  descriptionKey: string;
  milestone: string;
  available: boolean;
})[];

export type NavSection = (typeof NAV_ITEMS)[number]['section'];

export type NavPath = (typeof NAV_ITEMS)[number]['to'];

/** Global configuration page: accessible establishments (M0). */
export const ESTABLISHMENTS_NAV_ITEM = {
  to: '/configurazione',
  labelKey: 'nav.establishments',
  icon: BuildingIcon,
} as const;

/**
 * "Configurazione" (spec 15.1 item 9), scoped to the current establishment:
 * every path is under /e/$establishmentId.
 */
export const CONFIG_NAV_ITEMS = [
  { to: '/e/$establishmentId/setup', labelKey: 'nav.config.setup', icon: SparklesIcon },
  {
    to: '/e/$establishmentId/config/establishment',
    labelKey: 'nav.config.establishment',
    icon: BuildingIcon,
  },
  {
    to: '/e/$establishmentId/config/seasons',
    labelKey: 'nav.config.seasons',
    icon: CalendarRangeIcon,
  },
  { to: '/e/$establishmentId/config/closures', labelKey: 'nav.config.closures', icon: BanIcon },
  { to: '/e/$establishmentId/maps', labelKey: 'nav.config.maps', icon: LayoutGridIcon },
  { to: '/e/$establishmentId/config/zones', labelKey: 'nav.config.zones', icon: LayersIcon },
  { to: '/e/$establishmentId/config/pools', labelKey: 'nav.config.pools', icon: PackageIcon },
  {
    to: '/e/$establishmentId/config/catalog',
    labelKey: 'nav.config.catalog',
    icon: CreditCardIcon,
  },
  { to: '/e/$establishmentId/pricing', labelKey: 'nav.config.pricing', icon: EuroIcon },
  {
    to: '/e/$establishmentId/policies',
    labelKey: 'nav.config.policies',
    icon: ShieldCheckIcon,
  },
  {
    to: '/e/$establishmentId/config/online',
    labelKey: 'nav.config.online',
    icon: GlobeIcon,
  },
  {
    to: '/e/$establishmentId/config/payments',
    labelKey: 'nav.config.payments',
    icon: BanknoteIcon,
  },
  { to: '/e/$establishmentId/config/email', labelKey: 'nav.config.email', icon: MailIcon },
  {
    to: '/e/$establishmentId/config/team',
    labelKey: 'nav.config.team',
    icon: UserCogIcon,
    ownerOnly: true,
  },
  {
    to: '/e/$establishmentId/config/devices',
    labelKey: 'nav.config.devices',
    icon: SmartphoneIcon,
  },
  {
    to: '/e/$establishmentId/config/desktop',
    labelKey: 'nav.config.desktop',
    icon: MonitorIcon,
    ownerOnly: true,
    desktopOnly: true,
  },
] as const satisfies readonly (NavItemBase & {
  to: string;
  ownerOnly?: boolean;
  /** Shown only on the desktop edition (lib/meta useIsDesktopEdition, M6). */
  desktopOnly?: boolean;
})[];

export type ConfigNavPath = (typeof CONFIG_NAV_ITEMS)[number]['to'];
