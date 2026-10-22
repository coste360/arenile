import { useTranslation } from 'react-i18next';
import { PlaceholderPage } from '../components/PlaceholderPage';
import { EstablishmentsPage } from './EstablishmentsPage';

/** Configurazione: establishments list (M0) plus the sections coming in M1+. */
export function SettingsPage() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold">{t('nav.settings')}</h1>
      <EstablishmentsPage />
      <PlaceholderPage
        titleKey="admin.placeholder.title"
        descriptionKey="admin.placeholder.settings"
        milestone="M1"
      />
    </div>
  );
}
