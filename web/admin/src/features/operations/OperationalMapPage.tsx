import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the operations builder). */
export function OperationalMapPage() {
  const { t } = useTranslation('operations');
  return <ScaffoldPage title={t('map.title')} description={t('map.description')} />;
}
