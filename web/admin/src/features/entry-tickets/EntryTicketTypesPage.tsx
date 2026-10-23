import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Configurazione → "Biglietti d'ingresso": ticket types as packs on a
 * quantity zone (price by rate period, daily capacity = the zone pool).
 */
export function EntryTicketTypesPage() {
  const { t } = useTranslation('entryTickets');
  return <ScaffoldPage title={t('types.title')} description={t('types.description')} />;
}
