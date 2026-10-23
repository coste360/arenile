import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Gate check-in from the staff phone: scan the QR, one entry per ticket. */
export function EntryGatePage() {
  const { t } = useTranslation('entryTickets');
  return <ScaffoldPage title={t('gate.title')} description={t('gate.description')} />;
}
