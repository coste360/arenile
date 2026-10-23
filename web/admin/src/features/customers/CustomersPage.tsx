import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the customers builder). */
export function CustomersPage() {
  const { t } = useTranslation('customers');
  return <ScaffoldPage title={t('list.title')} description={t('list.description')} />;
}
