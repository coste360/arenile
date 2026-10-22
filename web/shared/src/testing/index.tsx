import { render, type RenderOptions } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { createTestI18n } from '../i18n';

/** Renders `ui` inside an Italian i18next provider. */
export function renderWithI18n(ui: ReactElement, options?: RenderOptions) {
  const i18n = createTestI18n();
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
  );
  return render(ui, { wrapper: Wrapper, ...options });
}
