import { useLazyNamespace, type NamespaceLoaders } from '../renewal/lazyNamespace';
import it from './locales/it.json';

/** Namespace of the personal waitlist page (M5B, spec 10.5). */
export const WAITLIST_NS = 'publicWaitlist';

const loaders: NamespaceLoaders = {
  en: () => import('./locales/en.json'),
  de: () => import('./locales/de.json'),
  fr: () => import('./locales/fr.json'),
};

export const waitlistLoaders = loaders;
export const waitlistItalian = it;

export function useWaitlistMessages(): void {
  useLazyNamespace(WAITLIST_NS, it, loaders);
}
