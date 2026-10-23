import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * "Vendita rapida" (Cassa): the daily sale form as a page, for the cash desk
 * without the map. The same form opens as VenditaGiornalieraSheet from the
 * day map ("Vendi giornata" on a free spot).
 */
export function DailySalePage() {
  const { t } = useTranslation('dailySale');
  return <ScaffoldPage title={t('sale.title')} description={t('sale.description')} />;
}
