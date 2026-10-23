import type { ReactNode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { docsI18n, type DocsLocale } from './i18n';

export interface IslandShellProps {
  locale: DocsLocale;
  children: ReactNode;
}

/** Providers every live component needs (translations of web/shared). */
export function IslandShell({ locale, children }: IslandShellProps) {
  return <I18nextProvider i18n={docsI18n(locale)}>{children}</I18nextProvider>;
}
