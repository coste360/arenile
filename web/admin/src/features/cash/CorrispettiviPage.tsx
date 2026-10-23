import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/** Placeholder of the final page (M3 scaffolding, owner: the cash builder). */
export function CorrispettiviPage() {
  const { t } = useTranslation('cash');
  return (
    <ScaffoldPage title={t('corrispettivi.title')} description={t('corrispettivi.description')} />
  );
}
