import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Configurazione → Modalità prova: the switch (getTestMode/updateTestMode),
 * the preview link of the public page and the bulk purge of test bookings.
 * The persistent banner while test mode is on lives in the shell header
 * (AppShell). Scaffold until the test-mode builder implements it.
 */
export function TestModePage() {
  const { t } = useTranslation('testMode');
  return <ScaffoldPage title={t('page.title')} description={t('page.description')} />;
}
