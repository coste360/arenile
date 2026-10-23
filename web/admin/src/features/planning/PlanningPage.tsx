import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the planning builder). */
export function PlanningPage() {
  const { t } = useTranslation('planning');
  return <ScaffoldPage title={t('title')} description={t('description')} />;
}
