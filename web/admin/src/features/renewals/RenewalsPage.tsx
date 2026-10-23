import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Renewals (spec 10.4, 15.1 "Abbonamenti … rinnovi"): campaigns, proposals
 * with pre-emption deadline, bulk send/accept/decline/release. Scaffold: the
 * renewals frontend builder replaces the body (Page + DataTable + FormSheet).
 */
export function RenewalsPage() {
  const { t } = useTranslation('renewals');
  return <ScaffoldPage title={t('title')} description={t('description')} />;
}
