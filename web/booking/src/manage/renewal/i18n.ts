import it from './locales/it.json';
import { useLazyNamespace, type NamespaceLoaders } from './lazyNamespace';

/** Namespace of the renewal proposal and credit sections of the manage area (M5B). */
export const RENEWAL_NS = 'publicRenewal';

const loaders: NamespaceLoaders = {
  en: () => import('./locales/en.json'),
  de: () => import('./locales/de.json'),
  fr: () => import('./locales/fr.json'),
};

export const renewalLoaders = loaders;
export const renewalItalian = it;

export function useRenewalMessages(): void {
  useLazyNamespace(RENEWAL_NS, it, loaders);
}
