import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the passes builder). */
export function SeasonPassesPage() {
  const { t } = useTranslation('passes');
  return <ScaffoldPage title={t('title')} description={t('description')} />;
}
