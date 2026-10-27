import { Page } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { FindSpotPanel } from './FindSpotPanel';

/** "Trova posto" as a page (spec 30.6). */
export function FindSpotPage() {
  const { t } = useTranslation('findspot');
  return (
    <Page title={t('title')} description={t('description')}>
      <FindSpotPanel />
    </Page>
  );
}
