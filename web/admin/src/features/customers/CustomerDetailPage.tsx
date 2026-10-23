import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the customers builder). */
export function CustomerDetailPage() {
  const { t } = useTranslation('customers');
  return <ScaffoldPage title={t('detail.title')} description={t('detail.description')} />;
}
