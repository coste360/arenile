import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * "Ticket del giorno": DataTable of the day's tickets (number, time, spot or
 * zone, state, amount, method), summary StatCards and CSV export.
 */
export function TicketsPage() {
  const { t } = useTranslation('dailySale');
  return <ScaffoldPage title={t('tickets.title')} description={t('tickets.description')} />;
}
