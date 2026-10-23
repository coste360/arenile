import type { Locale } from '@arenile/shared/i18n';
import bookingsIt from '../features/bookings/locales/it.json';
import cashIt from '../features/cash/locales/it.json';
import configurationIt from '../features/configuration/locales/it.json';
import customersIt from '../features/customers/locales/it.json';
import desktopIt from '../features/desktop/locales/it.json';
import devicesIt from '../features/devices/locales/it.json';
import devicesLanIt from '../features/devices/lan/locales/it.json';
import firstRunIt from '../features/first-run/locales/it.json';
import integrationsIt from '../features/integrations/locales/it.json';
import offlineIt from '../offline/locales/it.json';
import mapEditorIt from '../features/map-editor/locales/it.json';
import onlineIt from '../features/online/locales/it.json';
import operationsIt from '../features/operations/locales/it.json';
import passesIt from '../features/passes/locales/it.json';
import planningIt from '../features/planning/locales/it.json';
import policiesIt from '../features/policies/locales/it.json';
import pricingIt from '../features/pricing/locales/it.json';
import reportsIt from '../features/reports/locales/it.json';
import renewalsIt from '../features/renewals/locales/it.json';
import waitlistIt from '../features/waitlist/locales/it.json';
import creditsIt from '../features/credits/locales/it.json';
import seasonRolloverIt from '../features/season-rollover/locales/it.json';
import setupWizardIt from '../features/setup-wizard/locales/it.json';

type Messages = Record<string, unknown>;
type Loader = () => Promise<{ default: Messages }>;

export interface FeatureNamespace {
  /** i18next namespace: use `useTranslation('<ns>')` or `t('<ns>:key')`. */
  ns: string;
  /** Italian messages (source language), bundled. */
  it: Messages;
  /** Other languages, loaded on demand. */
  load: Record<Exclude<Locale, 'it'>, Loader>;
}

/**
 * One i18n namespace per admin feature (M1/M2, M3, M4, M5, M6). Each feature owns its
 * `features/<folder>/locales/{it,en,de,fr}.json`; the shared `translation`
 * namespace keeps the shell strings (navigation, common errors, actions).
 */
