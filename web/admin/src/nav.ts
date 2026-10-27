import {
  BanknoteIcon,
  CalculatorIcon,
  FlaskConicalIcon,
  PrinterIcon,
  UploadIcon,
  BarChart3Icon,
  BuildingIcon,
  CalendarDaysIcon,
  CalendarRangeIcon,
  ClipboardListIcon,
  CreditCardIcon,
  EuroIcon,
  GlobeIcon,
  HandshakeIcon,
  HourglassIcon,
  KeyRoundIcon,
  LayersIcon,
  LayoutGridIcon,
  MailIcon,
  MailPlusIcon,
  MapIcon,
  MonitorIcon,
  PackageIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  SmartphoneIcon,
  SparklesIcon,
  SunIcon,
  TicketIcon,
  UsersIcon,
  UserCogIcon,
  WalletIcon,
  WebhookIcon,
  BanIcon,
  ReceiptIcon,
  ZapIcon,
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
  // M8 (ADR 0028): partners, customer categories, partner account and
  // agreement reports, next to the customers they are an alternative to.
  {
    to: '/e/$establishmentId/convenzioni',
    section: 'convenzioni',
    labelKey: 'nav.partners',
    descriptionKey: 'admin.placeholder.partners',
    milestone: 'M8',
    icon: HandshakeIcon,
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
  // M5B: renewals next to the season passes, then the waitlist.
  {
    to: '/e/$establishmentId/rinnovi',
    section: 'rinnovi',
    labelKey: 'nav.renewals',
    descriptionKey: 'admin.placeholder.renewals',
    milestone: 'M5',
    icon: RefreshCwIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/lista-attesa',
    section: 'lista-attesa',
    labelKey: 'nav.waitlist',
    descriptionKey: 'admin.placeholder.waitlist',
    milestone: 'M5',
    icon: HourglassIcon,
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
  // Daily sale (ADR 0020): quick sale and today's tickets, next to Cassa.
  {
    to: '/e/$establishmentId/vendita-rapida',
    section: 'vendita-rapida',
    labelKey: 'nav.dailySale',
    descriptionKey: 'admin.placeholder.dailySale',
    milestone: 'M5',
    icon: ZapIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/ticket',
    section: 'ticket',
    labelKey: 'nav.tickets',
    descriptionKey: 'admin.placeholder.tickets',
    milestone: 'M5',
    icon: ReceiptIcon,
    available: true,
  },
  {
    to: '/e/$establishmentId/report',
    section: 'report',
    labelKey: 'nav.reports',
    descriptionKey: 'admin.placeholder.reports',
    milestone: 'M5',
    icon: BarChart3Icon,
    available: true,
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
  {
    to: '/e/$establishmentId/config/entry-tickets',
    labelKey: 'nav.config.entryTickets',
    icon: TicketIcon,
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
  // Editable customer email texts and automatic reminders (ADR 0022).
  {
    to: '/e/$establishmentId/config/customer-emails',
    labelKey: 'nav.config.customerEmails',
    icon: MailPlusIcon,
  },
  // Deferred features (ADR 0019): cash registers, test mode, CSV imports.
  {
    to: '/e/$establishmentId/config/cash-registers',
    labelKey: 'nav.config.cashRegisters',
    icon: CalculatorIcon,
  },
  {
    to: '/e/$establishmentId/config/test-mode',
    labelKey: 'nav.config.testMode',
    icon: FlaskConicalIcon,
  },
  {
    to: '/e/$establishmentId/config/import',
    labelKey: 'nav.config.imports',
    icon: UploadIcon,
  },
  // ESC/POS network printers (ADR 0021): desktop edition only; the page
  // stays reachable by deep link on the server edition and explains why.
  {
    to: '/e/$establishmentId/config/printers',
    labelKey: 'nav.config.printers',
    icon: PrinterIcon,
    desktopOnly: true,
  },
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
  // M5A integrations (owner only: permission users.manage).
  {
    to: '/e/$establishmentId/config/api-keys',
    labelKey: 'nav.config.apiKeys',
    icon: KeyRoundIcon,
    ownerOnly: true,
  },
  {
    to: '/e/$establishmentId/config/webhooks',
    labelKey: 'nav.config.webhooks',
    icon: WebhookIcon,
    ownerOnly: true,
  },
] as const satisfies readonly (NavItemBase & {
  to: string;
  ownerOnly?: boolean;
  /** Shown only on the desktop edition (lib/meta useIsDesktopEdition, M6). */
  desktopOnly?: boolean;
})[];

export type ConfigNavPath = (typeof CONFIG_NAV_ITEMS)[number]['to'];

/**
 * Pages reached from a configuration page (not listed in the sidebar) that
 * still need their own breadcrumb label: without an entry the breadcrumb
 * would show the parent's label ("Dispositivi" on the LAN page).
 */
export const BREADCRUMB_ONLY_ITEMS = [
  { to: '/e/$establishmentId/config/devices/lan', labelKey: 'nav.config.devicesLan' },
  { to: '/e/$establishmentId/config/seasons/rollover', labelKey: 'nav.config.seasonRollover' },
  { to: '/e/$establishmentId/ticket/varco', labelKey: 'nav.entryGate' },
  // M8 (ADR 0028).
  { to: '/e/$establishmentId/convenzioni/rendiconti', labelKey: 'nav.agreementsReports' },
  { to: '/e/$establishmentId/trova-posto', labelKey: 'nav.findSpot' },
] as const satisfies readonly { to: string; labelKey: string }[];
