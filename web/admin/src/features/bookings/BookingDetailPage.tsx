import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the bookings builder). */
export function BookingDetailPage() {
  const { t } = useTranslation('bookings');
  return <ScaffoldPage title={t('detail.title')} description={t('detail.description')} />;
}
