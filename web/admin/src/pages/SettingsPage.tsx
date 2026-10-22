import { PageHeader } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { PlaceholderPage } from '../components/PlaceholderPage';
import { EstablishmentsPage } from './EstablishmentsPage';

/** Configurazione: establishments list (M0) plus the sections coming next. */
export function SettingsPage() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <PageHeader title={t('nav.settings')} description={t('admin.placeholder.settings')} />
      <EstablishmentsPage />
      <PlaceholderPage
        titleKey="admin.placeholder.title"
        descriptionKey="admin.placeholder.settings"
        milestone="M1"
        asPage={false}
      />
    </div>
  );
}