export const FEATURE_NAMESPACES: readonly FeatureNamespace[] = [
  {
    ns: 'mapEditor',
    it: mapEditorIt,
    load: {
      en: () => import('../features/map-editor/locales/en.json'),
      de: () => import('../features/map-editor/locales/de.json'),
      fr: () => import('../features/map-editor/locales/fr.json'),
    },
  },
  {
    ns: 'configuration',
    it: configurationIt,
    load: {
      en: () => import('../features/configuration/locales/en.json'),
      de: () => import('../features/configuration/locales/de.json'),
      fr: () => import('../features/configuration/locales/fr.json'),
    },
  },
  {
    ns: 'setupWizard',
    it: setupWizardIt,
    load: {
      en: () => import('../features/setup-wizard/locales/en.json'),
      de: () => import('../features/setup-wizard/locales/de.json'),
      fr: () => import('../features/setup-wizard/locales/fr.json'),
    },
  },
  {
    ns: 'pricing',
    it: pricingIt,
    load: {
      en: () => import('../features/pricing/locales/en.json'),
      de: () => import('../features/pricing/locales/de.json'),
      fr: () => import('../features/pricing/locales/fr.json'),
    },
  },
  {
    ns: 'policies',
    it: policiesIt,
    load: {
      en: () => import('../features/policies/locales/en.json'),
      de: () => import('../features/policies/locales/de.json'),
      fr: () => import('../features/policies/locales/fr.json'),
    },
  },
  {
    ns: 'operations',
    it: operationsIt,
    load: {
      en: () => import('../features/operations/locales/en.json'),
      de: () => import('../features/operations/locales/de.json'),
      fr: () => import('../features/operations/locales/fr.json'),
    },
  },
  {
    ns: 'planning',
    it: planningIt,
    load: {
      en: () => import('../features/planning/locales/en.json'),
      de: () => import('../features/planning/locales/de.json'),
      fr: () => import('../features/planning/locales/fr.json'),
    },
  },
  {
    ns: 'bookings',
    it: bookingsIt,
    load: {
      en: () => import('../features/bookings/locales/en.json'),
      de: () => import('../features/bookings/locales/de.json'),
      fr: () => import('../features/bookings/locales/fr.json'),
    },
  },
  {
    ns: 'customers',
    it: customersIt,
    load: {
      en: () => import('../features/customers/locales/en.json'),
      de: () => import('../features/customers/locales/de.json'),
      fr: () => import('../features/customers/locales/fr.json'),
    },
  },
  {
    ns: 'passes',
    it: passesIt,
    load: {
      en: () => import('../features/passes/locales/en.json'),
      de: () => import('../features/passes/locales/de.json'),
      fr: () => import('../features/passes/locales/fr.json'),
    },
  },
  {
    ns: 'cash',
    it: cashIt,
    load: {
      en: () => import('../features/cash/locales/en.json'),
      de: () => import('../features/cash/locales/de.json'),
      fr: () => import('../features/cash/locales/fr.json'),
    },
  },
  {
    ns: 'online',
    it: onlineIt,
    load: {
      en: () => import('../features/online/locales/en.json'),
      de: () => import('../features/online/locales/de.json'),
      fr: () => import('../features/online/locales/fr.json'),
    },
  },
  {
    ns: 'devices',
    it: devicesIt,
    load: {
      en: () => import('../features/devices/locales/en.json'),
      de: () => import('../features/devices/locales/de.json'),
      fr: () => import('../features/devices/locales/fr.json'),
    },
  },
  {
    ns: 'devicesLan',
    it: devicesLanIt,
    load: {
      en: () => import('../features/devices/lan/locales/en.json'),
      de: () => import('../features/devices/lan/locales/de.json'),
      fr: () => import('../features/devices/lan/locales/fr.json'),
    },
  },
  {
    ns: 'desktop',
    it: desktopIt,
    load: {
      en: () => import('../features/desktop/locales/en.json'),
      de: () => import('../features/desktop/locales/de.json'),
      fr: () => import('../features/desktop/locales/fr.json'),
    },
  },
  {
    ns: 'firstRun',
    it: firstRunIt,
    load: {
      en: () => import('../features/first-run/locales/en.json'),
      de: () => import('../features/first-run/locales/de.json'),
      fr: () => import('../features/first-run/locales/fr.json'),
    },
  },
  // M5A: reports and integrations (API keys, outgoing webhooks).
  {
    ns: 'reports',
    it: reportsIt,
    load: {
      en: () => import('../features/reports/locales/en.json'),
      de: () => import('../features/reports/locales/de.json'),
      fr: () => import('../features/reports/locales/fr.json'),
    },
  },
  {
    ns: 'integrations',
    it: integrationsIt,
    load: {
      en: () => import('../features/integrations/locales/en.json'),
      de: () => import('../features/integrations/locales/de.json'),
      fr: () => import('../features/integrations/locales/fr.json'),
    },
  },
  // M5A: panel offline mode (spec 21.2): banner, queue, update prompt.
  {
    ns: 'offline',
    it: offlineIt,
    load: {
      en: () => import('../offline/locales/en.json'),
      de: () => import('../offline/locales/de.json'),
      fr: () => import('../offline/locales/fr.json'),
    },
  },
  // M5B: renewals, waitlist, customer credits, season rollover.
  {
    ns: 'renewals',
    it: renewalsIt,
    load: {
      en: () => import('../features/renewals/locales/en.json'),
      de: () => import('../features/renewals/locales/de.json'),
      fr: () => import('../features/renewals/locales/fr.json'),
    },
  },
  {
    ns: 'waitlist',
    it: waitlistIt,
    load: {
      en: () => import('../features/waitlist/locales/en.json'),
      de: () => import('../features/waitlist/locales/de.json'),
      fr: () => import('../features/waitlist/locales/fr.json'),
    },
  },
  {
    ns: 'credits',
    it: creditsIt,
    load: {
      en: () => import('../features/credits/locales/en.json'),
      de: () => import('../features/credits/locales/de.json'),
      fr: () => import('../features/credits/locales/fr.json'),
    },
  },
  {
    ns: 'seasonRollover',
    it: seasonRolloverIt,
    load: {
      en: () => import('../features/season-rollover/locales/en.json'),
      de: () => import('../features/season-rollover/locales/de.json'),
      fr: () => import('../features/season-rollover/locales/fr.json'),
    },
  },
];
