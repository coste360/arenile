import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Configurazione → Importa dati: upload a CSV, preview (dry run) with the
 * validation report, apply all-or-nothing (createImportJob, previewImportJob,
 * applyImportJob). Scaffold until the imports builder implements it.
 */
export function ImportsPage() {
  const { t } = useTranslation('imports');
  return <ScaffoldPage title={t('page.title')} description={t('page.description')} />;
}
