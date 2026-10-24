import { Page } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { EstablishmentsPage } from './EstablishmentsPage';

/** Configurazione: the establishments the user can open, and the way to add one. */
export function SettingsPage() {
  const { t } = useTranslation();
  return (
    <Page title={t('nav.settings')} description={t('admin.placeholder.settings')}>
      <EstablishmentsPage />
    </Page>
  );
}
