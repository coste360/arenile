import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Waitlist (spec 10.5): entries in priority order, offers, reorder, settings.
 * Scaffold: the waitlist frontend builder replaces the body.
 */
export function WaitlistPage() {
  const { t } = useTranslation('waitlist');
  return <ScaffoldPage title={t('title')} description={t('description')} />;
}
