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
