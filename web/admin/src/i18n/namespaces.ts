import type { Locale } from '@arenile/shared/i18n';
import bookingsIt from '../features/bookings/locales/it.json';
import cashIt from '../features/cash/locales/it.json';
import configurationIt from '../features/configuration/locales/it.json';
import customersIt from '../features/customers/locales/it.json';
import mapEditorIt from '../features/map-editor/locales/it.json';
import operationsIt from '../features/operations/locales/it.json';
import passesIt from '../features/passes/locales/it.json';
import planningIt from '../features/planning/locales/it.json';
import policiesIt from '../features/policies/locales/it.json';
import pricingIt from '../features/pricing/locales/it.json';
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
 * One i18n namespace per admin feature (M1/M2, M3). Each feature owns its
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
];
